package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// Bows (docs/konung_combat.md#bows-verified): the shoot intent (+2 bit
// 0x04), the line-of-fire test, the shot (action 4) that spawns an arrow,
// and the arrows' flight and hits, including a settlement building as the
// hero's target (incendiary arrows, burnTarget).

const intentShoot = 0x04

// lineOfFire is 0x0041130C for shooter w at target cell to.
func (s *startScene) lineOfFire(w *walker, e *gamestate.Entity, to mapdata.Cell) int {
	bow, ok := s.game.Item(e.Equipment[gamestate.WeaponBow])
	if !ok {
		return 0
	}
	occupied := s.occupants()
	return gamestate.LineOfFire(w.cell, int(w.dir), to, s.tables.GearRange(bow.Type),
		func(c mapdata.Cell) bool { return s.walk.ArrowBlocked(c.Row, c.Col) },
		func(c mapdata.Cell) bool { _, ok := occupied[c]; return ok })
}

// occupants maps the cells characters stand on to their entity (the grid's
// occupant bits; the dead are cleared from it).
func (s *startScene) occupants() map[mapdata.Cell]int {
	m := map[mapdata.Cell]int{}
	if s.hero != nil && len(s.party) > 0 && !s.party[0].Dead() {
		m[s.hero.cell] = s.party[0].Index
	}
	for _, n := range s.npcs {
		if !s.game.Entities[n.entity].Dead() {
			m[n.cell] = n.entity
		}
	}
	return m
}

// heroShoot is the planner's bow branch (0x0041ED8C) for the hero, and its
// shot action (0x0042255E). The hero never walks closer; it drops the order
// when the target dies, is too close or out of sight.
func (s *startScene) heroShoot() {
	h, hero := s.hero, &s.party[0]
	if h.act == gamestate.ActShoot {
		if s.shotTick(h, hero, s.targetCell) {
			h.act = 0
			h.look = lookOf(*hero) // the bow may have broken
		}
		return
	}
	if h.walking || len(h.path) > 0 {
		return
	}
	if !gamestate.CanShoot(*hero) {
		s.endFight()
		return
	}
	if s.burnTarget != 0 {
		s.shootBuilding(h, hero)
		return
	}
	if s.target < 0 {
		s.target = s.firstShootable(h, hero)
		if s.target < 0 {
			s.endFight()
			return
		}
	}
	tw, t := s.combatant(s.target)
	if !alive(tw, t) {
		s.endFight()
		return
	}
	s.targetCell = tw.cell
	switch s.lineOfFire(h, hero, tw.cell) {
	case 1:
		h.startAct(gamestate.ActShoot)
	default: // too close, blocked or out of range: the hero drops the order
		s.endFight()
	}
}

// shootBuilding is the planner's building branch (0x0041EF10): without a
// settlement on the map, or when the line of fire misses the building, the
// order ends; a hit moves the aim to the building cell the ray met and
// shoots.
func (s *startScene) shootBuilding(h *walker, hero *gamestate.Entity) {
	o := s.buildingObject(s.burnTarget - 1)
	bow, ok := s.game.Item(hero.Equipment[gamestate.WeaponBow])
	if s.game.SettlementOnMap(s.mapNumber) == nil || o == nil || !ok {
		s.endFight()
		return
	}
	occupied := s.occupants()
	r, aim := gamestate.LineOfFireBuilding(h.cell, int(h.dir), s.targetCell, s.tables.GearRange(bow.Type),
		func(c mapdata.Cell) bool { return s.walk.ArrowBlocked(c.Row, c.Col) },
		func(c mapdata.Cell) bool { _, ok := occupied[c]; return ok },
		o.buildingHit)
	if r <= 0 {
		s.endFight()
		return
	}
	s.targetCell = aim
	h.startAct(gamestate.ActShoot)
}

// firstShootable is 0x00410E84: the enemy group (0x0041FD74, here the group
// of the nearest enemy) and its first living member with a clear shot.
func (s *startScene) firstShootable(w *walker, e *gamestate.Entity) int {
	near := s.nearestEnemy()
	if near < 0 {
		return -1
	}
	gr := s.game.Groups[s.game.Entities[near].Group]
	for k := 0; k < gr.Count; k++ {
		i := gr.First + k
		if tw, t := s.combatant(i); alive(tw, t) && s.lineOfFire(w, e, tw.cell) > 0 {
			return i
		}
	}
	return -1
}

// npcShoot starts a shot of creature or villager n at the hero when the line
// is clear (0x00410E84 with the party as the enemy group). The caller checks
// gamestate.HasBow; the bow becomes the active weapon here. Out of range
// (beyond the bow's reach, or closer than 3) the caller falls back to melee.
func (s *startScene) npcShoot(n *npc) bool {
	e := &s.game.Entities[n.entity]
	h := s.hero
	if h == nil || len(s.party) == 0 || !alive(h, &s.party[0]) {
		return false
	}
	e.WeaponMode = gamestate.WeaponBow // +0x12 = 1 before the shot test
	if s.lineOfFire(&n.walker, e, h.cell) <= 0 {
		return false
	}
	n.look = lookOf(*e)
	n.target, n.targetCell, n.intent = s.party[0].Index, h.cell, intentShoot
	n.startAct(gamestate.ActShoot)
	return true
}

// shotTick is one tick of action 4 (0x0042255E): turn toward the target at
// step 0, release the arrow at step count − 6 (gamestate.Shoot), and report
// the end of the animation.
func (s *startScene) shotTick(w *walker, e *gamestate.Entity, to mapdata.Cell) bool {
	if w.actStep == 0 {
		if d := facing(w.cell, to); w.dir != d {
			w.turnToward(d)
			return false
		}
	}
	if w.actStep == w.actSteps()-gamestate.ShotStep {
		if a, ok := s.game.Shoot(e, w.cell, int(w.dir), to, s.tables, len(s.arrows)); ok {
			s.arrows = append(s.arrows, a)
		}
	}
	return !w.advanceAct()
}

// flyArrows is 0x00421170: every arrow flies one tick; the first character
// on its way (other than the shooter) is provoked and, if alive, hit.
func (s *startScene) flyArrows() {
	if len(s.arrows) == 0 {
		return
	}
	occupied := s.occupants()
	kept := s.arrows[:0]
	for i := range s.arrows {
		a := &s.arrows[i]
		hit, gone := a.Fly(
			func(c mapdata.Cell) bool {
				if !s.walk.ArrowBlocked(c.Row, c.Col) {
					return false
				}
				s.arrowOnBuilding(a)
				return true
			},
			func(c mapdata.Cell) int {
				v, ok := occupied[c]
				if !ok || v == a.Shooter {
					return -1
				}
				vw, ve := s.combatant(v)
				if ve.Group != 0 {
					s.provoke(ve.Group) // 0x004214EE
				}
				if !alive(vw, ve) {
					return -1 // the arrow passes the dying
				}
				return v
			})
		if hit >= 0 {
			s.arrowHit(*a, hit)
		}
		if !gone {
			kept = append(kept, *a)
		}
	}
	s.arrows = kept
}

// arrowOnBuilding is the blocked-cell branch of the flight (0x0042135D):
// when the shooter's target is a building and the arrow point is on its
// image, that building may catch fire and its village turns hostile and is
// alarmed (gamestate.ArrowOnBuilding).
func (s *startScene) arrowOnBuilding(a *gamestate.Arrow) {
	if len(s.party) == 0 || a.Shooter != s.party[0].Index || s.burnTarget == 0 {
		return
	}
	st := s.game.SettlementOnMap(s.mapNumber)
	o := s.buildingObject(s.burnTarget - 1)
	if st == nil || o == nil {
		return
	}
	if x, y := gamestate.ArrowPoint(a.X, a.Y); !o.buildingHit(x, y) {
		return
	}
	if s.game.ArrowOnBuilding(st, s.burnTarget-1, s.rng.IntN) {
		s.provoke(st.Group)
	}
}

// arrowHit applies an arrow to entity v (0x0041ACE0, 0x0041C29C).
func (s *startScene) arrowHit(a gamestate.Arrow, v int) {
	vw, ve := s.combatant(v)
	_, shooter := s.combatant(a.Shooter)
	if shooter == nil {
		shooter = &s.game.Entities[a.Shooter]
	}
	attack := s.game.ArrowAttack(a, *shooter, *ve, s.tables)
	switch r, _ := s.game.ArrowDamage(ve, a, attack, s.tables, s.rng.IntN); r {
	case gamestate.StrikeKill:
		s.kill(shooter, vw, ve, true)
	case gamestate.StrikeHeavy:
		s.flinch(vw)
	}
}

// drawArrows draws the flying arrows (INTR slot 0xB8 + 2·facing + time&1,
// 0x00426F4D) over the characters.
func (s *startScene) drawArrows(dst *ebiten.Image) {
	for _, a := range s.arrows {
		img := s.hud.slot(gamestate.ArrowImage + 2*a.Facing + s.gameTime()&1)
		if img == nil {
			continue
		}
		dx, dy := a.SpriteOffset()
		drawAt(dst, img, int(a.X)+dx-s.camX, int(a.Y)+dy-s.camY)
	}
}
