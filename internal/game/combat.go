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

// creatureBudget is the path searches the creature AI may start per game
// tick (0x004116CC; the game shares 3 among all characters).
const creatureBudget = 3

// startFight makes the hero attack entity target (right click in stance).
func (s *startScene) startFight(target int) {
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
	s.target = -1
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
	if s.gameTime()%gamestate.AlarmCheckTicks == 0 {
		s.alarmCheck()
	}
	if s.hero != nil && len(s.party) > 0 {
		s.heroTick()
	}
	s.aiBudget = creatureBudget
	for _, n := range s.npcs {
		if n.mode == npcCreature {
			s.creatureTick(n)
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
		s.alarmGroup(t.Group) // 0x00421F64
	}
	if aw.advanceAct() {
		return false
	}
	a.Variant, t.Variant = int(aw.dir), int(tw.dir)
	switch r, _ := s.game.Strike(a, t, s.tables, s.rng.IntN); r {
	case gamestate.StrikeKill:
		s.game.Kill(a, t, s.tables)
		tw.halt()
		tw.startAct(gamestate.ActDeath)
		return true
	case gamestate.StrikeHeavy:
		if !tw.walking {
			tw.path = nil
			tw.startAct(gamestate.ActFlinch)
		}
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
	switch w.act {
	case gamestate.ActFlinch:
		if !w.advanceAct() {
			w.act = 0
			if creature {
				s.alarmGroup(n.group) // 0x00412A21
			}
		}
	case gamestate.ActDeath:
		if !e.Dead() && w.actStep >= w.actSteps()-2 {
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

// alarmGroup is 0x004248A0 for a creature group: alarmed, against the party.
func (s *startScene) alarmGroup(g int) {
	if g > 0 && g < len(s.game.Groups) && s.game.Groups[g].CreatureGroup() {
		s.game.Groups[g].Alarm = gamestate.AlarmAttack
		s.game.Groups[g].Enemy = 0
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
	if s.target < 0 {
		s.retaliate()
		return
	}
	tw, t := s.combatant(s.target)
	if !alive(tw, t) {
		s.endFight()
		return
	}
	switch {
	case h.act == gamestate.ActAttack:
		if s.swing(h, hero, s.target, s.targetCell) {
			if !alive(tw, t) {
				s.endFight()
			} else {
				h.act = 0 // the target moved: approach again (0x0041F258)
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
			hero.Flags3 |= 0x04 // the swing enters the stance (0x004149F5)
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
		hero.Flags3 |= 0x04
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
	case n.act == gamestate.ActShoot && n.actStep > 0: // waking up (0x00412A36)
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
				e.Flags3 |= 0x04 // humanoids take the stance first (0x00413108)
				n.look = lookOf(*e)
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
	n.chase(tw.cell)
	return len(n.path) > 0
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
