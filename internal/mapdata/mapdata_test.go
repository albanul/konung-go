package mapdata

import (
	"os"
	"testing"
)

func TestKonung1Terrain(t *testing.T) {
	mapData, err := os.ReadFile("../../assets/Konung/KONUNG.1")
	if err != nil {
		t.Skip("KONUNG.1 not available:", err)
	}
	graph, err := os.ReadFile("../../assets/Konung/GRAPH.RES")
	if err != nil {
		t.Skip("GRAPH.RES not available:", err)
	}
	m, err := Parse(mapData)
	if err != nil {
		t.Fatal(err)
	}
	if m.LastRow != 114 || m.LastCol != 32 {
		t.Fatalf("extents = row %d col %d, want row 114 col 32", m.LastRow, m.LastCol)
	}
	tiles, err := LoadTerrainTiles(graph)
	if err != nil {
		t.Fatal(err)
	}
	for r := range m.Cells {
		for c := range m.Cells[r] {
			for l, b := range m.Cells[r][c] {
				if b != 0 && Tile(tiles, b) == nil {
					t.Fatalf("cell (%d,%d) layer %d references undecodable tile byte %d", r, c, l, b)
				}
			}
		}
	}
}

func TestCellOriginStagger(t *testing.T) {
	for _, tc := range []struct{ r, c, x, y int }{{0, 0, 0, 0}, {1, 0, 58, 32}, {2, 3, 348, 64}} {
		if x, y := CellOrigin(tc.r, tc.c); x != tc.x || y != tc.y {
			t.Errorf("CellOrigin(%d,%d) = %d,%d want %d,%d", tc.r, tc.c, x, y, tc.x, tc.y)
		}
	}
}

func TestKonung1Objects(t *testing.T) {
	mapData, err := os.ReadFile("../../assets/Konung/KONUNG.1")
	if err != nil {
		t.Skip("KONUNG.1 not available:", err)
	}
	graph, err := os.ReadFile("../../assets/Konung/GRAPH.RES")
	if err != nil {
		t.Skip("GRAPH.RES not available:", err)
	}
	objects, err := os.ReadFile("../../assets/Konung/OBJECTS.RES")
	if err != nil {
		t.Skip("OBJECTS.RES not available:", err)
	}
	decorations, err := ParseDecorations(mapData)
	if err != nil {
		t.Fatal(err)
	}
	if len(decorations) != 373 || decorations[0] != (Decoration{Tile: 182, X: 1624, Y: 320}) {
		t.Fatalf("decorations: %d, first %+v", len(decorations), decorations[0])
	}
	placements, err := ParsePlacements(mapData)
	if err != nil {
		t.Fatal(err)
	}
	if len(placements) != 195 {
		t.Fatalf("placements = %d, want 195", len(placements))
	}
	if p := placements[0]; p.ID != 117 || p.Col != 27 || p.Row != 33 {
		t.Fatalf("first placement %+v", p)
	}
	if x, y := placements[0].Pos(); x != 27*58-29 || y != 33*16 {
		t.Fatalf("first placement pos %d,%d", x, y)
	}
	objs, errs := LoadPlacedObjects(placements, objects, graph)
	if len(errs) != 0 {
		t.Fatalf("object decode errors: %v", errs)
	}
	sorted := DrawOrder(placements, objs)
	if len(sorted) != len(placements) {
		t.Fatalf("sorted %d of %d placements", len(sorted), len(placements))
	}
	for i := 1; i < len(sorted); i++ {
		if sorted[i-1].DepthKey() > sorted[i].DepthKey() {
			t.Fatalf("draw order not sorted at %d", i)
		}
	}
}

func TestFineGrid(t *testing.T) {
	for _, c := range []Cell{{86, 33}, {87, 33}, {0, 0}, {201, 12}} {
		x, y := CellCenter(c)
		if got := CellAt(x, y); got != c {
			t.Errorf("CellAt(CellCenter(%v)) = %v", c, got)
		}
		for _, n := range Neighbors(c) {
			nx, ny := CellCenter(n.Cell)
			if d := DirectionTo(float64(x), float64(y), float64(nx), float64(ny)); d != n.Dir {
				t.Errorf("neighbour %v of %v: DirectionTo = %d, table says %d", n.Cell, c, d, n.Dir)
			}
			dx, dy := nx-x, ny-y
			if dx*dx+dy*dy > 58*58 {
				t.Errorf("neighbour %v of %v is %d,%d px away", n.Cell, c, dx, dy)
			}
		}
	}
}

func TestFindPathKonung1(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/KONUNG.1")
	if err != nil {
		t.Skip(err)
	}
	g, err := ParseWalkGrid(data)
	if err != nil {
		t.Fatal(err)
	}
	blocked := func(c Cell) bool { return g.Blocked(c.Row, c.Col) }
	start := Cell{86, 33} // Volk's start
	goal := NearestFree(Cell{184, 40}, blocked, 6)
	path := FindPath(start, goal, blocked, 0)
	if len(path) == 0 || path[len(path)-1] != goal {
		t.Fatalf("no path from %v to %v", start, goal)
	}
	prev := start
	for _, c := range path {
		if blocked(c) {
			t.Fatalf("path crosses blocked cell %v", c)
		}
		ok := false
		for _, n := range Neighbors(prev) {
			ok = ok || n.Cell == c
		}
		if !ok {
			t.Fatalf("path step %v -> %v is not adjacent", prev, c)
		}
		prev = c
	}
}

// TestRenderSceneNoClip: objects reaching past the terrain are drawn, not
// clipped. KONUNG.21 (1450×1280 px of terrain) has objects up to 327 px
// above the top, 138 px left and 200 px right of it.
func TestRenderSceneNoClip(t *testing.T) {
	var data [3][]byte
	for i, f := range []string{"KONUNG.21", "GRAPH.RES", "OBJECTS.RES"} {
		b, err := os.ReadFile("../../assets/Konung/" + f)
		if err != nil {
			t.Skip(f, " not available:", err)
		}
		data[i] = b
	}
	m, err := Parse(data[0])
	if err != nil {
		t.Fatal(err)
	}
	tiles, err := LoadTerrainTiles(data[1])
	if err != nil {
		t.Fatal(err)
	}
	decorations, _ := ParseDecorations(data[0])
	placements, _ := ParsePlacements(data[0])
	objs, _ := LoadPlacedObjects(placements, data[2], data[1])
	sprites := ObjectSprites(DrawOrder(placements, objs))
	img := RenderScene(m, tiles, decorations, sprites)
	w, h := m.WorldSize()
	if b := img.Bounds(); b.Min.X > -138 || b.Min.Y > -327 || b.Max.X < w+200 || b.Max.Y < h {
		t.Fatalf("bounds %v, terrain %dx%d", b, w, h)
	}
	// The highest sprite's top row shows (nothing else is drawn above y = 0).
	top := sprites[0]
	for _, s := range sprites {
		if s.Y < top.Y {
			top = s
		}
	}
	ib := top.Image.Bounds()
	for x := ib.Min.X; x < ib.Max.X; x++ {
		if top.Image.RGBAAt(x, ib.Min.Y).A != 0 {
			if img.RGBAAt(top.X+x, top.Y+ib.Min.Y).A == 0 {
				t.Fatalf("pixel (%d,%d) of the top sprite was clipped", top.X+x, top.Y+ib.Min.Y)
			}
			return
		}
	}
	t.Fatal("the top sprite's first row is empty")
}
