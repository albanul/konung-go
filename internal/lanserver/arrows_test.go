package lanserver

import (
	"testing"
	"time"

	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/world"
)

// archer gives slot's hero a bow and arrows and puts it in the bow stance
// a few cells from map 2's first living creature, which it is ordered to
// shoot. It returns the map and the creature.
func archer(t *testing.T, w *World, slot int) (int, *world.NPC) {
	t.Helper()
	if _, _, err := w.AddPlayer(slot, 0, nil); err != nil {
		t.Fatal(err)
	}
	m, target := shootAt(t, w, slot)
	w.TakeEvents()
	return m, target
}

// shootAt is archer for a hero already in the world.
func shootAt(t *testing.T, w *World, slot int) (int, *world.NPC) {
	t.Helper()
	const m = 2
	s := w.Maps[m]
	var target *world.NPC
	for _, n := range s.NPCs {
		if n.Mode == world.NPCCreature && !s.Game.Entities[n.Entity].Dead() {
			target = n
			break
		}
	}
	if target == nil {
		t.Fatal("no creature on map 2")
	}
	items := append(w.SavedItems(slot),
		Item{Slot: SlotEquip, Index: gamestate.WeaponBow, Category: gamestate.CatBow, Type: 29, Condition: 50, MaxCondition: 50},
		Item{Slot: SlotArrows, Category: gamestate.CatArrows, Type: 30, Count: 40})
	at := target.Cell
	at.Row += 8
	if !w.MoveTo(slot, m, at) {
		t.Fatal("MoveTo")
	}
	w.LoadItems(slot, items)
	w.Weapon(slot, nil, gamestate.WeaponBow)
	w.Stance(slot, nil, true)
	w.Right(slot, nil, target.Entity, target.Cell.Row, target.Cell.Col)
	return m, target
}

// Every arrow released is reported once (EventArrow, on its map, at its
// tick), and the viewer's flight of the reported arrow (world.FlyShown, a
// tick after its release, on the characters as the previous tick left
// them) keeps the server's arrow's position tick by tick and ends on the
// same tick.
func TestArrowsReportedAndFlown(t *testing.T) {
	w := load(t)
	m, _ := archer(t, w, 0)
	s := w.Maps[m]
	var shown []gamestate.Arrow // the viewer's arrows
	type pending struct {
		a    gamestate.Arrow
		tick int
	}
	var waiting []pending
	reported, ended := 0, 0
	for range 600 {
		// The viewer's flight of tick w.Tick+1, before the server's step.
		next := w.Tick + 1
		kept := waiting[:0]
		for _, p := range waiting {
			if p.tick < next {
				shown = append(shown, p.a)
			} else {
				kept = append(kept, p)
			}
		}
		waiting = kept
		before := len(shown)
		shown = s.FlyShown(shown)
		ended += before - len(shown)
		w.Step()
		for _, ev := range w.TakeEvents() {
			if ev.Kind != EventArrow {
				continue
			}
			if ev.Map != m || ev.Tick != w.Tick || ev.Arrow.Shooter != 0 {
				t.Fatalf("arrow event %+v (map %d, tick %d)", ev, m, w.Tick)
			}
			reported++
			waiting = append(waiting, pending{ev.Arrow, ev.Tick})
		}
		// The server's arrows: the shown ones plus those released this tick.
		if len(s.Arrows) != len(shown)+len(waiting) {
			t.Fatalf("tick %d: server %d arrows, viewer %d flying + %d new", w.Tick, len(s.Arrows), len(shown), len(waiting))
		}
		for i, a := range shown {
			if b := s.Arrows[i]; a.X != b.X || a.Y != b.Y || a.Life != b.Life {
				t.Fatalf("tick %d arrow %d: viewer (%v,%v) life %d, server (%v,%v) life %d", w.Tick, i, a.X, a.Y, a.Life, b.X, b.Y, b.Life)
			}
		}
	}
	if reported < 2 || ended < 1 {
		t.Fatalf("%d arrows reported, %d ended", reported, ended)
	}
	t.Logf("%d arrows reported, %d ended in step with the server", reported, ended)
}

// Over the network the arrows go to the players on the shooter's map only:
// a hero on map 2 shoots, another hero there gets the arrows, a hero on
// another map gets none.
func TestArrowsOverNetwork(t *testing.T) {
	w := load(t)
	ss := startSession(t, w)
	a := enter(t, ss, "Ivar", 0)
	b := enter(t, ss, "Olaf", 0)
	c := enter(t, ss, "Sven", 1)
	ss.host.mu.Lock()
	m, target := shootAt(t, w, a.at.Slot)
	near := mapdata.NearestFree(mapdata.Cell{Row: target.Cell.Row + 12, Col: target.Cell.Col + 4}, w.Maps[m].Blocked, 8)
	if !w.MoveTo(b.at.Slot, m, near) {
		t.Fatal("MoveTo")
	}
	other := w.PlayerMap(c.at.Slot)
	ss.host.mu.Unlock()
	if other == m {
		t.Fatal("the third hero is on the shooter's map")
	}
	waitUntil(t, "arrows for the hero on the map", func() bool { return len(b.shots()) >= 2 })
	for _, e := range append(a.shots(), b.shots()...) {
		if e.Arrow == nil || e.Arrow.Shooter != a.at.Slot || e.Entity != a.at.Slot || e.Tick <= 0 || e.Arrow.Life <= 0 {
			t.Fatalf("arrow event %+v", e)
		}
	}
	if len(a.shots()) == 0 {
		t.Error("the shooter got no arrows")
	}
	time.Sleep(100 * time.Millisecond)
	if n := len(c.shots()); n != 0 {
		t.Errorf("the hero on map %d got %d arrows of map %d", other, n, m)
	}
}

func (v *viewer) shots() []netplay.Event {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]netplay.Event(nil), v.arrows...)
}
