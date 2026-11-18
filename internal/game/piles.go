package game

import (
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

// Items on the ground (GAME.N section 3, gamestate.Pile). The map loader
// (0x0043CF1A) activates the map's piles; the terrain pass draws the visible
// ones flat, centred on their cell (0x0042647E); a plain map click sets the
// pick-up intent and the hero picks up the pile on the cell where the walk
// ends (0x004222BC → 0x004110EC).
//
// Port addition: the game re-checks the respawn timer only when a map loads.
// The port also re-runs the loader's rule every game tick for this map's
// inactive piles; a picked nut or root comes back
// after gamestate.PileRespawnTicks.

// drawPiles draws the visible piles after the decorations.
func (s *startScene) drawPiles(dst *ebiten.Image) {
	// Rows top to bottom, columns left to right, like the terrain pass.
	cells := make([]mapdata.Cell, 0, len(s.PileAt))
	for c := range s.PileAt {
		cells = append(cells, c)
	}
	sort.Slice(cells, func(i, j int) bool {
		return cells[i].Row < cells[j].Row || cells[i].Row == cells[j].Row && cells[i].Col < cells[j].Col
	})
	for _, c := range cells {
		p := s.Game.Piles[s.PileAt[c]]
		if !p.Visible() {
			continue
		}
		img := s.hud.slot(s.Game.PileImage(s.Tables, p))
		if img == nil {
			continue
		}
		b := img.Bounds()
		x, y := mapdata.CellCenter(c)
		x, y = x-b.Dx()/2, y-b.Dy()/2
		if s.Game.Lit && s.assets.glow != nil {
			// 0x00426554: the glow first, centred on the image (0x0043DE4D).
			gx, gy := x+b.Dx()/2-resfmt.GlowWidth/2, y+b.Dy()/2-resfmt.GlowHeight/2
			if s.visible(gx, gy, resfmt.GlowWidth, resfmt.GlowHeight) {
				op := &ebiten.DrawImageOptions{Blend: glowBlend}
				op.GeoM.Translate(float64(gx-s.camX), float64(gy-s.camY))
				dst.DrawImage(s.assets.glow, op)
			}
		}
		if s.visible(x, y, b.Dx(), b.Dy()) {
			// The glowing piles use the raw palette, the others the tint.
			s.drawTinted(dst, img, x-s.camX, y-s.camY, s.Game.Lit)
		}
	}
}

// glowBlend brightens like 0x0043DE4D: dst' = src + dst·(1 − src) per
// channel, alpha kept. The mask (resfmt.GlowMask) has no blue, so blue stays.
var glowBlend = ebiten.Blend{
	BlendFactorSourceRGB:        ebiten.BlendFactorOne,
	BlendFactorSourceAlpha:      ebiten.BlendFactorZero,
	BlendFactorDestinationRGB:   ebiten.BlendFactorOneMinusSourceColor,
	BlendFactorDestinationAlpha: ebiten.BlendFactorOne,
	BlendOperationRGB:           ebiten.BlendOperationAdd,
	BlendOperationAlpha:         ebiten.BlendOperationAdd,
}

// pileUnderCursor returns the visible pile on the cell under the mouse.
func (s *startScene) pileUnderCursor() (cell mapdata.Cell, ok bool) {
	mx, my := ebiten.CursorPosition()
	if !inView(mx, my) || s.inv.open || (s.inv.showStrip() && my >= s.inv.strip.y-1) {
		return cell, false
	}
	cell = mapdata.CellAt(mx+s.camX, my+s.camY)
	i, on := s.PileAt[cell]
	return cell, on && s.Game.Piles[i].Visible()
}
