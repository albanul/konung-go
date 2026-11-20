package netplay

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const wait = 2 * time.Second

func startServer(t *testing.T, cfg ServerConfig) *Server {
	t.Helper()
	s, err := Listen("127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func dial(t *testing.T, s *Server, name string, model int) *Client {
	t.Helper()
	c, err := Dial(s.Addr().String(), JoinInfo{Name: name, Model: model}, Options{})
	if err != nil {
		t.Fatalf("join %s: %v", name, err)
	}
	t.Cleanup(c.Close)
	return c
}

// waitFor returns the next message of type want on c, skipping others.
func waitFor(t *testing.T, c *Conn, want MsgType) *Message {
	t.Helper()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case m, ok := <-c.Incoming():
			if !ok {
				t.Fatalf("connection closed waiting for %v: %v", want, c.Err())
			}
			if m.Type == want {
				return m
			}
		case <-timer.C:
			t.Fatalf("no %v within %v", want, wait)
		}
	}
}

func waitClosed(t *testing.T, c *Conn) {
	t.Helper()
	select {
	case <-c.Done():
	case <-time.After(wait):
		t.Fatal("connection still open")
	}
}

func TestJoinChatLeave(t *testing.T) {
	s := startServer(t, ServerConfig{Name: "Тест"})
	a := dial(t, s, "Volk", 1)
	if a.Session.Name != "Тест" || a.Session.MaxPlayers != 16 || a.Session.Minutes != 180 || len(a.Session.Players) != 1 {
		t.Errorf("welcome session = %+v", a.Session)
	}
	b := dial(t, s, "Ragnar", 2)
	if len(b.Session.Players) != 2 || b.Session.Players[0].Name != "Volk" {
		t.Errorf("second welcome players = %+v", b.Session.Players)
	}
	var joined PlayerInfo
	if err := waitFor(t, a.Conn, MsgPlayerJoined).Decode(&joined); err != nil || joined.ID != b.PlayerID || joined.Model != 2 {
		t.Errorf("player-joined = %+v, %v", joined, err)
	}

	if err := b.Chat("  привет  "); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*Client{a, b} {
		m := waitFor(t, c.Conn, MsgChat)
		var chat ChatPayload
		if m.Decode(&chat); chat.Text != "привет" || m.From != b.PlayerID {
			t.Errorf("chat = %q from %q", chat.Text, m.From)
		}
	}

	b.Close()
	var left PlayerLeftPayload
	if waitFor(t, a.Conn, MsgPlayerLeft).Decode(&left); left.ID != b.PlayerID {
		t.Errorf("player-left = %q, want %q", left.ID, b.PlayerID)
	}
	if n := len(s.Session().Players); n != 1 {
		t.Errorf("%d players after leave", n)
	}
}

func TestJoinRefusals(t *testing.T) {
	s := startServer(t, ServerConfig{MaxPlayers: 2})
	dial(t, s, "Volk", 1)
	for _, tc := range []struct {
		name  string
		model int
		clan  string
		code  string
	}{
		{"volk", 0, "", ErrName}, // taken, any case
		{"   ", 1, "", ErrName},
		{strings.Repeat("я", MaxNameLen+1), 1, "", ErrName},
		{"Olaf", 7, "", ErrModel},
		{"Olaf", 2, strings.Repeat("я", MaxClanLen+1), ErrClan},
	} {
		_, err := Dial(s.Addr().String(), JoinInfo{Name: tc.name, Clan: tc.clan, Model: tc.model}, Options{})
		var je *JoinError
		if !errors.As(err, &je) || je.Code != tc.code {
			t.Errorf("join %q/%d/%q: %v, want code %s", tc.name, tc.model, tc.clan, err, tc.code)
		}
	}
	dial(t, s, "Ragnar", 2)
	_, err := Dial(s.Addr().String(), JoinInfo{Name: "Third", Model: 1}, Options{})
	var je *JoinError
	if !errors.As(err, &je) || je.Code != ErrFull {
		t.Errorf("third join: %v, want %s", err, ErrFull)
	}
}

// rawLines connects without the client and returns the conn and a reader.
func rawLines(t *testing.T, s *Server) (net.Conn, *bufio.Scanner) {
	t.Helper()
	c, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	sc := bufio.NewScanner(c)
	sc.Buffer(nil, MaxMessageSize)
	return c, sc
}

// readError reads lines until an error message, then expects EOF.
func readError(t *testing.T, c net.Conn, sc *bufio.Scanner) ErrorPayload {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(wait))
	var e ErrorPayload
	for sc.Scan() {
		var m Message
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			t.Fatalf("bad line %q", sc.Text())
		}
		if m.Type == MsgError {
			m.Decode(&e)
			break
		}
	}
	if e.Code == "" {
		t.Fatalf("no error message: %v", sc.Err())
	}
	if sc.Scan() {
		t.Errorf("server kept talking after the error: %q", sc.Text())
	}
	return e
}

func TestProtocolRefusals(t *testing.T) {
	s := startServer(t, ServerConfig{JoinTimeout: 150 * time.Millisecond})

	c, sc := rawLines(t, s) // silent client
	if e := readError(t, c, sc); e.Code != ErrProtocol {
		t.Errorf("silent client: %+v", e)
	}

	c, sc = rawLines(t, s) // wrong version
	b, _ := json.Marshal(mustMessage(MsgJoin, JoinPayload{Version: 99, JoinInfo: JoinInfo{Name: "Old", Model: 1}}))
	c.Write(append(b, '\n'))
	if e := readError(t, c, sc); e.Code != ErrVersion {
		t.Errorf("old client: %+v", e)
	}

	c, sc = rawLines(t, s) // chat before join
	b, _ = json.Marshal(mustMessage(MsgChat, ChatPayload{Text: "hi"}))
	c.Write(append(b, '\n'))
	if e := readError(t, c, sc); e.Code != ErrProtocol {
		t.Errorf("chat before join: %+v", e)
	}
}

func TestOversizeMessageDisconnects(t *testing.T) {
	s := startServer(t, ServerConfig{})
	c, sc := rawLines(t, s)
	go c.Write([]byte(`{"type":2,"payload":"` + strings.Repeat("x", MaxMessageSize) + "\"}\n"))
	c.SetReadDeadline(time.Now().Add(wait))
	for sc.Scan() {
	}
	var ne net.Error
	if err := sc.Err(); errors.As(err, &ne) && ne.Timeout() {
		t.Error("server did not close the connection")
	}
}

func TestGameRouting(t *testing.T) {
	got := make(chan *Message, 1)
	s := startServer(t, ServerConfig{OnGame: func(m *Message) { got <- m }})
	a := dial(t, s, "Volk", 1)
	m, _ := NewMessage(MsgGame, map[string]int{"walk": 5})
	m.From = "someone-else" // a client cannot choose its sender ID
	a.Send(m)
	select {
	case m := <-got:
		var p map[string]int
		if m.From != a.PlayerID || m.Decode(&p) != nil || p["walk"] != 5 {
			t.Errorf("OnGame got %+v", m)
		}
	case <-time.After(wait):
		t.Fatal("OnGame not called")
	}
	// The server can answer one player.
	if err := s.Send(a.PlayerID, mustMessage(MsgGame, map[string]int{"ok": 1})); err != nil {
		t.Fatal(err)
	}
	waitFor(t, a.Conn, MsgGame)
}

func TestKeepAliveHoldsIdleConnection(t *testing.T) {
	opts := Options{PingEvery: 20 * time.Millisecond, IdleTimeout: 120 * time.Millisecond}
	s := startServer(t, ServerConfig{Conn: opts})
	a, err := Dial(s.Addr().String(), JoinInfo{Name: "Volk", Model: 1}, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	time.Sleep(5 * opts.IdleTimeout) // idle far past the timeout
	a.Chat("still here")
	waitFor(t, a.Conn, MsgChat)
}

func TestIdleTimeoutDetectsDeadPeer(t *testing.T) {
	near, far := net.Pipe()
	defer far.Close()
	go func() { // a peer that reads but never sends
		buf := make([]byte, 1024)
		for {
			if _, err := far.Read(buf); err != nil {
				return
			}
		}
	}()
	c := NewConn(near, Options{PingEvery: time.Hour, IdleTimeout: 100 * time.Millisecond})
	waitClosed(t, c)
	if c.Err() == nil {
		t.Error("no error after idle timeout")
	}
}

func TestStalledPeerIsDisconnectedNotBlocking(t *testing.T) {
	near, far := net.Pipe() // far never reads, so writes block
	defer far.Close()
	c := NewConn(near, Options{QueueSize: 4, WriteTimeout: time.Hour, IdleTimeout: time.Hour, PingEvery: time.Hour})
	done := make(chan error)
	go func() {
		var err error
		for i := 0; i < 100 && err == nil; i++ {
			err = c.Send(mustMessage(MsgChat, ChatPayload{Text: fmt.Sprint(i)}))
		}
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrSlowPeer) && !errors.Is(err, ErrClosed) {
			t.Errorf("send error = %v", err)
		}
	case <-time.After(wait):
		t.Fatal("Send blocked on a stalled peer")
	}
	waitClosed(t, c)
	if !errors.Is(c.Err(), ErrSlowPeer) {
		t.Errorf("Err = %v, want ErrSlowPeer", c.Err())
	}
}

// go-tetris Client.Close panicked when called twice ("close of closed
// channel"); read and write errors both call it.
// A client that chats and leaves at once must not lose the line.
func TestCloseGracefullyDeliversQueued(t *testing.T) {
	s := startServer(t, ServerConfig{})
	a := dial(t, s, "Volk", 1)
	b := dial(t, s, "Ragnar", 2)
	for i := range 20 {
		b.Chat(fmt.Sprint("line ", i))
	}
	b.CloseGracefully()
	for i := range 20 {
		var chat ChatPayload
		if waitFor(t, a.Conn, MsgChat).Decode(&chat); chat.Text != fmt.Sprint("line ", i) {
			t.Fatalf("got %q, want line %d", chat.Text, i)
		}
	}
	waitFor(t, a.Conn, MsgPlayerLeft)
}

func TestOnJoinOnLeave(t *testing.T) {
	events := make(chan string, 4)
	s := startServer(t, ServerConfig{
		OnJoin:  func(p PlayerInfo) { events <- "join " + p.Name },
		OnLeave: func(p PlayerInfo) { events <- "leave " + p.Name },
	})
	c, err := Dial(s.Addr().String(), JoinInfo{Name: "Volk", Model: 1}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	for _, want := range []string{"join Volk", "leave Volk"} {
		select {
		case got := <-events:
			if got != want {
				t.Errorf("event %q, want %q", got, want)
			}
		case <-time.After(wait):
			t.Fatalf("no %q", want)
		}
	}
}

func TestCloseConcurrentlyIsSafe(t *testing.T) {
	near, far := net.Pipe()
	defer far.Close()
	c := NewConn(near, Options{})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); c.Close() }()
		go func() { defer wg.Done(); c.Send(mustMessage(MsgChat, ChatPayload{Text: "x"})) }()
	}
	wg.Wait()
	waitClosed(t, c)
	if err := c.Send(mustMessage(MsgChat, ChatPayload{Text: "late"})); !errors.Is(err, ErrClosed) {
		t.Errorf("send after close = %v", err)
	}
}

// go-tetris GameLobby.AddPlayer deadlocked (write lock held while
// broadcasting takes the read lock). Sixteen players joining at once, each
// broadcast to the others, must all get in; the seventeenth is refused.
func TestSixteenPlayersJoinConcurrently(t *testing.T) {
	s := startServer(t, ServerConfig{})
	var wg sync.WaitGroup
	clients := make([]*Client, 16)
	errs := make([]error, 16)
	for i := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			clients[i], errs[i] = Dial(s.Addr().String(), JoinInfo{Name: fmt.Sprintf("p%02d", i), Model: i % 3}, Options{})
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("player %d: %v", i, err)
		}
		defer clients[i].Close()
	}
	if n := len(s.Session().Players); n != 16 {
		t.Fatalf("%d players", n)
	}
	var je *JoinError
	if _, err := Dial(s.Addr().String(), JoinInfo{Name: "late", Model: 1}, Options{}); !errors.As(err, &je) || je.Code != ErrFull {
		t.Errorf("17th join: %v", err)
	}
	// Everyone chats once; everyone hears all sixteen lines, in each
	// sender's order.
	for i, c := range clients {
		c.Chat(fmt.Sprintf("line %d", i))
	}
	for _, c := range clients {
		heard := map[string]bool{}
		for len(heard) < 16 {
			var chat ChatPayload
			waitFor(t, c.Conn, MsgChat).Decode(&chat)
			heard[chat.Text] = true
		}
	}
}

func TestServerCloseDisconnectsEveryone(t *testing.T) {
	s, err := Listen("127.0.0.1:0", ServerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	a := dial(t, s, "Volk", 1)
	pending, _ := net.Dial("tcp", s.Addr().String()) // connected, not joined
	defer pending.Close()
	closed := make(chan struct{})
	go func() { s.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(wait):
		t.Fatal("Close hung")
	}
	waitClosed(t, a.Conn)
	s.Close() // twice is fine
	if _, err := Dial(s.Addr().String(), JoinInfo{Name: "Late", Model: 1}, Options{}); err == nil {
		t.Error("joined a closed server")
	}
}

func TestDiscovery(t *testing.T) {
	s := startServer(t, ServerConfig{Name: "Home Server"})
	if err := s.ServeDiscovery("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	dial(t, s, "Volk", 1)
	target := s.DiscoveryAddr().String()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	found, err := Discover(ctx, []string{target, target}) // asked twice, listed once
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("found %d servers: %+v", len(found), found)
	}
	f := found[0]
	if f.Addr != s.Addr().String() || f.Session.Name != "Home Server" || len(f.Session.Players) != 1 {
		t.Errorf("found %+v", f)
	}
	// The discovered address joins.
	c, err := Dial(f.Addr, JoinInfo{Name: "Ragnar", Model: 2}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}

func TestDiscoveryNoServer(t *testing.T) {
	pc, _ := net.ListenPacket("udp4", "127.0.0.1:0") // nobody answers here
	addr := pc.LocalAddr().String()
	pc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	found, err := Discover(ctx, []string{addr})
	if err != nil || len(found) != 0 {
		t.Errorf("Discover = %+v, %v", found, err)
	}
}

func TestBroadcastAddrs(t *testing.T) {
	addrs := BroadcastAddrs()
	if len(addrs) == 0 || addrs[0] != "255.255.255.255" {
		t.Errorf("BroadcastAddrs = %v", addrs)
	}
	for _, a := range addrs {
		if net.ParseIP(a).To4() == nil {
			t.Errorf("%q is not IPv4", a)
		}
	}
}

// memAccounts is a minimal AccountStore for tests.
type memAccounts struct {
	mu   sync.Mutex
	pass map[string]string
}

func (a *memAccounts) Login(j JoinInfo) (*Hero, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := strings.ToLower(j.Name)
	p, ok := a.pass[key]
	if !ok {
		a.pass[key] = j.Password
		return &Hero{Level: 1, Gold: 1000}, true, nil
	}
	if p != j.Password {
		return nil, false, ErrWrongPassword
	}
	return &Hero{Level: 1, Gold: 1000}, false, nil
}

func TestAccounts(t *testing.T) {
	s := startServer(t, ServerConfig{Accounts: &memAccounts{pass: map[string]string{}}})
	first, err := Dial(s.Addr().String(), JoinInfo{Name: "Volk", Password: "секрет", Model: 1}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Session.Players[0].New {
		t.Error("first login did not create the account")
	}
	if first.Hero == nil || first.Hero.Gold != 1000 {
		t.Errorf("welcome hero = %+v", first.Hero)
	}
	first.Close()
	waitGone := time.Now().Add(wait)
	for len(s.Session().Players) > 0 && time.Now().Before(waitGone) {
		time.Sleep(10 * time.Millisecond)
	}
	var je *JoinError
	if _, err := Dial(s.Addr().String(), JoinInfo{Name: "volk", Password: "wrong", Model: 1}, Options{}); !errors.As(err, &je) || je.Code != ErrPassword {
		t.Errorf("wrong password: %v", err)
	}
	again, err := Dial(s.Addr().String(), JoinInfo{Name: "Volk", Password: "секрет", Model: 1}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if again.Session.Players[0].New {
		t.Error("second login created the account again")
	}
}

func TestEnter(t *testing.T) {
	entered := make(chan string, 1)
	s := startServer(t, ServerConfig{
		Start: func(p PlayerInfo, _ *Hero) (EnterPayload, error) {
			return EnterPayload{Position: Position{Map: p.Model*3 + 1, Row: 110, Col: 32, Facing: 2}}, nil
		},
		OnEnter: func(p PlayerInfo) { entered <- p.Name },
	})
	c := dial(t, s, "Volk", 0)
	if err := c.Enter(-1); err != nil {
		t.Fatal(err)
	}
	var at EnterPayload
	if err := waitFor(t, c.Conn, MsgEntered).Decode(&at); err != nil || at != (EnterPayload{Position: Position{1, 110, 32, 2}}) {
		t.Errorf("entered = %+v, %v", at, err)
	}
	if name := <-entered; name != "Volk" {
		t.Errorf("OnEnter(%q)", name)
	}
	// A server without a world refuses.
	s2 := startServer(t, ServerConfig{})
	c2 := dial(t, s2, "Volk", 0)
	c2.Enter(-1)
	waitFor(t, c2.Conn, MsgError)
}

func TestStartFromGameRes(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/GAME.RES")
	if err != nil {
		t.Skip("GAME.RES not available:", err)
	}
	start, err := StartFromGameRes(data)
	if err != nil {
		t.Fatal(err)
	}
	at, err := start(PlayerInfo{Model: 2}, nil)
	if err != nil || at != (EnterPayload{Position: Position{Map: 7, Row: 65, Col: 8, Facing: 4}}) {
		t.Errorf("model 2 start = %+v, %v", at, err)
	}
}

// A «Новый герой» build the screen cannot produce is refused at the door.
func TestJoinRefusesBadHero(t *testing.T) {
	srv, err := Listen("127.0.0.1:0", ServerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	var je *JoinError
	_, err = Dial(srv.Addr().String(), JoinInfo{Name: "Volk", NewHero: &HeroChoice{Class: 5, Stats: [4]int{30, 30, 30, 30}}}, Options{})
	if !errors.As(err, &je) || je.Code != ErrHero {
		t.Errorf("impossible hero: %v", err)
	}
	c, err := Dial(srv.Addr().String(), JoinInfo{Name: "Volk", NewHero: &HeroChoice{Class: 5, Skill: 0, Stats: [4]int{3, 15, 3, 10}}}, Options{})
	if err != nil {
		t.Fatalf("template hero: %v", err)
	}
	c.Close()
}

// The hero type is the one picked at «Ok», not at joining: the start, the
// team the others see and the look's palette follow it.
func TestEnterPicksModel(t *testing.T) {
	s := startServer(t, ServerConfig{Start: func(p PlayerInfo, _ *Hero) (EnterPayload, error) {
		return EnterPayload{Position: Position{Map: p.Model*3 + 1, Row: 110, Col: 32, Facing: 2}}, nil
	}})
	a := dial(t, s, "Volk", 0)
	a.Enter(-1)
	waitFor(t, a.Conn, MsgEntered)
	b := dial(t, s, "Mikhail", 1) // joined as Драгомир, picks Михаил before «Ok»
	b.Enter(2)
	var at EnterPayload
	if waitFor(t, b.Conn, MsgEntered).Decode(&at); at.Map != 7 {
		t.Errorf("start map %d, want 7 (Михаил's village)", at.Map)
	}
	if p := s.Session().Players; len(p) != 2 || p[1].Model != 2 {
		t.Errorf("session players %+v, want Mikhail as model 2", p)
	}
	if h := (Hero{Look: 3}).WithModel(2); h.Model != 2 || h.Palette != 37 {
		t.Errorf("WithModel = %+v", h)
	}
}

// The clock runs only while someone is in the world, reaches entering
// players with the speed and the session time left, and stops when the
// world empties.
func TestClock(t *testing.T) {
	s := startServer(t, ServerConfig{Speed: 1, Start: func(p PlayerInfo, _ *Hero) (EnterPayload, error) {
		return EnterPayload{Position: Position{Map: 1, Row: 110, Col: 32, Facing: 2}}, nil
	}})
	if c := s.Clock(); c.Running || c.Tick != 0 || c.SessionLeft != -1 {
		t.Errorf("idle clock = %+v", c)
	}
	s.SetSessionEnd(time.Now().Add(90 * time.Second))
	a := dial(t, s, "Volk", 0)
	a.Enter(-1)
	waitFor(t, a.Conn, MsgEntered)
	var c ClockPayload
	if waitFor(t, a.Conn, MsgClock).Decode(&c); !c.Running || c.Speed != 1 || c.SessionLeft < 85 || c.SessionLeft > 90 {
		t.Errorf("clock on entering = %+v", c)
	}
	time.Sleep(150 * time.Millisecond) // 2 × 150 / 60 = 5 ticks at double speed
	if c := s.Clock(); c.Tick < 4 {
		t.Errorf("clock after 150 ms at double speed = %+v", c)
	}
	a.Close()
	deadline := time.Now().Add(2 * time.Second)
	for s.Clock().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	stopped := s.Clock()
	time.Sleep(100 * time.Millisecond)
	if c := s.Clock(); c.Running || c.Tick != stopped.Tick {
		t.Errorf("clock with nobody in the world = %+v (was %+v)", c, stopped)
	}
}

// Orders reach OnOrder only after entering, with the player.
func TestOrders(t *testing.T) {
	got := make(chan OrderPayload, 4)
	s := startServer(t, ServerConfig{
		Start: func(p PlayerInfo, _ *Hero) (EnterPayload, error) {
			return EnterPayload{Position: Position{Map: 4, Row: 66, Col: 57}, Slot: 3}, nil
		},
		OnOrder: func(p PlayerInfo, o OrderPayload) {
			if p.Name == "Volk" {
				got <- o
			}
		},
	})
	a := dial(t, s, "Volk", 1)
	a.Order(OrderPayload{Kind: OrderWalk, Row: 1, Col: 2}) // not entered: dropped
	a.Enter(-1)
	var at EnterPayload
	if waitFor(t, a.Conn, MsgEntered).Decode(&at); at.Slot != 3 || at.Map != 4 {
		t.Errorf("entered %+v", at)
	}
	a.Order(OrderPayload{Kind: OrderRight, Target: 17, Row: 60, Col: 50})
	select {
	case o := <-got:
		if o.Kind != OrderRight || o.Target != 17 || o.Row != 60 {
			t.Errorf("order %+v", o)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no order")
	}
	select {
	case o := <-got:
		t.Errorf("a second order %+v (the one before entering was not dropped)", o)
	case <-time.After(100 * time.Millisecond):
	}
}

// A team line reaches only the players of the sender's hero type (the
// sender too); a line to everyone reaches all; the server names the
// sender and clears a client's System flag.
func TestTeamChat(t *testing.T) {
	s := startServer(t, ServerConfig{Name: "Тест"})
	a := dial(t, s, "Ivar", 1)
	b := dial(t, s, "Olaf", 1)
	c := dial(t, s, "Volk", 0)
	if err := a.ChatTeam("за мной"); err != nil {
		t.Fatal(err)
	}
	for _, cl := range []*Client{a, b} {
		var chat ChatPayload
		if waitFor(t, cl.Conn, MsgChat).Decode(&chat); chat.Text != "за мной" || chat.Name != "Ivar" || !chat.Team || chat.System {
			t.Errorf("team chat = %+v", chat)
		}
	}
	// Everyone: Volk gets this one, and it is the first chat he gets.
	if err := a.Send(mustMessage(MsgChat, ChatPayload{Text: "всем", System: true})); err != nil {
		t.Fatal(err)
	}
	var chat ChatPayload
	if waitFor(t, c.Conn, MsgChat).Decode(&chat); chat.Text != "всем" || chat.Name != "Ivar" || chat.Team || chat.System {
		t.Errorf("Volk's first chat = %+v, want the line to everyone", chat)
	}
}

// The pause between sessions: the clock stands and orders are ignored;
// the session's end line and the start's clear reach the players in the
// world, not one still outside it.
func TestSessionPause(t *testing.T) {
	got := make(chan OrderPayload, 4)
	s := startServer(t, ServerConfig{
		Start: func(p PlayerInfo, _ *Hero) (EnterPayload, error) {
			return EnterPayload{Position: Position{Map: 1, Row: 110, Col: 32}}, nil
		},
		OnOrder: func(p PlayerInfo, o OrderPayload) { got <- o },
	})
	a := dial(t, s, "Ivar", 1)
	a.Enter(-1)
	waitFor(t, a.Conn, MsgEntered)
	outside := dial(t, s, "Olaf", 1)

	s.EndChat("ИГРА ОКОНЧЕНА! Победа викингов!")
	s.SetPaused(true)
	var chat ChatPayload
	if waitFor(t, a.Conn, MsgChat).Decode(&chat); !chat.End || !chat.System || chat.Text != "ИГРА ОКОНЧЕНА! Победа викингов!" {
		t.Errorf("end line = %+v", chat)
	}
	var c ClockPayload
	if waitFor(t, a.Conn, MsgClock).Decode(&c); c.Running {
		t.Errorf("clock in the pause = %+v", c)
	}
	stood := s.Clock().Tick
	a.Order(OrderPayload{Kind: OrderWalk, Row: 1, Col: 1})
	time.Sleep(100 * time.Millisecond)
	if s.Clock().Tick != stood {
		t.Error("the clock ran in the pause")
	}
	select {
	case o := <-got:
		t.Fatalf("an order in the pause reached the world: %+v", o)
	default:
	}

	s.StartChat()
	s.SetPaused(false)
	if waitFor(t, a.Conn, MsgChat).Decode(&chat); !chat.Clear || chat.Text != "" {
		t.Errorf("start line = %+v", chat)
	}
	if waitFor(t, a.Conn, MsgClock).Decode(&c); !c.Running {
		t.Errorf("clock after the pause = %+v", c)
	}
	a.Order(OrderPayload{Kind: OrderWalk, Row: 2, Col: 2})
	select {
	case o := <-got:
		if o.Row != 2 {
			t.Errorf("order after the pause = %+v", o)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no order after the pause")
	}
	// Olaf, not in the world, got neither line.
	outside.Chat("hi")
	if waitFor(t, outside.Conn, MsgChat).Decode(&chat); chat.Text != "hi" {
		t.Errorf("Olaf's first chat = %+v, want his own line (no session lines outside the world)", chat)
	}
}
