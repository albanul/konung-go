package game

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// Melee (stage 1 of combat): the hero attacks a character picked with a
// right click in the combat stance. The game's flow, per game tick:
//
//   - Right click on a character in the stance (0x00423C8B): entity +6 =
//     target, +2 = 0x41 (melee intent).
//   - Idle with the intent: the planner 0x0041ED8C → 0x0041E384 keeps the
//     target's cell in +0x0C/+0x0E and paths there.
//   - Walking: a step into the target's cell becomes the swing instead
//     (0x004149B0, 0x00411758: only the target, in another group, counts).
//     If the target moves, the path is redone (0x0041F258).
//   - Swing, action type 5 (0x00421E8C): the attack stops when the target
//     left the cell or died (0x0041F258); the attacker turns one octant per
//     tick until it faces the target (0x0043976C, 0x004391BC); at step 2 the
//     target's group is alarmed (0x004248A0, not ported yet); after the last
//     step the blow lands (gamestate.Strike) and the swing restarts. A kill
//     plays the target's death and returns the attacker to idle (0x00414AB4);
//     a heavy hit makes the target flinch (type 2).
//   - Death (type 3, 0x00422447): at step count−2 the victim is marked dead
//     (+4 bit 0x80) and frees its cell; the last step stays.
//
// Not ported yet (stage 2): the target fighting back, creature alarm,
// village anger (settlement +0x4B2 |= 0x15), loot (0x00418510).

// startFight makes the hero attack entity target (right click in stance).
func (s *startScene) startFight(target int) {
	s.target = target
	s.pickIntent = false
	if tw := s.walkerOf(target); tw != nil {
		s.targetCell = tw.cell
	}
	s.hero.stopAt = func(c mapdata.Cell) bool {
		tw := s.walkerOf(s.target)
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

// walkerOf returns the walker of a non-party entity on this map, or nil.
func (s *startScene) walkerOf(entity int) *walker {
	if n, ok := s.byEntity[entity]; ok {
		return &n.walker
	}
	return nil
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

// combatTick runs once per game tick: the hero's attack, then the other
// characters' hit reactions and deaths.
func (s *startScene) combatTick() {
	if s.hero != nil && s.target >= 0 && len(s.party) > 0 {
		s.heroFight()
	}
	for _, n := range s.npcs {
		switch n.act {
		case gamestate.ActFlinch:
			if !n.advanceAct() {
				n.act = 0 // 0x00414AB4: back to idle
			}
		case gamestate.ActDeath:
			e := &s.game.Entities[n.entity]
			if !e.Dead() && n.actStep >= n.actSteps()-2 {
				e.Flags4 |= 0x80 // 0x0042248F
				delete(s.occ, n.cell)
				n.mode = npcStill
			}
			n.advanceAct()
		}
	}
}

// heroFight is one tick of the hero's melee intent (see the file comment).
func (s *startScene) heroFight() {
	h := s.hero
	hero := &s.party[0]
	t := &s.game.Entities[s.target]
	tw := s.walkerOf(s.target)
	if tw == nil || t.Dead() || tw.act == gamestate.ActDeath || hero.Dead() {
		s.endFight()
		return
	}
	switch {
	case h.act == gamestate.ActAttack:
		if tw.cell != s.targetCell { // 0x0041F258: the target moved
			h.act = 0
			return
		}
		if d := facing(h.cell, tw.cell); h.dir != d {
			h.turnToward(d)
			return
		}
		if h.advanceAct() {
			return
		}
		hero.Variant, t.Variant = int(h.dir), int(tw.dir)
		switch r, _ := s.game.Strike(hero, t, s.tables, s.rng.IntN); r {
		case gamestate.StrikeKill:
			s.game.Kill(hero, t, s.tables)
			tw.path, tw.walking = nil, false
			tw.startAct(gamestate.ActDeath)
			s.endFight()
			return
		case gamestate.StrikeHeavy:
			if !tw.walking {
				tw.startAct(gamestate.ActFlinch)
			}
		}
		h.look = lookOf(*hero) // a worn-out weapon or armour disappears
		h.startAct(gamestate.ActAttack)
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
