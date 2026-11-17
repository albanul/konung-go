package gamestate

import (
	"os"
	"testing"
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

func TestEnterLANWorld(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/GAME.0")
	if err != nil {
		t.Skip("GAME.0 not available:", err)
	}
	g, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	g.EnterLANWorld(1, 110, 32, 2)
	on := g.OnMap(1, func(int, int) bool { return false })
	if len(on) != 1 || on[0].Group != 0 || on[0].Row != 110 || on[0].Col != 32 || on[0].Variant != 2 {
		t.Errorf("map 1 holds %+v", on)
	}
	if g.Exits != nil || g.Piles != nil || g.Settlements != nil {
		t.Error("single-player exits, piles or settlements left")
	}
}
