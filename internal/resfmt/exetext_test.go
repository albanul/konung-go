package resfmt

import (
	"os"
	"testing"
)

func TestExeTextAndFonts(t *testing.T) {
	exe, err := os.ReadFile("../../assets/Konung/konung.exe")
	if err != nil {
		t.Skip("konung.exe not available:", err)
	}
	tb, err := NewExeTables(exe)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		va, i int
		want  string
	}{
		{CreatureNamesVA, 0, "Гигантский Муравей"}, {CreatureNamesVA, 14, "Ядовитый паук"},
		{FirstNamesVA, 10, "Волк"}, {FirstNamesVA, 3, "Бодрич"}, {NicknamesVA, 1, "Косой"},
	} {
		if got := CP866ToUTF8(tb.TableString(c.va, c.i)); got != c.want {
			t.Errorf("table %#x[%d] = %q, want %q", c.va, c.i, got, c.want)
		}
	}
	for _, va := range []int{FontLargeVA, FontSmallVA, FontSlotVA} {
		f, err := tb.Font(va)
		if err != nil {
			t.Fatal(err)
		}
		if f.First != 0x20 || f.Count != 209 || f.Glyph(' ') != nil || f.Glyph(0x80) == nil {
			t.Fatalf("font %#x: first %#x count %d", va, f.First, f.Count)
		}
		text := []byte("aaa bbb ccc ddd")
		w := f.Width([]byte("aaa bbb "))
		if lines := f.Wrap(text, w+1); len(lines) != 2 || string(lines[0]) != "aaa bbb " {
			t.Errorf("font %#x wrap: %q", va, lines)
		}
	}
}
