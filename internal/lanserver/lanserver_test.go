package lanserver

import (
	"os"
	"strconv"
	"testing"
	"time"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

func load(t testing.TB) *World {
	t.Helper()
	const dir = "../../assets/Konung/"
	read := func(name string) []byte {
		b, err := os.ReadFile(dir + name)
		if err != nil {
			t.Skip("assets missing:", err)
		}
		return b
	}
	f := Files{GameRes: read("GAME.RES"), MapsRes: read("MAPS.RES"), Graph: read("GRAPH.RES"), Objects: read("OBJECTS.RES"), Exe: read("konung.exe")}
	for m := range f.Games {
		f.Games[m] = read("GAME." + strconv.Itoa(m))
	}
	f.Quests = read("questr.RES")
	w, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// All 20 maps load with their characters; after a while the creatures
// are alive and some have moved (wandering villagers, idle loops).
func TestWorldRuns(t *testing.T) {
	w := load(t)
	start := map[int]mapdata.Cell{}
	total := 0
	for n := 1; n <= Maps; n++ {
		s := w.Maps[n]
		if len(s.NPCs) == 0 {
			t.Errorf("map %d has no characters", n)
		}
		total += len(s.NPCs)
		for _, c := range w.Snapshot(n).Chars {
			start[c.Entity] = mapdata.Cell{Row: c.Row, Col: c.Col}
		}
	}
	t0 := time.Now()
	const ticks = 600
	for range ticks {
		w.Step()
	}
	per := time.Since(t0) / ticks
	t.Logf("%d characters on 20 maps; one tick of all maps: %v", total, per)
	alive, moved := 0, 0
	for n := 1; n <= Maps; n++ {
		for _, c := range w.Snapshot(n).Chars {
			if !c.Dead {
				alive++
			}
			if start[c.Entity] != (mapdata.Cell{Row: c.Row, Col: c.Col}) {
				moved++
			}
		}
	}
	if alive < total*9/10 || moved == 0 {
		t.Errorf("after %d ticks: %d of %d alive, %d moved", ticks, alive, total, moved)
	}
	t.Logf("after %d ticks: %d alive, %d moved", ticks, alive, moved)
	if per > 20*time.Millisecond {
		t.Errorf("a tick takes %v (60 ms budget)", per)
	}
}

// A player's hero enters on its type's start, walks by an order and takes
// a map exit to the exit's target map.
func TestPlayerWalksAndChangesMap(t *testing.T) {
	w := load(t)
	m, c, err := w.AddPlayer(3, 1, &netplay.Hero{Level: 2, Model: 1, Palette: 28, Fencing: 100, Strength: 15, Dexterity: 3, Endurance: 10, Charisma: 3, Class: 5})
	if err != nil {
		t.Fatal(err)
	}
	if m != gamestate.LANStartMap(1) {
		t.Fatalf("start map %d", m)
	}
	// The nearest open exit away from the arrival, by a walkable cell of
	// its trigger rectangle.
	var exit gamestate.MapExit
	var goal mapdata.Cell
	best := 1 << 30
	walk := w.Maps[m].Walk
	for _, x := range w.Game.Exits {
		if x.From != m || x.To == m || !w.Game.ExitOpen(x) || x.Contains(c.Row, c.Col) {
			continue
		}
		for r := x.MinRow; r <= x.MaxRow; r++ {
			for col := x.MinCol; col <= x.MaxCol; col++ {
				if walk.Blocked(r, col) {
					continue
				}
				if d := gamestate.Dist(c.Row, c.Col, r, col); d < best {
					best, exit, goal = d, x, mapdata.Cell{Row: r, Col: col}
				}
			}
		}
	}
	if best == 1<<30 {
		t.Fatal("no open exit on the start map")
	}
	w.Walk(3, goal.Row, goal.Col)
	for i := 0; i < 3000 && w.PlayerMap(3) == m; i++ {
		w.Step()
	}
	if got := w.PlayerMap(3); got != exit.To {
		h, _ := w.Hero(3)
		t.Fatalf("hero on map %d at (%d,%d), want map %d via exit cell %+v", got, h.Row, h.Col, exit.To, goal)
	}
	changed := false
	for _, e := range w.TakeEvents() {
		changed = changed || e.Kind == EventMapChange && e.Entity == 3 && e.Map == exit.To
	}
	if !changed {
		t.Error("no map change event")
	}
	found := false
	for _, ch := range w.Snapshot(exit.To).Chars {
		found = found || ch.Player && ch.Entity == 3
	}
	if !found {
		t.Error("the hero is not in the target map's snapshot")
	}
	w.RemovePlayer(3)
	if w.PlayerMap(3) != 0 || len(w.Maps[exit.To].Members) != 0 {
		t.Error("the hero stayed after leaving")
	}
}

// A hero ordered to attack a creature fights it on the server: HP drops on
// one side or the other.
func TestFightCreature(t *testing.T) {
	w := load(t)
	if _, _, err := w.AddPlayer(0, 0, nil); err != nil {
		t.Fatal(err)
	}
	// Map 2's creatures; the hero next to the first living one.
	s := w.Maps[2]
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
	w.MoveTo(0, 2, target.Cell)
	w.Stance(0, true)
	w.Right(0, target.Entity, target.Cell.Row, target.Cell.Col)
	creatureHP := s.Game.Entities[target.Entity].HP
	fought := false
	for range 3000 {
		w.Step()
		h, ok := w.Hero(0)
		e := s.Game.Entities[target.Entity]
		if e.HP < creatureHP || e.Dead() || !ok || h.HP < gamestate.HPFull {
			fought = true
			t.Logf("creature HP %d → %d (dead %v), hero HP %d", creatureHP, e.HP, e.Dead(), h.HP)
			break
		}
	}
	if !fought {
		t.Fatal("no fight with the creature")
	}
	// To the end: the creature's death or the hero's.
	for range 20000 {
		h, _ := w.Hero(0)
		if s.Game.Entities[target.Entity].Dead() || h.Dead() {
			t.Logf("ended: creature dead %v (HP %d of %d), hero dead %v (HP %d)", s.Game.Entities[target.Entity].Dead(), s.Game.Entities[target.Entity].HP, creatureHP, h.Dead(), h.HP)
			return
		}
		w.Step()
	}
	t.Error("the fight did not end")
}

// Two players' heroes fight each other on the server.
func TestPlayersFight(t *testing.T) {
	w := load(t)
	w.AddPlayer(0, 1, nil)
	m, c, _ := w.AddPlayer(1, 1, nil)
	if w.PlayerMap(0) != m {
		t.Fatal("not on one map")
	}
	h0, _ := w.Hero(0)
	if (mapdata.Cell{Row: h0.Row, Col: h0.Col}) == c {
		t.Fatal("two heroes on one cell")
	}
	w.Stance(0, true)
	w.Right(0, 1, c.Row, c.Col)
	for range 3000 {
		w.Step()
		if h, _ := w.Hero(1); h.HP < gamestate.HPFull || h.Dead() {
			a, _ := w.Hero(0)
			t.Logf("hero 1 HP %d (dead %v); attacker HP %d", h.HP, h.Dead(), a.HP)
			// The snapshot shows the same.
			for _, ch := range w.Snapshot(m).Chars {
				if ch.Entity == 1 && (ch.HP != h.HP || ch.Dead != h.Dead()) {
					t.Errorf("snapshot HP %d dead %v, hero %d %v", ch.HP, ch.Dead, h.HP, h.Dead())
				}
			}
			return
		}
	}
	t.Error("hero 0 never hurt hero 1")
}

func BenchmarkTick(b *testing.B) {
	w := load(b)
	for range 100 {
		w.Step()
	}
	b.ResetTimer()
	for range b.N {
		w.Step()
	}
}
