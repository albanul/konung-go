package gamestate

import (
	"fmt"
	"testing"

	"konung1.5/internal/mapdata"
)

func TestExitsGame0(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Exits) != 118 {
		t.Fatalf("%d exits, want 118", len(g.Exits))
	}
	// KONUNG.1 (Volk's start) has three exits: to KONUNG.3, .11 and .42.
	want := map[int]MapExit{
		3:  {Index: 3, Facing: 1, From: 1, To: 3, Row: 142, Col: 134, MinRow: 158, MinCol: 4, MaxRow: 158, MaxCol: 4},
		11: {Index: 4, Facing: 3, From: 1, To: 11, Row: 110, Col: 4, MinRow: 155, MinCol: 59, MaxRow: 155, MaxCol: 59},
		42: {Index: 5, Facing: 0, From: 1, To: 42, Row: 62, Col: 37, MinRow: 218, MinCol: 17, MaxRow: 218, MaxCol: 17},
	}
	for _, x := range want {
		got, ok := g.ExitAt(1, x.MinRow, x.MinCol)
		if !ok || got != x {
			t.Errorf("ExitAt(1, %d, %d) = %+v, %v; want %+v", x.MinRow, x.MinCol, got, ok, x)
		}
	}
	if _, ok := g.ExitAt(1, 157, 4); ok {
		t.Error("ExitAt(1, 157, 4) found an exit outside every rectangle")
	}

	g.TakeExit(want[3])
	gr := g.Groups[0]
	hero := g.Entities[gr.First]
	if gr.Map != 3 || gr.Row != 142 || gr.Col != 134 || hero.Row != 0 || hero.Col != 0 || hero.Variant != 1 {
		t.Fatalf("after TakeExit: group map %d at (%d,%d), hero (%d,%d) facing %d", gr.Map, gr.Row, gr.Col, hero.Row, hero.Col, hero.Variant)
	}
	for _, p := range g.OnMap(3, nil) {
		if p.Index == gr.First {
			if p.Row != 142 || p.Col != 134 {
				t.Fatalf("hero placed at (%d,%d) on KONUNG.3, want the arrival cell (142,134)", p.Row, p.Col)
			}
			return
		}
	}
	t.Fatal("hero not placed on KONUNG.3")
}

// TestExitsUnderTeleports checks that the exits lie under teleport
// decorations (GRAPH.RES tile 285 or 286): the decoration's top-left is
// (−42, −21) ± 4 px from the centre of the exit's first cell. 109 of the 118
// exits match; the rest are larger areas (map edges, a gate) or use other
// tiles.
func TestExitsUnderTeleports(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	hits := 0
	for _, x := range g.Exits {
		data := load(t, fmt.Sprintf("KONUNG.%d", x.From))
		decs, err := mapdata.ParseDecorations(data)
		if err != nil {
			t.Fatal(err)
		}
		cx, cy := mapdata.CellCenter(mapdata.Cell{Row: x.MinRow, Col: x.MinCol})
		found := false
		for _, d := range decs {
			if (d.Tile == 285 || d.Tile == 286) && abs(d.X-cx+42) <= 4 && abs(d.Y-cy+21) <= 4 {
				found = true
			}
		}
		if found {
			hits++
		} else {
			t.Logf("exit %d (KONUNG.%d rows %d-%d cols %d-%d) has no teleport tile", x.Index, x.From, x.MinRow, x.MaxRow, x.MinCol, x.MaxCol)
		}
	}
	if hits != 109 {
		t.Errorf("%d exits under a teleport tile, want 109", hits)
	}
}
