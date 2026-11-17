package gamestate

import (
	"os"
	"testing"

	"konung1.5/internal/mapdata"
)

func TestLANStart(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/GAME.RES")
	if err != nil {
		t.Skip("GAME.RES not available:", err)
	}
	exits, err := LANExits(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(exits) != 192 {
		t.Errorf("%d exits, want 192", len(exits))
	}
	for model, want := range [][4]int{{1, 110, 32, 2}, {4, 66, 57, 0}, {7, 65, 8, 4}} {
		x, err := LANStart(exits, model)
		if err != nil {
			t.Fatal(err)
		}
		if got := [4]int{x.To, x.Row, x.Col, x.Facing}; got != want {
			t.Errorf("model %d start = %v, want %v", model, got, want)
		}
	}
}

// GAME.RES's exits connect the 20 LAN maps, 8 teleports per map (40 on map
// 12), and nearly all sit under a teleport stone of their MAPS.RES map.
func TestLANExitsOnMaps(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/GAME.RES")
	if err != nil {
		t.Skip("GAME.RES not available:", err)
	}
	mr, err := os.ReadFile("../../assets/Konung/MAPS.RES")
	if err != nil {
		t.Skip("MAPS.RES not available:", err)
	}
	exits, err := LANExits(data)
	if err != nil {
		t.Fatal(err)
	}
	maps, err := mapdata.SplitMapsRes(mr)
	if err != nil {
		t.Fatal(err)
	}
	per, hits := map[int]int{}, 0
	for _, x := range exits {
		if x.From < 1 || x.From > len(maps) || x.To < 1 || x.To > len(maps) {
			t.Fatalf("exit %d joins maps %d → %d", x.Index, x.From, x.To)
		}
		per[x.From]++
		decs, err := mapdata.ParseDecorations(maps[x.From-1].Map)
		if err != nil {
			t.Fatal(err)
		}
		cx, cy := mapdata.CellCenter(mapdata.Cell{Row: x.MinRow, Col: x.MinCol})
		for _, d := range decs {
			if (d.Tile == 285 || d.Tile == 286) && abs(d.X-cx+42) <= 4 && abs(d.Y-cy+21) <= 4 {
				hits++
				break
			}
		}
	}
	for m := 1; m <= len(maps); m++ {
		want := 8
		if m == 12 {
			want = 40
		}
		if per[m] != want {
			t.Errorf("map %d has %d exits, want %d", m, per[m], want)
		}
	}
	if hits != 177 {
		t.Errorf("%d exits under a teleport stone, want 177", hits)
	}
}

func TestEnterLANWorld(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/GAME.0")
	if err != nil {
		t.Skip("GAME.0 not available:", err)
	}
	g, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	lan := []MapExit{{Index: 3, From: 1, To: 15, Row: 66, Col: 57, MinRow: 65, MinCol: 7, MaxRow: 65, MaxCol: 7}}
	g.EnterLANWorld(1, 110, 32, 2, lan)
	on := g.OnMap(1, func(int, int) bool { return false })
	if len(on) != 1 || on[0].Group != 0 || on[0].Row != 110 || on[0].Col != 32 || on[0].Variant != 2 {
		t.Errorf("map 1 holds %+v", on)
	}
	if g.Piles != nil || g.Settlements != nil {
		t.Error("single-player piles or settlements left")
	}
	// The LAN exits replace the single-player ones; no settlement gates them.
	if x, ok := g.ExitAt(1, 65, 7); !ok || x.To != 15 || !g.ExitOpen(x) {
		t.Errorf("exit at (65,7) = %+v, %v", x, ok)
	}
}
