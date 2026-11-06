package resfmt

import (
	"os"
	"testing"
)

func TestWorldMap(t *testing.T) {
	exe, err := os.ReadFile("../../assets/Konung/konung.exe")
	if err != nil {
		t.Skip("konung.exe not available:", err)
	}
	tb, err := NewExeTables(exe)
	if err != nil {
		t.Fatal(err)
	}
	for mode, want := range []struct{ x, y int }{{13, 13}, {80, 60}} {
		m := tb.WorldMap(mode)
		if m.Area.Min.X != want.x || m.Area.Min.Y != want.y || m.Area.Dx() != 502 || m.Area.Dy() != 361 {
			t.Errorf("mode %d area %v", mode, m.Area)
		}
	}
	m := tb.WorldMap(1)
	regions := map[int]bool{}
	for y, row := range m.Rows {
		if len(row) == 0 || !row[len(row)-1].Last {
			t.Fatalf("row %d: %v", y, row)
		}
		for _, r := range row {
			regions[r.Region] = true
		}
	}
	// 30 locations; the three start maps (1, 4, 7) among them.
	if len(regions) != 30 || !regions[0] || !regions[3] || !regions[6] {
		t.Errorf("%d regions: %v", len(regions), regions)
	}
	if r, ok := m.RegionAt(250, 180); !ok || r != 13 { // «Верхний лагерь»
		t.Errorf("RegionAt(250, 180) = %d, %v", r, ok)
	}
	if _, ok := m.RegionAt(0, 10); ok {
		t.Error("x = 0 is outside the area")
	}

	graph, err1 := os.ReadFile("../../assets/Konung/GRAPH.RES")
	intr, err2 := os.ReadFile("../../assets/Konung/Intr800r.res")
	if err1 != nil || err2 != nil {
		t.Skip("GRAPH.RES / Intr800r.res not available")
	}
	pals, _ := Palettes(graph)
	res, err := ParseInterface(intr)
	if err != nil {
		t.Fatal(err)
	}
	reveal, err := res.Image(WorldMapRevealSlot, pals)
	if err != nil || reveal.Bounds().Dx() != m.Area.Dx() || reveal.Bounds().Dy() != m.Area.Dy() {
		t.Fatalf("reveal slot: %v %v", reveal.Bounds(), err)
	}
	only := func(k int) func(int) bool { return func(r int) bool { return r == k } }
	img := m.Compose(reveal, reveal, only(13), -1)
	// Every opaque pixel lies in region 13.
	for y := range m.Area.Dy() {
		for x := range m.Area.Dx() {
			if img.RGBAAt(x, y).A == 0 {
				continue
			}
			if r, ok := m.RegionAt(x, y); ok && r != 13 {
				t.Fatalf("pixel (%d,%d) of region %d drawn", x, y, r)
			}
		}
	}
}
