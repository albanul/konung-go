package gamestate

import (
	"encoding/binary"

	"konung1.5/internal/resfmt"
)

// The LAN villages (docs/konung_lan.md#villages-on-the-server).
//
// LAN_KONG.EXE's settlement pass 0x00440C70 (every 16 ticks, (time + 7) &
// 0xF = 0, for the three villages) is single player's 0x0041D39F with
// these differences:
//
//   - the humans count also needs +4 ≠ 0 (an entity record in use) and
//     the builders are humans − posts (no apprentices subtracted);
//   - no tribute, no surrender check;
//   - the alarm is per player (0x0043A180): the village attacks a player
//     slot whose bit is set in the aggressors (+0x4E) while one of its
//     group's members stands in the village group's rectangle;
//   - the healer and the smith run for each of the 16 player slots
//     (LANShopPass);
//   - a building's new state goes to every player (0x0043AB04, message
//     0x5E: map, placement, state).
//
// The aggressor bit of a slot is set when its hero or a hireling hurts a
// villager (melee 0x00445B73, arrows 0x00444EDF) and when it orders an
// attack on a player of the village's nation who is not an aggressor
// himself (0x004466FA, 0x004467C2); it is cleared for all three villages
// when the slot's player goes (0x0044B0A7) and at the session's end.

// LANClickBox is the in-hand box click on the LAN server: the LAN use
// 0x00440CC5 mixes a herb held over a boxed herb (0x0044DD38) but refuses
// one held over any other item, so neither poison on weapons nor oil on
// arrows exists in the network game; otherwise ClickBox.
func (g *Game) LANClickBox(h *Hand, e *Entity, t *resfmt.ExeTables) int {
	if h.Box != 0 && h.Holding() && h.Cursor == CursorHerb {
		if it, ok := g.Item(h.Box); ok && it.Category != CatHerb {
			h.Return(e)
			return MsgUnusable
		}
	}
	return g.ClickBox(h, e, t)
}

// MarkAggressor sets slot's bit in s's aggressors.
func (s *Settlement) MarkAggressor(slot int) {
	if slot >= 0 && slot < 16 {
		s.Aggressors |= 1 << uint(slot)
	}
}

// Aggressor reports slot's bit in s's aggressors.
func (s *Settlement) Aggressor(slot int) bool {
	return slot >= 0 && slot < 16 && s.Aggressors&(1<<uint(slot)) != 0
}

// ClearAggressor clears slot's bit in every settlement (0x0044B0A7).
func (g *Game) ClearAggressor(slot int) {
	for i := range g.Settlements {
		if slot >= 0 && slot < 16 {
			g.Settlements[i].Aggressors &^= 1 << uint(slot)
		}
	}
}

// Nation is the village's side (group kind & 3): the hero model whose
// village it is.
func (g *Game) Nation(s *Settlement) int { return int(g.Groups[s.Group].Kind & 3) }

// lanCanAct is the LAN pass's test of a group member (0x00440918,
// 0x0043A251): a record in use (+4 ≠ 0), alive, not in action 9 or 3.
func (e Entity) lanCanAct() bool { return e.Flags4 != 0 && e.canAct() }

// LANVillageAlarm is 0x0043A180: the village group attacks the first player
// slot (in slot order) among its aggressors one of whose members (party:
// the living players' heroes and hirelings on the village's map, Group =
// their slot, with their cells) stands in the group's rectangle; without
// one it is calm.
func (g *Game) LANVillageAlarm(s *Settlement, party []Entity) {
	gr := &g.Groups[s.Group]
	a, _ := gr.WanderArea()
	if s.Aggressors != 0 {
		for slot := range 16 {
			if !s.Aggressor(slot) {
				continue
			}
			for _, m := range party {
				if m.Group == slot && m.lanCanAct() && a.Contains(m.Row, m.Col) {
					gr.Enemy, gr.Alarm = slot, AlarmAttack
					return
				}
			}
		}
	}
	gr.Alarm = AlarmCalm
}

// LANSettlementPass is the LAN settlement pass 0x00440C70 for settlement i:
// the humans (lanCanAct, model < 3, still in the village) and posts are
// counted; a village without humans calms down and loses its anger
// (+0x1D = 0, flags &= 0xEA); the alarm (LANVillageAlarm); the buildings
// with humans − posts builders (none while alarmed); the shops of every
// player slot (LANShopPass); and, when no building stage started, the
// voevoda's training. changed reports a building that changed state.
func (g *Game) LANSettlementPass(i int, shops []LANShop, t *resfmt.ExeTables, party []Entity) (changed bool) {
	s := &g.Settlements[i]
	if s.Map == 0 {
		return false
	}
	gr := &g.Groups[s.Group]
	humans, posted := 0, 0
	for k := 0; k < gr.Count; k++ {
		e := g.Entities[gr.First+k]
		if e.Group == s.Group && e.Model < 3 && e.lanCanAct() {
			humans++
		}
	}
	for _, p := range s.Posts {
		if p.Entity != 0 {
			posted++
		}
	}
	if humans == 0 {
		gr.Alarm = AlarmCalm
		s.Flags &= settlementPeace
	}
	g.LANVillageAlarm(s, party)
	builders := 0
	if gr.Alarm != AlarmAttack {
		builders = humans - posted
	}
	started, ch := g.buildingPass(s, t, builders, s.Map)
	g.LANShopPass(i, shops, t)
	if !started {
		g.voevodaPass(s, t)
	}
	return ch
}

// BuildingStates is map m's settlement buildings: placement → state.
func (g *Game) BuildingStates(m int) map[int]int {
	out := map[int]int{}
	for i := range g.Settlements {
		s := &g.Settlements[i]
		if s.Map != m || m == 0 {
			continue
		}
		for r := range s.buildingCount() {
			b := s.building(r)
			if int8(b[0]) > -1 {
				out[int(binary.LittleEndian.Uint16(b[2:]))] = int(int8(b[1]))
			}
		}
	}
	return out
}

// SetBuildingState sets the state of map m's building on placement p (a
// LAN client applying the server's 0x5E); false when there is none.
func (g *Game) SetBuildingState(m, p, state int) bool {
	for i := range g.Settlements {
		s := &g.Settlements[i]
		if s.Map != m || m == 0 {
			continue
		}
		for r := range s.buildingCount() {
			b := s.building(r)
			if int8(b[0]) > -1 && int(binary.LittleEndian.Uint16(b[2:])) == p {
				b[1] = byte(int8(state))
				return true
			}
		}
	}
	return false
}
