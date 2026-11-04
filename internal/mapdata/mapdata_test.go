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
