package game

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// sceneWorld is the scene's side of the misc items that reach beyond their
// user (gamestate.World, 0x00414E64): the party copies, the walkers'
// cells, deaths, walk orders and the map change.
type sceneWorld struct{ s *startScene }

func (w sceneWorld) Hero() *gamestate.Entity {
	if len(w.s.party) == 0 {
		return &w.s.game.Entities[w.s.game.Groups[0].First]
	}
	return &w.s.party[0]
}

func (w sceneWorld) Entity(i int) *gamestate.Entity {
	if _, e := w.s.combatant(i); e != nil {
		return e
	}
	return &w.s.game.Entities[i]
}

func (w sceneWorld) Cell(i int) (int, int) {
	if tw, _ := w.s.combatant(i); tw != nil {
		return tw.cell.Row, tw.cell.Col
	}
	e := w.s.game.Entities[i]
	return e.Row, e.Col
}

func (w sceneWorld) Alive(i int) bool {
	tw, e := w.s.combatant(i)
	return alive(tw, e)
}

// Kill is the lamp's death (0x004143DC(e, 3 or 9), then 0x00418510(e, 0,
// level)): no experience, the loot with the hero's level.
func (w sceneWorld) Kill(i, level int) {
	tw, e := w.s.combatant(i)
	if tw == nil {
		return
	}
	tw.halt()
	tw.startAct(gamestate.ActDeath)
	w.s.dropLoot(e, tw.cell, 0, level)
}

// Walk is 0x004142D4(e, row, col, 8): a walk order (+2 = 8).
func (w sceneWorld) Walk(i, row, col int) {
	if n, ok := w.s.byEntity[i]; ok {
		n.intent = intentWalk
		n.walkTo(mapdata.Cell{Row: row, Col: col})
	}
}

func (w sceneWorld) Blocked(row, col int) bool {
	return w.s.blocked(mapdata.Cell{Row: row, Col: col})
}

// EnemyGroup is 0x0041FD74 for the party: the group of the enemy nearest
// to the leader.
func (w sceneWorld) EnemyGroup() int {
	l := w.s.lead()
	if l == nil {
		return -1
	}
	i := w.s.nearestEnemy(l.cell)
	if i < 0 {
		return -1
	}
	return w.s.game.Entities[i].Group
}

// Jump is 0x004106E0: the map changes once the click is handled (the main
// loop's 0x00437799).
func (w sceneWorld) Jump(x gamestate.MapExit) {
	x.Index = -1
	w.s.jump = &x
}

// miscRequests carries out what the misc items asked of the scene during
// this frame's clicks: closing the window (the shovel) and a map change (the
// magic orbs, the keys). It reports a map change; the scene has then been
// replaced.
func (s *startScene) miscRequests() bool {
	if s.game.CloseWindow {
		s.game.CloseWindow = false
		if s.inv.open {
			s.inv.open = false // 0x00415504: window 0; the strip stays
			s.inv.quick = true
		}
	}
	if x := s.jump; x != nil {
		s.jump = nil
		s.changeMap(*x)
		return true
	}
	return false
}
