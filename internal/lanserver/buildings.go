package lanserver

import (
	"sort"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

// Village buildings for the players (docs/konung_lan.md#villages-on-the-server).
// LAN_KONG.EXE's settlement pass sends a building's new state to every
// player as it changes (0x0043AB04: message 0x5E with the map, the
// placement and the state, kept in the table 0x00C467B0 by map·1000 +
// placement). The port sends each player the buildings of the map it is
// on whose state differs from what its client has: GAME.RES's states (the
// client's world when it enters) and what was sent since, so a player that
// enters or changes map gets the buildings that changed before.

// buildingKey is a building by map and placement.
type buildingKey struct{ m, p int }

// buildingStates0 is GAME.RES's building states of map m.
func (w *World) buildingStates0(m int) map[int]int {
	if w.buildings0[m] == nil {
		lw, err := gamestate.ParseLANWorld(w.gameRes)
		if err != nil {
			return nil
		}
		for n := 1; n <= Maps; n++ {
			w.buildings0[n] = lw.Game.BuildingStates(n)
		}
	}
	return w.buildings0[m]
}

// buildingUpdates is what slot's client needs of map m's buildings, given
// sent (the states it was sent, updated here): EventBuilding events in
// placement order.
func (w *World) buildingUpdates(m int, sent map[buildingKey]int) []netplay.Event {
	cur := w.Game.BuildingStates(m)
	base := w.buildingStates0(m)
	var ps []int
	for p, st := range cur {
		was, ok := sent[buildingKey{m, p}]
		if !ok {
			was = base[p]
		}
		if st != was {
			ps = append(ps, p)
		}
	}
	sort.Ints(ps)
	var out []netplay.Event
	for _, p := range ps {
		sent[buildingKey{m, p}] = cur[p]
		out = append(out, netplay.Event{Kind: netplay.EventBuilding, Building: &netplay.Building{Placement: p, State: cur[p]}})
	}
	return out
}

// allyAttack is the attack order's village rule (0x004466FA, 0x004467C2):
// slot ordering an attack on t, another player's hero or hireling, on map
// m becomes an aggressor of m's village when t's player is of the
// village's nation and not an aggressor itself. The original checks it
// for a melee or bow attack order; the port for every attack order on a
// character (World.Right with a target, in the stance).
func (w *World) allyAttack(slot, m int, t *gamestate.Entity) {
	if t == nil || t.Group < 0 || t.Group >= world.LANPlayers || t.Group == slot {
		return
	}
	if _, ok := w.players[t.Group]; !ok {
		return
	}
	st := w.Game.SettlementOnMap(m)
	if st == nil || st.Aggressor(t.Group) {
		return
	}
	if _, me := w.Maps[m].Member(slot); me == nil || !me.InStance() {
		return
	}
	if w.Game.Entities[t.Group].Model == w.Game.Nation(st) {
		st.MarkAggressor(slot)
	}
}
