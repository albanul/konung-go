package world

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// Attach gives a new walker what the runner needs to show it: p is the
// entity as placed, live its current record (stance, weapon and gear change
// in fights). The game loads its sprites and sets Walker.Frames; the server
// sets frame counts only. nil leaves the walker without frames.
type Attach func(w *Walker, p gamestate.Placed, live func() gamestate.Entity)

// Populate is the map loader's character part (0x0043C612) for the
// world's map: unless restoring a save, creatures are placed at random in
// their territory and respawned (gamestate.Game.EnterMap) and a party that
// arrived by an exit or a jump is placed around the group cell
// (0x004118DC, PlaceParty); then the party copies, a walker per party member
// (members keep +2 except the fight orders) and one per other character on
// the map: creatures (calm, dormant shooters), villagers with an area
// (posts, the move roll), others standing.
func (s *World) Populate(restore bool, attach Attach) {
	game, mapNumber, walk, tables := s.Game, s.MapNumber, s.Walk, s.Tables
	reserve := func(c mapdata.Cell) bool {
		if s.Occ[c] {
			return false
		}
		s.Occ[c] = true
		return true
	}
	release := func(c mapdata.Cell) { delete(s.Occ, c) }
	taken := map[[2]int]bool{}
	for _, gr := range game.Groups[1:] {
		if gr.Map != mapNumber || gr.Kind == 0 || (gr.Kind&0x80 == 0 && int8(gr.Kind) < 0x51) {
			continue
		}
		for k := 0; k < gr.Count; k++ {
			if e := game.Entities[gr.First+k]; !e.Dead() && (e.Row != 0 || e.Col != 0) {
				taken[[2]int{e.Row, e.Col}] = true
			}
		}
	}
	if !restore {
		freeCell := func(r, c int) bool { return !walk.Blocked(r, c) && !taken[[2]int{r, c}] }
		game.EnterMap(mapNumber, tables, s.Rng.IntN,
			func(r, c int) bool { return !freeCell(r, c) },
			func(r, c int) { taken[[2]int{r, c}] = true })
		// 0x004118DC after a map exit or a jump (TakeExit cleared the
		// party's cells): the hero on the group cell, the others around it.
		if g0 := game.Groups[0]; g0.Map == mapNumber && g0.Count > 0 && tables != nil {
			if h := game.Entities[g0.First]; h.Row == 0 && h.Col == 0 {
				game.PlaceParty(tables, s.Rng.IntN, freeCell, func(r, c int) { taken[[2]int{r, c}] = true })
			}
		}
	}
	if g0 := game.Groups[0]; g0.Map == mapNumber {
		for k := 0; k < g0.Count; k++ {
			s.Party = append(s.Party, game.Entities[g0.First+k])
		}
		s.Members = make([]*Member, len(s.Party))
	}
	game.Reviver = s.Revive
	for _, p := range game.OnMap(mapNumber, walk.Blocked) {
		if p.Group != 0 && p.Dead() {
			continue // the leave cleanup (0x00436AD0) removes the dead
		}
		cell := mapdata.Cell{Row: p.Row, Col: p.Col}
		if !p.Dead() {
			s.Occ[cell] = true
		}
		i := p.Index
		cur := func() gamestate.Entity { return game.Entities[i] }
		k := -1
		if p.Group == 0 {
			k = s.PartyIndex(p.Index)
		}
		if k >= 0 {
			cur = func() gamestate.Entity { return s.Party[k] } // stance and weapon change at runtime
		}
		w := Walker{
			Entity: p.Index, Cell: cell, Dir: mapdata.Direction(p.Variant & 7), Idle: -1,
			Look:    LookOf(p.Entity),
			Reserve: reserve, Release: release,
			Sound: s.actionSound,
		}
		if attach != nil {
			attach(&w, p, cur)
		}
		if k >= 0 {
			w.Blocked = s.Blocked
			m := &Member{Walker: w, K: k, Target: -1}
			// +2 survives a map change or a save; the fight orders don't.
			m.Intent = byte(p.Flags2) & (gamestate.IntentStrike | gamestate.IntentFollow | gamestate.IntentIdle)
			if k == 0 {
				m.Intent &^= gamestate.IntentFollow
			}
			if p.Dead() {
				m.Act, m.ActStep = gamestate.ActDeath, 1<<10 // a corpse: the last death step
			}
			s.Members[k] = m
			if k == 0 {
				s.Hero = &m.Walker
			}
			continue
		}
		n := &NPC{Walker: w, Rng: s.Rng.IntN}
		if k := p.Flags4; k == 0x4D || k == 0x4F {
			n.Hold = n.PauseHold
		}
		gr := game.Groups[p.Group]
		area, hasArea := gr.WanderArea()
		n.Area = area
		n.Target = -1
		switch {
		case gr.CreatureGroup():
			n.Mode, n.Group = NPCCreature, p.Group
			n.Blocked = s.Blocked
			if !restore {
				game.Groups[p.Group].Alarm = gamestate.AlarmCalm // 0x0043C66D
			}
			if p.Type%6 == gamestate.ActShoot {
				n.Act = gamestate.ActShoot // dormant (kinds 0x46, 0x4C, 0x55)
			}
			s.SetStopAt(n)
		case gr.VillageGroup() && hasArea:
			n.Group = p.Group
			s.SetStopAt(n)
			n.Mode = NPCVillager
			n.Post, n.Posted = game.PostOf(p.Entity)
			n.MoveAt = gamestate.VillageMoveThreshold(p.Entity)
			n.Blocked = s.Blocked
		default:
			n.Blocked = s.Blocked
		}
		n.LiveMode = n.Mode
		s.NPCs = append(s.NPCs, n)
		s.ByEntity[p.Index] = n
	}
}

// InitSelection keeps the selection of the world left (Sel as carried
// over) for the members on this map that can act; an empty selection
// becomes the hero (the game starts with the hero selected). prevCur is
// the character window's character before, -1 without a previous world.
func (s *World) InitSelection(prevCur int) {
	var sel []int
	for _, i := range s.Sel {
		if m, e := s.PartyMember(i); m != nil && !e.Dead() && !gamestate.Selected(sel, i) {
			sel = append(sel, i)
		}
	}
	if len(sel) == 0 && s.Lead() != nil && !s.Party[0].Dead() {
		sel = []int{s.Party[0].Index}
	}
	s.Sel = sel
	if prevCur >= 0 && prevCur < len(s.Party) {
		s.Cur = prevCur
	}
}
