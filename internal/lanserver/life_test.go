package lanserver

import (
	"sync"
	"testing"
	"time"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
)

// kill makes a's hero kill b's (stance, right click) and waits for b's
// death as a sees it.
func kill(t *testing.T, a, b *viewer) {
	t.Helper()
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderStance, On: true})
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderRight, Target: b.at.Slot, Row: b.at.Row, Col: b.at.Col})
	waitUntil(t, "the kill", func() bool {
		_, _, chars := a.state()
		return chars[b.at.Slot].Dead
	})
}

// A dead hero stays in the world, its player connected and watching; a
// living player's «Слова Воскрешения» on the same map brings it back,
// wearing what it wore, saved alive; it takes orders again.
func TestDeadStaysAndRevives(t *testing.T) {
	w := load(t)
	ss := startSession(t, w)
	var mu sync.Mutex
	var saves, deaths []string
	ss.host.Save = func(p netplay.PlayerInfo, _ *netplay.Hero, _ []Item) {
		mu.Lock()
		saves = append(saves, p.Name)
		mu.Unlock()
	}
	ss.host.Died = func(p netplay.PlayerInfo, _ *netplay.Hero, _ []Item) {
		mu.Lock()
		deaths = append(deaths, p.Name)
		mu.Unlock()
	}
	a := enter(t, ss, "Ivar", 1)
	b := enter(t, ss, "Olaf", 1)
	kill(t, a, b)
	time.Sleep(300 * time.Millisecond)
	if _, ok := ss.host.Slot(b.c.PlayerID); !ok {
		t.Fatal("the dead hero's player was taken out of the world")
	}
	_, _, mine := b.state()
	if !mine[b.at.Slot].Dead {
		t.Error("the dead player does not see its own corpse")
	}
	mu.Lock()
	if len(deaths) != 1 || deaths[0] != "Olaf" {
		t.Errorf("deaths %v", deaths)
	}
	mu.Unlock()
	// Orders from the dead do nothing.
	b.c.Order(netplay.OrderPayload{Kind: netplay.OrderWalk, Row: b.at.Row + 10, Col: b.at.Col})
	time.Sleep(200 * time.Millisecond)
	_, _, chars := b.state()
	if c := chars[b.at.Slot]; c.Walking || !c.Dead {
		t.Errorf("the dead walked: %+v", c)
	}

	// Ivar reads the words: Olaf comes back.
	ss.host.mu.Lock()
	_, e := w.Maps[w.PlayerMap(a.at.Slot)].Member(a.at.Slot)
	e.Bag[0] = w.Game.SetItemKind(0, gamestate.CatQuest, gamestate.MiscWords)
	ss.host.mu.Unlock()
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderItem, Item: netplay.ItemBag, Index: 0, On: true})
	waitUntil(t, "Olaf revived", func() bool {
		_, _, chars := b.state()
		c := chars[b.at.Slot]
		return !c.Dead && c.HP > 0
	})
	waitUntil(t, "Olaf saved alive", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, n := range saves {
			if n == "Olaf" {
				return true
			}
		}
		return false
	})
	ss.host.mu.Lock()
	if e, _ := w.Hero(b.at.Slot); e.Equipment[gamestate.WeaponBlade] == 0 {
		t.Error("the revived hero lost its knife")
	}
	ss.host.mu.Unlock()
	_, _, chars = b.state()
	before := chars[b.at.Slot]
	b.c.Order(netplay.OrderPayload{Kind: netplay.OrderWalk, Row: before.Row - 6, Col: before.Col})
	waitUntil(t, "the revived hero walks", func() bool {
		_, _, chars := b.state()
		c := chars[b.at.Slot]
		return c.Row != before.Row || c.Walking
	})
}

// fakeClock is a settable clock for the logout rule.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// Leaving away from the home village leaves the hero in the world for
// Linger: its player cannot enter meanwhile (message 8), then it goes and
// is saved; leaving in the home village takes it out at once.
func TestLingerOnLeave(t *testing.T) {
	w := load(t)
	ss := startSession(t, w)
	clk := &fakeClock{t: time.Now()}
	ss.host.now = clk.now
	var mu sync.Mutex
	saved := map[string]int{}
	ss.host.Save = func(p netplay.PlayerInfo, _ *netplay.Hero, _ []Item) {
		mu.Lock()
		saved[p.Name]++
		mu.Unlock()
	}
	// At home (Драгомир's village, map 4): gone at once.
	a := enter(t, ss, "Ivar", 1)
	a.c.Close()
	waitUntil(t, "Ivar gone at home", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return saved["Ivar"] == 1
	})
	if ss.host.Lingering("Ivar") {
		t.Error("a hero leaving at home lingers")
	}

	// Away from home (map 5): it stays.
	b := enter(t, ss, "Olaf", 1)
	ss.host.mu.Lock()
	w.MoveTo(b.at.Slot, 5, mapdata.Cell{Row: 60, Col: 30})
	ss.host.mu.Unlock()
	b.c.Close()
	waitUntil(t, "Olaf lingers", func() bool { return ss.host.Lingering("Olaf") })
	ss.host.mu.Lock()
	if w.PlayerMap(b.at.Slot) != 5 {
		t.Error("the lingering hero left the world")
	}
	ss.host.mu.Unlock()

	// Entering again meanwhile: refused with the original's message 8.
	c, err := netplay.Dial(ss.srv.Addr().String(), netplay.JoinInfo{Name: "Olaf", Model: 1}, netplay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Enter(1)
	deadline := time.After(5 * time.Second)
	for refused := false; !refused; {
		select {
		case m := <-c.Incoming():
			if m.Type == netplay.MsgEntered {
				t.Fatal("entered while the hero lingers")
			}
			if m.Type == netplay.MsgError {
				var e netplay.ErrorPayload
				m.Decode(&e)
				if e.Code != netplay.ErrLeaving {
					t.Fatalf("refused with %q, want %q", e.Code, netplay.ErrLeaving)
				}
				refused = true
			}
		case <-deadline:
			t.Fatal("no answer to entering")
		}
	}
	mu.Lock()
	if saved["Olaf"] != 0 {
		t.Error("the lingering hero was saved before its time was up")
	}
	mu.Unlock()

	// Three minutes later it goes and is saved; entering works again.
	clk.add(LingerTime + time.Second)
	waitUntil(t, "Olaf gone after the linger", func() bool { return !ss.host.Lingering("Olaf") })
	mu.Lock()
	if saved["Olaf"] != 1 {
		t.Errorf("Olaf saved %d times after the linger", saved["Olaf"])
	}
	mu.Unlock()
	c.Close() // the refused game logs out
	waitUntil(t, "the refused login gone", func() bool {
		for _, p := range ss.srv.Session().Players {
			if p.Name == "Olaf" {
				return false
			}
		}
		return true
	})
	enter(t, ss, "Olaf", 1)
}

// The health a hero left with comes back (a port rule against leaving to
// heal); the session's end brings every hero back alive at its start with
// full HP.
func TestHealthAndSessionEnd(t *testing.T) {
	w := load(t)
	h := NewHost(w, nil)
	var saved *netplay.Hero
	h.Save = func(_ netplay.PlayerInfo, hero *netplay.Hero, _ []Item) { saved = hero }
	p := netplay.PlayerInfo{ID: "p1", Name: "Ivar", Model: 1}
	if _, err := h.Start(p, &netplay.Hero{Level: 1, Model: 1, Palette: 28, HP: 700, Poison: 3}); err != nil {
		t.Fatal(err)
	}
	slot, _ := h.Slot("p1")
	if e, _ := w.Hero(slot); e.HP != 700 || e.Poison != 3 {
		t.Fatalf("entered with HP %d poison %d, want the saved 700 and 3", e.HP, e.Poison)
	}
	// Hurt to 500, leave at home: saved with 500.
	_, e := w.Maps[w.PlayerMap(slot)].Member(slot)
	e.HP = 500
	h.Leave("p1")
	if saved == nil || saved.HP != 500 {
		t.Fatalf("saved %+v, want HP 500", saved)
	}
	// Enter again, die away from home, the session ends: back at the
	// start, alive, full HP, saved.
	if _, err := h.Start(p, saved); err != nil {
		t.Fatal(err)
	}
	slot, _ = h.Slot("p1")
	w.MoveTo(slot, 5, mapdata.Cell{Row: 60, Col: 30})
	_, e = w.Maps[5].Member(slot)
	e.HP = 0
	e.Flags4 |= 0x80
	h.Advance(w.Tick, 0) // the death is noticed
	saved = nil
	h.EndSession()
	got, _ := w.Hero(slot)
	if w.PlayerMap(slot) != gamestate.LANStartMap(1) || got.Dead() || got.HP != gamestate.HPFull {
		t.Errorf("after the session's end: map %d, dead %v, HP %d", w.PlayerMap(slot), got.Dead(), got.HP)
	}
	if saved == nil || saved.HP != gamestate.HPFull {
		t.Errorf("not saved alive at the session's end: %+v", saved)
	}
}
