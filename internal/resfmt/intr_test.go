package resfmt

import (
	"os"
	"testing"
)

func TestInterfaceResources(t *testing.T) {
	graph, err := os.ReadFile("../../assets/Konung/GRAPH.RES")
	if err != nil {
		t.Skip("GRAPH.RES not available:", err)
	}
	pals, err := Palettes(graph)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		file          string
		frameW, portW int
	}{{"INTR640R.RES", 640, 52}, {"Intr800r.res", 800, 62}} {
		data, err := os.ReadFile("../../assets/Konung/" + c.file)
		if err != nil {
			t.Skip(c.file, "not available:", err)
		}
		r, err := ParseInterface(data)
		if err != nil {
			t.Fatal(c.file, err)
		}
		used := 0
		for i := 0; i < interfaceSlots; i++ {
			if _, _, ok := r.Slot(i); !ok {
				continue
			}
			used++
			if _, err := r.Image(i, pals); err != nil {
				t.Errorf("%s slot %d: %v", c.file, i, err)
			}
		}
		if used != 275 {
			t.Errorf("%s: %d used slots, want 275", c.file, used)
		}
		// Slot 3 is the HUD frame, 200..274 the portraits (palette 1).
		frame, _ := r.Image(3, pals)
		port, _ := r.Image(215, pals)
		if frame == nil || frame.Bounds().Dx() != c.frameW || port == nil || port.Bounds().Dx() != c.portW {
			t.Errorf("%s: unexpected frame/portrait size", c.file)
		}
		if _, pal, _ := r.Slot(215); pal != 0x200 {
			t.Errorf("%s: portrait palette offset %#x", c.file, pal)
		}
	}
}
