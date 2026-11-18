package world

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// access is the world's side of the misc items that reach beyond their
// user (gamestate.World, 0x00414E64): the party copies, the walkers'
// cells, deaths, walk orders and the map change.
type access struct{ s *World }

func (w access) Hero() *gamestate.Entity {
	if len(w.s.Party) == 0 {
		return &w.s.Game.Entities[w.s.Game.Groups[0].First]
	}
	return &w.s.Party[0]
}

func (w access) Entity(i int) *gamestate.Entity {
	if _, e := w.s.Combatant(i); e != nil {
		return e
	}
	return &w.s.Game.Entities[i]
}

func (w access) Cell(i int) (int, int) {
	if tw, _ := w.s.Combatant(i); tw != nil {
		return tw.Cell.Row, tw.Cell.Col
	}
	e := w.s.Game.Entities[i]
	return e.Row, e.Col
}

func (w access) Alive(i int) bool {
	tw, e := w.s.Combatant(i)
	return Alive(tw, e)
}

// Kill is the lamp's death (0x004143DC(e, 3 or 9), then 0x00418510(e, 0,
// level)): no experience, the loot with the hero's level.
func (w access) Kill(i, level int) {
	tw, e := w.s.Combatant(i)
	if tw == nil {
		return
	}
	tw.Halt()
	tw.StartAct(gamestate.ActDeath)
	w.s.DropLoot(e, tw.Cell, 0, level)
}

// Walk is 0x004142D4(e, row, col, 8): a walk order (+2 = 8).
func (w access) Walk(i, row, col int) {
	if n, ok := w.s.ByEntity[i]; ok {
		n.Intent = IntentWalk
		n.WalkTo(mapdata.Cell{Row: row, Col: col})
	}
}

func (w access) Blocked(row, col int) bool {
	return w.s.Blocked(mapdata.Cell{Row: row, Col: col})
}

// EnemyGroup is 0x0041FD74 for the party: the group of the enemy nearest
// to the leader.
func (w access) EnemyGroup() int {
	l := w.s.Lead()
	if l == nil {
		return -1
	}
	i := w.s.NearestEnemy(l.Cell)
	if i < 0 {
		return -1
	}
	return w.s.Game.Entities[i].Group
}

// Jump is 0x004106E0: the map changes once the click is handled (the main
// loop's 0x00437799).
func (w access) Jump(x gamestate.MapExit) {
	x.Index = -1
	w.s.Jump = &x
}

// Access is the world as the misc items see it (gamestate.Game.World).
func (s *World) Access() gamestate.World { return access{s} }
