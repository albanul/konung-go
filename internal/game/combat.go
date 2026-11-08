package game

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// Combat in the scene, one step per game tick; see docs/konung_combat.md.
//
// Melee, for party members and creatures alike:
//
//   - A walk toward the target ends before its cell (walker.stopAt; the game
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
	intentMelee     = 0x01 // go for the target (0x004142D4 flags 1)
	intentRetaliate = 0x02 // stationary or retaliating swing (0x00412F62, 0x0041EC0C)
	intentWalk      = 0x08 // walk order: going home (0x004142D4 flags 8)
)

// creatureBudget is the path searches the combat AI (creatures and alarmed
// villagers) may start per game tick (0x004116CC; the game shares 3 among
// all characters).
const creatureBudget = 3

// Attack order search range (0x0041FD74 for the party, +0x1E bit 0x20): the
// leader's cell ± (W + W/2) with W = the view's height/32 + 3 rows and
// width/116 + 4 cols (0x00427C8C, 800×600).
var (
	attackRangeRows = (viewBottom-viewTop+1)/32 + 3
	attackRangeCols = (viewRight-viewLeft+1)/116 + 4
)

// nearestEnemy is 0x0041E384's search for a new target (0x0041FD74 for the
// party, then 0x004203A0 / 0x004202A8): among groups other than the party on
// this map that are not villages (unless alarmed) and not of kind 0x53, the
// living member inside the attack range closest to cell h (gamestate.Dist).
// The game centres the range on the leader; the port on the member asking.
// -1 when there is none.
func (s *startScene) nearestEnemy(h mapdata.Cell) int {
	best, bestD := -1, 1<<30
	for _, n := range s.npcs {
		e := &s.game.Entities[n.entity]
		if e.Group <= 0 || !alive(&n.walker, e) {
			continue
		}
		g := s.game.Groups[e.Group]
		if (g.Kind&0x80 != 0 && g.Alarm != gamestate.AlarmAttack) || g.Kind == 0x53 {
			continue
		}
		dr, dc := n.cell.Row-h.Row, n.cell.Col-h.Col
		if max(dr, -dr) > attackRangeRows+attackRangeRows/2 || max(dc, -dc) > attackRangeCols+attackRangeCols/2 {
			continue
		}
		if d := gamestate.Dist(h.Row, h.Col, n.cell.Row, n.cell.Col); d < bestD {
			best, bestD = n.entity, d
		}
	}
	return best
}

// setStopAt makes a creature's walks end before its target's cell.
func (s *startScene) setStopAt(n *npc) {
	n.stopAt = func(c mapdata.Cell) bool {
		tw, _ := s.combatant(n.target)
		return n.intent&intentMelee != 0 && tw != nil && c == tw.cell
	}
}

// combatant returns the walker and live entity record of entity i on this
// map: a party member's party copy, or the GAME.N record of anyone else.
func (s *startScene) combatant(i int) (*walker, *gamestate.Entity) {
	if i < 0 {
		return nil, nil
	}
	if m, e := s.partyMember(i); m != nil {
		return &m.walker, e
	}
	if n, ok := s.byEntity[i]; ok {
		return &n.walker, &s.game.Entities[i]
	}
	return nil, nil
}

// facing is the direction from cell a to cell b: the neighbour table for
// adjacent cells, the pixel direction otherwise (0x0043976C).
func facing(a, b mapdata.Cell) mapdata.Direction {
	for _, n := range mapdata.Neighbors(a) {
		if n.Cell == b {
			return n.Dir
		}
	}
	ax, ay := mapdata.CellCenter(a)
	bx, by := mapdata.CellCenter(b)
	return mapdata.DirectionTo(float64(ax), float64(ay), float64(bx), float64(by))
}

func adjacent(a, b mapdata.Cell) bool {
	for _, n := range mapdata.Neighbors(a) {
		if n.Cell == b {
			return true
		}
	}
	return false
}

// alive reports a combatant that can still fight or be fought.
func alive(w *walker, e *gamestate.Entity) bool {
	return w != nil && !e.Dead() && w.act != gamestate.ActDeath
}

// combatTick runs once per game tick: the alarm check, the hero, the
// creatures, then everyone's hit reactions and deaths.
func (s *startScene) combatTick() {
	s.flyArrows() // 0x00421170 runs before the AI
	if s.gameTime()%gamestate.AlarmCheckTicks == 0 {
		s.alarmCheck()
		s.upkeep()
	}
	if (s.gameTime()+gamestate.SettlementTickOffset)%gamestate.UpkeepTicks == 0 {
		if len(s.party) > 0 && s.tables != nil {
			// Healer and smith (0x0041768C, 0x00417A58) are frozen while
			// window 7 is open; tribute, buildings and training are not.
			s.storeWorld()
			if s.game.SettlementPass(s.tables, s.party[0].Index, s.mapNumber, s.trade.open()) {
				s.updateBuildings()
			}
			s.syncParty()
		}
		s.settlementTick()
	}
	s.partyTick()
	// Path searches are shared round-robin (0x004116CC with the last served
	// entity 0x00A3E690): the pass starts after the last member that used
	// one, so members whose search keeps failing can't starve the rest.
	s.aiBudget = creatureBudget
	if len(s.npcs) > 0 {
		start := s.fightNext % len(s.npcs)
		for i := range s.npcs {
			k := (start + i) % len(s.npcs)
			before := s.aiBudget
			switch n := s.npcs[k]; n.mode {
			case npcCreature:
				s.creatureTick(n)
			case npcVillager:
				s.villagerTick(n)
			}
			if s.aiBudget < before {
				s.fightNext = k + 1
			}
		}
	}
	for _, m := range s.members {
		if m != nil {
			s.actTick(&m.walker, &s.party[m.k], nil)
		}
	}
	for _, n := range s.npcs {
		s.actTick(&n.walker, &s.game.Entities[n.entity], n)
	}
}

// swing is one tick of the melee swing of attacker (aw, a) at target; it
// reports whether the swing stopped (the attacker should go idle).
func (s *startScene) swing(aw *walker, a *gamestate.Entity, target int, targetCell mapdata.Cell) bool {
	tw, t := s.combatant(target)
	if !alive(tw, t) || tw.cell != targetCell {
		return true
	}
	if d := facing(aw.cell, tw.cell); aw.dir != d {
		aw.turnToward(d)
		return false
	}
	if aw.actStep == 2 && a.Group == 0 {
		s.provoke(t.Group) // 0x00421F00: village anger, then 0x004248A0
	}
	if aw.advanceAct() {
		return false
	}
	a.Variant, t.Variant = int(aw.dir), int(tw.dir)
	switch r, _ := s.game.Strike(a, t, s.tables, s.rng.IntN); r {
	case gamestate.StrikeDodged:
		if a.Group == 0 {
			s.playAt(gamestate.SoundDodge, tw.cell) // 0x0042204B
		}
	case gamestate.StrikeKill:
		s.kill(a, tw, t, false)
		return true
	case gamestate.StrikeHeavy:
		s.flinch(tw)
	}
	if a.Group == 0 {
		aw.look = lookOf(*a) // a worn-out weapon or armour disappears
	}
	if s.retreat(a) {
		return true
	}
	aw.startAct(gamestate.ActAttack)
	return false
}

// kindRetreat is the creature kind (entity +4) that runs off after a blow.
const kindRetreat = 0x49

// retreat is 0x004126A1 in the creature swing: after a blow that did not
// kill, a creature of kind 0x49 picks a cell far off (0x004101E4,
// gamestate.FleeOffset) and walks there with +2 = 8 (0x004142D4) instead of
// swinging again. It reports whether the walk started.
func (s *startScene) retreat(a *gamestate.Entity) bool {
	n, ok := s.byEntity[a.Index]
	if !ok || n.mode != npcCreature || a.Flags4 != kindRetreat {
		return false
	}
	dr, dc, ok := gamestate.FleeOffset(s.rng.IntN, n.cell.Row, n.cell.Col,
		gamestate.VillageRowRange, gamestate.VillageColRange, func(r, c int) bool {
			return s.blocked(mapdata.Cell{Row: r, Col: c})
		})
	if !ok {
		return false
	}
	n.walkTo(mapdata.Cell{Row: n.cell.Row + dr, Col: n.cell.Col + dc})
	if len(n.path) == 0 {
		return false
	}
	n.intent = intentWalk
	return true
}

// actTick plays hit reactions and deaths (n is nil for party members).
func (s *startScene) actTick(w *walker, e *gamestate.Entity, n *npc) {
	creature := n != nil && n.mode == npcCreature
	villager := n != nil && s.game.Groups[n.group].VillageGroup()
	switch w.act {
	case gamestate.ActFlinch:
		if !w.advanceAct() {
			w.act = 0
			if creature {
				s.alarmGroup(n.group) // 0x00412A21
			}
			if villager {
				s.provoke(n.group) // 0x0041658E: a hurt villager angers the village
			}
		}
	case gamestate.ActDeath:
		if !e.Dead() && w.actStep >= w.actSteps()-2 {
			if villager {
				s.provoke(n.group) // 0x0041651C
			}
			if creature {
				s.alarmGroup(n.group) // 0x00412989
				if gamestate.Revives(e) {
					n.target, n.intent = -1, 0
					w.startAct(gamestate.ActShoot) // dormant again
					return
				}
			}
			e.Flags4 |= 0x80 // 0x0042248F
			e.Flags2 = 0
			delete(s.occ, w.cell)
			if n != nil {
				n.mode = npcStill
			} else {
				s.sel = gamestate.Deselect(s.sel, e.Index) // port choice: the dead leave the selection
			}
		}
		w.advanceAct()
	}
}

// provoke is what a blow or an arrow does to the victim's group: a village
// gets angry (settlement +0x4B2 |= 0x15) and every group is alarmed
// (0x004248A0).
func (s *startScene) provoke(g int) {
	if g <= 0 || g >= len(s.game.Groups) {
		return
	}
	if st := s.game.SettlementOf(g); st != nil {
		s.game.Anger(st) // only the village group itself (0x00421F39 tests +0x1E & 0x80)
	}
	s.alarmGroup(g)
}

// kill is the kill path (0x00422082 party, 0x0041263E creatures, 0x004215BA
// arrows): experience (gamestate.Kill / KillByArrow), the victim's death and
// its loot. Party and village killers pass their kill experience and level
// to the loot (0x00418510(victim, xp, level)); creatures pass 0, 0.
func (s *startScene) kill(a *gamestate.Entity, tw *walker, t *gamestate.Entity, arrow bool) {
	var xp int
	if arrow {
		xp = s.game.KillByArrow(a, t, s.tables)
	} else {
		xp = s.game.Kill(a, t, s.tables)
	}
	tw.halt()
	tw.startAct(gamestate.ActDeath)
	bonus, level := xp, a.Level
	if a.Group > 0 && s.game.Groups[a.Group].CreatureGroup() {
		bonus, level = 0, 0
	}
	s.dropLoot(t, tw.cell, bonus, level)
}

// flinch starts the hit reaction (type 2/8) of a character hit hard: the
// swings (0x00422131 party, 0x00412657 creatures) and arrows start it
// whatever the victim is doing, so a walk ends at once (the walker goes
// back to the cell it was leaving). A party member's walk order goes on
// afterwards (+2 bit 0x08 stays; idleTick).
func (s *startScene) flinch(tw *walker) {
	if m, _ := s.partyMember(tw.entity); m != nil && &m.walker == tw && (tw.walking || len(tw.path) > 0) &&
		m.intent&(gamestate.IntentMelee|gamestate.IntentShoot) == 0 && m.intent&gamestate.IntentWalk != 0 {
		m.resume, m.resumeSet = tw.goal, true
	}
	tw.halt()
	tw.startAct(gamestate.ActFlinch)
}

// alarmGroup is 0x004248A0: group g is alarmed against the party (+0x1D =
// 1), and every member without the melee intent (and not of kind 0x49) has
// its walk cut after the current step (+0x14 = 0xFF), so all of them decide
// again at once instead of finishing a calm walk first.
func (s *startScene) alarmGroup(g int) {
	if g <= 0 || g >= len(s.game.Groups) {
		return
	}
	s.game.Groups[g].Alarm = gamestate.AlarmAttack
	s.game.Groups[g].Enemy = 0
	for _, n := range s.npcs {
		if e := s.game.Entities[n.entity]; e.Group == g && n.intent&intentMelee == 0 && e.Flags4 != 0x49 {
			n.stopWalk()
		}
	}
}

// alarmCheck is 0x0041D0FC → 0x00412C30 for the creature groups on this map.
func (s *startScene) alarmCheck() {
	party := s.partyPositions()
	members := map[int][][2]int{}
	for _, n := range s.npcs {
		if n.mode == npcCreature && alive(&n.walker, &s.game.Entities[n.entity]) {
			members[n.group] = append(members[n.group], [2]int{n.cell.Row, n.cell.Col})
		}
	}
	for g, m := range members {
		gr := &s.game.Groups[g]
		gr.Alarm = gr.AlarmState(party, m)
		if gr.Alarm == gamestate.AlarmAttack {
			gr.Enemy = 0
		}
	}
}

// creatureTick is one game tick of creature n (0x004123BC's member loop).
func (s *startScene) creatureTick(n *npc) {
	e := &s.game.Entities[n.entity]
	if !alive(&n.walker, e) || n.act == gamestate.ActFlinch {
		return
	}
	g := &s.game.Groups[n.group]
	switch {
	case n.act == gamestate.ActAttack:
		if s.swing(&n.walker, e, n.target, n.targetCell) {
			n.act = 0 // 0x00410450; the intent stays, the next decision re-targets
			if tw, t := s.combatant(n.target); !alive(tw, t) {
				n.target = -1
			}
		}
		return
	case n.act == gamestate.ActShoot && n.intent&intentShoot != 0: // bow shot (0x00412AE5)
		if s.shotTick(&n.walker, e, n.targetCell) {
			n.act, n.intent = 0, n.intent&^intentShoot
		}
		return
	case n.act == gamestate.ActShoot && n.actStep > 0: // waking up (0x00412A36)
		if n.actStep == 1 && e.Animated() {
			s.playAt(gamestate.WakeSound(*e), n.cell) // 0x00412A60
		}
		if !n.advanceAct() {
			n.act = 0
		}
		return
	case n.walking || len(n.path) > 0:
		s.creatureChase(n, g)
		return
	}
	s.creatureDecide(n, e, g)
}

// creatureChase is 0x00411FD0 for a walking creature with the melee intent.
func (s *startScene) creatureChase(n *npc, g *gamestate.Group) {
	if n.intent&intentMelee == 0 {
		return
	}
	tw, t := s.combatant(n.target)
	switch {
	case !alive(tw, t):
		n.target = -1
		n.stopWalk()
	case tw.cell == n.targetCell:
	case !g.LeashArea().Contains(tw.cell.Row, tw.cell.Col):
		n.target, n.intent = -1, 0 // the target escaped the leash
		n.stopWalk()
	default:
		n.targetCell = tw.cell
		n.chase(tw.cell)
	}
}

// creatureDecide is 0x00412E00 for an idle creature (or a dormant one).
func (s *startScene) creatureDecide(n *npc, e *gamestate.Entity, g *gamestate.Group) {
	partyOK := len(s.livingMembers()) > 0
	tactic := gamestate.TacticOf(e.Flags4)
	switch g.Alarm {
	case gamestate.AlarmAttack:
		if !partyOK || g.Enemy != 0 {
			return
		}
		switch tactic {
		case gamestate.TacticStationary:
			s.stationarySwing(n)
			return
		case gamestate.TacticDormant:
			if n.act == gamestate.ActShoot {
				n.advanceAct() // wake up (0x0041309D)
				return
			}
		case gamestate.TacticNearest:
			if !e.Animated() && !e.InStance() {
				s.game.ReEquip(e)        // 0x004105A0
				gamestate.EnterStance(e) // humanoids take the stance first (0x00413108)
				n.look = lookOf(*e)
				return
			}
			if !e.Animated() && gamestate.HasBow(*e) && s.npcShoot(n) {
				return
			}
		}
		if t := s.creatureTarget(n, tactic, nil, true); t >= 0 {
			s.goFor(n, t)
		}
	case gamestate.AlarmReturn:
		if n.act == gamestate.ActShoot {
			return
		}
		leash := g.LeashArea()
		inLeash := func(c mapdata.Cell) bool { return leash.Contains(c.Row, c.Col) }
		if n.intent&intentMelee != 0 && partyOK && s.nearestMember(n.cell, inLeash, nil) >= 0 {
			if t := s.returnTarget(n, tactic, inLeash); t >= 0 && s.goFor(n, t) {
				return
			}
		}
		n.intent &^= intentMelee
		if area, _ := g.WanderArea(); !area.Contains(n.cell.Row, n.cell.Col) && s.aiBudget > 0 {
			s.aiBudget--
			blocked := func(r, c int) bool { return s.blocked(mapdata.Cell{Row: r, Col: c}) }
			if r, c, ok := gamestate.WanderTarget(area, n.rng, blocked); ok {
				n.intent = intentWalk
				n.walkTo(mapdata.Cell{Row: r, Col: c})
			}
		}
	}
}

// creatureTarget is the target selector of the attack state by tactic,
// with the party as the enemy group: assist (0x00420178) a group-mate's
// living target, unengaged and dormant (0x004203A0) the nearest member no
// group-mate attacks; with fallback, or for the other tactics, the nearest
// member (0x004202A8). inside, when set, limits the candidates. -1 = none.
func (s *startScene) creatureTarget(n *npc, tactic gamestate.Tactic, inside func(mapdata.Cell) bool, fallback bool) int {
	switch tactic {
	case gamestate.TacticAssist:
		for _, m := range s.npcs {
			if m == n || m.group != n.group || m.intent&intentMelee == 0 || !alive(&m.walker, &s.game.Entities[m.entity]) {
				continue
			}
			if tw, t := s.combatant(m.target); alive(tw, t) && t.Group == 0 && (inside == nil || inside(tw.cell)) {
				return m.target
			}
		}
	case gamestate.TacticUnengaged, gamestate.TacticDormant:
		if t := s.nearestMember(n.cell, inside, func(i int) bool { return s.mateTargets(n, i) }); t >= 0 {
			return t
		}
	default:
		return s.nearestMember(n.cell, inside, nil)
	}
	if !fallback {
		return -1
	}
	return s.nearestMember(n.cell, inside, nil)
}

// returnTarget is the kind switch of the return state (0x004132FE): who may
// keep fighting. The assist and unengaged selectors run without the
// nearest fallback, over the members inside the leash; the others (and
// stationary creatures) find nothing there.
func (s *startScene) returnTarget(n *npc, tactic gamestate.Tactic, inLeash func(mapdata.Cell) bool) int {
	switch tactic {
	case gamestate.TacticAssist, gamestate.TacticUnengaged, gamestate.TacticDormant:
		return s.creatureTarget(n, tactic, inLeash, false)
	}
	return -1
}

// mateTargets reports a living group-mate of n with the melee intent on
// target (0x00420178, 0x004203A0).
func (s *startScene) mateTargets(n *npc, target int) bool {
	for _, m := range s.npcs {
		if m != n && m.group == n.group && m.intent&intentMelee != 0 && m.target == target &&
			alive(&m.walker, &s.game.Entities[m.entity]) {
			return true
		}
	}
	return false
}

// goFor sets target and walks toward it (0x0041326B → 0x004142D4 flags 1),
// or swings at once when it is adjacent. It reports success.
func (s *startScene) goFor(n *npc, target int) bool {
	tw, t := s.combatant(target)
	if !alive(tw, t) {
		return false
	}
	n.target, n.targetCell, n.intent = target, tw.cell, intentMelee
	if adjacent(n.cell, tw.cell) {
		n.startAct(gamestate.ActAttack)
		return true
	}
	if s.aiBudget <= 0 {
		return true // decide again next tick
	}
	s.aiBudget--
	if !s.reachable(tw.cell) {
		return false // the search would fail: every cell next to the target is taken
	}
	n.chase(tw.cell)
	return len(n.path) > 0
}

// reachable reports a free cell next to c, without which a chase toward c
// can only fail; it saves the search (the budget is still spent).
func (s *startScene) reachable(c mapdata.Cell) bool {
	for _, n := range mapdata.Neighbors(c) {
		if !s.blocked(n.Cell) {
			return true
		}
	}
	return false
}

// stationarySwing is 0x00412F62: swing at an adjacent member of the enemy
// group (the first living member next to it), never move.
func (s *startScene) stationarySwing(n *npc) {
	for _, m := range s.livingMembers() {
		if !adjacent(n.cell, m.cell) {
			continue
		}
		n.target, n.targetCell, n.intent = s.party[m.k].Index, m.cell, intentRetaliate
		n.dir = facing(n.cell, m.cell)
		n.startAct(gamestate.ActAttack)
		return
	}
}

// gameOverDelay is how long the dead hero stays on screen before the menu
// returns (the game plays GAMEOVER.FLM instead, 0x004376CD).
const gameOverDelay = 120

// gameOver reports that the hero has been dead for gameOverDelay updates.
func (s *startScene) gameOver() bool {
	if len(s.party) == 0 || !s.party[0].Dead() {
		return false
	}
	if s.deadAt == 0 {
		s.deadAt = s.frame
	}
	return s.frame-s.deadAt >= gameOverDelay
}
