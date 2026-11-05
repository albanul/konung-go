package resfmt

import (
	"fmt"
	"image"
)

// The lit terrain path (0x004257A9, see docs/konung_daylight.md): at night
// with «Световые эффекты» on, a terrain cell whose LIGHTS.N byte L is set is
// composed into a 114×64 buffer (0x004DEA70) and drawn by 0x0043DFE1 with
// light mask L − 1 of LIGHTS.RES, which brightens red and green.
const (
	LightMaskWidth  = 0x72 // 114
	LightMaskHeight = 0x40 // 64
	LightMaskCount  = 19
	lightMaskSize   = LightMaskWidth * LightMaskHeight // 0x1C80
	lightMaskOffset = glowOffset + GlowWidth*GlowHeight
)

// LightMasks returns the 19 light masks of LIGHTS.RES (0x005B1010): one
// byte per pixel of the 114×64 cell buffer, a percentage 0–31.
func LightMasks(data []byte) ([][]byte, error) {
	if len(data) != lightsSize {
		return nil, fmt.Errorf("LIGHTS.RES has size %d, want %d", len(data), lightsSize)
	}
	m := make([][]byte, LightMaskCount)
	for i := range m {
		o := lightMaskOffset + i*lightMaskSize
		m[i] = data[o : o+lightMaskSize]
	}
	return m, nil
}

// tintTables caches TintTable for −99…99.
var tintTables = func() (t [199][32]uint8) {
	for n := -99; n <= 99; n++ {
		t[n+99] = TintTable(n)
	}
	return
}()

func tintTable(n int) *[32]uint8 { return &tintTables[max(-99, min(99, n))+99] }

// LitTerrainLight is the light of one frame for the lit terrain path: the
// day tint 0x0051EC7C and the flicker percentages for red and green
// (0x0043FB99's arguments).
type LitTerrainLight struct {
	Day            Tint
	FlickR, FlickG int
}

// TerrainCellBuffer is a terrain cell composed into the 114×64 buffer
// (0x0043FD85 per layer): RGB555, 0 = skipped (uncovered or black: the
// buffer entries are b, g, r, 0, so black reads as empty).
type TerrainCellBuffer [lightMaskSize]uint16

// ComposeTerrainCell decodes a cell's layers (nil = none; the overlay over
// the base) into the buffer, clipped to 114×64. The game never clears the
// buffer between cells; this starts each cell empty.
func ComposeTerrainCell(base, overlay *image.RGBA) *TerrainCellBuffer {
	buf := &TerrainCellBuffer{}
	for _, layer := range []*image.RGBA{base, overlay} {
		if layer == nil {
			continue
		}
		b := layer.Bounds()
		for y := 0; y < min(b.Dy(), LightMaskHeight); y++ {
			row := layer.Pix[layer.PixOffset(b.Min.X, b.Min.Y+y):]
			for x := 0; x < min(b.Dx(), LightMaskWidth); x++ {
				p := row[4*x : 4*x+4]
				if p[3] == 0 {
					continue
				}
				c5 := func(v uint8) uint16 { return uint16((int(v)*31 + 127) / 255) }
				buf[y*LightMaskWidth+x] = c5(p[0])<<10 | c5(p[1])<<5 | c5(p[2])
			}
		}
	}
	return buf
}

// LitTables are one frame's channel tables for LightTerrainCell, indexed
// [light][5-bit value] and giving 8-bit output.
type LitTables struct {
	r, g [32][32]uint8
	b    [32]uint8
}

// NewLitTables builds the tables of 0x0043FB99 for a frame. With light
// L > 0, green = darken(min(day G + L, 0))[flicker(G)[g]] (the pointer for L
// is the identity once day + L ≥ 0), red likewise; with L = 0 green and red
// take the plain day tint. Blue always takes the day tint.
func NewLitTables(light LitTerrainLight) *LitTables {
	t := &LitTables{}
	day := light.Day
	to8 := func(v uint8) uint8 { return uint8(int(v) * 255 / 31) }
	fg, fr := tintTable(light.FlickG), tintTable(light.FlickR)
	for c := range 32 {
		t.b[c] = to8(tintTable(day.B)[c])
		t.g[0][c] = to8(tintTable(day.G)[c])
		t.r[0][c] = to8(tintTable(day.R)[c])
		for l := 1; l < 32; l++ {
			t.g[l][c] = to8(tintTable(min(day.G+l, 0))[fg[c]])
			t.r[l][c] = to8(tintTable(min(day.R+l, 0))[fr[c]])
		}
	}
	return t
}

// LightTerrainCell lights a composed cell into dst (114×64, top-left
// origin) with a light mask, like 0x0043DFE1's RGB555 path. Skipped
// buffer pixels are transparent.
func LightTerrainCell(dst *image.RGBA, buf *TerrainCellBuffer, mask []byte, t *LitTables) {
	pix := dst.Pix
	for i, c := range buf {
		o := 4 * i
		if c == 0 {
			pix[o], pix[o+1], pix[o+2], pix[o+3] = 0, 0, 0, 0
			continue
		}
		l := mask[i] & 31
		pix[o] = t.r[l][c>>10&0x1F]
		pix[o+1] = t.g[l][c>>5&0x1F]
		pix[o+2] = t.b[c&0x1F]
		pix[o+3] = 0xFF
	}
}
