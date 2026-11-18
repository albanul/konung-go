package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// Party play (docs/konung_party.md): every member of group 0 on the map is
// a member with its own walker and orders; the selection list (0x0083B93C)
// says who takes the player's orders. The party branch of the AI dispatcher
// 0x00421DA4 runs the same code for every member: the hero is only special
// in that it never follows (0x00414AB4 sets +2 bit 0x20 for the others).

// member is a party member on this map: its walker and the orders the
// party AI works on (entity +2, +6, +0x0C/+0x0E, +3 bit 0x40).

// shiftHeld is 0x00A3E754 (set and cleared around VK_SHIFT, 0x00437374).
func shiftHeld() bool { return ebiten.IsKeyPressed(ebiten.KeyShift) }

// portraitClick is a left click with an empty hand on portrait k
// (0x00422EA8): a dead member is ignored, as in windows 1, 2 and 5. In the
// character window (window 3) the member becomes its character and is
// selected; otherwise an unselected member is selected (0x00425084, added
// with Shift) and becomes the strip's character when the strip shows, and
// a selected one toggles its bag strip.
func (s *startScene) portraitClick(k int) {
	m, e := s.PartyMember(s.Party[k].Index)
	if e == nil {
		e = &s.Party[k]
	}
	if e.Dead() || (m != nil && m.Act == gamestate.ActDeath) {
		return
	}
	i := e.Index
	switch {
	case s.inv.open:
		s.Cur = k
		s.inv.scroll = 0
		if !gamestate.Selected(s.Sel, i) {
			s.Sel = gamestate.Select(s.Sel, i, shiftHeld())
		}
	case !gamestate.Selected(s.Sel, i):
		s.Sel = gamestate.Select(s.Sel, i, shiftHeld())
		if s.inv.quick {
			s.Cur = k
			s.inv.scroll = 0
		}
	case s.inv.quick && s.Cur == k:
		s.inv.quick = false // 0x0042308C
	default:
		s.Cur = k
		s.inv.scroll = 0
		s.inv.quick = true
	}
}

// portraitRightClick is a right click on portrait k (0x00423D02): with Ctrl
// on a member with a dialogue, the dialogue; otherwise the member leaves
// the selection (0x00439230), and when it was the strip's character the
// first selected member takes its place.
func (s *startScene) portraitRightClick(k int) {
	e := s.Party[k]
	if e.Dead() {
		return
	}
	if ebiten.IsKeyPressed(ebiten.KeyControl) && e.Dialogue != gamestate.NoDialogue && s.assets != nil && s.assets.quests != nil {
		s.inv.quick = false
		s.talkTo(e.Index)
		return
	}
	s.Sel = gamestate.Deselect(s.Sel, e.Index)
	if s.Cur == k && len(s.Sel) > 0 {
		if j := s.PartyIndex(s.Sel[0]); j >= 0 {
			s.Cur = j
			s.inv.scroll = 0
		}
	}
}

// selectInWorld is a right click on a party member on the map
// (0x00423A5C): with Shift it toggles the member in the selection,
// otherwise the member becomes the selection.
func (s *startScene) selectInWorld(i int) {
	if shiftHeld() && gamestate.Selected(s.Sel, i) {
		s.Sel = gamestate.Deselect(s.Sel, i)
		return
	}
	s.Sel = gamestate.Select(s.Sel, i, shiftHeld())
}

// gatherOrder is the F key / HUD button 4 «Все ко мне» (0x004226C0): return
// a held item, press the button, then the order (World.GatherOrder).
func (s *startScene) gatherOrder() {
	s.returnHand(s.Current())
	s.hud.press(hudGatherButton, hudGatherImage)
	s.GatherOrder()
}

// attackOrder is the A key / HUD button 5 (0x00421698): return a held item,
// press the button, then the order (World.AttackOrder).
func (s *startScene) attackOrder() {
	s.returnHand(s.Current())
	s.hud.press(hudAttackButton, hudAttackImage)
	s.AttackOrder()
}

// rightOrder is World.RightOrder with the building under the cursor.
func (s *startScene) rightOrder(m *member, hovered int, c mapdata.Cell) {
	s.RightOrder(m, hovered, c, s.hoveredBuilding)
}

// selectWeapon is 1/2/3 and the HUD weapon button (0x0042192C), for the
// whole selection with Shift (World.SelectWeapon).
func (s *startScene) selectWeapon(mode int) { s.SelectWeapon(mode, shiftHeld()) }
