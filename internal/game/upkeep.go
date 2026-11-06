package game

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// The 16-tick passes (docs/konung_combat.md#upkeep-poison-and-respawn-verified,
// #village-anger-verified), loot on the ground and the villagers' fight.

// upkeep is 0x0041D0FC's group pass: poison, potion timers and respawn
// counters for every group (gamestate.Upkeep), the party included. Poison
// deaths on this map start the death action and drop loot with the hero's
// level.
func (s *startScene) upkeep() {
	for _, e := range s.party {
		s.game.Entities[e.Index] = e
	}
	var died []int
	s.game.Upkeep(s.tables, s.mapNumber,
		func(i int) bool { w, _ := s.combatant(i); return w != nil && w.act == gamestate.ActDeath },
		func(i int) { died = append(died, i) })
	s.syncParty()
	level := 0
	if len(s.party) > 0 {
		level = s.party[0].Level
	}
	for _, i := range died {
		w, e := s.combatant(i)
		if w == nil {
			continue
		}
		w.halt()
		w.startAct(gamestate.ActDeath)
		s.dropLoot(e, w.cell, 0, level)
	}
}

// dropLoot is 0x00418510 for victim on cell c: the pile there (or a new one
// while fewer than MaxActivePiles are active) gets the loot.
func (s *startScene) dropLoot(victim *gamestate.Entity, c mapdata.Cell, bonus, level int) {
	victim.Row, victim.Col = c.Row, c.Col
	at, on := s.pileAt[c]
	if !on {
		at = -1
	}
	curios := 0
	if len(s.party) > 0 {
		curios = s.party[0].Skills[4]
	}
	i, ok := s.game.DropLoot(victim, bonus, level, s.mapNumber, at, len(s.pileAt) < gamestate.MaxActivePiles,
		&curios, s.tables, s.rng.IntN)
	if len(s.party) > 0 {
		s.party[0].Skills[4] = curios // identifying a weapon trains the hero's curios (+0x4A)
	}
	if ok {
		s.pileAt[c] = i
	}
}

// settlementTick is the settlement pass 0x0041D39F for the village on this
// map: surrender, then the alarm check 0x00416700.
func (s *startScene) settlementTick() {
	st := s.game.SettlementOnMap(s.mapNumber)
	if st == nil {
		return
	}
	party := s.partyOnCells()
	if s.game.Surrenders(st, party, s.tables) {
		for _, n := range s.npcs {
			if n.group == st.Group {
				n.target, n.intent = -1, 0
				if n.act == gamestate.ActAttack || n.act == gamestate.ActShoot {
					n.act = 0
				}
			}
		}
		s.endAllFights()
		s.showMessage(s.tables.CString(gamestate.MsgVillageSurrenders))
	}
	s.game.VillageAlarm(st, party)
}

// fighting reports a villager that the village AI must leave alone: its
// village is alarmed or it is still in a fight action.
func (s *startScene) fighting(n *npc) bool {
	return s.game.Groups[n.group].Alarm == gamestate.AlarmAttack || n.act != 0 || n.intent != 0
}

// villagerTick is the alarmed branch of the village AI (0x00416DA6) for n:
// women and children (+4 0x51/0x52) take random steps; men take the stance,
// shoot when they can, else go for the nearest party member in melee.
func (s *startScene) villagerTick(n *npc) {
	e := &s.game.Entities[n.entity]
	if !alive(&n.walker, e) || n.act == gamestate.ActFlinch {
		return
	}
	g := &s.game.Groups[n.group]
	switch {
	case n.act == gamestate.ActAttack:
		if s.swing(&n.walker, e, n.target, n.targetCell) {
			n.act = 0
		}
		return
	case n.act == gamestate.ActShoot:
		if s.shotTick(&n.walker, e, n.targetCell) {
			n.act, n.intent = 0, n.intent&^intentShoot
		}
		return
	case n.walking || len(n.path) > 0:
		if n.intent&intentMelee != 0 {
			tw, t := s.combatant(n.target)
			switch {
			case !alive(tw, t):
				n.target, n.intent = -1, 0
				n.stopWalk()
			case tw.cell != n.targetCell:
				n.targetCell = tw.cell
				n.chase(tw.cell)
			}
		}
		return
	}
	if g.Alarm != gamestate.AlarmAttack {
		n.target, n.intent = -1, 0 // calm again: the village AI takes over
		if e.InStance() {
			gamestate.LeaveStance(e) // 0x00416934 drops the stance first
			n.look = lookOf(*e)
		}
		return
	}
	partyOK := len(s.livingMembers()) > 0
	if e.Flags4 == 0x51 || e.Flags4 == 0x52 {
		if s.aiBudget <= 0 {
			return
		}
		s.aiBudget--
		blocked := func(r, c int) bool { return s.blocked(mapdata.Cell{Row: r, Col: c}) }
		if r, c, ok := gamestate.VillageStepTarget(n.cell.Row, n.cell.Col, n.area,
			gamestate.VillageRowRange, gamestate.VillageColRange, n.rng, blocked); ok {
			n.intent = intentWalk
			n.walkTo(mapdata.Cell{Row: r, Col: c})
		}
		return
	}
	if !partyOK {
		return
	}
	if !e.InStance() {
		s.game.ReEquip(e) // 0x004105A0
		gamestate.EnterStance(e)
		n.look = lookOf(*e)
		return
	}
	if gamestate.HasBow(*e) && s.npcShoot(n) {
		return
	}
	if e.WeaponMode == gamestate.WeaponBow {
		e.WeaponMode = gamestate.WeaponBlade
		if e.Equipment[gamestate.WeaponAxe] != 0 {
			e.WeaponMode = gamestate.WeaponAxe
		}
		n.look = lookOf(*e)
	}
	if t := s.nearestMember(n.cell, nil, nil); t >= 0 {
		s.goFor(n, t) // 0x004202A8: the nearest party member
	}
}
