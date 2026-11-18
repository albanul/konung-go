package world

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

// gameTime is the game clock 0x00A3E770, one tick per AI tick.
func (s *World) GameTime() int { return s.Tick }

// activatePiles adds the map's piles that the loader rule allows and that
// aren't active yet.
func (s *World) ActivatePiles() {
	now := s.GameTime()
	for i, p := range s.Game.Piles {
		if _, on := s.PileAt[mapdata.Cell{Row: p.Row, Col: p.Col}]; !on && p.Active(s.MapNumber, now) {
			s.PileAt[mapdata.Cell{Row: p.Row, Col: p.Col}] = i
		}
	}
}

// arrive runs when member m's walk ends with the pick-up intent set
// (0x004110EC(e)): the pile on its cell goes into its bag.
func (s *World) Arrive(m *Member) {
	if len(s.Party) == 0 || s.windowOpen() { // 0x004110F8: not while its window is open
		return
	}
	s.Arriving = m // whose pick-up the message and the pile window are (LAN)
	defer func() { s.Arriving = nil }()
	e := &s.Party[m.K]
	i, on := s.PileAt[m.Cell]
	if !on || !s.Game.CanPickUp(s.Game.Piles[i], *e) {
		return
	}
	if s.Game.Piles[i].Index == 0 {
		// 0x00411154: record 0 (map 46) is the end of the game. Next
		// state 7 (the menu; the game stays running, so «Вернуться в
		// игру» returns to it), then WIN.FLM. Nothing is taken.
		s.Film, s.FilmToMenu = resfmt.FilmWin, true
		return
	}
	msg, removed := s.Game.PickUp(e, s.Tables, i, s.GameTime())
	if removed {
		delete(s.PileAt, m.Cell) // 0x0041096C clears the cell's pile flag
	}
	if msg == gamestate.MsgBagFull || msg == gamestate.MsgTooHeavy {
		s.openPileWindow(i) // 0x0041122A: the rest can be swapped in the pile window
	}
	if msg != 0 {
		s.message(s.Tables.CString(msg))
	}
}
