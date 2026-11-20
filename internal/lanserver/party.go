package lanserver

import (
	"slices"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/world"
)

// Hirelings (docs/konung_lan.md#hirelings): a player's hero hires plain
// villagers through their dialogue (gamestate.LANHireDialogue: the price,
// the party limit, script 291). LAN_KONG.EXE's server (0x0044BED7) moves
// the villager from its village group's member list into the player's
// group (its group byte +5 = the slot, +2 = 0x60, +0x9C = 0xFF) and tells
// the clients on the map; the port keeps the entity's index and makes it a
// member of the hero's map (world.World.Hire) that follows the hero, takes
// the player's orders when selected, goes with the hero through exits and
// jumps, and fights like a single-player party member.
//
// The original wipes the hero's group, hirelings included, when the hero
// leaves the world (0x00443C1C), and its session-end reset re-reads
// GAME.RES (0x0044687C), which brings the villagers back. The port's
// session end does the same (World.Reset); a leaving hero's living
// hirelings go back to their village at once (sendHome, a port rule: the
// original's wiped records stay empty until the reset); a dead hireling
// stays with the party until the hero leaves the map, then it is gone
// (dropHireling), as the single-player leave cleanup removes the party's
// dead.

// hireling is a hired villager and where it came from.
type hireling struct {
	entity int
	group  int // its village group
	home   int // its village's map
	cell   mapdata.Cell
	dir    int
}

// Party is slot's hirelings (entity indices, in hiring order).
func (w *World) Party(slot int) []int {
	var out []int
	for _, h := range w.hired[slot] {
		out = append(out, h.entity)
	}
	return out
}

// PartySize is slot's party: the hero and its hirelings (the player
// group's member count, 349).
func (w *World) PartySize(slot int) int { return 1 + len(w.hired[slot]) }

// hireling reports whether entity i is one of slot's hirelings.
func (w *World) hireling(slot, i int) bool {
	return slices.ContainsFunc(w.hired[slot], func(h hireling) bool { return h.entity == i })
}

// hire makes villager i on slot's hero's map its hireling (the dialogue's
// 291, after its price and the party limit).
func (w *World) hire(slot, i int) bool {
	m, ok := w.players[slot]
	if !ok || i < world.LANPlayers || i >= len(w.Game.Entities) {
		return false
	}
	s := w.Maps[m]
	n := s.ByEntity[i]
	e := w.Game.Entities[i]
	if n == nil || n.Mode != world.NPCVillager || e.Dead() {
		return false
	}
	h := hireling{entity: i, group: e.Group, home: m, cell: n.Cell, dir: int(n.Dir)}
	if s.Hire(i, slot, w.attach) == nil {
		return false
	}
	w.hired[slot] = append(w.hired[slot], h)
	return true
}

// moveParty takes slot's hirelings to the hero's map after the hero moved
// there (an exit, a jump, MoveTo): the living ones on cells around the
// hero, the dead dropped.
func (w *World) moveParty(slot int) {
	to, ok := w.players[slot]
	if !ok {
		return
	}
	dst := w.Maps[to]
	hero, _ := dst.Member(slot)
	var kept []hireling
	for _, h := range w.hired[slot] {
		from := w.hirelingMap(h.entity)
		if from == to {
			kept = append(kept, h)
			continue
		}
		if st := w.items[slot]; st != nil && st.holder == h.entity {
			w.returnHand(slot) // the held item back in its bag before it moves
		}
		e, on := gamestate.Entity{}, false
		if from > 0 {
			e, on = w.Maps[from].RemoveMember(h.entity)
		}
		if !on || e.Dead() || hero == nil {
			w.dropHireling(h)
			continue
		}
		w.Game.Entities[h.entity] = e
		m := dst.AddMember(h.entity, hero.Cell, int(hero.Dir), w.attach)
		m.Intent = gamestate.IntentGather
		kept = append(kept, h)
	}
	w.hired[slot] = kept
}

// hirelingMap is the map hireling i is a member on, 0 if none.
func (w *World) hirelingMap(i int) int {
	for n := 1; n <= Maps; n++ {
		if mem, _ := w.Maps[n].Member(i); mem != nil {
			return n
		}
	}
	return 0
}

// dropHireling takes a dead hireling out of the world: back in its village
// group as a removed record (+3 bit 0x80, as a logged-out hero), not on
// any map.
func (w *World) dropHireling(h hireling) {
	if from := w.hirelingMap(h.entity); from > 0 {
		w.Maps[from].RemoveMember(h.entity)
	}
	e := &w.Game.Entities[h.entity]
	e.Group = h.group
	e.Flags3 |= 0x80
}

// sendHome sends slot's hirelings back to their villages (the hero leaves
// the world, the session ends): the living ones as villagers on the cell
// they were hired on (or the nearest free one), in their village's area;
// the dead are dropped.
func (w *World) sendHome(slot int) {
	w.returnHand(slot) // an item taken from a hireling goes back to it first
	for _, h := range w.hired[slot] {
		w.goHome(h)
	}
	delete(w.hired, slot)
}

// goHome takes hireling h out of its player's party: alive, a villager
// again on the cell it was hired on (or the nearest free one) in its
// village's area; dead, dropped.
func (w *World) goHome(h hireling) {
	from := w.hirelingMap(h.entity)
	e, on := gamestate.Entity{}, false
	if from > 0 {
		e, on = w.Maps[from].RemoveMember(h.entity)
	}
	if !on || e.Dead() {
		w.dropHireling(h)
		return
	}
	e.Group, e.Flags2 = h.group, 0
	gamestate.LeaveStance(&e)
	w.Game.Entities[h.entity] = e
	s := w.Maps[h.home]
	area, hasArea := w.VillageArea(h.home)
	n := s.AddNPC(h.entity, h.cell, h.dir, w.attach)
	if hasArea && n.Mode == world.NPCVillager {
		n.Area = area
		n.StepRows, n.StepCols = lanVillageRows, lanVillageCols
	}
}

// dismiss is the port's 332 (the own hireling's dialogue,
// gamestate.LANMemberDialogue): slot's hireling i leaves the party and
// goes home (goHome, as at the hero's logout; the original's server case
// 0x0044C154 puts it in the current map's village, but its dialogues never
// reach it). The items it carries stay its own; amulet items among them
// vanish and are put out again (0x0043B7FC, as in the original's case).
func (w *World) dismiss(slot, i int) {
	k := slices.IndexFunc(w.hired[slot], func(h hireling) bool { return h.entity == i })
	if k < 0 {
		return
	}
	h := w.hired[slot][k]
	if st := w.items[slot]; st != nil && st.holder == i {
		w.returnHand(slot) // the held item back in its bag first
	}
	if m := w.hirelingMap(i); m > 0 {
		if _, e := w.Maps[m].Member(i); e != nil {
			w.amuletsVanish(e)
		}
	}
	w.hired[slot] = slices.Delete(w.hired[slot], k, k+1)
	w.goHome(h)
}

// orderMembers are the members of slot's party that take an order: the
// living ones of sel (the player's selection: its hero and hirelings on
// the hero's map), the hero when none is; nil while the hero is dead.
func (w *World) orderMembers(slot int, sel []int) (*world.World, []*world.Member) {
	s, hero, _ := w.member(slot)
	if hero == nil {
		return nil, nil
	}
	var out []*world.Member
	for _, i := range sel {
		if i != slot && !w.hireling(slot, i) {
			continue
		}
		if m, e := s.Member(i); m != nil && world.Alive(&m.Walker, e) && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		out = []*world.Member{hero}
	}
	return s, out
}

// partyMembers are the living members of slot's party on the hero's map:
// the hero and its hirelings there (A attacks with all, F gathers them).
func (w *World) partyMembers(slot int) (*world.World, []*world.Member) {
	s, hero, _ := w.member(slot)
	if hero == nil {
		return nil, nil
	}
	out := []*world.Member{hero}
	for _, h := range w.hired[slot] {
		if m, e := s.Member(h.entity); m != nil && world.Alive(&m.Walker, e) {
			out = append(out, m)
		}
	}
	return s, out
}

// Gather is «Все ко мне» (F): slot's living hirelings on the hero's map
// stop and follow the hero (world.World.Gather).
func (w *World) Gather(slot int) {
	s, ms := w.partyMembers(slot)
	for _, m := range ms {
		if !s.Leads(m) {
			s.Gather(m)
		}
	}
}
