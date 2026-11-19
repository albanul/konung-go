package lanserver

import (
	"sync"
	"testing"
	"time"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
)

// session is a netplay server with the world behind it (as the dedicated
// server wires it) and a clock driver.
type session struct {
	srv  *netplay.Server
	host *Host
	stop chan struct{}

	mu    sync.Mutex
	kills []Kill
}

func startSession(t *testing.T, w *World) *session {
	t.Helper()
	ss := &session{stop: make(chan struct{})}
	var srv *netplay.Server
	ss.host = NewHost(w, func(id string, m *netplay.Message) error { return srv.Send(id, m) })
	ss.host.OnKill = func(k Kill) { ss.mu.Lock(); ss.kills = append(ss.kills, k); ss.mu.Unlock() }
	srv, err := netplay.Listen("127.0.0.1:0", netplay.ServerConfig{
		Start:   ss.host.Start,
		OnOrder: ss.host.Order,
		OnLeave: func(p netplay.PlayerInfo) { ss.host.Leave(p.ID) },
		Speed:   netplay.MaxSpeed, // triple speed: shorter tests
	})
	if err != nil {
		t.Fatal(err)
	}
	ss.srv = srv
	go func() {
		tk := time.NewTicker(5 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-ss.stop:
				return
			case <-tk.C:
				ss.host.Advance(int(srv.Clock().Tick), 8)
			}
		}
	}()
	t.Cleanup(func() { close(ss.stop); srv.Close() })
	return ss
}

// viewer is a client that keeps the server's view of its map.
type viewer struct {
	c     *netplay.Client
	at    netplay.EnterPayload
	mu    sync.Mutex
	chars map[int]netplay.CharState
	tick  int
	m     int
	bytes int // view message bytes received
	views int
	hits  []netplay.CharState // states with a flinch or death of another
	ply   map[int]netplay.PlayerPayload
	// Items (items_test.go): the last inventory, piles and pile window,
	// the messages, and the inventory messages' bytes.
	inv      netplay.InventoryPayload
	invs     int
	invBytes int
	piles    netplay.PilesPayload
	pileWin  netplay.PileWindowPayload
	msgs     [][]byte
	dlg      netplay.DialoguePayload
	pty      netplay.PartyPayload
	changes  []netplay.EnterPayload // map changes (MsgMapChange)
	arrows   []netplay.Event        // EventArrow (arrows_test.go)
}

func enter(t *testing.T, ss *session, name string, model int) *viewer {
	t.Helper()
	c, err := netplay.Dial(ss.srv.Addr().String(), netplay.JoinInfo{Name: name, Model: model}, netplay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	v := &viewer{c: c, chars: map[int]netplay.CharState{}, ply: map[int]netplay.PlayerPayload{}}
	c.Enter(model)
	deadline := time.After(5 * time.Second)
	for {
		select {
		case m := <-c.Incoming():
			if m.Type == netplay.MsgEntered {
				m.Decode(&v.at)
				go v.read()
				return v
			}
		case <-deadline:
			t.Fatal("no MsgEntered")
		}
	}
}

func (v *viewer) read() {
	for m := range v.c.Incoming() {
		switch m.Type {
		case netplay.MsgView:
			var p netplay.ViewPayload
			if m.Decode(&p) != nil {
				continue
			}
			v.mu.Lock()
			v.bytes += len(m.Payload)
			v.views++
			if p.Full {
				v.chars = map[int]netplay.CharState{}
			}
			v.m, v.tick = p.Map, p.Tick
			for _, c := range p.Chars {
				v.chars[c.Entity] = c
				if c.Entity != v.at.Slot && (c.Act == gamestate.ActFlinch || c.Act == gamestate.ActDeath || c.HP < gamestate.HPFull) && c.Player {
					v.hits = append(v.hits, c)
				}
			}
			for _, e := range p.Gone {
				delete(v.chars, e)
			}
			v.mu.Unlock()
		case netplay.MsgPlayer:
			var p netplay.PlayerPayload
			if m.Decode(&p) == nil {
				v.mu.Lock()
				v.ply[p.Slot] = p
				v.mu.Unlock()
			}
		case netplay.MsgInventory:
			var p netplay.InventoryPayload
			if m.Decode(&p) == nil {
				v.mu.Lock()
				v.inv = p
				v.invs++
				v.invBytes += len(m.Payload)
				v.mu.Unlock()
			}
		case netplay.MsgPiles:
			var p netplay.PilesPayload
			if m.Decode(&p) == nil {
				v.mu.Lock()
				v.piles = p
				v.mu.Unlock()
			}
		case netplay.MsgPileWindow:
			var p netplay.PileWindowPayload
			if m.Decode(&p) == nil {
				v.mu.Lock()
				v.pileWin = p
				v.mu.Unlock()
			}
		case netplay.MsgMapChange:
			var p netplay.EnterPayload
			if m.Decode(&p) == nil {
				v.mu.Lock()
				v.changes = append(v.changes, p)
				v.mu.Unlock()
			}
		case netplay.MsgParty:
			var p netplay.PartyPayload
			if m.Decode(&p) == nil {
				v.mu.Lock()
				v.pty = p
				v.mu.Unlock()
			}
		case netplay.MsgDialogue:
			var p netplay.DialoguePayload
			if m.Decode(&p) == nil {
				v.mu.Lock()
				v.dlg = p
				v.mu.Unlock()
			}
		case netplay.MsgEvents:
			var p netplay.EventsPayload
			if m.Decode(&p) == nil {
				v.mu.Lock()
				for _, e := range p.Events {
					if e.Kind == netplay.EventMsg {
						v.msgs = append(v.msgs, e.Text)
					}
					if e.Kind == netplay.EventArrow {
						v.arrows = append(v.arrows, e)
					}
				}
				v.mu.Unlock()
			}
		}
	}
}

func (v *viewer) state() (int, int, map[int]netplay.CharState) {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := map[int]netplay.CharState{}
	for k, c := range v.chars {
		out[k] = c
	}
	return v.m, v.tick, out
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for " + what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Two clients over the network see the same characters on the same cells;
// a walk order moves the hero; heroes fight by orders; the team option
// protects teammates; the views stay small.
func TestHostOverNetwork(t *testing.T) {
	w := load(t)
	ss := startSession(t, w)
	a := enter(t, ss, "Ivar", 1)
	b := enter(t, ss, "Olaf", 1)
	if a.at.Map != b.at.Map || a.at.Slot == b.at.Slot || (a.at.Row == b.at.Row && a.at.Col == b.at.Col) {
		t.Fatalf("entered %+v and %+v", a.at, b.at)
	}
	// Both see each other and the village's people; Olaf's look reached Ivar.
	waitUntil(t, "both heroes in both views", func() bool {
		_, _, ca := a.state()
		_, _, cb := b.state()
		_, aSeesB := ca[b.at.Slot]
		_, bSeesA := cb[a.at.Slot]
		return aSeesB && bSeesA && len(ca) > 10
	})
	a.mu.Lock()
	_, gotLook := a.ply[b.at.Slot]
	a.mu.Unlock()
	if !gotLook {
		t.Error("Ivar got no MsgPlayer for Olaf")
	}
	// The same cells for the same tick: compare the characters standing
	// still in both (a view arrives at each client a little apart).
	ma, _, ca := a.state()
	mb, _, cb := b.state()
	same, diff := 0, 0
	for e, x := range ca {
		if y, ok := cb[e]; ok && !x.Walking && !y.Walking {
			if x.Row == y.Row && x.Col == y.Col {
				same++
			} else {
				diff++
			}
		}
	}
	if ma != mb || same < 10 || diff > 2 {
		t.Errorf("maps %d/%d; standing characters on the same cell: %d, different: %d", ma, mb, same, diff)
	}
	// A walk order.
	target := netplay.CharState{Row: a.at.Row - 6, Col: a.at.Col}
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderWalk, Row: target.Row, Col: target.Col})
	waitUntil(t, "Ivar's walk", func() bool {
		_, _, cb := b.state()
		c := cb[a.at.Slot]
		return c.Row != a.at.Row || c.Col != a.at.Col
	})
	// Bandwidth: a second of views on the village map (triple speed).
	a.mu.Lock()
	b0, v0 := a.bytes, a.views
	a.mu.Unlock()
	time.Sleep(time.Second)
	a.mu.Lock()
	t.Logf("views on map %d: %d bytes in %d views per second at triple speed (%d ticks/s)", ma, a.bytes-b0, a.views-v0, 3*1000/60)
	a.mu.Unlock()

	// Team-safe: Ivar's blow at Olaf (one type) does nothing.
	w.TeamSafe = true
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderStance, On: true})
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderRight, Target: b.at.Slot, Row: b.at.Row, Col: b.at.Col})
	time.Sleep(1500 * time.Millisecond)
	_, _, cb = b.state()
	if c := cb[b.at.Slot]; c.HP < gamestate.HPFull {
		t.Errorf("a protected teammate was hurt: HP %d", c.HP)
	}
	// Without it, the blows land; Olaf's view shows its own HP drop.
	w.TeamSafe = false
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderRight, Target: b.at.Slot, Row: b.at.Row, Col: b.at.Col})
	waitUntil(t, "a hit on Olaf", func() bool {
		_, _, cb := b.state()
		return cb[b.at.Slot].HP < gamestate.HPFull
	})
	waitUntil(t, "Olaf's death", func() bool {
		_, _, cb := b.state()
		return cb[b.at.Slot].Dead
	})
	ss.mu.Lock()
	defer ss.mu.Unlock()
	found := false
	for _, k := range ss.kills {
		if k.Killer.Name == "Ivar" && k.VictimPlayer && k.Victim.Name == "Olaf" {
			found = true
		}
	}
	if !found {
		t.Errorf("no kill report: %+v", ss.kills)
	}
}
