package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// miscRequests carries out what the misc items asked of the scene during
// this frame's clicks: closing the window (the shovel) and a map change (the
// magic orbs, the keys). It reports a map change; the scene has then been
// replaced.
func (s *startScene) miscRequests() bool {
	if s.Game.CloseWindow {
		s.Game.CloseWindow = false
		if s.inv.open {
			s.inv.open = false // 0x00415504: window 0; the strip stays
			s.inv.quick = true
		}
	}
	if x := s.Jump; x != nil {
		s.Jump = nil
		s.changeMap(*x)
		return true
	}
	return false
}

// godMode is 0x00A3E768. The startup init (0x0043A3F2) clears it once per
// run; new games and loads keep it, and saves do not store it.
var godMode bool

// godModeKey is L with Ctrl and Shift held (0x004372F2 → 0x00436E7F, which
// tests 0x00A3E754 and 0x00A3E714).
func godModeKey() bool {
	return inpututil.IsKeyJustPressed(ebiten.KeyL) && ebiten.IsKeyPressed(ebiten.KeyControl) && ebiten.IsKeyPressed(ebiten.KeyShift)
}
