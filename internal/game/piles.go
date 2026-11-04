package game

import (
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/mapdata"
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

// gameTime is the game clock 0x00A3E770, one tick per AI tick.
func (s *startScene) gameTime() int { return s.tick / walkTicksPerStep }

// activatePiles adds the map's piles that the loader rule allows and that
// aren't active yet.
func (s *startScene) activatePiles() {
	now := s.gameTime()
	for i, p := range s.game.Piles {
		if _, on := s.pileAt[mapdata.Cell{Row: p.Row, Col: p.Col}]; !on && p.Active(s.mapNumber, now) {
			s.pileAt[mapdata.Cell{Row: p.Row, Col: p.Col}] = i
		}
	}
}

// drawPiles draws the visible piles after the decorations.
func (s *startScene) drawPiles(dst *ebiten.Image) {
	// Rows top to bottom, columns left to right, like the terrain pass.
	cells := make([]mapdata.Cell, 0, len(s.pileAt))
	for c := range s.pileAt {
		cells = append(cells, c)
	}
	sort.Slice(cells, func(i, j int) bool {
		return cells[i].Row < cells[j].Row || cells[i].Row == cells[j].Row && cells[i].Col < cells[j].Col
	})
	for _, c := range cells {
		p := s.game.Piles[s.pileAt[c]]
		if !p.Visible() {
			continue
		}
		img := s.hud.slot(s.game.PileImage(s.tables, p))
		if img == nil {
			continue
		}
		b := img.Bounds()
		x, y := mapdata.CellCenter(c)
		x, y = x-b.Dx()/2, y-b.Dy()/2
		if s.visible(x, y, b.Dx(), b.Dy()) {
			drawAt(dst, img, x-s.camX, y-s.camY)
		}
	}
}

// pileUnderCursor returns the visible pile on the cell under the mouse.
func (s *startScene) pileUnderCursor() (cell mapdata.Cell, ok bool) {
	mx, my := ebiten.CursorPosition()
	if !inView(mx, my) || s.inv.open || (s.inv.showStrip() && my >= s.inv.strip.y-1) {
		return cell, false
	}
	cell = mapdata.CellAt(mx+s.camX, my+s.camY)
	i, on := s.pileAt[cell]
	return cell, on && s.game.Piles[i].Visible()
}

// arrive runs when the hero's walk ends with the pick-up intent set.
func (s *startScene) arrive() {
	if len(s.party) == 0 || s.inv.open { // 0x004110F8: not while its window is open
		return
	}
	i, on := s.pileAt[s.hero.cell]
	if !on || !s.game.CanPickUp(s.game.Piles[i], s.party[0]) {
		return
	}
	msg, removed := s.game.PickUp(&s.party[0], s.tables, i, s.gameTime())
	if removed {
		delete(s.pileAt, s.hero.cell) // 0x0041096C clears the cell's pile flag
	}
	if msg != 0 {
		s.showMessage(s.tables.CString(msg))
	}
}
