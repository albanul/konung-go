package game

import (
	"os"
	"testing"

	"konung1.5/internal/resfmt"
)

func TestNetworkLabelText(t *testing.T) {
	if got := resfmt.CP866ToUTF8(networkLabel); got != "Сетевая игра" {
		t.Errorf("networkLabel = %q", got)
	}
}

func TestNetworkButton(t *testing.T) {
	exe, err := os.ReadFile("../../assets/Konung/konung.exe")
	if err != nil {
		t.Skip("konung.exe not available:", err)
	}
	tables, err := resfmt.NewExeTables(exe)
	if err != nil {
		t.Fatal(err)
	}
	b, err := networkButton(tables)
	if err != nil {
		t.Fatal(err)
	}
	// Large font: the text is 111 px wide and its glyphs 20 px tall; scaled 2x
	// plus the 2 px shadow offset.
	if w, h := b[0].Bounds().Dx(), b[0].Bounds().Dy(); w != 224 || h != 42 {
		t.Errorf("label %dx%d, want 224x42", w, h)
	}
	// It must stay on the menu stone, above «Загрузить игру» (y 250).
	if networkLabelY+b[0].Bounds().Dy() > menuButtons800[itemLoad][1] {
		t.Error("label overlaps «Загрузить игру»")
	}
	if b[0].Bounds() != b[1].Bounds() {
		t.Error("normal and selected labels differ in size")
	}
}

func TestNetworkItemVisible(t *testing.T) {
	m := &Menu{lan: &lanUI{}}
	if !m.visible(itemNetwork) || m.visible(itemReturn) || !m.visible(itemLoad) {
		t.Error("without a game: want «Сетевая игра» and «Загрузить игру», not «Вернуться в игру»")
	}
	m.running = true
	if m.visible(itemNetwork) || !m.visible(itemReturn) {
		t.Error("in a game: want «Вернуться в игру», not «Сетевая игра»")
	}
	m = &Menu{}
	if m.visible(itemNetwork) {
		t.Error("«Сетевая игра» shown without the LAN files")
	}
}

// The installed MENU800R.RES (15 frames) must pass: «Сетевая игра» has no
// frames of its own.
func TestMenuFramesInstalled(t *testing.T) {
	const path = "../../assets/Konung/MENU800R.RES"
	frames, err := resfmt.LoadType5File(path)
	if err != nil {
		t.Skip("MENU800R.RES not available:", err)
	}
	if err := checkMenuFrames(path, len(frames)); err != nil {
		t.Error(err)
	}
	if err := checkMenuFrames(path, 14); err == nil {
		t.Error("14 frames accepted")
	}
}
