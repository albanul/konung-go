package world

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

// The 16-tick passes (docs/konung_combat.md#upkeep-poison-and-respawn-verified,
// #village-anger-verified), loot on the ground and the villagers' fight.

// upkeep is 0x0041D0FC's group pass: poison, potion timers and respawn
// counters for every group (gamestate.Upkeep), the party included. Poison
// deaths on this map start the death action and drop loot with the hero's
// level.
func (s *World) Upkeep() {
	for _, e := range s.Party {
		s.Game.Entities[e.Index] = e
	}
	var died []int
	s.Game.Upkeep(s.Tables, s.MapNumber,
		func(i int) bool { w, _ := s.Combatant(i); return w != nil && w.Act == gamestate.ActDeath },
		func(i int) { died = append(died, i) })
	s.SyncParty()
	level := 0
	if len(s.Party) > 0 {
		level = s.Party[0].Level
	}
	for _, i := range died {
		w, e := s.Combatant(i)
		if w == nil {
			continue
		}
		w.Halt()
		w.StartAct(gamestate.ActDeath)
		s.DropLoot(e, w.Cell, 0, level)
	}
}

// dropLoot is 0x00418510 for victim on cell c: the pile there (or a new one
// while fewer than MaxActivePiles are active) gets the loot.
func (s *World) DropLoot(victim *gamestate.Entity, c mapdata.Cell, bonus, level int) {
	victim.Row, victim.Col = c.Row, c.Col
	at, on := s.PileAt[c]
	if !on {
		at = -1
	}
	curios := 0
	if len(s.Party) > 0 {
		curios = s.Party[0].Skills[4]
	}
	if s.LAN && s.PlayerGroup(victim.Group) {
		// A player's hero keeps its gold (LAN_KONG.EXE's loot 0x43BD54
		// drops only creatures' gold); its bag goes into the pile.
		gold := victim.Gold
		victim.Gold = 0
		defer func() { victim.Gold = gold }()
	}
	i, ok := s.Game.DropLoot(victim, bonus, level, s.MapNumber, at, len(s.PileAt) < gamestate.MaxActivePiles,
		&curios, s.Tables, s.Rng.IntN)
	if len(s.Party) > 0 {
		s.Party[0].Skills[4] = curios // identifying a weapon trains the hero's curios (+0x4A)
	}
	if ok {
		s.PileAt[c] = i
	}
}

// settlementTick is the settlement pass 0x0041D39F for the village on this
// map: surrender, then the alarm check 0x00416700.
func (s *World) SettlementTick() {
	st := s.Game.SettlementOnMap(s.MapNumber)
	if st == nil {
		return
	}
	party := s.PartyOnCells()
	if s.Game.Surrenders(st, party, s.Tables) {
		for _, n := range s.NPCs {
			if n.Group == st.Group {
				n.Target, n.Intent = -1, 0
				if n.Act == gamestate.ActAttack || n.Act == gamestate.ActShoot {
					n.Act = 0
				}
			}
		}
		s.EndAllFights()
		s.message(s.Tables.CString(gamestate.MsgVillageSurrenders))
		// 0x0041D777: next state 0 (the game), then DEFEAT<map>.FLM.
		s.Film, s.FilmToMenu = resfmt.DefeatFilm(s.MapNumber), false
	}
	s.Game.VillageAlarm(st, party)
}

// fighting reports a villager that the village AI must leave alone: its
// village is alarmed or it is still in a fight action.
func (s *World) Fighting(n *NPC) bool {
	return s.Game.Groups[n.Group].Alarm == gamestate.AlarmAttack || n.Act != 0 || n.Intent != 0
}

// villagerTick is the alarmed branch of the village AI (0x00416DA6) for n:
// women and children (+4 0x51/0x52) take random steps; men take the stance,
// shoot when they can, else go for the nearest party member in melee.
func (s *World) VillagerTick(n *NPC) {
	e := &s.Game.Entities[n.Entity]
	if !Alive(&n.Walker, e) || n.Act == gamestate.ActFlinch {
		return
	}
	g := &s.Game.Groups[n.Group]
	switch {
	case n.Act == gamestate.ActAttack:
		if s.Swing(&n.Walker, e, n.Target, n.TargetCell) {
			n.Act = 0
		}
		return
	case n.Act == gamestate.ActShoot:
		if s.ShotTick(&n.Walker, e, n.TargetCell) {
			n.Act, n.Intent = 0, n.Intent&^IntentShoot
		}
		return
	case n.Walking || len(n.Path) > 0:
		if n.Intent&IntentMelee != 0 {
			tw, t := s.Combatant(n.Target)
			switch {
			case !Alive(tw, t):
				n.Target, n.Intent = -1, 0
				n.StopWalk()
			case tw.Cell != n.TargetCell:
				n.TargetCell = tw.Cell
				n.Chase(tw.Cell)
			}
		}
		return
	}
	if g.Alarm != gamestate.AlarmAttack {
		n.Target, n.Intent = -1, 0 // calm again: the village AI takes over
		if e.InStance() {
			gamestate.LeaveStance(e) // 0x00416934 drops the stance first
			n.Look = LookOf(*e)
		}
		return
	}
	partyOK := len(s.LivingMembers()) > 0
	if e.Flags4 == 0x51 || e.Flags4 == 0x52 {
		if s.AIBudget <= 0 {
			return
		}
		s.AIBudget--
		blocked := func(r, c int) bool { return s.Blocked(mapdata.Cell{Row: r, Col: c}) }
		if r, c, ok := gamestate.VillageStepTarget(n.Cell.Row, n.Cell.Col, n.Area,
			gamestate.VillageRowRange, gamestate.VillageColRange, n.Rng, blocked); ok {
			n.Intent = IntentWalk
			n.WalkTo(mapdata.Cell{Row: r, Col: c})
		}
		return
	}
	if !partyOK {
		return
	}
	if !e.InStance() {
		s.Game.ReEquip(e, s.Tables) // 0x004105A0
		s.AIBudget--                // it counts a path search (0x00A3E720)
		gamestate.EnterStance(e)
		n.Look = LookOf(*e)
		return
	}
	if gamestate.HasBow(*e) && s.NpcShoot(n) {
		return
	}
	if e.WeaponMode == gamestate.WeaponBow {
		e.WeaponMode = gamestate.WeaponBlade
		if e.Equipment[gamestate.WeaponAxe] != 0 {
			e.WeaponMode = gamestate.WeaponAxe
		}
		n.Look = LookOf(*e)
	}
	if t := s.NearestMember(n.Cell, nil, nil); t >= 0 {
		s.GoFor(n, t) // 0x004202A8: the nearest party member
	}
}
