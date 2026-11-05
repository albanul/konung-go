package resfmt

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
)

// RGB555Palette converts a 256-color Konung palette to RGBA. The game's
// magenta key color is transparent, matching its indexed image decoders.
func RGB555Palette(src []byte) ([256]color.RGBA, error) {
	var palette [256]color.RGBA
	if len(src) < 512 {
		return palette, fmt.Errorf("palette truncated: got %d bytes, need 512", len(src))
	}
	for i := range palette {
		v := binary.LittleEndian.Uint16(src[i*2:])
		palette[i] = color.RGBA{
			R: uint8(((v >> 10) & 31) * 255 / 31),
			G: uint8(((v >> 5) & 31) * 255 / 31),
			B: uint8((v & 31) * 255 / 31),
			A: 255,
		}
		if v == 0x7c1f {
			palette[i].A = 0
		}
	}
	return palette, nil
}

// DecodeIndexedRLEFrame decodes the 8-bit indexed row-RLE used by GRAPH.RES
// terrain and OBJECTS.RES frames. rowSizes bounds each row's command stream.
func DecodeIndexedRLEFrame(src []byte, palette *[256]color.RGBA) (*image.RGBA, error) {
	if len(src) < 4 {
		return nil, fmt.Errorf("indexed frame header truncated")
	}
	w := int(binary.LittleEndian.Uint16(src))
	h := int(binary.LittleEndian.Uint16(src[2:]))
	if w < 1 || h < 1 || len(src) < 4+h*2 {
		return nil, fmt.Errorf("invalid indexed frame dimensions %dx%d", w, h)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	pos := 4 + h*2
	for y := 0; y < h; y++ {
		rowSize := int(binary.LittleEndian.Uint16(src[4+y*2:]))
		end := pos + rowSize
		if end > len(src) {
			return nil, fmt.Errorf("indexed frame row %d truncated", y)
		}
		x := 0
		for pos < end {
			n := int(src[pos])
			pos++
			if n == 0 {
				break
			}
			if n&0x80 != 0 {
				x += n & 0x7f
				continue
			}
			if pos+n > end {
				return nil, fmt.Errorf("indexed frame row %d pixel run truncated", y)
			}
			for _, index := range src[pos : pos+n] {
				if x < w {
					img.SetRGBA(x, y, palette[index])
				}
				x++
			}
			pos += n
		}
		pos = end
	}
	return img, nil
}

// DecodeIndexedRLELegacyFrame handles the older GRAPH/OBJECTS row-budget
// variant used by a few large images. Its row table counts the decoder's
// command budget rather than the exact byte length of each row.
func DecodeIndexedRLELegacyFrame(src []byte, palette *[256]color.RGBA) (*image.RGBA, error) {
	if len(src) < 4 {
		return nil, fmt.Errorf("legacy indexed frame header truncated")
	}
	w := int(binary.LittleEndian.Uint16(src))
	h := int(binary.LittleEndian.Uint16(src[2:]))
	if w < 1 || h < 1 || len(src) < 4+h*2 {
		return nil, fmt.Errorf("invalid legacy indexed frame dimensions %dx%d", w, h)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	pos := 4 + h*2
	remainingPixels := w * h
	for y := 0; y < h && remainingPixels > 0; y++ {
		budget := int(binary.LittleEndian.Uint16(src[4+y*2:]))
		x := 0
		for budget > 0 && pos < len(src) && remainingPixels > 0 {
			cmd := int(src[pos])
			pos++
			budget--
			if cmd == 0 {
				break
			}
			if cmd&0x80 != 0 {
				skip := cmd & 0x7f
				x += skip
				remainingPixels -= min(skip, remainingPixels)
				continue
			}
			count := min(cmd, len(src)-pos)
			for i := 0; i < count; i++ {
				if x+i < w {
					img.SetRGBA(x+i, y, palette[src[pos+i]])
				}
			}
			pos += count
			x += count
			budget -= min(budget, count)
			remainingPixels -= min(count, remainingPixels)
		}
	}
	return img, nil
}

// TintPalette applies the game's palette tint 0x0043FD18(r, g, b) to an RGB555
// palette (expanded to RGBA by RGB555Palette): per 5-bit channel c and
// percentage n ≥ 0, c' = c + trunc((31 − c)·n·0.01); n < 0 darkens,
// c' = trunc((1 − |n|·0.01)·c). The game builds lookup tables with float32
// arithmetic at runtime (0x0043ADF1); the transparent key is kept.
func TintPalette(p [256]color.RGBA, r, g, b int) [256]color.RGBA {
	tr, tg, tb := TintTable(r), TintTable(g), TintTable(b)
	tint := func(v uint8, t *[32]uint8) uint8 {
		return uint8(int(t[(int(v)*31+127)/255]) * 255 / 31)
	}
	for i := range p {
		if p[i].A == 0 {
			continue
		}
		p[i].R, p[i].G, p[i].B = tint(p[i].R, &tr), tint(p[i].G, &tg), tint(p[i].B, &tb)
	}
	return p
}

// TintTable is one of the game's per-percentage channel tables (built by
// 0x0043B27C, 100 entries each way): the 5-bit channel value c maps to
// c + trunc((31 − c)·s) for n ≥ 0 and trunc((1 − s)·c) for n < 0, where s
// is 0.01 added |n| times in float32. |n| is clamped to 99.
func TintTable(n int) [32]uint8 {
	var step float32
	for range min(abs(n), 99) {
		step += 0.01
	}
	var t [32]uint8
	for c := range t {
		if n >= 0 {
			t[c] = uint8(c + int(float32(31-c)*step))
		} else {
			t[c] = uint8(int((1 - step) * float32(c)))
		}
	}
	return t
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
