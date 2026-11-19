package game

import (
	"os"
	"strconv"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/lanserver"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

// The viewer's arrows (flyViewArrows, addViewArrow) stay with the server's:
// a hero on the dedicated server's world shoots a creature; the viewer gets
// each released arrow as its event and, before every server tick, flies to
// that tick on the characters as they stand (as a view's arrows fly before
// its characters are applied). Every tick the viewer's flying arrows are
// the server's (the ones released this tick still waiting), at the same
// positions, and end on the same tick.
func TestViewArrowsFollowServer(t *testing.T) {
	const dir = "../../assets/Konung/"
	read := func(name string) []byte {
		b, err := os.ReadFile(dir + name)
		if err != nil {
			t.Skip("assets missing:", err)
		}
		return b
	}
	f := lanserver.Files{GameRes: read("GAME.RES"), MapsRes: read("MAPS.RES"), Graph: read("GRAPH.RES"), Objects: read("OBJECTS.RES"), Exe: read("konung.exe")}
	for m := range f.Games {
		f.Games[m] = read("GAME." + strconv.Itoa(m))
	}
	w, err := lanserver.Load(f)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.AddPlayer(0, 0, nil); err != nil {
		t.Fatal(err)
	}
	const m = 2
	sw := w.Maps[m]
	var target *world.NPC
	for _, n := range sw.NPCs {
		if n.Mode == world.NPCCreature && !sw.Game.Entities[n.Entity].Dead() {
			target = n
			break
		}
	}
	if target == nil {
		t.Fatal("no creature on map 2")
	}
	at := target.Cell
	at.Row += 8
	if !w.MoveTo(0, m, at) {
		t.Fatal("MoveTo")
	}
	w.LoadItems(0, append(w.SavedItems(0),
		lanserver.Item{Slot: lanserver.SlotEquip, Index: gamestate.WeaponBow, Category: gamestate.CatBow, Type: 29, Condition: 50, MaxCondition: 50},
		lanserver.Item{Slot: lanserver.SlotArrows, Category: gamestate.CatArrows, Type: 30, Count: 40}))
	w.Weapon(0, nil, gamestate.WeaponBow)
	w.Stance(0, nil, true)
	w.Right(0, nil, target.Entity, target.Cell.Row, target.Cell.Col)
	w.TakeEvents()

	// The viewer's scene: the server's map's characters, its own arrows.
	cw := *sw
	cw.Arrows = nil
	s := &startScene{World: &cw, view: &lanView{}}
	reported := 0
	for range 600 {
		s.flyViewArrows(w.Tick + 1)
		w.Step()
		waiting := 0
		for _, ev := range w.TakeEvents() {
			if ev.Kind != lanserver.EventArrow {
				continue
			}
			a := ev.Arrow
			reported++
			waiting++
			s.addViewArrow(netplay.Event{Kind: netplay.EventArrow, Entity: ev.Entity, Tick: ev.Tick,
				Arrow: &netplay.Arrow{X: a.X, Y: a.Y, VX: a.VX, VY: a.VY, Shooter: a.Shooter, Facing: a.Facing, Life: a.Life, Special: a.Special}})
		}
		if len(sw.Arrows) != len(s.Arrows)+waiting {
			t.Fatalf("tick %d: server %d arrows, viewer %d flying, %d new", w.Tick, len(sw.Arrows), len(s.Arrows), waiting)
		}
		for i, a := range s.Arrows {
			if b := sw.Arrows[i]; a.X != b.X || a.Y != b.Y || a.Life != b.Life {
				t.Fatalf("tick %d arrow %d: viewer (%v,%v) life %d, server (%v,%v) life %d", w.Tick, i, a.X, a.Y, a.Life, b.X, b.Y, b.Life)
			}
		}
	}
	if reported < 2 || len(sw.Arrows) != 0 {
		t.Fatalf("%d arrows reported, %d still flying", reported, len(sw.Arrows))
	}
	t.Logf("%d arrows, the viewer's in step with the server's", reported)
}

// An arrow released at a view's own tick arrives (its event) before that
// view: the view leaves it where it left the bow; the next view's tick is
// its first flight.
func TestViewArrowFirstFlight(t *testing.T) {
	s := &startScene{World: &world.World{Walk: &mapdata.WalkGrid{}}, view: &lanView{arrowTick: 9}}
	a := netplay.Arrow{X: 100, Y: 100, VX: 2, VY: 1, Shooter: 3, Life: 10}
	s.addViewArrow(netplay.Event{Kind: netplay.EventArrow, Entity: 3, Tick: 10, Arrow: &a})
	s.flyViewArrows(10)
	if len(s.Arrows) != 1 || s.Arrows[0].X != 100 || s.Arrows[0].Life != 10 {
		t.Fatalf("after its release's view: %+v", s.Arrows)
	}
	s.flyViewArrows(11)
	if len(s.Arrows) != 1 || s.Arrows[0].Life != 9 || s.Arrows[0].X != 100+2*gamestate.ArrowSubSteps {
		t.Fatalf("after the next view: %+v", s.Arrows)
	}
}
