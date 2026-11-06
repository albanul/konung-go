package resfmt

import (
	"fmt"
	"image"
	"image/color"
)

// LIGHTS.RES is read whole by 0x0043B7FC: the 64×64 arrow glow →
// 0x005B0010, the 64×43 glow map → 0x00625518, then 19 × 0x1C80 light
// masks → 0x005B1010 (LightMasks). See docs/konung_potions.md.
const (
	GlowWidth  = 64
	GlowHeight = 43
	// ArrowGlowSize is the arrow glow's width and height; it is drawn
	// with its top-left 32 px left of and above the arrow's position
	// (floats at 0x00450224 / 0x00450228 = −32).
	ArrowGlowSize = 64
	glowOffset    = 0x1000
	lightsSize    = glowOffset + GlowWidth*GlowHeight + 19*0x1C80
	glowLevels    = 100 // rows of the brighten tables (0x0043B27C)
	glowStepVA    = 0x00450721
	glowStepVal   = float32(0.01) // the float at glowStepVA
)

// LightsGlow returns the glow map of LIGHTS.RES: one intensity byte per
// pixel, row-major, 0 = untouched (GAME data: 0..43).
func LightsGlow(data []byte) ([]byte, error) {
	if len(data) != lightsSize {
		return nil, fmt.Errorf("LIGHTS.RES has size %d, want %d", len(data), lightsSize)
	}
	return data[glowOffset : glowOffset+GlowWidth*GlowHeight], nil
}

// ArrowGlow returns the incendiary arrows' glow (the first 0x1000 bytes,
// 0x005B0010): 64×64 intensity bytes, row-major, a soft disc of 0..43.
// The arrow pass blends it under every arrow with special 0 (0x00426F5C,
// 0x00427489) through 0x0043DE4D.
func ArrowGlow(data []byte) ([]byte, error) {
	if len(data) != lightsSize {
		return nil, fmt.Errorf("LIGHTS.RES has size %d, want %d", len(data), lightsSize)
	}
	return data[:ArrowGlowSize*ArrowGlowSize], nil
}

// GlowFactor is the brightening fraction for intensity k: the table builder
// 0x0043B27C adds 0.01 in float32 once per row.
func GlowFactor(k int) float32 {
	var f float32
	for i := 0; i < k && i < glowLevels-1; i++ {
		f += glowStepVal
	}
	return f
}

// Brighten555 is the RGB555 path of the glow blit 0x0043DE4D on one screen
// pixel: red and green move toward 31 by trunc((31 − c)·f) (tables
// 0x004A17F0 and 0x0047CB2C), blue is kept.
func Brighten555(c uint16, k int) uint16 {
	if k == 0 {
		return c
	}
	f := GlowFactor(k)
	up := func(v uint16) uint16 { return v + uint16(int(float32(31-v)*f)) }
	r, g, b := c>>10&0x1F, c>>5&0x1F, c&0x1F
	return up(r)<<10 | up(g)<<5 | b
}

// GlowMask returns the glow map as a premultiplied mask for an additive
// blend dst' = src + dst·(1 − src) per channel: red and green are f·255,
// blue 0, so blue stays and red/green move toward full by f — Brighten555
// without the 5-bit truncation.
// w is the map's width (GlowWidth or ArrowGlowSize).
func GlowMask(glow []byte, w int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, len(glow)/w))
	for i, k := range glow {
		if k == 0 {
			continue
		}
		v := uint8(GlowFactor(int(k))*255 + 0.5)
		img.SetRGBA(i%w, i/w, color.RGBA{R: v, G: v, A: v})
	}
	return img
}
