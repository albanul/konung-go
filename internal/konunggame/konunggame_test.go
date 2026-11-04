package konunggame

import (
	"os"
	"testing"

	"konung1.5/internal/konungmap"
)

func load(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../assets/Konung/" + name)
	if err != nil {
		t.Skip(name, "not available:", err)
	}
	return b
}

func TestGame0Konung1(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	p := g.Groups[0]
	if p.Map != 1 || p.Row != 86 || p.Col != 33 || p.First != 0 || p.Count != 1 {
		t.Fatalf("player group = %+v", p)
	}
	hero := g.Entities[0]
	if hero.Animated() || hero.Model != 1 || hero.PaletteOffset/512 != 33 {
		t.Fatalf("hero = %+v", hero)
	}
	if x, y := WorldPos(86, 33); x != 33*58+58 || y != 86*16+16 {
		t.Fatalf("WorldPos = %d,%d", x, y)
	}

	mapData := load(t, "KONUNG.1")
	walk, err := konungmap.ParseWalkGrid(mapData)
	if err != nil {
		t.Fatal(err)
	}
	placed := g.OnMap(1, walk.Blocked)
	if len(placed) != 50 {
		t.Fatalf("entities on map 1 = %d, want 50 (player + 49)", len(placed))
	}
	objects, graph := load(t, "OBJECTS.RES"), load(t, "GRAPH.RES")
	seen := map[[2]int]bool{}
	for _, e := range placed {
		if _, _, _, _, _, _, err := e.LoadSprite(objects, graph); err != nil {
			t.Error(err)
		}
		k := [2]int{e.Row, e.Col}
		if seen[k] {
			t.Errorf("two entities share cell %v", k)
		}
		seen[k] = true
		if e.Spawned && walk.Blocked(e.Row, e.Col) {
			t.Errorf("spawned entity %d placed on a blocked cell", e.Index)
		}
	}
}
