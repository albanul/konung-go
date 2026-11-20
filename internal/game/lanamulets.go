package game

import (
	"strconv"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
)

// The amulet of the dragon in a LAN world (docs/konung_lan.md#amulets):
// the server's MsgAmulets (the original's 0x16, client 0x004222E8) are kept
// for the map window's status (worldmap.go drawAmulets); a change of a
// nation's count plays sound 0x15 (0x00422347).

// applyAmulets takes the server's amulet state.
func (s *startScene) applyAmulets(p netplay.AmuletsPayload) {
	if s.amulets != nil && s.amulets.Nations != p.Nations && s.Hero != nil {
		s.playAt(gamestate.AmuletSound, s.Hero.Cell)
	}
	s.amulets = &p
	s.talk.lan = s.lanQ() // the holders and counts of the Q window
}

// setAmuletStatus gives the map window the amulet status (0x0041ED44): the
// three items' icons (the LAN misc record's icon, the same as the
// single-player item's) and "a : b : c   m:s", the nations' counts and the
// session's minutes and seconds left, numbers as the original's itoa
// writes them (no padding).
func (s *startScene) setAmuletStatus() {
	w := s.wmap
	w.amuletLine = nil
	if s.amulets == nil || s.Tables == nil {
		return
	}
	for k, p := range s.amulets.Parts {
		w.amuletIcons[k] = -1
		if p.Kind > 0 {
			w.amuletIcons[k] = s.Tables.ItemIcon(resfmt.ItemTableMisc, gamestate.AmuletKindType(p.Kind))
		}
	}
	n := s.amulets.Nations
	line := strconv.Itoa(n[0]) + " : " + strconv.Itoa(n[1]) + " : " + strconv.Itoa(n[2]) + "   "
	if s.clock != nil {
		if left, ok := s.clock.sessionLeft(); ok {
			sec := int(left.Seconds())
			line += strconv.Itoa(sec/60) + ":" + strconv.Itoa(sec%60)
		}
	}
	w.amuletLine = []byte(line)
}
