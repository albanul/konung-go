package netplay

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Connection errors.
var (
	ErrClosed   = errors.New("netplay: connection closed")
	ErrSlowPeer = errors.New("netplay: peer not reading, send queue full")
)

// Options tune a connection. Zero values take the defaults.
type Options struct {
	PingEvery    time.Duration // idle keep-alive interval (default 5 s)
	IdleTimeout  time.Duration // no data at all from the peer for this long closes (default 20 s)
	WriteTimeout time.Duration // one write may take this long (default 10 s)
	QueueSize    int           // outgoing and incoming queue length (default 256)
}

func (o Options) withDefaults() Options {
	if o.PingEvery <= 0 {
		o.PingEvery = 5 * time.Second
	}
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = 20 * time.Second
	}
	if o.WriteTimeout <= 0 {
		o.WriteTimeout = 10 * time.Second
	}
	if o.QueueSize <= 0 {
		o.QueueSize = 256
	}
	return o
}

// Conn is one message connection (adapted from go-tetris Client). Messages
// arrive in order on Incoming, which is closed when the connection ends; Err
// then gives the reason (nil after a local Close). Send never blocks: a peer
// that stops reading fills the queue and is disconnected with ErrSlowPeer
// rather than losing messages silently. Pings are sent while idle and never
// reach Incoming.
type Conn struct {
	c    net.Conn
	opts Options
	out  chan *Message // nil entry = close after everything before it is written
	in   chan *Message
	done chan struct{}

	once    sync.Once
	mu      sync.Mutex
	err     error
	closing atomic.Bool // CloseGracefully's queue is written; waiting for the peer to hang up
}

// NewConn starts the read and write loops on c.
func NewConn(c net.Conn, opts Options) *Conn {
	opts = opts.withDefaults()
	cn := &Conn{
		c:    c,
		opts: opts,
		out:  make(chan *Message, opts.QueueSize),
		in:   make(chan *Message, opts.QueueSize),
		done: make(chan struct{}),
	}
	go cn.readLoop()
	go cn.writeLoop()
	return cn
}

// Incoming delivers the peer's messages; it is closed when the connection ends.
func (c *Conn) Incoming() <-chan *Message { return c.in }

// Done is closed when the connection ends.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Err is why the connection ended (nil while open or after Close).
func (c *Conn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// RemoteAddr is the peer's address.
func (c *Conn) RemoteAddr() net.Addr { return c.c.RemoteAddr() }

// Send queues m for the peer.
func (c *Conn) Send(m *Message) error {
	if m == nil {
		return errors.New("netplay: nil message")
	}
	select {
	case <-c.done:
		return ErrClosed
	default:
	}
	select {
	case c.out <- m:
		return nil
	case <-c.done:
		return ErrClosed
	default:
		c.fail(ErrSlowPeer)
		return ErrSlowPeer
	}
}

// SendAndClose queues m, then closes once it and everything queued before
// it are written (or after WriteTimeout).
func (c *Conn) SendAndClose(m *Message) {
	if c.Send(m) != nil {
		return
	}
	c.CloseGracefully()
}

// CloseGracefully closes once everything already queued is written (or
// after WriteTimeout). Close drops the queue. Over TCP it then half-closes
// and waits for the peer to hang up: closing a socket with unread incoming
// data makes the kernel send a reset, which can destroy the last messages
// before the peer reads them. Messages arriving meanwhile are delivered while
// Incoming has room and dropped otherwise.
func (c *Conn) CloseGracefully() {
	select {
	case c.out <- nil:
	case <-c.done:
	default:
		c.Close()
		return
	}
	// The write loop closes; make sure we do not linger on a dead peer.
	time.AfterFunc(c.opts.WriteTimeout, c.Close)
}

// Close ends the connection at once, dropping anything still queued;
// calling it more than once is safe.
func (c *Conn) Close() { c.fail(nil) }

func (c *Conn) fail(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.err = err
		c.mu.Unlock()
		close(c.done)
		c.c.Close()
	})
}

func (c *Conn) readLoop() {
	defer close(c.in)
	sc := bufio.NewScanner(c.c)
	sc.Buffer(make([]byte, 0, 4096), MaxMessageSize)
	for {
		c.c.SetReadDeadline(time.Now().Add(c.opts.IdleTimeout))
		if !sc.Scan() {
			if c.closing.Load() {
				c.fail(nil) // the peer hung up after our graceful close
				return
			}
			err := sc.Err()
			if err == nil {
				err = ErrClosed // EOF: the peer hung up
			}
			c.fail(fmt.Errorf("netplay: read: %w", err))
			return
		}
		var m Message
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			c.fail(fmt.Errorf("netplay: bad message: %w", err))
			return
		}
		if m.Type == MsgPing {
			continue
		}
		if c.closing.Load() {
			select {
			case c.in <- &m:
			default: // nobody is reading any more; keep draining
			}
			continue
		}
		select {
		case c.in <- &m:
		case <-c.done:
			return
		}
	}
}

func (c *Conn) writeLoop() {
	ping := time.NewTicker(c.opts.PingEvery)
	defer ping.Stop()
	pingLine, _ := json.Marshal(Message{Type: MsgPing})
	pingLine = append(pingLine, '\n')
	write := func(b []byte) bool {
		c.c.SetWriteDeadline(time.Now().Add(c.opts.WriteTimeout))
		if _, err := c.c.Write(b); err != nil {
			c.fail(fmt.Errorf("netplay: write: %w", err))
			return false
		}
		return true
	}
	for {
		select {
		case <-c.done:
			return
		case m := <-c.out:
			if m == nil {
				c.closing.Store(true)
				if cw, ok := c.c.(interface{ CloseWrite() error }); ok && cw.CloseWrite() == nil {
					return // the read loop ends when the peer closes
				}
				c.Close()
				return
			}
			b, err := json.Marshal(m)
			if err != nil {
				c.fail(fmt.Errorf("netplay: encode: %w", err))
				return
			}
			if len(b) >= MaxMessageSize {
				c.fail(fmt.Errorf("netplay: %v message of %d bytes is too large", m.Type, len(b)))
				return
			}
			if !write(append(b, '\n')) {
				return
			}
			ping.Reset(c.opts.PingEvery)
		case <-ping.C:
			if !write(pingLine) {
				return
			}
		}
	}
}
