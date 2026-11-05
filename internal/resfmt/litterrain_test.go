package resfmt

import (
	"image"
	"image/color"
	"os"
	"testing"
)

// LIGHTS.RES holds 19 light masks of 114×64 after the glow map; mask 0
// lights the whole cell, the others are gradients, all within 0–31.
func TestLightMasks(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/LIGHTS.RES")
	if err != nil {
		t.Skip("LIGHTS.RES not available:", err)
	}
	m, err := LightMasks(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != LightMaskCount {
		t.Fatalf("%d masks", len(m))
	}
	for i, mask := range m {
		for _, v := range mask {
			if v > 31 {
				t.Fatalf("mask %d has value %d", i, v)
			}
		}
	}
	if m[0][32*LightMaskWidth+57] != 31 {
		t.Fatal("mask 0 centre is not fully lit")
	}
}

func TestLightTerrainCell(t *testing.T) {
	c5 := func(v int) uint8 { return uint8(v * 255 / 31) }
	tile := image.NewRGBA(image.Rect(0, 0, 116, 70))
	tile.SetRGBA(0, 0, color.RGBA{c5(20), c5(20), c5(20), 255}) // lit pixel
	tile.SetRGBA(1, 0, color.RGBA{c5(20), c5(20), c5(20), 255}) // unlit pixel
	tile.SetRGBA(2, 0, color.RGBA{0, 0, 0, 255})                // black: skipped
	tile.SetRGBA(115, 0, color.RGBA{c5(20), 0, 0, 255})         // outside the buffer
	mask := make([]byte, LightMaskWidth*LightMaskHeight)
	mask[0] = 30
	dst := image.NewRGBA(image.Rect(0, 0, LightMaskWidth, LightMaskHeight))
	light := LitTerrainLight{Day: Tint{R: -60, G: -50, B: -70}, FlickR: 10, FlickG: 5}
	LightTerrainCell(dst, ComposeTerrainCell(tile, nil), mask, NewLitTables(light))
	got5 := func(x int) [4]int {
		p := dst.RGBAAt(x, 0)
		return [4]int{(int(p.R)*31 + 127) / 255, (int(p.G)*31 + 127) / 255, (int(p.B)*31 + 127) / 255, int(p.A)}
	}
	// Lit: red = darken 30 of brighten 10 of 20, green = darken 20 of
	// brighten 5 of 20, blue = darken 70.
	r := int(TintTable(-30)[TintTable(10)[20]])
	g := int(TintTable(-20)[TintTable(5)[20]])
	b := int(TintTable(-70)[20])
	if want := [4]int{r, g, b, 255}; got5(0) != want {
		t.Fatalf("lit pixel %v, want %v", got5(0), want)
	}
	if want := [4]int{int(TintTable(-60)[20]), int(TintTable(-50)[20]), b, 255}; got5(1) != want {
		t.Fatalf("unlit pixel %v, want %v", got5(1), want)
	}
	if dst.RGBAAt(2, 0).A != 0 || dst.RGBAAt(113, 1).A != 0 {
		t.Fatal("black and uncovered pixels must stay transparent")
	}
	// Light beyond the darkness clamps to the identity, no brightening past
	// the flicker.
	mask[0] = 31
	LightTerrainCell(dst, ComposeTerrainCell(tile, nil), mask, NewLitTables(LitTerrainLight{Day: Tint{G: -20}, FlickG: 5}))
	if got := got5(0)[1]; got != int(TintTable(5)[20]) {
		t.Fatalf("clamped green %d", got)
	}
}
