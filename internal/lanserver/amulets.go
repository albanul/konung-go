package lanserver

import (
	"sort"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
	"konung1.5/internal/world"
)

// The amulet of the dragon (docs/konung_lan.md#amulets): LAN_KONG.EXE's
// server puts the three parts at their places (0x004349FC) at a session's
// start; a player joins them (in the in-hand box, 0x0044DB54) and drops the
// whole amulet standing on its nation's spot (0x00437A5C): 25 points
// (0x0043B67C), the nation's count goes up, and the parts are put out again
// unless the nation reached the win limit, which ends the session
// (0x00434438). Amulet items vanish, and their parts are put out again,
// when a hero carrying them leaves the world (0x00443E92) and when they lie
// on the ground for 90 passes after being dropped (0x00440549).

// AmuletRules are a server's amulet settings: the SERVER file's six places
// and the delivered amulets that win (0: none).
type AmuletRules struct {
	Points gamestate.LANAmuletPoints
	Limit  int
}

// DefaultAmuletRules are LAN_KONG.EXE's without a SERVER file.
var DefaultAmuletRules = AmuletRules{Points: gamestate.DefaultAmuletPoints, Limit: gamestate.DefaultAmuletLimit}

// amulets is the world's amulet state.
type amulets struct {
	rules   AmuletRules
	nations [3]int // delivered by nation (0x00E23948)
	// items are the amulet items in play (at most three; amuletItem).
	items []amuletItem
	won   int // the nation that reached the limit (−1: none; −2 reported), for the Host
}

// amuletItem is an amulet item in play: its carrier after the last scan
// (player slot, −1 none), whether it still lies where it was put out (no
// ground timer), and its ground timer: the pile it was seen in and the
// passes left (0: not counting).
type amuletItem struct {
	item   int
	home   bool
	pile   int
	passes int
	holder int
}

// startAmulets is a session's start (0x00448C00): no amulets delivered, the
// three parts at their places.
func (w *World) startAmulets() {
	w.amulets.nations = [3]int{}
	w.amulets.items = nil
	w.amulets.won = -1
	for k := range 3 {
		w.placePart(k)
	}
}

// placePart is 0x004349FC(k): a new item, part k (the right, left or bottom
// part of the amulet), on the ground at place k: into the pile there (its
// first free slot, else over its last item), or a new pile.
func (w *World) placePart(k int) {
	p := w.amulets.rules.Points[k]
	m := p[0]
	if m < 1 || m > Maps {
		return
	}
	item := w.Game.SetItemKind(0, gamestate.CatQuest, gamestate.AmuletKindType(k+1))
	if item == 0 {
		return
	}
	s := w.Maps[m]
	c := mapdata.Cell{Row: p[1], Col: p[2]}
	at, on := s.PileAt[c]
	if on {
		if _, ok := w.Game.DropItem(at, m, c.Row, c.Col, item); !ok {
			pl := &w.Game.Piles[at]
			pl.Items[len(pl.Items)-1] = item
		}
	} else {
		at, _ = w.Game.DropItem(-1, m, c.Row, c.Col, item)
		s.PileAt[c] = at
	}
	w.forgetAmulet(item) // a record freed by a join, a delivery or a logout, in use again
	w.amulets.items = append(w.amulets.items, amuletItem{item: item, home: true, pile: at, holder: -1})
}

// forgetAmulet drops item from the amulet items in play.
func (w *World) forgetAmulet(item int) {
	items := w.amulets.items[:0]
	for _, it := range w.amulets.items {
		if it.item != item {
			items = append(items, it)
		}
	}
	w.amulets.items = items
}

// placeParts puts out the parts amulet item kind k is made of.
func (w *World) placeParts(k int) {
	for _, p := range gamestate.AmuletParts(k) {
		w.placePart(p)
	}
}

// amuletKind is item i's amulet kind (0: not an amulet item).
func (w *World) amuletKind(i int) int {
	if c, typ := w.Game.ItemKind(i); c == gamestate.CatQuest {
		return gamestate.AmuletKind(typ)
	}
	return 0
}

// carried calls f with every item reference slot's hero and hirelings
// carry: their bags and equipment (the records on their maps: a map's
// party copy is the live one) and the hero's hand.
func (w *World) carried(slot int, f func(item *int)) {
	each := func(e *gamestate.Entity) {
		for k := range e.Bag {
			f(&e.Bag[k])
		}
		for k := range e.Equipment {
			f(&e.Equipment[k])
		}
	}
	m, ok := w.players[slot]
	if !ok {
		return
	}
	if _, e := w.Maps[m].Member(slot); e != nil {
		each(e)
	}
	if st := w.items[slot]; st != nil && st.hand.Holding() {
		f(&st.hand.Item)
	}
	for _, h := range w.hired[slot] {
		if n := w.hirelingMap(h.entity); n > 0 {
			if _, e := w.Maps[n].Member(h.entity); e != nil {
				each(e)
			}
		}
	}
}

// scanAmulets drops the items that are no amulet items any more (joined
// into another, delivered, vanished) and finds each one's carrier.
func (w *World) scanAmulets() {
	a := &w.amulets
	items := a.items[:0]
	for _, it := range a.items {
		if w.amuletKind(it.item) != 0 {
			it.holder = -1
			items = append(items, it)
		}
	}
	a.items = items
	if len(items) == 0 {
		return
	}
	for slot := range w.players {
		w.carried(slot, func(p *int) {
			for k := range a.items {
				if a.items[k].item == *p && *p != 0 {
					a.items[k].holder = slot
					a.items[k].home = false
				}
			}
		})
	}
}

// amuletPass is the settlement pass's part (0x00440549, every 16 ticks): an
// amulet item lying on the ground since it was dropped counts down
// AmuletGroundPasses passes and then vanishes: it becomes an empty jar
// (0x0044062A) and its parts are put out again. One put out at its place
// lies there for good.
func (w *World) amuletPass() {
	a := &w.amulets
	for k := range a.items {
		it := &a.items[k]
		if it.holder >= 0 || it.home {
			it.passes = 0
			continue
		}
		pile := w.pileOf(it.item)
		switch {
		case pile < 0: // not on the ground: nothing to count
			it.passes, it.pile = 0, -1
		case pile != it.pile || it.passes == 0: // dropped: the timer starts (0x00437D75)
			it.pile, it.passes = pile, gamestate.AmuletGroundPasses
		default:
			if it.passes--; it.passes == 0 {
				kind := w.amuletKind(it.item)
				w.Game.SetItemKind(it.item, gamestate.CatHerb, gamestate.HerbEmptyJar)
				w.placeParts(kind)
			}
		}
	}
	w.scanAmulets()
}

// pileOf is the pile holding item i, −1 if none.
func (w *World) pileOf(i int) int {
	for p := range w.Game.Piles {
		for _, v := range w.Game.Piles[p].Items {
			if v == i {
				return p
			}
		}
	}
	return -1
}

// amuletsLeave is a hero leaving the world (0x00443C1C → 0x0043B7FC): the
// amulet items it and its hirelings carry vanish (they are freed with its
// items) and their parts are put out again.
func (w *World) amuletsLeave(slot int) {
	var kinds []int
	w.carried(slot, func(p *int) {
		if k := w.amuletKind(*p); k != 0 {
			kinds = append(kinds, k)
			w.Game.SetItemKind(*p, -1, 0)
			w.forgetAmulet(*p)
			*p = 0 // its record may be the new part's
		}
	})
	for _, k := range kinds {
		w.placeParts(k)
	}
	w.scanAmulets()
}

// deliver is 0x00437A5C for a held item dropped by slot's hero on map s:
// the whole amulet dropped on the spot of the hero's nation is delivered —
// the item is gone, the player scores (EventAmulet), the nation's count
// goes up; the nation that reaches the limit wins the session (the Host
// ends it; every player is told), else the three parts are put out again.
// It reports whether the item was delivered.
func (w *World) deliver(s *world.World, mem *world.Member, slot, item int) bool {
	if w.amuletKind(item) != gamestate.AmuletWhole {
		return false
	}
	nation := w.Game.Entities[slot].Model
	if nation < 0 || nation > 2 {
		return false
	}
	p := w.amulets.rules.Points[3+nation]
	if s.MapNumber != p[0] || mem.Cell.Row != p[1] || mem.Cell.Col != p[2] {
		return false
	}
	w.Game.SetItemKind(item, -1, 0)
	w.forgetAmulet(item)
	a := &w.amulets
	a.nations[nation]++
	w.events = append(w.events, Event{Kind: EventAmulet, Map: s.MapNumber, Entity: slot, Victim: nation})
	if a.rules.Limit > 0 && a.nations[nation] >= a.rules.Limit && a.won < 0 {
		a.won = nation
		text := cp866(gamestate.NationWins[nation])
		for k := range w.players {
			w.say(k, text)
		}
	} else if a.won < 0 {
		w.placeParts(gamestate.AmuletWhole)
	}
	w.scanAmulets()
	return true
}

// TakeWin reports the nation that reached the win limit since the last call
// (the session is to end, 0x00434438), and clears it.
func (w *World) TakeWin() (int, bool) {
	n := w.amulets.won
	if n < 0 {
		return 0, false
	}
	w.amulets.won = -2 // reported; the session's end (Reset) starts anew
	return n, true
}

// Nations are the amulets delivered by nation this session.
func (w *World) Nations() [3]int { return w.amulets.nations }

// carriesAmulet reports whether slot's hero or hirelings carry an amulet
// item (after the last scan): a kill of the hero scores double (0x0043B67C).
func (w *World) carriesAmulet(slot int) bool {
	for _, it := range w.amulets.items {
		if it.holder == slot {
			return true
		}
	}
	return false
}

// Amulets is the amulets' state for the players (the original's 0x16): the
// items, the joined ones first then by kind (as the original's three slots
// end up: 0x00439082, 0x0043B843), the nations' counts and the limit.
func (w *World) Amulets() netplay.AmuletsPayload {
	items := append([]amuletItem(nil), w.amulets.items...)
	rank := func(it amuletItem) int {
		k := w.amuletKind(it.item)
		if k >= gamestate.AmuletWhole {
			return k - gamestate.AmuletWhole // joined first
		}
		return 10 + k
	}
	sort.SliceStable(items, func(i, j int) bool { return rank(items[i]) < rank(items[j]) })
	var p netplay.AmuletsPayload
	for k := 0; k < len(items) && k < len(p.Parts); k++ {
		p.Parts[k] = netplay.AmuletPart{Kind: w.amuletKind(items[k].item), Holder: items[k].holder}
	}
	for k := len(items); k < len(p.Parts); k++ {
		p.Parts[k].Holder = -1
	}
	p.Nations, p.Limit = w.amulets.nations, w.amulets.rules.Limit
	return p
}

// cp866 is s in the game's encoding (the status line's).
func cp866(s string) []byte {
	var out []byte
	for _, r := range s {
		if c, ok := resfmt.RuneToCP866(r); ok {
			out = append(out, c)
		}
	}
	return out
}
