package resfmt

import (
	"os"
	"testing"
)

func TestLightsGlow(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/LIGHTS.RES")
	if err != nil {
		t.Skip("LIGHTS.RES not available:", err)
	}
	glow, err := LightsGlow(data)
	if err != nil {
		t.Fatal(err)
	}
	peak, lit := 0, 0
	for _, k := range glow {
		peak = max(peak, int(k))
		if k != 0 {
			lit++
		}
	}
	if peak != 43 || lit != 2334 {
		t.Fatalf("glow map: peak %d, %d lit pixels", peak, lit)
	}
	// The centre row is brightest; the corners are untouched.
	if glow[21*GlowWidth+32] != 43 || glow[0] != 0 {
		t.Fatalf("centre %d, corner %d", glow[21*GlowWidth+32], glow[0])
	}
}

func TestBrighten555(t *testing.T) {
	if got := Brighten555(0x7FFF, 43); got != 0x7FFF {
		t.Fatalf("white: %#x", got)
	}
	// Black at k = 43: red and green trunc(31·0.43) = 13, blue stays 0.
	if got := Brighten555(0, 43); got != 13<<10|13<<5 {
		t.Fatalf("black: %#x", got)
	}
	if got := Brighten555(0x001F, 0); got != 0x001F {
		t.Fatalf("k = 0: %#x", got)
	}
}
