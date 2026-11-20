package lanserver

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/world"
)

// miscWorld is the world a misc item sees on the server (gamestate.World):
// map s's, from the member using the item (user, of player slot) — its
// "hero" (LAN_KONG.EXE's 0x0043F288 trains the user's own identification)
// and the enemy group nearest to it (0x00442658). There is no map jump
// and no lamp in the network game.
type miscWorld struct {
	gamestate.World
	s    *world.World
	user *world.Member
	slot int
}

// Hero is the member using the item.
func (m miscWorld) Hero() *gamestate.Entity {
	_, e := m.s.Member(m.user.Entity)
	return e
}

// EnemyGroup is the group of the enemy nearest to the user (an alarmed
// village only when its player is an aggressor there).
func (m miscWorld) EnemyGroup() int {
	i := m.s.NearestEnemy(m.user.Cell, m.slot)
	if i < 0 {
		return -1
	}
	return m.s.Game.Entities[i].Group
}

// Jump: no orbs or keys in the network game.
func (miscWorld) Jump(gamestate.MapExit) {}

// Kill: no skeleton lamp in the network game.
func (miscWorld) Kill(int, int) {}
