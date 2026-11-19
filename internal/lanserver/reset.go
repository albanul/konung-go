package lanserver

import (
	"sort"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/world"
)

// The session's end builds the world anew (docs/konung_lan.md#session-end).

// Reset is the world part of a session's end (LAN_KONG.EXE: 0x00434438
// saves every hero in the world and its items, 0x0044687C reads GAME.RES
// again, and the next session's start 0x00448BB8 gives them their items back
// and puts them on their start):
//
//   - GAME.RES again: the items, the groups (the players' with their hero
//     alone: hirelings are gone, back in their villages as GAME.RES has
//     them), the piles (dropped items vanish, herbs and nut-flowers are
//     back), the entities 16–319 (creatures killed are back, villagers as
//     at the start) and the settlements (anger, aggressors, buildings);
//     every map's world is built again (the map entry: creatures placed in
//     their territory).
//   - The shops: every stock emptied (0x00446AD1), the merchants of the
//     heroes in the world restocked (0x00446B45); the loans as GAME.RES has
//     them.
//   - Every hero in the world keeps its values and its items as saved
//     (0x00444918) and loaded back (0x0044444C, gamestate.LANKeepsOnLoad:
//     amulet parts do not come back); a dead hero's bag lay where it fell,
//     so it comes with what it wore. It stands on its type's start
//     (0x00448D76: the first exit leading to map model·3 + 1, placed by
//     0x00435830), alive with HP 1600 (0x00448E01), no poison (+0x2A), no
//     enchantments on itself (+0x3B/+0x3C = −1), out of the stance (+3 bit
//     4), idle (+0 = −1).
//   - The players' windows close (the hand goes back); their quest flags
//     stay (the original keeps them in each client).
//
// A hero whose map changed is reported (EventMapChange). Players not in the
// world are not touched (their items are in the store).
func (w *World) Reset() error {
	lw, err := gamestate.ParseLANWorld(w.gameRes)
	if err != nil {
		return err
	}
	type back struct {
		slot, from int
		e          gamestate.Entity
		items      []Item
	}
	var keep []back
	for slot, m := range w.players {
		w.closeItems(slot)
		e, ok := w.Hero(slot)
		if !ok {
			continue
		}
		var items []Item
		for _, it := range w.SavedItems(slot) {
			if gamestate.LANKeepsOnLoad(it.Slot == SlotBag, it.Category, it.Type) {
				items = append(items, it)
			}
		}
		keep = append(keep, back{slot: slot, from: m, e: e, items: items})
	}
	sort.Slice(keep, func(i, j int) bool { return keep[i].slot < keep[j].slot })

	ticks := [Maps + 1]int{}
	for n := 1; n <= Maps; n++ {
		ticks[n] = w.Maps[n].Tick
	}
	w.sessions++
	w.buildingsVersion++ // the buildings are GAME.RES's again
	w.Game, w.exits = lw.Game, lw.Game.Exits
	w.players, w.moving, w.hired = map[int]int{}, nil, map[int][]hireling{}
	for si := range gamestate.LANSettlements {
		for k := range world.LANPlayers {
			sh := lw.Shop(si, k) // the smith's job as GAME.RES has it
			sh.Healer, sh.Merchant, sh.Smith, sh.SmithTimer = [0x38]byte{}, [0x40]byte{}, [0x2E]byte{}, 0
			w.shops[si][k] = sh
		}
		w.loans[si] = lw.Loans(si)
	}
	for n := 1; n <= Maps; n++ {
		if err := w.buildMap(n, uint64(1+w.sessions)); err != nil {
			return err
		}
		w.Maps[n].Tick = ticks[n] // the time of day goes on
	}

	for _, b := range keep {
		e := b.e
		model := min(max(e.Model, 0), len(w.tmpl)-1)
		start, err := w.Start(model)
		if err != nil {
			return err
		}
		e.Equipment, e.Extra, e.Bag, e.Quiver = [6]int{}, [5]int{}, [32]int{}, 0
		e.HP, e.Poison = gamestate.HPFull, 0
		e.Enchants = [2]int{-1, -1}
		gamestate.LeaveStance(&e)
		e.Flags4 &^= 0x80
		e.Flags3 &^= 0x80
		e.Index, e.Group = b.slot, b.slot
		w.Game.Entities[b.slot] = e
		g := &w.Game.Groups[b.slot]
		g.First, g.Count = b.slot, 1
		mem := w.place(b.slot, start.To, mapdata.Cell{Row: start.Row, Col: start.Col}, start.Facing)
		w.LoadItems(b.slot, b.items)
		for si := range w.shops {
			w.restock(si, b.slot)
		}
		if start.To != b.from {
			w.events = append(w.events, Event{Kind: EventMapChange, Map: start.To, From: b.from, Entity: b.slot,
				Row: mem.Cell.Row, Col: mem.Cell.Col, Facing: start.Facing})
		}
	}
	return nil
}
