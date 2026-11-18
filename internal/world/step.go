package world

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// storeWorld writes the scene's runtime state back into s.Game: the party
// (cell, facing, +2 intents, stance, weapon, inventory) and every other
// character's cell and facing.
// The game keeps all characters in one global array (0x007AB378) and runs
// the AI only for the current map, so a map looks as it was left on the
// next visit; the map loader (0x0043C612) puts them back on their stored
// cells, idle.
func (s *World) StoreWorld() {
	for _, m := range s.Members {
		if m == nil {
			continue
		}
		e := &s.Party[m.K]
		e.Row, e.Col, e.Variant = m.Cell.Row, m.Cell.Col, int(m.Dir)
		e.Flags2 = m.Intent
	}
	for _, e := range s.Party {
		s.Game.Entities[e.Index] = e
	}
	for _, n := range s.NPCs {
		e := &s.Game.Entities[n.Entity]
		e.Row, e.Col, e.Variant = n.Cell.Row, n.Cell.Col, int(n.Dir)
	}
}

// updateNPCs runs the village AI and advances every NPC's walk by one game
// tick, like 0x00421DA4 in the game loop; at most VillagePathBudget
// villagers may start a walk per tick, served round-robin (0x004116CC). A
// creature with a walk throttle +0x11 = m walks only on ticks where
// time & m == m (0x0041271A).
func (s *World) UpdateNPCs() {
	if len(s.NPCs) > 0 {
		budget := gamestate.VillagePathBudget
		for i := range s.NPCs {
			k := (s.AINext + i) % len(s.NPCs)
			if n := s.NPCs[k]; n.Mode == NPCVillager && !s.Fighting(n) {
				apprentice := func() (gamestate.Post, bool) { return s.Game.ApprenticePost(n.Entity) }
				if used := n.VillageTick(budget, apprentice); used > 0 {
					budget -= used
					s.AINext = k + 1
				}
			}
		}
	}
	for _, n := range s.NPCs {
		if n.Act != 0 {
			continue
		}
		if m := s.Game.Entities[n.Entity].Throttle; n.Mode == NPCCreature && m != 0 && s.GameTime()&m != m {
			continue
		}
		n.Update()
	}
}

// blocked reports cells a character cannot path through: walk-grid
// obstacles, cells outside the map, and cells other characters hold.
func (s *World) Blocked(c mapdata.Cell) bool {
	if s.Walk.Blocked(c.Row, c.Col) || s.Occ[c] {
		return true
	}
	x, y := mapdata.CellCenter(c)
	return x < 0 || y < 0 || x >= s.WorldW || y >= s.WorldH
}

// step advances the world by one game tick (60 ms): characters, the
// pick-up on arrival, map exits, pile respawn and the AI.
func (s *World) Step() {
	s.Tick++
	for _, m := range s.Members {
		if m == nil {
			continue
		}
		moving := m.Walking || len(m.Path) > 0
		if m.Act == 0 {
			m.Update()
		}
		stopped := !m.Walking && len(m.Path) == 0
		if moving && stopped && m.Cell != m.Goal && m.Intent&gamestate.IntentWalk != 0 {
			// 0x0042218D: the path ran out short of the goal (31 steps,
			// mapdata.PathSteps) or a step was blocked; with the walk bit
			// the planner (0x0041ED8C) searches again. If that fails the
			// walk ends there, without a pick-up or an exit.
			if p := mapdata.FindPath(m.Cell, m.Goal, m.Blocked); len(p) > 0 {
				m.Path = p
				continue
			}
		}
		atGoal := m.Cell == m.Goal
		switch {
		case m.PickIntent && stopped:
			m.PickIntent = false
			if atGoal {
				s.Arrive(m)
			}
		case moving && stopped && atGoal:
			// 0x00422346: a member's walk that ends at its goal without the
			// pick-up intent on an exit cell takes the exit (0x00421BB8). A
			// village's exit needs its bridge (ExitOpen, 0x00421C82).
			if x, ok := s.Game.ExitAt(s.MapNumber, m.Cell.Row, m.Cell.Col); ok && s.Game.ExitOpen(x) {
				if s.LAN {
					if s.Hooks.MemberExit != nil {
						s.Hooks.MemberExit(m, x) // this hero alone; the runner moves it after the step
					}
					continue
				}
				s.changeMap(x)
				return
			}
		}
	}
	s.ActivatePiles()
	s.CombatTick()
	s.animate()
	s.UpdateNPCs()
	for _, m := range s.Members {
		if m != nil {
			s.IdleLoop(&m.Walker, s.Party[m.K].Dead())
		}
	}
	for _, n := range s.NPCs {
		s.IdleLoop(&n.Walker, s.Game.Entities[n.Entity].Dead())
	}
	s.afterStep()
}

// idleLoop is the idle case of the action tables (party 0x004223AD,
// creatures 0x0041291C) for a walker that is not walking or acting: the
// idle action starts at frame 0 (0x00410450 → 0x004143DC), advances one
// frame per tick (0x00413DE4) and starts over after its last frame. Every
// start rolls the idle sound (0x004143DC: human voice on rand() % 100 >
// 95; animated models their own rule).
func (s *World) IdleLoop(w *Walker, dead bool) {
	if dead || w.Walking || len(w.Path) > 0 || w.Act != 0 {
		w.Idle = -1
		return
	}
	if w.Idle >= 0 && w.Idle+1 < w.FrameCount(0) {
		w.Idle++
		return
	}
	w.Idle = 0
	s.actionSound(w, gamestate.ActIdle)
}

// Leave is the world's part of map exit x to another map (0x00421CCD,
// then the main loop's map change at 0x004377C6): the characters go back
// into the game (StoreWorld), the leave cleanup runs while the party is
// still on the map it leaves (so dead party members are removed too), the
// selection and the character window's character are renumbered, and the
// party moves to the exit's target. The caller then builds the target map's
// world; undo restores the groups, entities and settlements if that fails
// (items freed by the cleanup stay freed).
func (s *World) Leave(x gamestate.MapExit) (undo func()) {
	s.StoreWorld()
	groups := append([]gamestate.Group(nil), s.Game.Groups...)
	entities := append([]gamestate.Entity(nil), s.Game.Entities...)
	settlements := append([]gamestate.Settlement(nil), s.Game.Settlements...)
	g0 := s.Game.Groups[0]
	curEntity := s.Current().Index
	var kept []int // the party's entities that survive the cleanup, in order
	for k := 0; k < g0.Count; k++ {
		if !s.Game.Entities[g0.First+k].Dead() {
			kept = append(kept, g0.First+k)
		}
	}
	s.Game.LeaveMap(s.MapNumber) // 0x00436AD0: the dead of the map left are removed
	// The survivors moved down to fill the gaps: renumber the selection and
	// the character window's character.
	moved := map[int]int{}
	for k, i := range kept {
		moved[i] = g0.First + k
	}
	var sel []int
	for _, i := range s.Sel {
		if j, ok := moved[i]; ok {
			sel = append(sel, j)
		}
	}
	s.Sel, s.Cur = sel, 0
	if j, ok := moved[curEntity]; ok {
		s.Cur = j - g0.First
	}
	s.Game.TakeExit(x)
	return func() {
		s.Game.Groups, s.Game.Entities, s.Game.Settlements = groups, entities, settlements
	}
}
