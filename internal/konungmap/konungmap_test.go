package konungmap

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
