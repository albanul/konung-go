package netplay

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Limits on what clients send.
const (
	MaxNameLen = 31 // characters; the login field holds 32 bytes with the cursor (0x0042CB94)
	MaxClanLen = 30 // the clan field holds 31 bytes with the cursor
	MaxChatLen = 78 // the chat field holds 79 bytes with the cursor
)

// ServerConfig describes the session a server hosts. Zero values take the
// LAN version's defaults (SERVER file "TEAM 3 180", 16 players).
type ServerConfig struct {
	Name        string        // session name (default "Home Server", as in the shipped SERVER file)
	MaxPlayers  int           // default 16
	Minutes     int           // session length (default 180)
	JoinTimeout time.Duration // a new connection must join within this (default 5 s)
	Conn        Options
	// OnGame receives MsgGame messages with From set to the sender. It runs
	// on the sender's connection goroutine; set it before Listen.
	OnGame func(m *Message)
	// OnJoin is called after a player has joined.
	OnJoin func(p PlayerInfo)
	// OnLeave is called after a player has left (also for kicked players).
	OnLeave func(p PlayerInfo)
}

func (c ServerConfig) withDefaults() ServerConfig {
	if c.Name == "" {
		c.Name = "Home Server"
	}
	if c.MaxPlayers <= 0 {
		c.MaxPlayers = 16
	}
	if c.Minutes <= 0 {
		c.Minutes = 180
	}
	if c.JoinTimeout <= 0 {
		c.JoinTimeout = 5 * time.Second
	}
	return c
}

type serverPlayer struct {
	info PlayerInfo
	conn *Conn
}

// Server hosts one session: it accepts joins, keeps the player list, relays
// chat and hands gameplay messages to OnGame. Adapted from go-tetris's
// Server and GameLobby. One lock guards the player list; membership messages
// (welcome, joined, left) are queued under it so every client sees them in
// the same order as the list. Queueing never blocks (Conn.Send enqueues or
// disconnects a stalled peer), so this cannot deadlock like go-tetris did.
type Server struct {
	cfg  ServerConfig
	ln   net.Listener
	port int

	mu      sync.Mutex
	players map[string]*serverPlayer
	order   []string // join order
	pending map[*Conn]bool
	closed  bool

	disc *announcer
	wg   sync.WaitGroup
}

// Listen starts a server on addr (":7777", "127.0.0.1:0", ...).
func Listen(addr string, cfg ServerConfig) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("netplay: listen: %w", err)
	}
	s := &Server{
		cfg:     cfg.withDefaults(),
		ln:      ln,
		port:    ln.Addr().(*net.TCPAddr).Port,
		players: map[string]*serverPlayer{},
		pending: map[*Conn]bool{},
	}
	s.wg.Add(1)
	go s.acceptLoop()
	return s, nil
}

// Addr is the listening address.
func (s *Server) Addr() net.Addr { return s.ln.Addr() }

// ServeDiscovery answers LAN discovery queries on the UDP address addr
// (":7777" by default with the game port); see Discover.
func (s *Server) ServeDiscovery(addr string) error {
	a, err := listenAnnouncer(addr, s.Session)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		a.close()
		return ErrClosed
	}
	if s.disc != nil {
		a.close()
		return errors.New("netplay: discovery already served")
	}
	s.disc = a
	return nil
}

// DiscoveryAddr is the UDP address discovery answers on, or nil.
func (s *Server) DiscoveryAddr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disc == nil {
		return nil
	}
	return s.disc.pc.LocalAddr()
}

// Session is the current session description.
func (s *Server) Session() SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionLocked()
}

func (s *Server) sessionLocked() SessionInfo {
	info := SessionInfo{Name: s.cfg.Name, Port: s.port, MaxPlayers: s.cfg.MaxPlayers, Minutes: s.cfg.Minutes, Players: []PlayerInfo{}}
	for _, id := range s.order {
		info.Players = append(info.Players, s.players[id].info)
	}
	return info
}

// Send sends m to one player.
func (s *Server) Send(playerID string, m *Message) error {
	s.mu.Lock()
	p := s.players[playerID]
	s.mu.Unlock()
	if p == nil {
		return fmt.Errorf("netplay: no player %q", playerID)
	}
	return p.conn.Send(m)
}

// Broadcast sends m to every player except the IDs in except.
func (s *Server) Broadcast(m *Message, except ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.broadcastLocked(m, except...)
}

func (s *Server) broadcastLocked(m *Message, except ...string) {
next:
	for _, id := range s.order {
		for _, e := range except {
			if id == e {
				continue next
			}
		}
		s.players[id].conn.Send(m) // a failing peer is disconnected by its Conn
	}
}

// Kick disconnects a player.
func (s *Server) Kick(playerID string) {
	s.mu.Lock()
	p := s.players[playerID]
	s.mu.Unlock()
	if p != nil {
		p.conn.Close()
	}
}

// Close stops accepting, disconnects everyone and waits for the server's
// goroutines.
func (s *Server) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	var conns []*Conn
	for _, p := range s.players {
		conns = append(conns, p.conn)
	}
	for c := range s.pending {
		conns = append(conns, c)
	}
	disc := s.disc
	s.mu.Unlock()

	s.ln.Close()
	if disc != nil {
		disc.close()
	}
	for _, c := range conns {
		c.Close()
	}
	s.wg.Wait()
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		c, err := s.ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			time.Sleep(10 * time.Millisecond) // e.g. too many open files
			continue
		}
		conn := NewConn(c, s.cfg.Conn)
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			conn.Close()
			continue
		}
		s.pending[conn] = true
		s.wg.Add(1)
		s.mu.Unlock()
		go s.serve(conn)
	}
}

// serve runs one connection: the join handshake, then its messages.
func (s *Server) serve(conn *Conn) {
	defer s.wg.Done()
	p, ok := s.join(conn)
	s.mu.Lock()
	delete(s.pending, conn)
	s.mu.Unlock()
	if !ok {
		return
	}
	id := p.info.ID
	for m := range conn.Incoming() {
		switch m.Type {
		case MsgChat:
			var chat ChatPayload
			if m.Decode(&chat) != nil {
				continue
			}
			chat.Text = clip(strings.TrimSpace(chat.Text), MaxChatLen)
			if chat.Text == "" {
				continue
			}
			out := mustMessage(MsgChat, chat)
			out.From = id
			s.Broadcast(out)
		case MsgGame:
			if s.cfg.OnGame != nil {
				m.From = id
				s.cfg.OnGame(m)
			}
		}
		// Anything else from a joined client (a second join, pings are
		// already filtered) is ignored.
	}
	s.leave(p)
}

// join waits for the client's join message and admits or refuses it.
func (s *Server) join(conn *Conn) (*serverPlayer, bool) {
	refuse := func(code, msg string) (*serverPlayer, bool) {
		conn.SendAndClose(mustMessage(MsgError, ErrorPayload{Code: code, Message: msg}))
		<-conn.Done()
		return nil, false
	}
	timer := time.NewTimer(s.cfg.JoinTimeout)
	defer timer.Stop()
	var m *Message
	select {
	case m = <-conn.Incoming():
		if m == nil { // closed before joining
			return nil, false
		}
	case <-timer.C:
		return refuse(ErrProtocol, "no join message")
	}
	if m.Type != MsgJoin {
		return refuse(ErrProtocol, fmt.Sprintf("expected join, got %v", m.Type))
	}
	var j JoinPayload
	if err := m.Decode(&j); err != nil {
		return refuse(ErrProtocol, err.Error())
	}
	if j.Version != ProtocolVersion {
		return refuse(ErrVersion, fmt.Sprintf("server speaks version %d, client %d", ProtocolVersion, j.Version))
	}
	name := strings.TrimSpace(j.Name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaxNameLen {
		return refuse(ErrName, fmt.Sprintf("name must be 1–%d characters", MaxNameLen))
	}
	if j.Model < 0 || j.Model >= Models {
		return refuse(ErrModel, "unknown hero model")
	}
	clan := strings.TrimSpace(j.Clan)
	if !utf8.ValidString(clan) || utf8.RuneCountInString(clan) > MaxClanLen {
		return refuse(ErrClan, fmt.Sprintf("clan must be at most %d characters", MaxClanLen))
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		conn.Close()
		return nil, false
	}
	if len(s.players) >= s.cfg.MaxPlayers {
		s.mu.Unlock()
		return refuse(ErrFull, "session is full")
	}
	for _, p := range s.players {
		if strings.EqualFold(p.info.Name, name) {
			s.mu.Unlock()
			return refuse(ErrName, "name is taken")
		}
	}
	p := &serverPlayer{info: PlayerInfo{ID: newID(), Name: name, Clan: clan, Model: j.Model}, conn: conn}
	s.players[p.info.ID] = p
	s.order = append(s.order, p.info.ID)
	conn.Send(mustMessage(MsgWelcome, WelcomePayload{PlayerID: p.info.ID, Session: s.sessionLocked()}))
	s.broadcastLocked(mustMessage(MsgPlayerJoined, p.info), p.info.ID)
	s.mu.Unlock()
	if s.cfg.OnJoin != nil {
		s.cfg.OnJoin(p.info)
	}
	return p, true
}

func (s *Server) leave(p *serverPlayer) {
	p.conn.Close()
	s.mu.Lock()
	delete(s.players, p.info.ID)
	for i, id := range s.order {
		if id == p.info.ID {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	s.broadcastLocked(mustMessage(MsgPlayerLeft, PlayerLeftPayload{ID: p.info.ID}))
	s.mu.Unlock()
	if s.cfg.OnLeave != nil {
		s.cfg.OnLeave(p.info)
	}
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand does not fail on supported platforms
	}
	return hex.EncodeToString(b[:])
}
