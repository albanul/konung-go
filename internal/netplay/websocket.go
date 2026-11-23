//go:build !js

package netplay

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Browsers cannot open TCP connections or send UDP, so the game's browser
// build (GOOS=js) talks to the server over WebSocket on the game port: a
// connection that starts with an HTTP request is served by web (sniff);
// any other is the JSON-line protocol as before. web answers GET /info
// with the SessionInfo (the browser's discovery, as the UDP answer) and
// upgrades any other request to a WebSocket that carries the same JSON
// lines as TCP (websocket.NetConn, text messages).

// sniffTimeout bounds an HTTP request's header (the web server).
const sniffTimeout = 10 * time.Second

// sniff serves connection c by its first bytes: an HTTP request (a
// browser) goes to the web server, anything else to the JSON-line
// protocol — also a connection that stays silent for the join timeout, so
// the protocol's own timeout answers it (ErrProtocol). Close closes the
// connections still being sniffed.
func (s *Server) sniff(c net.Conn) {
	defer s.wg.Done()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		c.Close()
		return
	}
	s.sniffing[c] = true
	s.mu.Unlock()
	c.SetReadDeadline(time.Now().Add(s.cfg.JoinTimeout))
	br := bufio.NewReader(c)
	head, err := br.Peek(4)
	c.SetReadDeadline(time.Time{})
	s.mu.Lock()
	delete(s.sniffing, c)
	closed := s.closed
	s.mu.Unlock()
	pc := &peekedConn{Conn: c, r: br}
	var ne net.Error
	switch {
	case closed:
		c.Close()
		return
	case err != nil && !(errors.As(err, &ne) && ne.Timeout()):
		c.Close() // gone before saying anything
		return
	case err != nil:
		s.accept(pc) // silent: the join timeout answers it
		return
	}
	switch string(head) {
	case "GET ", "OPTI", "HEAD":
		select {
		case s.webListener().conns <- pc:
		case <-s.web.ln.done:
			c.Close()
		}
	default:
		s.accept(pc)
	}
}

// peekedConn is a connection whose first bytes were peeked.
type peekedConn struct {
	net.Conn
	r *bufio.Reader
}

func (p *peekedConn) Read(b []byte) (int, error) { return p.r.Read(b) }

// webState is the web server for browsers (started on first use).
type webState struct {
	once sync.Once
	ln   *chanListener
	srv  *http.Server
}

// webListener is the web server's listener: the connections sniff hands
// over.
func (s *Server) webListener() *chanListener {
	w := &s.web
	w.once.Do(func() {
		w.ln = &chanListener{conns: make(chan net.Conn), done: make(chan struct{}), addr: s.ln.Addr()}
		w.srv = &http.Server{Handler: http.HandlerFunc(s.serveWeb), ReadHeaderTimeout: sniffTimeout}
		go w.srv.Serve(w.ln)
	})
	return w.ln
}

// serveWeb is the web server: GET /info answers the session, any other
// request becomes a WebSocket connection to the session.
func (s *Server) serveWeb(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/info" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.Session())
		return
	}
	// Any page may connect (a LAN game: the server has no origin to trust).
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	ws.SetReadLimit(MaxMessageSize + 1)
	// The handler returns when the connection ends (the request's context
	// lives that long).
	done := make(chan struct{})
	nc := websocket.NetConn(r.Context(), ws, websocket.MessageText)
	s.accept(&notifyConn{Conn: nc, done: done})
	<-done
}

// notifyConn closes done when the connection is closed.
type notifyConn struct {
	net.Conn
	done chan struct{}
	once sync.Once
}

func (n *notifyConn) Close() error {
	err := n.Conn.Close()
	n.once.Do(func() { close(n.done) })
	return err
}

// chanListener is a net.Listener of the connections sent to conns.
type chanListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
	addr  net.Addr
}

func (l *chanListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *chanListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *chanListener) Addr() net.Addr { return l.addr }

// closeWeb stops the web server.
func (s *Server) closeWeb() {
	s.web.once.Do(func() {}) // never start it after this
	if s.web.srv != nil {
		s.web.srv.Close()
	}
	if s.web.ln != nil {
		s.web.ln.Close()
	}
}
