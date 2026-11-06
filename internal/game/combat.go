package game

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// Combat in the scene, one step per game tick; see docs/konung_combat.md.
//
// Melee, for the hero and creatures alike:
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
// The hero: a right click on a character in the stance sets the target
// (0x00423C8B); while idle without a target it strikes back at a neighbour
// attacking it (0x0041EC0C). Its death ends the game (gameOver).
//
// Creatures (AI 0x004123BC): every AlarmCheckTicks the group's alarm is
// recomputed (0x00412C30). Idle members then decide by the alarm
// (0x00412E00): calm → stay; attack → by gamestate.TacticOf go for the
// party (the port's party is the hero, so every selector picks the hero);
// return → keep fighting while the hero is inside the leash rectangle and a
// selector allows it, else walk back into the territory (0x004132FE). A
// chase ends when the target leaves the leash rectangle (0x00411FD0).
//
// Not ported: bows (humanoid creatures with a bow shoot instead, action 4),
// village anger (settlement +0x4B2 |= 0x15) and the village alarm AI, loot
// (0x00418510), kind 0x49's retreat after a blow (0x004101E4), kinds
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

// attackOrder is the A key / HUD button 5 (0x00421698): return a held item,
// press the button, and give every living party member the melee intent
// without a target, entering the stance. The planner then picks the nearest
// enemy (nearestEnemy). With the bow active the game sets the shoot intent
// (+2 = 0x44) instead (heroShoot).
func (s *startScene) attackOrder(hero *gamestate.Entity) {
	s.hand.Return(hero)
	s.hud.press(hudAttackButton, hudAttackImage)
	if hero.Dead() || s.hero.act == gamestate.ActDeath {
		return
	}
	if !hero.InStance() {
		gamestate.ToggleStance([]*gamestate.Entity{hero})
	}
	s.endFight()
	if hero.WeaponMode == gamestate.WeaponBow {
		s.shoot = true // +2 = 0x44: the planner shoots or drops the order
		return
	}
	s.hero.path = s.hero.path[:min(len(s.hero.path), 1)] // finish the step in progress
	s.pickIntent = false
	s.melee = true
}

// gatherOrder is the F key / HUD button 4 «Все ко мне» (0x004226C0): return a
// held item, press the button, and set every living party member other than
// the hero to +2 = 0x60 (idle, following the leader through 0x0041F5D8). The
// port's party is the hero alone, so only the first two steps show; the
// hero's own intents are left alone.
func (s *startScene) gatherOrder(hero *gamestate.Entity) {
	s.hand.Return(hero)
	s.hud.press(hudGatherButton, hudGatherImage)
}

// nearestEnemy is 0x0041E384's search for a new target (0x0041FD74 for the
// party, then 0x004203A0 / 0x004202A8): among groups other than the party on
// this map that are not villages (unless alarmed) and not of kind 0x53, the
// living member inside the attack range closest to the hero (gamestate.Dist).
// -1 when there is none.
func (s *startScene) nearestEnemy() int {
	h := s.hero.cell
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

// startFight makes the hero attack entity target (right click in stance).
func (s *startScene) startFight(target int) {
	s.melee = true
	s.target = target
	s.pickIntent = false
	if tw, _ := s.combatant(target); tw != nil {
		s.targetCell = tw.cell
	}
	s.hero.stopAt = func(c mapdata.Cell) bool {
		tw, _ := s.combatant(s.target)
		return tw != nil && c == tw.cell
	}
}

// endFight drops the hero's target and intent (a new walk order, the target
// dead or gone).
func (s *startScene) endFight() {
	s.target, s.melee, s.shoot = -1, false, false
	if s.hero != nil {
		s.hero.stopAt = nil
		if s.hero.act == gamestate.ActAttack {
			s.hero.act = 0
		}
	}
}

// setStopAt makes a creature's walks end before its target's cell.
func (s *startScene) setStopAt(n *npc) {
	n.stopAt = func(c mapdata.Cell) bool {
		tw, _ := s.combatant(n.target)
		return n.intent&intentMelee != 0 && tw != nil && c == tw.cell
	}
}

// combatant returns the walker and live entity record of entity i on this
// map: the hero's party copy, or the GAME.N record of anyone else.
func (s *startScene) combatant(i int) (*walker, *gamestate.Entity) {
	if i < 0 {
		return nil, nil
	}
	if s.hero != nil && len(s.party) > 0 && s.party[0].Index == i {
		return s.hero, &s.party[0]
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
	if s.hero != nil && len(s.party) > 0 {
		s.heroTick()
	}
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
	if s.hero != nil && len(s.party) > 0 {
		s.actTick(s.hero, &s.party[0], nil)
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
	aw.startAct(gamestate.ActAttack)
	return false
}

// actTick plays hit reactions and deaths (n is nil for the hero).
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

// flinch starts the hit reaction (type 2/8) of a character that isn't
// walking.
func (s *startScene) flinch(tw *walker) {
	if !tw.walking {
		tw.path = nil
		tw.startAct(gamestate.ActFlinch)
	}
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
	var party [][2]int
	if s.hero != nil && len(s.party) > 0 && alive(s.hero, &s.party[0]) {
		party = append(party, [2]int{s.hero.cell.Row, s.hero.cell.Col})
	}
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

// heroTick is the hero's part of a game tick: its order (see startFight) or,
// without one, retaliation.
func (s *startScene) heroTick() {
	h, hero := s.hero, &s.party[0]
	if !alive(h, hero) || h.act == gamestate.ActFlinch {
		return
	}
	if s.shoot {
		s.heroShoot()
		return
	}
	if s.target >= 0 {
		if tw, t := s.combatant(s.target); !alive(tw, t) {
			s.target = -1 // 0x0041E384: look for another enemy
			if h.act == gamestate.ActAttack {
				h.act = 0
			}
		}
	}
	if s.target < 0 {
		if !s.melee {
			s.retaliate()
			return
		}
		if h.walking {
			return // the step in progress ends first
		}
		t := s.nearestEnemy()
		if t < 0 {
			s.endFight() // no enemy left: the intent ends (0x0041E453)
			return
		}
		s.startFight(t)
	}
	tw, t := s.combatant(s.target)
	switch {
	case h.act == gamestate.ActAttack:
		if s.swing(h, hero, s.target, s.targetCell) {
			h.act = 0 // killed or moved: next tick finds a new target or approaches again (0x0041F258)
			if !alive(tw, t) {
				s.target = -1
			}
		}
	case h.walking || len(h.path) > 0:
		if tw.cell != s.targetCell {
			s.targetCell = tw.cell
			h.chase(tw.cell)
		}
	default: // idle with the intent: the planner 0x0041ED8C
		s.targetCell = tw.cell
		if adjacent(h.cell, tw.cell) {
			gamestate.EnterStance(hero) // the swing enters the stance (0x004149F5)
			h.look = lookOf(*hero)
			h.startAct(gamestate.ActAttack)
			return
		}
		h.chase(tw.cell)
		if len(h.path) == 0 {
			s.endFight() // unreachable
		}
	}
}

// retaliate is 0x0041EC0C for the idle hero: a neighbour of another group
// going for the hero (melee intent, target = hero) is struck back.
func (s *startScene) retaliate() {
	h, hero := s.hero, &s.party[0]
	if h.walking || len(h.path) > 0 || h.act != 0 {
		return
	}
	for _, n := range s.npcs {
		if n.intent&intentMelee == 0 || n.target != hero.Index || !adjacent(h.cell, n.cell) ||
			!alive(&n.walker, &s.game.Entities[n.entity]) {
			continue
		}
		s.startFight(n.entity)
		s.targetCell = n.cell
		gamestate.EnterStance(hero)
		h.look = lookOf(*hero)
		h.dir = facing(h.cell, n.cell)
		h.startAct(gamestate.ActAttack)
		return
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
	heroOK := s.hero != nil && len(s.party) > 0 && alive(s.hero, &s.party[0])
	tactic := gamestate.TacticOf(e.Flags4)
	switch g.Alarm {
	case gamestate.AlarmAttack:
		if !heroOK || g.Enemy != 0 {
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
		s.goFor(n, s.party[0].Index)
	case gamestate.AlarmReturn:
		if n.act == gamestate.ActShoot {
			return
		}
		if n.intent&intentMelee != 0 && heroOK && g.LeashArea().Contains(s.hero.cell.Row, s.hero.cell.Col) {
			if s.returnTarget(n, tactic) && s.goFor(n, s.party[0].Index) {
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

// returnTarget is the kind switch of the return state (0x004132FE): who may
// keep fighting. With the hero as the only enemy, the assist and unengaged
// selectors find it unless (unengaged) a group-mate already goes for it;
// the others (and stationary creatures) find nothing there.
func (s *startScene) returnTarget(n *npc, tactic gamestate.Tactic) bool {
	switch tactic {
	case gamestate.TacticAssist:
		return s.mateTargets(n, s.party[0].Index)
	case gamestate.TacticUnengaged, gamestate.TacticDormant:
		return !s.mateTargets(n, s.party[0].Index)
	}
	return false
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

// stationarySwing is 0x00412F62: swing at an adjacent enemy, never move.
func (s *startScene) stationarySwing(n *npc) {
	h := s.hero
	if !adjacent(n.cell, h.cell) {
		return
	}
	n.target, n.targetCell, n.intent = s.party[0].Index, h.cell, intentRetaliate
	n.dir = facing(n.cell, h.cell)
	n.startAct(gamestate.ActAttack)
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
