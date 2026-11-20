package world

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// The LAN server's maps (World.LAN): the heroes of the player slots on a
// map are its party members, each its own leader. LAN_KONG.EXE's server
// keeps player slot k in group k with entity k (groups 0–15; 0x0044ADB4).

// LANPlayers is the number of player slots (groups and entities 0–15).
const LANPlayers = 16

// PlayerGroup reports whether group g is a player's party: group 0 in
// single-player, groups 0–15 in a LAN world.
func (s *World) PlayerGroup(g int) bool {
	return g == 0 || s.LAN && g > 0 && g < LANPlayers
}

// AddMember puts entity i (its record in Game.Entities) on this map as a
// party member on cell c, or the nearest free cell when c is taken or
// blocked, facing dir, idle. attach gives the walker its frames.
func (s *World) AddMember(i int, c mapdata.Cell, dir int, attach Attach) *Member {
	c = mapdata.NearestFree(c, s.Blocked, 8)
	e := s.Game.Entities[i]
	e.Row, e.Col, e.Variant = c.Row, c.Col, dir&7
	s.Game.Entities[i] = e
	k := len(s.Party)
	s.Party = append(s.Party, e)
	reserve := func(c mapdata.Cell) bool {
		if s.Occ[c] {
			return false
		}
		s.Occ[c] = true
		return true
	}
	w := Walker{
		Entity: i, Cell: c, Goal: c, Dir: mapdata.Direction(dir & 7), Idle: -1,
		Look:    LookOf(e),
		Reserve: reserve, Release: func(c mapdata.Cell) { delete(s.Occ, c) },
		Sound: s.actionSound, Blocked: s.Blocked,
	}
	if attach != nil {
		attach(&w, gamestate.Placed{Entity: e, Row: c.Row, Col: c.Col}, func() gamestate.Entity { return s.Party[s.memberK(i)] })
	}
	m := &Member{Walker: w, K: k, Target: -1, Intent: gamestate.IntentIdle}
	s.Members = append(s.Members, m)
	if !e.Dead() {
		s.Occ[c] = true
	}
	if k == 0 {
		s.Hero = &m.Walker
	}
	return m
}

// memberK is entity i's index in the party list (-1 when absent).
func (s *World) memberK(i int) int { return s.PartyIndex(i) }

// RemoveMember takes entity i's member off this map: its record (cell,
// stance, items, HP) goes back into Game.Entities, its cells are freed,
// fights aimed at it end, and the other members are renumbered. It returns
// the record, false when i is not a member here.
func (s *World) RemoveMember(i int) (gamestate.Entity, bool) {
	k := s.PartyIndex(i)
	if k < 0 {
		return gamestate.Entity{}, false
	}
	m := s.Members[k]
	e := &s.Party[k]
	e.Row, e.Col, e.Variant = m.Cell.Row, m.Cell.Col, int(m.Dir)
	e.Flags2 = m.Intent
	s.Game.Entities[i] = *e
	out := *e
	delete(s.Occ, m.Cell)
	if m.Walking {
		delete(s.Occ, m.From)
	}
	s.Party = append(s.Party[:k], s.Party[k+1:]...)
	s.Members = append(s.Members[:k], s.Members[k+1:]...)
	for j, o := range s.Members {
		o.K = j
		if o.Target == i {
			s.EndFight(o)
		}
	}
	for _, n := range s.NPCs {
		if n.Target == i {
			n.Target = -1
			n.Intent &^= IntentMelee
			n.StopAt = nil
		}
	}
	s.Sel = gamestate.Deselect(s.Sel, i)
	s.Hero = nil
	if len(s.Members) > 0 {
		s.Hero = &s.Members[0].Walker
	}
	return out, true
}

// Member returns entity i's member and its party record on this map.
func (s *World) Member(i int) (*Member, *gamestate.Entity) { return s.PartyMember(i) }

// SetStance puts member m into the combat stance or out of it, like Space
// for one hero (gamestate.ToggleStance); leaving it ends a melee fight.
func (s *World) SetStance(m *Member, on bool) {
	e := &s.Party[m.K]
	if e.InStance() != on {
		gamestate.ToggleStance([]*gamestate.Entity{e})
	}
	if !e.InStance() {
		s.EndFight(m)
	}
	m.Look = LookOf(*e)
}

// SetWeapon selects member m's weapon (1/2/3 or the HUD button,
// gamestate.SelectWeapon).
func (s *World) SetWeapon(m *Member, mode int) {
	e := &s.Party[m.K]
	gamestate.SelectWeapon(e, mode)
	m.Look = LookOf(*e)
}

// AttackNearest is the A key for one hero (AttackOrder's rule): the
// stance, then the nearest enemy in melee or the bow.
func (s *World) AttackNearest(m *Member) {
	e := &s.Party[m.K]
	if !Alive(&m.Walker, e) {
		return
	}
	if !e.InStance() {
		gamestate.ToggleStance([]*gamestate.Entity{e})
		m.Look = LookOf(*e)
	}
	s.EndFight(m)
	m.PickIntent, m.ResumeSet = false, false
	if e.WeaponMode == gamestate.WeaponBow {
		m.Intent = gamestate.IntentShoot | gamestate.IntentIdle
		return
	}
	m.Path = m.Path[:min(len(m.Path), 1)]
	m.Intent = gamestate.IntentMelee | gamestate.IntentIdle
}

// Leads reports whether member m leads itself: the hero in single player
// (party index 0); in a LAN world every player's hero (entity < LANPlayers),
// while its hirelings (entities of the world in its group) follow it.
func (s *World) Leads(m *Member) bool {
	if s.LAN {
		return s.Party[m.K].Index < LANPlayers
	}
	return m.K == 0
}

// leaderOf is whom member m follows: the hero (Lead) in single player; in
// a LAN world its player's hero (the member whose entity is m's group),
// nil when that hero is not on this map.
func (s *World) leaderOf(m *Member) *Member {
	if !s.LAN {
		return s.Lead()
	}
	l, _ := s.PartyMember(s.Party[m.K].Group)
	return l
}

// Hire makes character i on this map (a villager, an NPC here) a member of
// player group group (a LAN hireling, the server's 291, 0x0044BED7: the
// entity's group byte +5 becomes the player's slot, +2 = 0x60, +0x9C =
// 0xFF): its walker leaves the NPCs and comes back as a party member on its
// cell, idle and following its player's hero. The entity keeps its index
// (the LAN server keeps groups as member lists; GAME.RES's village group
// range still spans it, so code over a group's range checks the entity's
// own group). It returns nil when i is not an NPC here.
func (s *World) Hire(i, group int, attach Attach) *Member {
	n := s.ByEntity[i]
	if n == nil {
		return nil
	}
	s.dropNPC(n)
	e := &s.Game.Entities[i]
	e.Group, e.Flags2 = group, gamestate.IntentGather
	e.Raw[0x14] = 0xFF
	m := s.AddMember(i, n.Cell, int(n.Dir), attach)
	m.Intent = gamestate.IntentGather
	return m
}

// dropNPC takes NPC n off this map: the NPC list, the index and its cells.
func (s *World) dropNPC(n *NPC) {
	delete(s.Occ, n.Cell)
	if n.Walking {
		delete(s.Occ, n.From)
	}
	delete(s.ByEntity, n.Entity)
	for k, o := range s.NPCs {
		if o == n {
			s.NPCs = append(s.NPCs[:k], s.NPCs[k+1:]...)
			if s.AINext > k {
				s.AINext--
			}
			break
		}
	}
}

// AddNPC puts character i (its record in Game.Entities, of a world group)
// on this map as an NPC on cell c, or the nearest free cell, facing dir,
// with the AI its group gives it (as the map entry, Populate). A LAN
// hireling going back to its village (lanserver).
func (s *World) AddNPC(i int, c mapdata.Cell, dir int, attach Attach) *NPC {
	c = mapdata.NearestFree(c, s.Blocked, 8)
	e := &s.Game.Entities[i]
	e.Row, e.Col, e.Variant = c.Row, c.Col, dir&7
	if !e.Dead() {
		s.Occ[c] = true
	}
	return s.addNPC(gamestate.Placed{Entity: *e, Row: c.Row, Col: c.Col}, true, attach)
}

// Gather is «Все ко мне» (GatherOrder) for member m: idle, following its
// leader, the walk ending after the current step.
func (s *World) Gather(m *Member) {
	s.EndFight(m)
	m.Intent = gamestate.IntentGather
	m.PickIntent, m.ResumeSet = false, false
	m.StopWalk()
}
