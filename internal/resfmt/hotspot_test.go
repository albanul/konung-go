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

func TestItemTablesAndCharWindow(t *testing.T) {
	exe, err := os.ReadFile("../../assets/Konung/konung.exe")
	if err != nil {
		t.Skip("konung.exe not available:", err)
	}
	tb, err := NewExeTables(exe)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		category, typ int
		name          string
	}{
		{2, 0x12, "Нож"}, {9, 5, "Лечебный бальзам"}, {11, 0x1A, "Левая часть амулета Дракона"}, {6, 0, "Ожерелье"},
	} {
		table := ItemTableOf(c.category)
		if got := CP866ToUTF8(tb.ItemName(table, c.typ)); got != c.name {
			t.Errorf("category %d type %#x = %q, want %q", c.category, c.typ, got, c.name)
		}
		if tb.ItemIcon(table, c.typ) < 0 {
			t.Errorf("%s has no icon", c.name)
		}
	}
	w := tb.CharWindow800()
	// Axe and blade share the hand box; the 25th number is «Свободный опыт».
	if w.Equipment[0] != w.Equipment[2] || w.Equipment[3].X != 255 || w.Stats[24].X != 700 || w.NameY != 9 || w.TitleY != 31 {
		t.Errorf("layout %+v", w)
	}
	if got := CP866ToUTF8(tb.CharWindowTitle(5)); got != "Воин" {
		t.Errorf("title 5 = %q", got)
	}
}
