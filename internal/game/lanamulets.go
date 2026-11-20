package game

import (
	"fmt"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
)

// The amulet of the dragon in a LAN world (docs/konung_lan.md#amulets):
// the server's MsgAmulets (the original's 0x16, client 0x004222E8) are kept
// for the status line (drawLANStatus); a change of a
// nation's count plays sound 0x15 (0x00422347).

// applyAmulets takes the server's amulet state.
func (s *startScene) applyAmulets(p netplay.AmuletsPayload) {
	if s.amulets != nil && s.amulets.Nations != p.Nations && s.Hero != nil {
		s.playAt(gamestate.AmuletSound, s.Hero.Cell)
	}
	s.amulets = &p
	s.talk.lan = s.lanQ() // the holders and counts of the Q window
}

// The LAN status line (docs/konung_lan.md#status-line): the original
// draws it only in the map window (0x0041ED44, part of 0x0041E8EC): the
// nations' delivered amulets and the session's minutes and seconds left,
// "a : b : c   m:s" in font 0x0078C1A0 at (678, 5), and the three amulet
// items' icons at x 700, y 15, 70, 125 (places 0x0047EFC4 + 32·mode,
// 0x0047EFE4 at 800×600). Port
// rule: the line and the icons are drawn always in a LAN world (the map
// window draws nothing of its own), the line right-aligned at the view's
// top right, the time as hours:minutes:seconds (h:mm:ss) and left out
// while unknown; the icons below it at the view's right edge.

// lanStatusIconY are the amulet icons' heights (0x0047EFE4 table, 800×600).
var lanStatusIconY = [3]int{15, 70, 125}

// lanStatusLine is the status line's text: the nations' counts (Русские,
// Викинги, Византийцы; 0 before the first MsgAmulets) and, when known,
// the session's time left.
func (s *startScene) lanStatusLine() []byte {
	var n [3]int
	if s.amulets != nil {
		n = s.amulets.Nations
	}
	line := strconv.Itoa(n[0]) + " : " + strconv.Itoa(n[1]) + " : " + strconv.Itoa(n[2])
	if s.clock != nil {
		if left, ok := s.clock.sessionLeft(); ok {
			sec := int(left.Seconds())
			line += fmt.Sprintf("   %d:%02d:%02d", sec/3600, sec/60%60, sec%60)
		}
	}
	return []byte(line)
}

// lanStatusIcons are the INTR slots of the three amulet items' icons (the
// LAN misc record's icon, the same as the single-player item's): the item
// each of the three amulet state words holds (0x0041EEB4: kind & 7, none
// when 0), −1 for none.
func (s *startScene) lanStatusIcons() [3]int {
	icons := [3]int{-1, -1, -1}
	if s.amulets == nil || s.Tables == nil {
		return icons
	}
	for k, p := range s.amulets.Parts {
		if k < len(icons) && p.Kind > 0 {
			icons[k] = s.Tables.ItemIcon(resfmt.ItemTableMisc, gamestate.AmuletKindType(p.Kind))
		}
	}
	return icons
}

// drawLANStatus draws the status line and the amulet icons over the view.
func (s *startScene) drawLANStatus(dst *ebiten.Image) {
	if s.font == nil {
		return
	}
	line := s.lanStatusLine()
	s.font.drawLine(dst, line, viewRight-4-s.font.width(line), 5)
	for k, n := range s.lanStatusIcons() {
		img := s.hud.slot(n)
		if n < 0 || img == nil {
			continue
		}
		drawAt(dst, img, viewRight+1-8-img.Bounds().Dx(), lanStatusIconY[k])
	}
}
