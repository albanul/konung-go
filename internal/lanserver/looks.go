package lanserver

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/lanfight"
	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

// equipLook is record e's equipment as the players draw it.
type equipLook = [netplay.EquipSlots]netplay.EquipLook

// baseLooks are the world's characters' equipment as GAME.RES has it, the
// players' copies too (by entity).
func baseLooks(g *gamestate.Game) []equipLook {
	out := make([]equipLook, len(g.Entities))
	for i, e := range g.Entities {
		out[i] = lanfight.LookOf(g, e).Equip
	}
	return out
}

// Looks are the world's characters on map m (villagers, creatures,
// hirelings: entities LANPlayers and up) whose equipment differs from what
// the player was told (sent; an entity not in it: its GAME.RES look), for
// MsgLooks; sent is updated. A hireling's equipment changes in its
// player's character window, a villager's by the smith's work. The
// player's own hirelings are left out: its client has their items
// (MsgInventory), and once one leaves the party it is told again.
func (w *World) Looks(slot, m int, sent map[int]equipLook) map[int]equipLook {
	if m < 1 || m > Maps {
		return nil
	}
	s := w.Maps[m]
	var out map[int]equipLook
	check := func(e *gamestate.Entity) {
		i := e.Index
		if i < len(w.base) && i >= world.LANPlayers && e.Group != slot {
			now := lanfight.LookOf(w.Game, *e).Equip
			was, ok := sent[i]
			if !ok {
				was = w.base[i]
			}
			if now != was {
				if out == nil {
					out = map[int]equipLook{}
				}
				out[i], sent[i] = now, now
			}
		}
	}
	for _, n := range s.NPCs {
		check(&w.Game.Entities[n.Entity])
	}
	for k := range s.Party {
		check(&s.Party[k])
	}
	return out
}
