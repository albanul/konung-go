package game

import (
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
)

// The Q window in a LAN world (docs/konung_lan.md#player-list): window 2,
// the quest log, followed by the players in the world and the nations'
// amulet counts (LAN_KONG.EXE 0x004258C3, state 2 of 0x00425594).

// LAN player list strings (0x004703C0 …).
const (
	lanClanText   = " клан "            // 0x004703C0
	lanHolderText = " Владелец амулета" // 0x004703C7
	lanTeamsText  = "Счет команд"       // 0x004703D9
)

// lanQList is what window 2 shows in a LAN world besides the quest log.
type lanQList struct {
	log     []uint32 // the own quest log (MsgPlayers)
	rows    []lanQRow
	nations [3]int // amulets delivered by nation (MsgAmulets)
}

// lanQRow is a player's line: its text and whether it is of the own
// hero's nation (drawn with the highlight font 0x007AB36F, others with
// 0x007A5142).
type lanQRow struct {
	text []byte
	own  bool
}

// applyPlayers takes the server's player list and the own quest log.
func (s *startScene) applyPlayers(p netplay.PlayersPayload) {
	if s.view == nil {
		return
	}
	s.view.list = &p
	s.talk.lan = s.lanQ()
}

// lanQ builds window 2's LAN part from the last player list and amulet
// state (0x00425A2F): by slot, «name», « клан clan» when the clan is set,
// « Владелец амулета» when one of the three amulet items' holder is the
// slot; a row is highlighted when its nation is the own hero's.
func (s *startScene) lanQ() *lanQList {
	v := s.view
	if v == nil || v.list == nil {
		return nil
	}
	q := &lanQList{log: v.list.Log}
	own := -1
	for _, p := range v.list.Players {
		if p.Slot == v.slot {
			own = p.Nation
		}
	}
	for _, p := range v.list.Players {
		text := cp866(p.Name)
		if p.Clan != "" {
			text = append(append(text, cp866(lanClanText)...), cp866(p.Clan)...)
		}
		if s.amulets != nil {
			for _, a := range s.amulets.Parts {
				if a.Kind > 0 && a.Holder == p.Slot {
					text = append(text, cp866(lanHolderText)...)
					break
				}
			}
		}
		q.rows = append(q.rows, lanQRow{text: text, own: p.Nation == own})
	}
	if s.amulets != nil {
		q.nations = s.amulets.Nations
	}
	return q
}

// drawLANList draws the players from y on (15 px a line, at the log's
// left; the original draws them however far down they go) and «Счет
// команд» from the strip's top: the title 15 px right of the view's middle
// (0x00A6EBFC / 2, 799 / 2 at 800×600) in the highlight font, the three
// nations' names below it and their counts 100 px further right.
func (w *talkWindow) drawLANList(dst *ebiten.Image, x, y, stripTop int) {
	q := w.lan
	for _, r := range q.rows {
		f := w.small
		if r.own {
			f = w.hi
		}
		f.drawLine(dst, r.text, x, y)
		y += 15
	}
	mid := lanViewRight / 2
	w.hi.drawLine(dst, cp866(lanTeamsText), mid+15, stripTop)
	for k, name := range gamestate.NationNames {
		w.small.drawLine(dst, cp866(name), mid, stripTop+15*(k+1))
		w.small.drawLine(dst, []byte(strconv.Itoa(q.nations[k])), mid+100, stripTop+15*(k+1))
	}
}

// lanViewRight is the game view's right edge at 800×600 (0x00A6EBFC =
// 0x31F, 0x0041C0AA).
const lanViewRight = 0x31F
