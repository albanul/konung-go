package gamestate

import "konung1.5/internal/resfmt"

// Village anger; see docs/konung_combat.md#village-anger-verified.

// Settlement flags (+0x4B2).
const (
	SettlementHostile     = 0x01 // any party member inside the village alarms it
	SettlementStrangers   = 0x04 // armed party members of another tribe alarm it
	SettlementIndependent = 0x10 // not a vassal
	SettlementSurrendered = 0x80
	settlementAnger       = SettlementHostile | SettlementStrangers | SettlementIndependent
	settlementPeace       = 0xEA
)

// MsgVillageSurrenders is «Довольно крови, князь. Деревня сдается на твою
// милость!» (0x0041D6B9).
const MsgVillageSurrenders = 0x0045D37C

// SettlementTickOffset: the settlement pass 0x0041D39F runs when
// (time + 7) & 0xF == 0.
const SettlementTickOffset = 7

// InitSettlementFlags is the GAME.N loader's setup (0x0043BD5C) for hero
// model heroModel: every used settlement becomes independent, and a fenced
// one of another tribe (village group +0x1E & 3) also refuses armed
// strangers.
func (g *Game) InitSettlementFlags(heroModel int) {
	for i := range g.Settlements {
		s := &g.Settlements[i]
		if s.Map == 0 {
			continue
		}
		s.Flags = SettlementIndependent
		if s.Fence != 0 && heroModel != int(g.Groups[s.Group].Kind&3) {
			s.Flags |= SettlementStrangers
		}
	}
}

// SettlementOf returns the settlement whose village group is gi, or nil.
func (g *Game) SettlementOf(gi int) *Settlement {
	if s := g.settlementOn(g.Groups[gi].Map); s != nil && s.Group == gi {
		return s
	}
	return nil
}

// Anger is `+0x4B2 |= 0x15` followed by the alarm of the village group
// (0x004248A0): a villager hurt or killed, an arrow on a building, the
// party's swing at a villager.
func (g *Game) Anger(s *Settlement) {
	s.Flags |= settlementAnger
	g.Groups[s.Group].Alarm = AlarmAttack
}

// Armed reports a weapon equipped (axe, bow, blade) or one in the bag
// (category ≤ 2): 0x00416700's test for strangers.
func (g *Game) Armed(e Entity) bool {
	if e.Equipment[WeaponAxe] != 0 || e.Equipment[WeaponBow] != 0 || e.Equipment[WeaponBlade] != 0 {
		return true
	}
	for _, i := range e.Bag {
		if it, ok := g.Item(i); ok && it.Category <= CatBlade {
			return true
		}
	}
	return false
}

// VillageAlarm is 0x00416700(s): the village group is alarmed (+0x1A = the
// party, +0x1D = 1) when a living party member stands in its rectangle and
// the village is hostile, or refuses armed strangers and the member is one;
// otherwise it calms down. party holds the living members with their cells.
func (g *Game) VillageAlarm(s *Settlement, party []Entity) {
	gr := &g.Groups[s.Group]
	gr.Alarm = AlarmCalm
	if s.Flags&(SettlementHostile|SettlementStrangers) == 0 {
		return
	}
	a, _ := gr.WanderArea()
	for _, m := range party {
		if !a.Contains(m.Row, m.Col) {
			continue
		}
		if s.Flags&SettlementHostile != 0 ||
			(int(gr.Kind&3) != m.Model && g.Armed(m)) {
			gr.Enemy, gr.Alarm = 0, AlarmAttack
			return
		}
	}
}

// Surrenders is the settlement pass's check (0x0041D39F) for an alarmed,
// not yet conquered village: true when the party's strength (sum of
// KillExperience) is at least twice that of the living men (model < 3). A
// village without men calms down and forgets its anger. On surrender the
// alarm ends and the village becomes a peaceful vassal (flags |= 0x80,
// &= 0xEA); the caller idles everyone and shows MsgVillageSurrenders.
func (g *Game) Surrenders(s *Settlement, party []Entity, t *resfmt.ExeTables) bool {
	gr := &g.Groups[s.Group]
	men, v := 0, 0.0
	for k := 0; k < gr.Count; k++ {
		if e := g.Entities[gr.First+k]; !e.Dead() && e.Model < 3 {
			men++
			v += float64(g.KillExperience(e, t))
		}
	}
	if men == 0 {
		gr.Alarm = AlarmCalm
		s.Flags &= settlementPeace
		return false
	}
	if gr.Alarm != AlarmAttack || s.Flags&SettlementSurrendered != 0 {
		return false
	}
	p := 0.0
	for _, m := range party {
		p += float64(g.KillExperience(m, t))
	}
	if v == 0 || p/v < 2.0 {
		return false
	}
	gr.Alarm = AlarmCalm
	s.Flags = (s.Flags | SettlementSurrendered) & settlementPeace // 0xEA keeps 0x80
	return true
}
