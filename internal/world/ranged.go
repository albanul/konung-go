package world

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// Bows (docs/konung_combat.md#bows-verified): the shoot intent (+2 bit
// 0x04), the line-of-fire test, the shot (action 4) that spawns an arrow,
// and the arrows' flight and hits, including a settlement building as the
// target of a party member (incendiary arrows, member.BurnTarget).

// lineOfFire is 0x0041130C for shooter w at target cell to.
func (s *World) LineOfFire(w *Walker, e *gamestate.Entity, to mapdata.Cell) int {
	bow, ok := s.Game.Item(e.Equipment[gamestate.WeaponBow])
	if !ok {
		return 0
	}
	occupied := s.Occupants()
	return gamestate.LineOfFire(w.Cell, int(w.Dir), to, s.Tables.GearRange(bow.Type),
		func(c mapdata.Cell) bool { return s.Walk.ArrowBlocked(c.Row, c.Col) },
		func(c mapdata.Cell) bool { _, ok := occupied[c]; return ok })
}

// occupants maps the cells characters stand on to their entity (the grid's
// occupant bits; the dead are cleared from it).
func (s *World) Occupants() map[mapdata.Cell]int {
	m := map[mapdata.Cell]int{}
	for _, pm := range s.Members {
		if pm != nil && !s.Party[pm.K].Dead() {
			m[pm.Cell] = s.Party[pm.K].Index
		}
	}
	for _, n := range s.NPCs {
		if !s.Game.Entities[n.Entity].Dead() {
			m[n.Cell] = n.Entity
		}
	}
	return m
}

// memberShoot is the planner's bow branch (0x0041ED8C) for member m, and
// its shot action (0x0042255E). A shooter never walks closer. A dead target
// or no enemy in sight ends the order; out of line (0) the target is
// dropped, and the hero also drops the order (0x0041EFC8), others look
// again next tick; too close (< 0) ends it.
func (s *World) MemberShoot(m *Member) {
	e := &s.Party[m.K]
	if m.Act == gamestate.ActShoot {
		if s.ShotTick(&m.Walker, e, m.TargetCell) {
			m.Act = 0
			m.Look = LookOf(*e) // the bow may have broken
		}
		return
	}
	if m.Walking || len(m.Path) > 0 {
		return
	}
	if !gamestate.CanShoot(*e) {
		s.EndFight(m)
		return
	}
	if m.BurnTarget != 0 {
		s.ShootBuilding(m, e)
		return
	}
	if m.Target < 0 {
		m.Target = s.FirstShootable(&m.Walker, e)
		if m.Target < 0 {
			s.EndFight(m)
			return
		}
	}
	tw, t := s.Combatant(m.Target)
	if !Alive(tw, t) {
		s.EndFight(m)
		return
	}
	m.TargetCell = tw.Cell
	switch r := s.LineOfFire(&m.Walker, e, tw.Cell); {
	case r > 0:
		m.StartAct(gamestate.ActShoot)
	case r == 0 && m.K != 0:
		m.Target = -1
	default:
		s.EndFight(m)
	}
}

// shootBuilding is the planner's building branch (0x0041EF10): without a
// settlement on the map, or when the line of fire misses the building, the
// order ends; a hit moves the aim to the building cell the ray met and
// shoots.
func (s *World) ShootBuilding(m *Member, e *gamestate.Entity) {
	hit := s.buildingHit(m.BurnTarget - 1)
	bow, ok := s.Game.Item(e.Equipment[gamestate.WeaponBow])
	if s.Game.SettlementOnMap(s.MapNumber) == nil || hit == nil || !ok {
		s.EndFight(m)
		return
	}
	occupied := s.Occupants()
	r, aim := gamestate.LineOfFireBuilding(m.Cell, int(m.Dir), m.TargetCell, s.Tables.GearRange(bow.Type),
		func(c mapdata.Cell) bool { return s.Walk.ArrowBlocked(c.Row, c.Col) },
		func(c mapdata.Cell) bool { _, ok := occupied[c]; return ok },
		hit)
	if r <= 0 {
		s.EndFight(m)
		return
	}
	m.TargetCell = aim
	m.StartAct(gamestate.ActShoot)
}

// firstShootable is 0x00410E84: the enemy group (0x0041FD74, here the group
// of the enemy nearest to the shooter) and its first living member with a
// clear shot.
func (s *World) FirstShootable(w *Walker, e *gamestate.Entity) int {
	near := s.NearestEnemy(w.Cell)
	if near < 0 {
		return -1
	}
	gr := s.Game.Groups[s.Game.Entities[near].Group]
	for k := 0; k < gr.Count; k++ {
		i := gr.First + k
		if tw, t := s.Combatant(i); Alive(tw, t) && s.LineOfFire(w, e, tw.Cell) > 0 {
			return i
		}
	}
	return -1
}

// npcShoot starts a shot of creature or villager n at the party when a
// line is clear (0x00410E84 with the party as the enemy group: its first
// living member with a clear line). The caller checks gamestate.HasBow;
// the bow becomes the active weapon here. Out of range (beyond the bow's
// reach, or closer than 3) the caller falls back to melee.
func (s *World) NpcShoot(n *NPC) bool {
	e := &s.Game.Entities[n.Entity]
	e.WeaponMode = gamestate.WeaponBow // +0x12 = 1 before the shot test
	for _, m := range s.LivingMembers() {
		if s.LineOfFire(&n.Walker, e, m.Cell) <= 0 {
			continue
		}
		n.Look = LookOf(*e)
		n.Target, n.TargetCell, n.Intent = s.Party[m.K].Index, m.Cell, IntentShoot
		n.StartAct(gamestate.ActShoot)
		return true
	}
	return false
}

// shotTick is one tick of action 4 (0x0042255E): turn toward the target at
// step 0, release the arrow at step count − 6 (gamestate.Shoot), and report
// the end of the animation.
func (s *World) ShotTick(w *Walker, e *gamestate.Entity, to mapdata.Cell) bool {
	if w.ActStep == 0 {
		if d := Facing(w.Cell, to); w.Dir != d {
			w.TurnToward(d)
			return false
		}
	}
	if w.ActStep == w.ActSteps()-gamestate.ShotStep {
		if a, ok := s.Game.Shoot(e, w.Cell, int(w.Dir), to, s.Tables, len(s.Arrows)); ok {
			s.Arrows = append(s.Arrows, a)
		}
	}
	return !w.AdvanceAct()
}

// flyArrows is 0x00421170: every arrow flies one tick; the first character
// on its way (other than the shooter) is provoked and, if alive, hit.
func (s *World) FlyArrows() {
	if len(s.Arrows) == 0 {
		return
	}
	occupied := s.Occupants()
	kept := s.Arrows[:0]
	for i := range s.Arrows {
		a := &s.Arrows[i]
		hit, gone := a.Fly(
			func(c mapdata.Cell) bool {
				if !s.Walk.ArrowBlocked(c.Row, c.Col) {
					return false
				}
				s.ArrowOnBuilding(a)
				return true
			},
			func(c mapdata.Cell) int {
				v, ok := occupied[c]
				if !ok || v == a.Shooter {
					return -1
				}
				vw, ve := s.Combatant(v)
				if !s.PlayerGroup(ve.Group) {
					s.Provoke(ve.Group) // 0x004214EE
				}
				if !Alive(vw, ve) {
					return -1 // the arrow passes the dying
				}
				return v
			})
		if hit >= 0 {
			s.ArrowHit(*a, hit)
		}
		if !gone {
			kept = append(kept, *a)
		}
	}
	s.Arrows = kept
}

// arrowOnBuilding is the blocked-cell branch of the flight (0x0042135D):
// when the shooter's target is a building and the arrow point is on its
// image, that building may catch fire and its village turns hostile and is
// alarmed (gamestate.ArrowOnBuilding).
func (s *World) ArrowOnBuilding(a *gamestate.Arrow) {
	m, _ := s.PartyMember(a.Shooter)
	if m == nil || m.BurnTarget == 0 {
		return
	}
	st := s.Game.SettlementOnMap(s.MapNumber)
	hit := s.buildingHit(m.BurnTarget - 1)
	if st == nil || hit == nil {
		return
	}
	if x, y := gamestate.ArrowPoint(a.X, a.Y); !hit(x, y) {
		return
	}
	if s.Game.ArrowOnBuilding(st, m.BurnTarget-1, s.Rng.IntN) {
		s.Provoke(st.Group)
	}
}

// arrowHit applies an arrow to entity v (0x0041ACE0, 0x0041C29C).
func (s *World) ArrowHit(a gamestate.Arrow, v int) {
	vw, ve := s.Combatant(v)
	_, shooter := s.Combatant(a.Shooter)
	if shooter == nil {
		shooter = &s.Game.Entities[a.Shooter]
	}
	if s.protected(shooter, ve) {
		return // a teammate the server protects: no harm
	}
	attack := s.Game.ArrowAttack(a, *shooter, *ve, s.Tables)
	switch r, _ := s.Game.ArrowDamage(ve, a, attack, s.Tables, s.Rng.IntN); r {
	case gamestate.StrikeKill:
		s.Kill(shooter, vw, ve, true)
	case gamestate.StrikeHeavy:
		s.Flinch(vw)
	}
}
