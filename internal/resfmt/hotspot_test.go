package resfmt

import (
	"os"
	"testing"
)

func TestHUDHotspot800(t *testing.T) {
	exe, err := os.ReadFile("../../assets/Konung/konung.exe")
	if err != nil {
		t.Skip("konung.exe not available:", err)
	}
	tb, err := NewExeTables(exe)
	if err != nil {
		t.Fatal(err)
	}
	// Panel buttons 1..7 sit under the button images (0x0045AF00 positions),
	// portraits 30..39 under the portrait cells; the game view has none.
	for _, c := range []struct{ x, y, want int }{
		{20, 345, 1}, {20, 420, 2}, {100, 420, 3}, {20, 500, 4}, {110, 500, 5}, {20, 580, 6}, {100, 580, 7},
		{30, 30, 30}, {100, 300, 39}, {400, 300, 0}, {799, 0, 0},
	} {
		if got := tb.HUDHotspot800(c.x, c.y); got != c.want {
			t.Errorf("hotspot(%d,%d) = %d, want %d", c.x, c.y, got, c.want)
		}
	}
}
