package game

import (
	"os"
	"strconv"
	"testing"

	"konung1.5/internal/lanserver"
)

// The viewer between reports (applyView with the reports advanced by
// netplay.CharState.Predict):
// a Волк hero in slot 3 walks about map 1 of the dedicated server's world
// while the villagers wander; after every tick's view, every walker of the
// scene stands, walks, faces and acts exactly as the server's character,
// though the server reports only the characters the viewer cannot predict.
func TestLANViewPredictsServer(t *testing.T) {
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
	const slot = 3
	if _, _, err := w.AddPlayer(slot, 0, nil); err != nil || w.PlayerMap(slot) != 1 {
		t.Fatalf("AddPlayer: %v, map %d", err, w.PlayerMap(slot))
	}
	s := viewScene(t)
	var v lanserver.Viewer
	home, _ := w.Hero(slot)
	reports, chars := 0, 0
	for tick := range 1500 {
		if tick%60 == 0 {
			r, c := home.Row, home.Col
			if tick/60%2 == 1 {
				r, c = r+10, c+5
			}
			w.Walk(slot, nil, r, c)
		}
		w.Step()
		view, ok := w.View(slot, &v)
		if !ok {
			t.Fatalf("tick %d: no view", tick)
		}
		s.applyView(view)
		reports += len(view.Chars)
		snap := w.Snapshot(1)
		chars += len(snap.Chars)
		if len(s.NPCs)+1 != len(snap.Chars) {
			t.Fatalf("tick %d: %d walkers, the server has %d characters", tick, len(s.NPCs)+1, len(snap.Chars))
		}
		for _, c := range snap.Chars {
			walker := s.Hero
			if c.Entity != slot {
				n := s.ByEntity[c.Entity]
				if n == nil {
					t.Fatalf("tick %d: no walker for %d", tick, c.Entity)
				}
				walker = &n.Walker
			}
			e := s.Entity(c.Entity)
			got := [8]int{walker.Cell.Row, walker.Cell.Col, b2int(walker.Walking), walker.StepT, int(walker.Dir), walker.Act, walker.ActStep, e.HP}
			want := [8]int{c.Row, c.Col, b2int(c.Walking), c.Step, c.Facing, c.Act, c.ActStep, c.HP}
			if c.Walking && (walker.From.Row != c.FromRow || walker.From.Col != c.FromCol) {
				t.Fatalf("tick %d entity %d: walking from %v, the server from %d,%d", tick, c.Entity, walker.From, c.FromRow, c.FromCol)
			}
			if got != want {
				t.Fatalf("tick %d entity %d: viewer [row col walking step dir act actstep hp] %v, server %v", tick, c.Entity, got, want)
			}
		}
	}
	t.Logf("%d reports for %d character-ticks", reports, chars)
	if reports*10 > chars {
		t.Errorf("%d reports for %d character-ticks: little predicted", reports, chars)
	}
}

func b2int(b bool) int {
	if b {
		return 1
	}
	return 0
}
