package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/resfmt"
)

// drawArrows draws the flying arrows (INTR slot 0xB8 + 2·facing + time&1,
// 0x00426F4D) over the characters; incendiary ones (special 0) over their
// glow.
func (s *startScene) drawArrows(dst *ebiten.Image) {
	for _, a := range s.Arrows {
		if a.Special == 0 && s.assets != nil && s.assets.arrowGlow != nil {
			// 0x00426F5C: incendiary arrows glow, centred on the position.
			gx := int(a.X-resfmt.ArrowGlowSize/2) - s.camX
			gy := int(a.Y-resfmt.ArrowGlowSize/2) - s.camY
			op := &ebiten.DrawImageOptions{Blend: glowBlend}
			op.GeoM.Translate(float64(gx), float64(gy))
			dst.DrawImage(s.assets.arrowGlow, op)
		}
		img := s.hud.slot(gamestate.ArrowImage + 2*a.Facing + s.GameTime()&1)
		if img == nil {
			continue
		}
		dx, dy := a.SpriteOffset()
		drawAt(dst, img, int(a.X)+dx-s.camX, int(a.Y)+dy-s.camY)
	}
}
