package game

import (
	"image"
	"image/color"

	"konung1.5/internal/resfmt"
)

// The multiplayer entry is a port addition: the single-player konung.exe has
// no network game and MENU800R.RES no button for it. It opens the LAN
// version's screens (lanui.go), traced from LAN_KONG.EXE.

// networkLabel is «Сетевая игра» in CP866, the encoding of the game's fonts.
var networkLabel = []byte{0x91, 0xA5, 0xE2, 0xA5, 0xA2, 0xA0, 0xEF, ' ', 0xA8, 0xA3, 0xE0, 0xA0}

// The label sits at the top of the menu stone, where «Сохранить игру» is
// drawn while a game runs (the item is shown only without a game). It is the
// large font scaled 2x and tinted like the painted buttons' lettering
// (MENU800R.RES frames 9/10 average about 171,121,77 and 190,146,104).
const (
	networkLabelX, networkLabelY = 262, 196
	networkLabelScale            = 2
)

var (
	networkTint         = color.RGBA{215, 150, 95, 255}
	networkTintSelected = color.RGBA{255, 215, 160, 255}
	networkShadow       = color.RGBA{20, 14, 8, 200}
)

// engravedLabel draws text with font f scaled by scale: each glyph pixel
// becomes tint times its brightness, over a dark shadow offset by scale
// pixels, so the label reads as cut into the stone like the painted buttons.
func engravedLabel(f *resfmt.Font, text []byte, scale int, tint color.RGBA) *image.RGBA {
	h := 0
	for _, c := range text {
		if g := f.Glyph(c); g != nil {
			h = max(h, g.Bounds().Dy())
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, f.Width(text)*scale+scale, h*scale+scale))
	for _, shadow := range []bool{true, false} {
		x := 0
		for _, c := range text {
			if g := f.Glyph(c); g != nil {
				b := g.Bounds()
				for gy := range b.Dy() {
					for gx := range b.Dx() {
						p := g.RGBAAt(b.Min.X+gx, b.Min.Y+gy)
						if p.A == 0 {
							continue
						}
						out, off := networkShadow, scale
						if !shadow {
							l := (int(p.R) + int(p.G) + int(p.B)) / 3
							out = color.RGBA{uint8(int(tint.R) * l / 255), uint8(int(tint.G) * l / 255), uint8(int(tint.B) * l / 255), 255}
							off = 0
						}
						for dy := range scale {
							for dx := range scale {
								img.SetRGBA(x+gx*scale+dx+off, gy*scale+dy+off, out)
							}
						}
					}
				}
			}
			x += f.Advance(c) * scale
		}
	}
	return img
}

// networkButton returns the normal and selected «Сетевая игра» images.
func networkButton(t *resfmt.ExeTables) ([2]*image.RGBA, error) {
	f, err := t.Font(resfmt.FontLargeVA)
	if err != nil {
		return [2]*image.RGBA{}, err
	}
	return [2]*image.RGBA{
		engravedLabel(f, networkLabel, networkLabelScale, networkTint),
		engravedLabel(f, networkLabel, networkLabelScale, networkTintSelected),
	}, nil
}
