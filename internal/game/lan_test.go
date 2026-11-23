package game

import (
	"testing"

	"konung1.5/internal/resfmt"
)

func TestNetworkButton(t *testing.T) {
	b, err := networkButton()
	if err != nil {
		t.Fatal(err)
	}
	// The pictures: 171×51, transparent around the letters.
	if w, h := b[0].Bounds().Dx(), b[0].Bounds().Dy(); w != 171 || h != 51 {
		t.Errorf("label %dx%d, want 171x51", w, h)
	}
	for k, img := range b {
		clear, solid := 0, 0
		for y := range img.Bounds().Dy() {
			for x := range img.Bounds().Dx() {
				switch img.RGBAAt(x, y).A {
				case 0:
					clear++
				case 255:
					solid++
				}
			}
		}
		if clear == 0 || solid == 0 {
			t.Errorf("label %d: %d clear and %d solid pixels: not cut out of its background", k, clear, solid)
		}
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
