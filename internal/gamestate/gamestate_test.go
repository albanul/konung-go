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
	g.HeroModelFromGame(0)
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
	g.HeroModelFromGame(0)
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

func TestPartyHUD(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	g.HeroModelFromGame(0)
	hero := g.Entities[g.Groups[0].First]
	if hero.HP != HPFull {
		t.Fatalf("Volk starts with HP %d", hero.HP)
	}
	// Volk (body model 0): slots 215 (healthy) .. 219 (dead).
	for _, c := range []struct{ hp, slot, bar, rows int }{
		{1600, 215, 0xAE, 40}, {1201, 215, 0xAE, 30}, {1200, 216, 0xAE, 30},
		{800, 217, 0xB1, 20}, {400, 218, 0xB0, 10}, {10, 218, 0xB0, 1},
	} {
		e := hero
		e.HP = c.hp
		slot, rows, ok := HPBar(e, 40)
		if PortraitSlot(e, true) != c.slot || !ok || slot != c.bar || rows != c.rows {
			t.Errorf("HP %d: portrait %d bar %#x/%d", c.hp, PortraitSlot(e, true), slot, rows)
		}
	}
	dead := hero
	dead.HP, dead.Flags4 = 0, dead.Flags4|0x80
	if _, _, ok := HPBar(dead, 40); PortraitSlot(dead, true) != 219 || ok {
		t.Error("dead hero")
	}
	// A companion uses its face (+4 & 7): villager 355 has face 2 -> 205.
	if s := PortraitSlot(g.Entities[355], false); s != 205 {
		t.Errorf("villager 355 portrait %d", s)
	}
	if PortraitSlot(g.Entities[367], false) != -1 {
		t.Error("animated models have no portrait")
	}
	if b := HUDButtonSlots(hero); b != [7]int{16, 15, 14, 11, 7, 12, 13} {
		t.Errorf("buttons %v", b)
	}
}

func TestStatusLines(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	exe := load(t, "konung.exe")
	tb, err := resfmt.NewExeTables(exe)
	if err != nil {
		t.Fatal(err)
	}
	g.HeroModelFromGame(0)
	for _, c := range []struct {
		i    int
		want string
	}{
		{0, "Волк, Воин, Уров:1, Здор:100, воин дружины Хар:10 Сил:15 Лов:10 Вын:10"},
		{355, "Бодрич, Купец, Уров:1, Здор:100, староста"},
		{356, "Волеслав, Храбрец, Уров:1, Здор:100, знахарь"},
		{359, "Исток, Воин, Уров:1, Здор:100"},
		{367, "Велиславна"},
		{536, "Путята, Охотник, Уров:3, Здор:100, искусный строитель"},
		{921, "Гигантский Муравей, Уров:1, Здор:100"},
	} {
		if got := resfmt.CP866ToUTF8(g.StatusLine(g.Entities[c.i], tb)); got != c.want {
			t.Errorf("entity %d: %q, want %q", c.i, got, c.want)
		}
	}
}

func TestSelectionMarker(t *testing.T) {
	for hp, want := range map[int]int{1600: 178, 801: 178, 800: 126, 401: 126, 400: 172, 0: 172} {
		if got := SelectionMarker(Entity{HP: hp}); got != want {
			t.Errorf("HP %d: slot %d, want %d", hp, got, want)
		}
	}
	// A full walk cycle (12 steps) carries the marker about one cell: E 58 px, S 32 px.
	if dx, dy := SelectionOffset(4, 11, true); dx != -29+55 || dy != -16 {
		t.Errorf("E step 11: %d,%d", dx, dy)
	}
	if dx, dy := SelectionOffset(6, 11, true); dx != -29 || dy != -16+27 {
		t.Errorf("S step 11: %d,%d", dx, dy)
	}
	if dx, dy := SelectionOffset(5, 3, false); dx != -29 || dy != -16 {
		t.Errorf("idle: %d,%d", dx, dy)
	}
}

// TestApprenticePost: an apprentice of post k (+4 bits 0x38 = k << 3, script
// 354) follows post k while it is held (0x00416A7A); with the post empty its
// bits clear (0x00416BEE). Women and children are never apprentices.
func TestApprenticePost(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	e := &g.Entities[359] // KONUNG.1 villager; posts 1–3 held, 4 empty
	if _, ok := g.ApprenticePost(359); ok {
		t.Fatal("a plain villager is an apprentice")
	}
	e.Flags4 |= 2 << 3
	p, ok := g.ApprenticePost(359)
	if !ok || p.Slot != 3 || p.Entity != 357 {
		t.Fatalf("post %+v, %v; want slot 3 held by 357", p, ok)
	}
	e.Flags4 = e.Flags4&^0x38 | 4<<3
	if _, ok := g.ApprenticePost(359); ok || e.Flags4&0x38 != 0 {
		t.Fatalf("empty post 4: ok %v, bits %#x", ok, e.Flags4&0x38)
	}
	if _, ok := g.ApprenticePost(367); ok { // +4 = 0x51 or 0x52: bits 0x10
		t.Fatal("a woman or child is an apprentice")
	}
}
