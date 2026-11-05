package resfmt

import (
	"encoding/binary"
	"image"
)

// FLAMES.RES: 74 Type 5 frames (loader 0x0043BA50, frame pointers
// 0x0083B990), drawn over burning map objects by the sprite pass
// (0x0042708D). See docs/konung_flames.md.
const (
	FlameFrames  = 74
	FlameTypes   = 7
	flameTypesVA = 0x0045AEC8 // per type: start, end frame (int32 each)
)

// FlameCycle returns a flame type's frame range: the draw advances the
// frame and wraps to start once it reaches end (0x0042719D).
func (t *ExeTables) FlameCycle(typ int) (start, end int) {
	if typ < 0 || typ >= FlameTypes {
		return 0, 0
	}
	b := t.at(flameTypesVA + 8*typ)
	return int(int32(binary.LittleEndian.Uint32(b))), int(int32(binary.LittleEndian.Uint32(b[4:])))
}

// Flame is one flame of a static object (header +0x1C + 5i): its type and
// its offset from the placement position.
type Flame struct {
	Type   int
	DX, DY int
}

// flameLuma is the brightness 0x00427D9C and 0x0043B0CC use for an RGB555
// colour: (2·R8 + G8 + B8) / 32 with 8-bit channels c << 3, i.e.
// (2r + g + b) / 4, 0–31.
func flameLuma(v uint16) int {
	r, g, b := int(v>>10&0x1F), int(v>>5&0x1F), int(v&0x1F)
	return (2*(r<<3) + g<<3 + b<<3) >> 5
}

// flamePremultiply is the per-pixel conversion 0x00427D9C applies to every
// flame frame at load when transparency is on: each channel times L / 32.
func flamePremultiply(v uint16) uint16 {
	l := flameLuma(v)
	r, g, b := int(v>>10&0x1F), int(v>>5&0x1F), int(v&0x1F)
	return uint16(r*l/32)<<10 | uint16(g*l/32)<<5 | uint16(b*l/32)
}

// flameKeep is the table 0x00465E2C (RGB555 build 0x0043B0CC): for a
// converted pixel, the fraction f/32 of the screen pixel the blend 0x0043D8EF
// keeps (the table stores 2f = 0x40 − 2L). Several colours convert to the
// same value; the last one in index order wins, as in the game's loop.
var flameKeep = func() (t [0x8000]uint8) {
	for v := range 0x8000 {
		t[flamePremultiply(uint16(v))] = uint8(32 - flameLuma(uint16(v)))
	}
	return
}()

// TransparentFlame converts a decoded flame frame for the transparent
// draw: the game premultiplies the pixels at load and blends
// screen' = pixel + screen·f/32 per channel (0x0043D8EF), which is
// premultiplied source-over with alpha 1 − f/32. The result is a
// premultiplied RGBA image for that blend. The game's 5-bit truncation of
// the screen term (and the carry into the next channel) is not reproduced.
func TransparentFlame(img *image.RGBA) *image.RGBA {
	out := image.NewRGBA(img.Bounds())
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i+3] == 0 {
			continue
		}
		c5 := func(v uint8) uint16 { return uint16((int(v)*31 + 127) / 255) }
		v := flamePremultiply(c5(img.Pix[i])<<10 | c5(img.Pix[i+1])<<5 | c5(img.Pix[i+2]))
		out.Pix[i] = uint8(int(v>>10&0x1F) * 255 / 31)
		out.Pix[i+1] = uint8(int(v>>5&0x1F) * 255 / 31)
		out.Pix[i+2] = uint8(int(v&0x1F) * 255 / 31)
		out.Pix[i+3] = uint8((32 - int(flameKeep[v])) * 255 / 32)
	}
	return out
}
