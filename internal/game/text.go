package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/resfmt"
)

// gameFont draws CP866 text with one of the fonts embedded in konung.exe.
// Glyphs are top-aligned and advance by their own width (0x0043F821).
type gameFont struct {
	font   *resfmt.Font
	glyphs map[byte]*ebiten.Image
}

func newGameFont(f *resfmt.Font) *gameFont {
	return &gameFont{font: f, glyphs: map[byte]*ebiten.Image{}}
}

func (g *gameFont) glyph(c byte) *ebiten.Image {
	if img, ok := g.glyphs[c]; ok {
		return img
	}
	var img *ebiten.Image
	if src := g.font.Glyph(c); src != nil {
		img = toEbiten(src)
	}
	g.glyphs[c] = img
	return img
}

// drawLine draws one line of text with its top-left corner at (x, y).
func (g *gameFont) drawLine(dst *ebiten.Image, text []byte, x, y int) {
	for _, c := range text {
		if img := g.glyph(c); img != nil {
			drawAt(dst, img, x, y)
		}
		x += g.font.Advance(c)
	}
}

// drawWrapped draws text wrapped to width like 0x0042CBBC (15 px lines).
func (g *gameFont) drawWrapped(dst *ebiten.Image, text []byte, x, y, width int) {
	for _, line := range g.font.Wrap(text, width) {
		g.drawLine(dst, line, x, y)
		y += resfmt.LineHeight
	}
}
