package gamestate

import (
	"fmt"
	"os"
	"testing"

	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
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
	walk, err := mapdata.ParseWalkGrid(mapData)
	if err != nil {
		t.Fatal(err)
	}
	placed := g.OnMap(1, walk.Blocked)
	if len(placed) != 50 {
		t.Fatalf("entities on map 1 = %d, want 50 (player + 49)", len(placed))
	}
	objects, graph := load(t, "OBJECTS.RES"), load(t, "GRAPH.RES")
	tables, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if got := g.Overlays(g.Entities[355], tables); len(got) != 4 {
		t.Errorf("villager 355 overlays = %v, want 4 layers", got)
	}
	g.HeroModelFromGame(0, PlaceholderHeroPalette)
	if g.Entities[0].Model != 0 {
		t.Fatalf("hero model = %d, want 0 (Volk)", g.Entities[0].Model)
	}
	player := placed[0]
	for _, e := range placed {
		if e.Group == 0 {
			player = e
		}
	}
	for dir := 0; dir < 8; dir++ {
		for _, typ := range []int{player.Type, player.Type + 1} { // idle 6, walk 7
			q := player
			q.Type, q.Variant = typ, dir
			anim, err := g.LoadAnimation(q, objects, graph, tables)
			if err != nil || len(anim) != 12 {
				t.Errorf("hero type %d dir %d: %d steps, err %v", typ, dir, len(anim), err)
			}
		}
	}
	seen := map[[2]int]bool{}
	for _, e := range placed {
		if _, _, _, _, _, _, err := g.LoadSprite(e, objects, graph, tables); err != nil {
			t.Error(err)
		}
		anim, err := g.LoadAnimation(e, objects, graph, tables)
		if err != nil {
			t.Error(err)
		} else if len(anim) != 12 {
			t.Errorf("entity %d idle loop has %d steps, want 12", e.Index, len(anim))
		} else if anim[0].Sprite.Image == anim[2].Sprite.Image && !e.Animated() && len(g.Overlays(e.Entity, tables)) == 0 {
			t.Errorf("entity %d idle steps 0 and 2 share an image; expected different frames", e.Index)
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

func TestVillageWanderArea(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	a, ok := g.Groups[27].WanderArea()
	if !ok || a != (Area{36, 136, 24, 50}) {
		t.Fatalf("village area = %+v ok %v, want rows 36-136 cols 24-50", a, ok)
	}
	if _, ok := g.Groups[0].WanderArea(); ok {
		t.Fatal("player group must not wander")
	}
	walk, err := mapdata.ParseWalkGrid(load(t, "KONUNG.1"))
	if err != nil {
		t.Fatal(err)
	}
	seed := 1
	intn := func(n int) int { seed = seed*1103515245 + 12345; return int(uint(seed)>>16) % n }
	for i := 0; i < 200; i++ {
		r, c, ok := WanderTarget(a, intn, walk.Blocked)
		if !ok || !a.Contains(r, c) || walk.Blocked(r, c) {
			t.Fatalf("target %d,%d ok %v is outside the area or blocked", r, c, ok)
		}
	}
}

func TestKonung1WalkAnimations(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	walk, err := mapdata.ParseWalkGrid(load(t, "KONUNG.1"))
	if err != nil {
		t.Fatal(err)
	}
	objects, graph := load(t, "OBJECTS.RES"), load(t, "GRAPH.RES")
	tables, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	g.HeroModelFromGame(0, PlaceholderHeroPalette)
	for _, p := range g.OnMap(1, walk.Blocked) {
		for dir := 0; dir < 8; dir++ {
			q := p
			q.Type, q.Variant = p.Type+1, dir
			anim, err := g.LoadAnimation(q, objects, graph, tables)
			if err != nil || len(anim) < 2 {
				t.Errorf("entity %d (model %d animated %v) walk dir %d: %d steps, err %v", p.Index, p.Model, p.Animated(), dir, len(anim), err)
				break
			}
		}
	}
}

func TestVillagePosts(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	// KONUNG.1 village (group 27, settlement 9): the elder 355 and the role
	// characters 356-358 are posted on their file cells; slot 4 is empty.
	// Child 371 has its own dialogue but no post.
	if !g.Groups[27].VillageGroup() || g.Groups[3].VillageGroup() || g.Groups[64].VillageGroup() {
		t.Fatal("village group detection")
	}
	var posted []string
	gr := g.Groups[27]
	for k := 0; k < gr.Count; k++ {
		e := g.Entities[gr.First+k]
		if p, ok := g.PostOf(e); ok {
			if p.Row != e.Row || p.Col != e.Col || p.Facing != e.Variant || p.Dialogue != e.Dialogue {
				t.Errorf("entity %d post %+v does not match its record", e.Index, p)
			}
			posted = append(posted, fmt.Sprint(e.Index))
		}
	}
	if fmt.Sprint(posted) != "[355 356 357 358]" {
		t.Fatalf("posted villagers = %v", posted)
	}
	if VillageMoveThreshold(g.Entities[367]) != 75 || VillageMoveThreshold(g.Entities[359]) != 50 {
		t.Fatal("move thresholds")
	}
	walk, err := mapdata.ParseWalkGrid(load(t, "KONUNG.1"))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := gr.WanderArea()
	seed := 7
	intn := func(n int) int { seed = seed*1103515245 + 12345; return int(uint(seed)>>16) % n }
	for i := 0; i < 200; i++ {
		e := g.Entities[359+i%8]
		r, c, ok := VillageStepTarget(e.Row, e.Col, a, VillageRowRange, VillageColRange, intn, walk.Blocked)
		if ok && (r <= a.RowMin || r >= a.RowMax || c <= a.ColMin || c >= a.ColMax || walk.Blocked(r, c)) {
			t.Fatalf("target %d,%d outside the rectangle or blocked", r, c)
		}
	}
}
