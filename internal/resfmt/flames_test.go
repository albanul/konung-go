package resfmt

import (
	"image"
	"image/color"
	"os"
	"testing"
)

func TestFlames(t *testing.T) {
	exe, err := os.ReadFile("../../assets/Konung/konung.exe")
	if err != nil {
		t.Skip("konung.exe not available:", err)
	}
	tb, err := NewExeTables(exe)
	if err != nil {
		t.Fatal(err)
	}
	want := [FlameTypes][2]int{{0, 10}, {11, 21}, {22, 32}, {33, 43}, {44, 54}, {55, 59}, {60, 73}}
	for typ, w := range want {
		if s, e := tb.FlameCycle(typ); s != w[0] || e != w[1] {
			t.Errorf("type %d: %d–%d, want %v", typ, s, e, w)
		}
	}
	frames, err := LoadType5File("../../assets/Konung/FLAMES.RES")
	if err != nil {
		t.Skip("FLAMES.RES not available:", err)
	}
	if len(frames) != FlameFrames {
		t.Fatalf("%d frames", len(frames))
	}
	objs, err1 := os.ReadFile("../../assets/Konung/OBJECTS.RES")
	graph, err2 := os.ReadFile("../../assets/Konung/GRAPH.RES")
	if err1 != nil || err2 != nil {
		t.Skip("OBJECTS.RES / GRAPH.RES not available")
	}
	// Slot 48, a burning log house: five flames, drawn untinted, no shadow.
	o, err := LoadMapObject(objs, graph, 48, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Flames) != 5 || !o.SelfLit || o.Shadow != nil || o.Flames[0] != (Flame{Type: 3, DX: -169, DY: 24}) {
		t.Fatalf("slot 48 flames %+v selfLit %v", o.Flames, o.SelfLit)
	}
}

// The transparent conversion premultiplies by the brightness
// L = (2r + g + b) / 4 and sets alpha (32 − f)/32 from the keep table.
func TestTransparentFlame(t *testing.T) {
	c8 := func(v int) uint8 { return uint8(v * 255 / 31) }
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.SetRGBA(0, 0, color.RGBA{c8(31), c8(20), c8(4), 255})
	out := TransparentFlame(img)
	// L = (62 + 20 + 4) / 4 = 21: (31·21/32, 20·21/32, 4·21/32) = (20, 13, 2).
	p := out.RGBAAt(0, 0)
	if p.R != c8(20) || p.G != c8(13) || p.B != c8(2) {
		t.Fatalf("premultiplied %v", p)
	}
	if want := uint8((32 - int(flameKeep[20<<10|13<<5|2])) * 255 / 32); p.A != want || p.A == 0 {
		t.Fatalf("alpha %d, want %d", p.A, want)
	}
	if out.RGBAAt(1, 0).A != 0 {
		t.Fatal("transparent pixel must stay transparent")
	}
}
