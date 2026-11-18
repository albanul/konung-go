package world

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// Combat in the world, one step per game tick; see docs/konung_combat.md.
//
// Melee, for party members and creatures alike:
//
//   - A walk toward the target ends before its cell (walker.StopAt; the game
//     turns that step into the swing, 0x004149B0). Next to the target the
//     attacker swings (action type 5).
//   - Swing (0x00421E8C party, 0x0041248D creatures): the attack stops when
//     the target left the cell or died (0x0041F258 / 0x00411FD0); the
//     attacker turns one octant per tick until it faces the target; at step
//     2 a party attacker alarms the target's group (0x004248A0); after the
//     last step the blow lands (gamestate.Strike) and the swing restarts. A
//     kill starts the target's death; a heavy hit its hit reaction (type 2).
//   - Hit reaction (type 2) and death (type 3, 0x00422447 / 0x00412944): the
//     victim is marked dead at step count−2 and frees its cell; a hit or
//     dying creature re-alarms its group (0x00412A21, 0x00412989); a dying
//     kind-0x4C creature with revives left rises again (gamestate.Revives).
//
// Party members (party.go): a right click on a character in the stance
// sets the target (0x00423C8B); while idle without a target a member strikes
// back at a neighbour attacking it (0x0041EC0C). The hero's death ends the
// game (gameOver).
//
// Creatures (AI 0x004123BC): every AlarmCheckTicks the group's alarm is
// recomputed (0x00412C30). Idle members then decide by the alarm
// (0x00412E00): calm → stay; attack → by gamestate.TacticOf go for a party
// member (creatureTarget); return → keep fighting while a member is inside
// the leash rectangle and a selector allows it, else walk back into the
// territory (0x004132FE). A chase ends when the target leaves the leash
// rectangle (0x00411FD0).
//
// Not ported: kind 0x49's retreat after a blow (0x004101E4), kinds
// 0x4D/0x4F's pauses between steps, walking speed (+0x11).

// Creature intent bits (entity +2).
const (
	IntentMelee     = 0x01 // go for the target (0x004142D4 flags 1)
	IntentRetaliate = 0x02 // stationary or retaliating swing (0x00412F62, 0x0041EC0C)
	IntentShoot     = 0x04 // shoot (0x00416DA6)
	IntentWalk      = 0x08 // walk order: going home (0x004142D4 flags 8)
)

// creatureBudget is the path searches the combat AI (creatures and alarmed
// villagers) may start per game tick (0x004116CC; the game shares 3 among
// all characters).
const creatureBudget = 3

// Attack order search range (0x0041FD74 for the party, +0x1E bit 0x20): the
// leader's cell ± (W + W/2) with W = the view's height/32 + 3 rows and
// width/116 + 4 cols (0x00427C8C, 800×600: the view is 660×556).
const (
	AttackRangeRows = 556/32 + 3
	AttackRangeCols = 660/116 + 4
)

// KindRetreat is the creature kind (entity +4) that runs off after a blow.
const KindRetreat = 0x49

// nearestEnemy is 0x0041E384's search for a new target (0x0041FD74 for the
// party, then 0x004203A0 / 0x004202A8): among groups other than the party on
// this map that are not villages (unless alarmed) and not of kind 0x53, the
// living member inside the attack range closest to cell h (gamestate.Dist).
// The game centres the range on the leader; the port on the member asking.
// -1 when there is none.
func (s *World) NearestEnemy(h mapdata.Cell) int {
	best, bestD := -1, 1<<30
	for _, n := range s.NPCs {
		e := &s.Game.Entities[n.Entity]
		if e.Group < 0 || s.PlayerGroup(e.Group) || !Alive(&n.Walker, e) {
			continue
		}
		if r := s.isRemote(n.Entity); r && !s.canTarget(n.Entity) {
			continue // a protected teammate
		}
		g := s.Game.Groups[e.Group]
		if (g.Kind&0x80 != 0 && g.Alarm != gamestate.AlarmAttack) || g.Kind == 0x53 {
			continue
		}
		dr, dc := n.Cell.Row-h.Row, n.Cell.Col-h.Col
		if max(dr, -dr) > AttackRangeRows+AttackRangeRows/2 || max(dc, -dc) > AttackRangeCols+AttackRangeCols/2 {
			continue
		}
		if d := gamestate.Dist(h.Row, h.Col, n.Cell.Row, n.Cell.Col); d < bestD {
			best, bestD = n.Entity, d
		}
	}
	return best
}

// setStopAt makes a creature's walks end before its target's cell.
func (s *World) SetStopAt(n *NPC) {
	n.StopAt = func(c mapdata.Cell) bool {
		tw, _ := s.Combatant(n.Target)
		return n.Intent&IntentMelee != 0 && tw != nil && c == tw.Cell
	}
}

// combatant returns the walker and live entity record of entity i on this
// map: a party member's party copy, or the GAME.N record of anyone else.
func (s *World) Combatant(i int) (*Walker, *gamestate.Entity) {
	if i < 0 {
		return nil, nil
	}
	if m, e := s.PartyMember(i); m != nil {
		return &m.Walker, e
	}
	if n, ok := s.ByEntity[i]; ok {
		return &n.Walker, &s.Game.Entities[i]
	}
	return nil, nil
}

// facing is the direction from cell a to cell b: the neighbour table for
// adjacent cells, the pixel direction otherwise (0x0043976C).
func Facing(a, b mapdata.Cell) mapdata.Direction {
	for _, n := range mapdata.Neighbors(a) {
		if n.Cell == b {
			return n.Dir
		}
	}
	ax, ay := mapdata.CellCenter(a)
	bx, by := mapdata.CellCenter(b)
	return mapdata.DirectionTo(float64(ax), float64(ay), float64(bx), float64(by))
}

func Adjacent(a, b mapdata.Cell) bool {
	for _, n := range mapdata.Neighbors(a) {
		if n.Cell == b {
			return true
		}
	}
	return false
}

// alive reports a combatant that can still fight or be fought.
func Alive(w *Walker, e *gamestate.Entity) bool {
	return w != nil && !e.Dead() && w.Act != gamestate.ActDeath
}

// combatTick runs once per game tick: the alarm check, the hero, the
// creatures, then everyone's hit reactions and deaths.
func (s *World) CombatTick() {
	s.FlyArrows() // 0x00421170 runs before the AI
	if s.GameTime()%gamestate.AlarmCheckTicks == 0 {
		s.AlarmCheck()
		s.Upkeep()
	}
	if (s.GameTime()+gamestate.SettlementTickOffset)%gamestate.UpkeepTicks == 0 {
		if len(s.Party) > 0 && s.Tables != nil && !s.LAN {
			// The LAN keeps the settlements' stocks per player slot (not
			// ported): no settlement pass there yet.
			// Healer and smith (0x0041768C, 0x00417A58) are frozen while
			// window 7 is open; tribute, buildings and training are not.
			s.StoreWorld()
			if s.Game.SettlementPass(s.Tables, s.Party[0].Index, s.MapNumber, s.tradeOpen()) {
				s.buildingsChanged()
			}
			s.SyncParty()
		}
		s.SettlementTick()
	}
	s.PartyTick()
	// Path searches are shared round-robin (0x004116CC with the last served
	// entity 0x00A3E690): the pass starts after the last member that used
	// one, so members whose search keeps failing can't starve the rest.
	s.AIBudget = creatureBudget
	if len(s.NPCs) > 0 {
		start := s.FightNext % len(s.NPCs)
		for i := range s.NPCs {
			k := (start + i) % len(s.NPCs)
			before := s.AIBudget
			switch n := s.NPCs[k]; n.Mode {
			case NPCCreature:
				s.CreatureTick(n)
			case NPCVillager:
				s.VillagerTick(n)
			}
			if s.AIBudget < before {
				s.FightNext = k + 1
			}
		}
	}
	for _, m := range s.Members {
		if m != nil {
			s.ActTick(&m.Walker, &s.Party[m.K], nil)
		}
	}
	for _, n := range s.NPCs {
		s.ActTick(&n.Walker, &s.Game.Entities[n.Entity], n)
	}
}

// swing is one tick of the melee swing of attacker (aw, a) at target; it
// reports whether the swing stopped (the attacker should go idle).
func (s *World) Swing(aw *Walker, a *gamestate.Entity, target int, targetCell mapdata.Cell) bool {
	tw, t := s.Combatant(target)
	if !Alive(tw, t) || tw.Cell != targetCell {
		return true
	}
	if d := Facing(aw.Cell, tw.Cell); aw.Dir != d {
		aw.TurnToward(d)
		return false
	}
	remote := s.isRemote(target)
	if aw.ActStep == 2 && s.PlayerGroup(a.Group) && !remote {
		s.Provoke(t.Group) // 0x00421F00: village anger, then 0x004248A0
	}
	if aw.AdvanceAct() {
		return false
	}
	a.Variant, t.Variant = int(aw.Dir), int(tw.Dir)
	if remote { // another player's hero: the server decides (lanfight.go)
		s.strikeRemote(a, target, false)
		aw.StartAct(gamestate.ActAttack)
		return false
	}
	switch r, _ := s.Game.Strike(a, t, s.Tables, s.Rng.IntN); r {
	case gamestate.StrikeDodged:
		if s.PlayerGroup(a.Group) {
			s.playAt(gamestate.SoundDodge, tw.Cell) // 0x0042204B
		}
	case gamestate.StrikeKill:
		s.Kill(a, tw, t, false)
		return true
	case gamestate.StrikeHeavy:
		s.Flinch(tw)
	}
	if s.PlayerGroup(a.Group) {
		aw.Look = LookOf(*a) // a worn-out weapon or armour disappears
	}
	if s.Retreat(a) {
		return true
	}
	aw.StartAct(gamestate.ActAttack)
	return false
}

// retreat is 0x004126A1 in the creature swing: after a blow that did not
// kill, a creature of kind 0x49 picks a cell far off (0x004101E4,
// gamestate.FleeOffset) and walks there with +2 = 8 (0x004142D4) instead of
// swinging again. It reports whether the walk started.
func (s *World) Retreat(a *gamestate.Entity) bool {
	n, ok := s.ByEntity[a.Index]
	if !ok || n.Mode != NPCCreature || a.Flags4 != KindRetreat {
		return false
	}
	dr, dc, ok := gamestate.FleeOffset(s.Rng.IntN, n.Cell.Row, n.Cell.Col,
		gamestate.VillageRowRange, gamestate.VillageColRange, func(r, c int) bool {
			return s.Blocked(mapdata.Cell{Row: r, Col: c})
		})
	if !ok {
		return false
	}
	n.WalkTo(mapdata.Cell{Row: n.Cell.Row + dr, Col: n.Cell.Col + dc})
	if len(n.Path) == 0 {
		return false
	}
	n.Intent = IntentWalk
	return true
}

// actTick plays hit reactions and deaths (n is nil for party members).
func (s *World) ActTick(w *Walker, e *gamestate.Entity, n *NPC) {
	creature := n != nil && n.Mode == NPCCreature
	villager := n != nil && s.Game.Groups[n.Group].VillageGroup()
	switch w.Act {
	case gamestate.ActFlinch:
		if !w.AdvanceAct() {
			w.Act = 0
			if creature {
				s.AlarmGroup(n.Group) // 0x00412A21
			}
			if villager {
				s.Provoke(n.Group) // 0x0041658E: a hurt villager angers the village
			}
		}
	case gamestate.ActDeath:
		if !e.Dead() && w.ActStep >= w.ActSteps()-2 {
			if villager {
				s.Provoke(n.Group) // 0x0041651C
			}
			if creature {
				s.AlarmGroup(n.Group) // 0x00412989
				if gamestate.Revives(e) {
					n.Target, n.Intent = -1, 0
					w.StartAct(gamestate.ActShoot) // dormant again
					return
				}
			}
			e.Flags4 |= 0x80 // 0x0042248F
			e.Flags2 = 0
			delete(s.Occ, w.Cell)
			if n != nil {
				n.Mode = NPCStill
			} else {
				s.Sel = gamestate.Deselect(s.Sel, e.Index) // port choice: the dead leave the selection
			}
		}
		w.AdvanceAct()
	}
}

// provoke is what a blow or an arrow does to the victim's group: a village
// gets angry (settlement +0x4B2 |= 0x15) and every group is alarmed
// (0x004248A0).
func (s *World) Provoke(g int) {
	if g <= 0 || g >= len(s.Game.Groups) {
		return
	}
	if st := s.Game.SettlementOf(g); st != nil {
		s.Game.Anger(st) // only the village group itself (0x00421F39 tests +0x1E & 0x80)
	}
	s.AlarmGroup(g)
}

// kill is the kill path (0x00422082 party, 0x0041263E creatures, 0x004215BA
// arrows): experience (gamestate.Kill / KillByArrow), the victim's death and
// its loot. Party and village killers pass their kill experience and level
// to the loot (0x00418510(victim, xp, level)); creatures pass 0, 0.
func (s *World) Kill(a *gamestate.Entity, tw *Walker, t *gamestate.Entity, arrow bool) {
	var xp int
	if arrow {
		xp = s.Game.KillByArrow(a, t, s.Tables)
	} else {
		xp = s.Game.Kill(a, t, s.Tables)
	}
	tw.Halt()
	tw.StartAct(gamestate.ActDeath)
	bonus, level := xp, a.Level
	if a.Group > 0 && s.Game.Groups[a.Group].CreatureGroup() {
		bonus, level = 0, 0
	}
	s.DropLoot(t, tw.Cell, bonus, level)
}

// flinch starts the hit reaction (type 2/8) of a character hit hard: the
// swings (0x00422131 party, 0x00412657 creatures) and arrows start it
// whatever the victim is doing, so a walk ends at once (the walker stays
// in the cell it was entering, walker.Halt). A party member's walk order goes on
// afterwards (+2 bit 0x08 stays; idleTick).
func (s *World) Flinch(tw *Walker) {
	if m, _ := s.PartyMember(tw.Entity); m != nil && &m.Walker == tw && (tw.Walking || len(tw.Path) > 0) &&
		m.Intent&(gamestate.IntentMelee|gamestate.IntentShoot) == 0 && m.Intent&gamestate.IntentWalk != 0 {
		m.Resume, m.ResumeSet = tw.Goal, true
	}
	tw.Halt()
	tw.StartAct(gamestate.ActFlinch)
}

// alarmGroup is 0x004248A0: group g is alarmed against the party (+0x1D =
// 1), and every member without the melee intent (and not of kind 0x49) has
// its walk cut after the current step (+0x14 = 0xFF), so all of them decide
// again at once instead of finishing a calm walk first.
func (s *World) AlarmGroup(g int) {
	if g <= 0 || g >= len(s.Game.Groups) {
		return
	}
	s.Game.Groups[g].Alarm = gamestate.AlarmAttack
	s.Game.Groups[g].Enemy = 0
	for _, n := range s.NPCs {
		if e := s.Game.Entities[n.Entity]; e.Group == g && n.Intent&IntentMelee == 0 && e.Flags4 != 0x49 {
			n.StopWalk()
		}
	}
}

// alarmCheck is 0x0041D0FC → 0x00412C30 for the creature groups on this map.
func (s *World) AlarmCheck() {
	party := s.PartyPositions()
	members := map[int][][2]int{}
	for _, n := range s.NPCs {
		if n.Mode == NPCCreature && Alive(&n.Walker, &s.Game.Entities[n.Entity]) {
			members[n.Group] = append(members[n.Group], [2]int{n.Cell.Row, n.Cell.Col})
		}
	}
	for g, m := range members {
		gr := &s.Game.Groups[g]
		gr.Alarm = gr.AlarmState(party, m)
		if gr.Alarm == gamestate.AlarmAttack {
			gr.Enemy = 0
		}
	}
}

// creatureTick is one game tick of creature n (0x004123BC's member loop).
func (s *World) CreatureTick(n *NPC) {
	e := &s.Game.Entities[n.Entity]
	if !Alive(&n.Walker, e) || n.Act == gamestate.ActFlinch {
		return
	}
	g := &s.Game.Groups[n.Group]
	switch {
	case n.Act == gamestate.ActAttack:
		if s.Swing(&n.Walker, e, n.Target, n.TargetCell) {
			n.Act = 0 // 0x00410450; the intent stays, the next decision re-targets
			if tw, t := s.Combatant(n.Target); !Alive(tw, t) {
				n.Target = -1
			}
		}
		return
	case n.Act == gamestate.ActShoot && n.Intent&IntentShoot != 0: // bow shot (0x00412AE5)
		if s.ShotTick(&n.Walker, e, n.TargetCell) {
			n.Act, n.Intent = 0, n.Intent&^IntentShoot
		}
		return
	case n.Act == gamestate.ActShoot && n.ActStep > 0: // waking up (0x00412A36)
		if n.ActStep == 1 && e.Animated() {
			s.playAt(gamestate.WakeSound(*e), n.Cell) // 0x00412A60
		}
		if !n.AdvanceAct() {
			n.Act = 0
		}
		return
	case n.Walking || len(n.Path) > 0:
		s.CreatureChase(n, g)
		return
	}
	s.CreatureDecide(n, e, g)
}

// creatureChase is 0x00411FD0 for a walking creature with the melee intent.
func (s *World) CreatureChase(n *NPC, g *gamestate.Group) {
	if n.Intent&IntentMelee == 0 {
		return
	}
	tw, t := s.Combatant(n.Target)
	switch {
	case !Alive(tw, t):
		n.Target = -1
		n.StopWalk()
	case tw.Cell == n.TargetCell:
	case !g.LeashArea().Contains(tw.Cell.Row, tw.Cell.Col):
		n.Target, n.Intent = -1, 0 // the target escaped the leash
		n.StopWalk()
	default:
		n.TargetCell = tw.Cell
		n.Chase(tw.Cell)
	}
}

// creatureDecide is 0x00412E00 for an idle creature (or a dormant one).
func (s *World) CreatureDecide(n *NPC, e *gamestate.Entity, g *gamestate.Group) {
	// 0x00412E4F: women, children and kind 0x53 (the lone wizards,
	// hermits and the like of the 0x53 groups) never act in the creature
	// AI, even when their group is alarmed.
	if k := e.Flags4; k == 0x51 || k == 0x52 || k == 0x53 {
		return
	}
	partyOK := len(s.LivingMembers()) > 0
	tactic := gamestate.TacticOf(e.Flags4)
	switch g.Alarm {
	case gamestate.AlarmAttack:
		if !partyOK || g.Enemy != 0 {
			return
		}
		switch tactic {
		case gamestate.TacticStationary:
			s.StationarySwing(n)
			return
		case gamestate.TacticDormant:
			if n.Act == gamestate.ActShoot {
				n.AdvanceAct() // wake up (0x0041309D)
				return
			}
		case gamestate.TacticNearest:
			if !e.Animated() && !e.InStance() {
				s.Game.ReEquip(e, s.Tables) // 0x004105A0
				s.AIBudget--                // it counts a path search (0x00A3E720)
				gamestate.EnterStance(e)    // humanoids take the stance first (0x00413108)
				n.Look = LookOf(*e)
				return
			}
			if !e.Animated() && gamestate.HasBow(*e) && s.NpcShoot(n) {
				return
			}
		}
		if t := s.CreatureTarget(n, tactic, nil, true); t >= 0 {
			s.GoFor(n, t)
		}
	case gamestate.AlarmReturn:
		if n.Act == gamestate.ActShoot {
			return
		}
		leash := g.LeashArea()
		inLeash := func(c mapdata.Cell) bool { return leash.Contains(c.Row, c.Col) }
		if n.Intent&IntentMelee != 0 && partyOK && s.NearestMember(n.Cell, inLeash, nil) >= 0 {
			if t := s.ReturnTarget(n, tactic, inLeash); t >= 0 && s.GoFor(n, t) {
				return
			}
		}
		n.Intent &^= IntentMelee
		if area, _ := g.WanderArea(); !area.Contains(n.Cell.Row, n.Cell.Col) && s.AIBudget > 0 {
			s.AIBudget--
			blocked := func(r, c int) bool { return s.Blocked(mapdata.Cell{Row: r, Col: c}) }
			if r, c, ok := gamestate.WanderTarget(area, n.Rng, blocked); ok {
				n.Intent = IntentWalk
				n.WalkTo(mapdata.Cell{Row: r, Col: c})
			}
		}
	}
}

// creatureTarget is the target selector of the attack state by tactic,
// with the party as the enemy group: assist (0x00420178) a group-mate's
// living target, unengaged and dormant (0x004203A0) the nearest member no
// group-mate attacks; with fallback, or for the other tactics, the nearest
// member (0x004202A8). inside, when set, limits the candidates. -1 = none.
func (s *World) CreatureTarget(n *NPC, tactic gamestate.Tactic, inside func(mapdata.Cell) bool, fallback bool) int {
	switch tactic {
	case gamestate.TacticAssist:
		for _, m := range s.NPCs {
			if m == n || m.Group != n.Group || m.Intent&IntentMelee == 0 || !Alive(&m.Walker, &s.Game.Entities[m.Entity]) {
				continue
			}
			if tw, t := s.Combatant(m.Target); Alive(tw, t) && s.PlayerGroup(t.Group) && (inside == nil || inside(tw.Cell)) {
				return m.Target
			}
		}
	case gamestate.TacticUnengaged, gamestate.TacticDormant:
		if t := s.NearestMember(n.Cell, inside, func(i int) bool { return s.MateTargets(n, i) }); t >= 0 {
			return t
		}
	default:
		return s.NearestMember(n.Cell, inside, nil)
	}
	if !fallback {
		return -1
	}
	return s.NearestMember(n.Cell, inside, nil)
}

// returnTarget is the kind switch of the return state (0x004132FE): who may
// keep fighting. The assist and unengaged selectors run without the
// nearest fallback, over the members inside the leash; the others (and
// stationary creatures) find nothing there.
func (s *World) ReturnTarget(n *NPC, tactic gamestate.Tactic, inLeash func(mapdata.Cell) bool) int {
	switch tactic {
	case gamestate.TacticAssist, gamestate.TacticUnengaged, gamestate.TacticDormant:
		return s.CreatureTarget(n, tactic, inLeash, false)
	}
	return -1
}

// mateTargets reports a living group-mate of n with the melee intent on
// target (0x00420178, 0x004203A0).
func (s *World) MateTargets(n *NPC, target int) bool {
	for _, m := range s.NPCs {
		if m != n && m.Group == n.Group && m.Intent&IntentMelee != 0 && m.Target == target &&
			Alive(&m.Walker, &s.Game.Entities[m.Entity]) {
			return true
		}
	}
	return false
}

// goFor sets target and walks toward it (0x0041326B → 0x004142D4 flags 1),
// or swings at once when it is adjacent. It reports success.
func (s *World) GoFor(n *NPC, target int) bool {
	tw, t := s.Combatant(target)
	if !Alive(tw, t) {
		return false
	}
	n.Target, n.TargetCell, n.Intent = target, tw.Cell, IntentMelee
	if Adjacent(n.Cell, tw.Cell) {
		n.StartAct(gamestate.ActAttack)
		return true
	}
	if s.AIBudget <= 0 {
		return true // decide again next tick
	}
	s.AIBudget--
	if !s.Reachable(tw.Cell) {
		return false // the search would fail: every cell next to the target is taken
	}
	n.Chase(tw.Cell)
	return len(n.Path) > 0
}

// reachable reports a free cell next to c, without which a chase toward c
// can only fail; it saves the search (the budget is still spent).
func (s *World) Reachable(c mapdata.Cell) bool {
	for _, n := range mapdata.Neighbors(c) {
		if !s.Blocked(n.Cell) {
			return true
		}
	}
	return false
}

// stationarySwing is 0x00412F62: swing at an adjacent member of the enemy
// group (the first living member next to it), never move.
func (s *World) StationarySwing(n *NPC) {
	for _, m := range s.LivingMembers() {
		if !Adjacent(n.Cell, m.Cell) {
			continue
		}
		n.Target, n.TargetCell, n.Intent = s.Party[m.K].Index, m.Cell, IntentRetaliate
		n.Dir = Facing(n.Cell, m.Cell)
		n.StartAct(gamestate.ActAttack)
		return
	}
}

// gameOver is the main loop's check 0x004376A3: the hero's +4 bit 0x80,
// which its death animation sets on its second-to-last frame (0x0042248F),
// ends the game at once with GAMEOVER.FLM.
func (s *World) GameOver() bool {
	return len(s.Party) > 0 && s.Party[0].Dead()
}
