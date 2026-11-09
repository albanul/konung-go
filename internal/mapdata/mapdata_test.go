package mapdata

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"konung1.5/internal/resfmt"
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

// TestCellAtDiamonds: 0x00439BDC picks the cell whose 58 × 32 diamond
// around CellCenter holds the pixel.
func TestCellAtDiamonds(t *testing.T) {
	for y := 40; y < 400; y += 3 {
		for x := 70; x < 700; x += 7 {
			c := CellAt(x, y)
			cx, cy := CellCenter(c)
			dx, dy := x-cx, y-cy
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			if dx*FineCellHeight+dy*(FineCellWidth/2) > FineCellHeight*FineCellWidth/2 {
				t.Fatalf("(%d,%d) -> %v centred at (%d,%d): outside its diamond", x, y, c, cx, cy)
			}
		}
	}
	// A pixel the nearest-centre rule got wrong: inside the left corner of
	// (11, 5)'s diamond, but nearer to the centre of its NW neighbour.
	cx, cy := CellCenter(Cell{11, 5})
	if c := CellAt(cx-27, cy); c != (Cell{11, 5}) {
		t.Errorf("left corner -> %v", c)
	}
	if c := CellAt(-50, -50); c != (Cell{0, 0}) {
		t.Errorf("clamp -> %v", c)
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
	// A walker searches again where a 31-step path ends (0x0042218D).
	prev, segments := start, 0
	for prev != goal && segments < 50 {
		path := FindPath(prev, goal, blocked)
		if len(path) == 0 || len(path) > PathSteps {
			t.Fatalf("no path from %v to %v", prev, goal)
		}
		segments++
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
	if prev != goal || segments < 2 {
		t.Fatalf("stopped at %v after %d segments", prev, segments)
	}
}

// TestFindPathRules: the rules of 0x0043EDB7 on an open grid.
func TestFindPathRules(t *testing.T) {
	open := func(Cell) bool { return false }
	dirs := func(from Cell, path []Cell) []int {
		var out []int
		for _, c := range path {
			for _, n := range Neighbors(from) {
				if n.Cell == c {
					out = append(out, int(n.Dir))
				}
			}
			from = c
		}
		return out
	}
	start := Cell{100, 50}
	// East: straight steps (14 each) beat a zigzag of diagonals (2 × 10).
	if d := dirs(start, FindPath(start, Cell{100, 54}, open)); fmt.Sprint(d) != "[4 4 4 4]" {
		t.Errorf("east: %v", d)
	}
	// North: row −2 is one straight step.
	if d := dirs(start, FindPath(start, Cell{96, 50}, open)); fmt.Sprint(d) != "[2 2]" {
		t.Errorf("north: %v", d)
	}
	// One row and one column: diagonals first in the even-row order (NW, NE).
	if d := dirs(start, FindPath(start, Cell{99, 50}, open)); fmt.Sprint(d) != "[1]" {
		t.Errorf("north-west neighbour: %v", d)
	}
	// Long paths keep 31 steps.
	if p := FindPath(start, Cell{100, 120}, open); len(p) != PathSteps {
		t.Errorf("long path has %d steps, want %d", len(p), PathSteps)
	}
	// A blocked goal, a goal with no free neighbour, and start = goal fail.
	wall := Cell{100, 54}
	if FindPath(start, wall, func(c Cell) bool { return c == wall }) != nil {
		t.Error("path into a wall")
	}
	shut := func(c Cell) bool {
		for _, n := range Neighbors(wall) {
			if n.Cell == c {
				return true
			}
		}
		return false
	}
	if FindPath(start, wall, shut) != nil {
		t.Error("path to an enclosed goal")
	}
	if FindPath(start, start, open) != nil {
		t.Error("path to the start")
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

// TestWalkGridValues: section 1 of every installed KONUNG.N holds only
// 0x00 (open), 0x0F (wall), 0x40 (open, stops arrows) and 0x4F (wall that
// stops arrows). The word bits 0x1000 (map exit) and 0x2000 (pile) are set
// at run time; 0x8000 and the low-nibble values 1–14 never occur.
func TestWalkGridValues(t *testing.T) {
	paths, _ := filepath.Glob("../../assets/Konung/KONUNG.*")
	n := 0
	for _, p := range paths {
		if _, err := strconv.Atoi(strings.TrimPrefix(filepath.Ext(p), ".")); err != nil {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		g, err := ParseWalkGrid(data)
		if err != nil {
			t.Fatal(err)
		}
		n++
		for r := range g {
			for c, b := range g[r] {
				if b != 0x00 && b != 0x0F && b != 0x40 && b != 0x4F {
					t.Fatalf("%s cell (%d,%d) = %#x", filepath.Base(p), r, c, b)
				}
			}
		}
	}
	if n == 0 {
		t.Skip("no KONUNG.N installed")
	}
}

// TestGraphTerrainBlockEnd: the GRAPH.RES loader reads the terrain block's
// 8-byte header, its 0x1000-byte table and then the header's size of image
// data (0x0043B69D–0x0043B6D6), and goes straight on to the Type 0 blocks.
// The data ends exactly at resfmt.Type0BlockStart: there is no gap.
func TestGraphTerrainBlockEnd(t *testing.T) {
	graph, err := os.ReadFile("../../assets/Konung/GRAPH.RES")
	if err != nil {
		t.Skip("GRAPH.RES not available:", err)
	}
	size := int(binary.LittleEndian.Uint32(graph[graphTileBlock+4:]))
	if end := graphTileBlock + 8 + 0x1000 + size; end != resfmt.Type0BlockStart {
		t.Fatalf("terrain block ends at %#x, Type 0 blocks start at %#x", end, resfmt.Type0BlockStart)
	}
}

// TestObjectAnimation: 0x0042480C shows each variant for period ticks and
// wraps; KONUNG.42's cracked ground (slot 372, 2 variants, period 9) has
// two variant sprites at the same place and size.
func TestObjectAnimation(t *testing.T) {
	v, c := 1, 3
	var seen []int
	for range 9 {
		v, c = StepObject(v, c, 3, 3)
		seen = append(seen, v)
	}
	if want := []int{1, 1, 2, 2, 2, 0, 0, 0, 1}; fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Errorf("variants %v, want %v", seen, want)
	}
	if v, c := StepObject(0, 0, 3, 3); v != 0 || c != 0 {
		t.Errorf("a still object changed: %d %d", v, c)
	}

	dir := "../../assets/Konung/"
	data, err1 := os.ReadFile(dir + "KONUNG.42")
	objects, err2 := os.ReadFile(dir + "OBJECTS.RES")
	graph, err3 := os.ReadFile(dir + "GRAPH.RES")
	if err1 != nil || err2 != nil || err3 != nil {
		t.Skip("assets missing")
	}
	placements, err := ParsePlacements(data)
	if err != nil {
		t.Fatal(err)
	}
	objs, _ := LoadPlacedObjects(placements, objects, graph)
	animated := map[int]int{}
	for _, p := range DrawOrder(placements, objs) {
		sprites, err := ObjectVariantSprites(p, objects, graph)
		if err != nil {
			t.Fatal(err)
		}
		if sprites == nil {
			if p.Object.Period > 0 {
				t.Errorf("object %d: period %d but no variants", p.ID, p.Object.Period)
			}
			continue
		}
		animated[resfmt.MapObjectSlot(p.ID)]++
		if len(sprites) != p.Object.Variants {
			t.Fatalf("object %d: %d sprites, want %d", p.ID, len(sprites), p.Object.Variants)
		}
		for _, sp := range sprites[1:] {
			if sp.X != sprites[0].X || sp.Y != sprites[0].Y || sp.Depth != sprites[0].Depth || sp.Image == sprites[0].Image {
				t.Errorf("object %d: variant moved or repeated", p.ID)
			}
		}
		if p.Object.Variants == 2 && p.Object.Period == 9 {
			if v, c := StartObject(p.Object, func(n int) int { return n - 1 }); v != 1 || c != 9 {
				t.Errorf("start %d %d, want 1 9", v, c)
			}
		}
	}
	if animated[372] != 6 || animated[373] != 4 || animated[385] != 1 {
		t.Errorf("animated placements %v, want 372:6 373:4 385:1", animated)
	}
}
