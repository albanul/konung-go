package lanserver

import (
	"slices"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

// villagers7 are map 7's plain villagers (GAME.RES dialogue 109), alive,
// NPCs there.
func villagers7(w *World) []int {
	var out []int
	for _, n := range w.Maps[7].NPCs {
		if e := w.Game.Entities[n.Entity]; e.Dialogue == 109 && !e.Dead() && n.Mode == world.NPCVillager {
			out = append(out, n.Entity)
		}
	}
	return out
}

// chief makes slot's hero a Вождь with base Хар charisma and gold coins.
func chief(w *World, slot, charisma, gold int) *gamestate.Entity {
	e := hero(w, slot)
	e.Flags4 = e.Flags4&^7 | gamestate.ClassChief
	e.BaseStats[0], e.Gold = charisma, gold
	return e
}

// hireVia talks slot's hero next to villager v through its dialogue until
// v is hired; false when no reply path hires it.
func hireVia(t *testing.T, w *World, slot, v int) bool {
	t.Helper()
	nextTo(t, w, slot, v)
	return explore(w, slot, v, func() bool { return w.hireling(slot, v) })
}

// A plain villager talks (single player's hire dialogue 1) and is hired for
// its price (300 coins at level < 4: the LAN villages are 302's
// "independent"); it becomes a member of the hero's map in the player's
// group and follows the hero. «Макс. отряд» limits the party: a Вождь with
// base Хар 15 leads 2 (the hero and one hireling).
func TestHireAndFollow(t *testing.T) {
	w := load(t)
	const slot = 0
	if _, _, err := w.AddPlayer(slot, 2, nil); err != nil {
		t.Fatal(err)
	}
	vs := villagers7(w)
	if len(vs) < 2 {
		t.Fatalf("map 7 has %d plain villagers", len(vs))
	}
	v := vs[0]
	nextTo(t, w, slot, v)
	chief(w, slot, 15, 1000)
	w.Talk(slot, v)
	if w.state(slot).talk == nil {
		t.Fatal("a plain villager does not talk")
	}
	w.state(slot).talk = nil
	if !hireVia(t, w, slot, v) {
		t.Fatal("no reply hires the villager")
	}
	price := 300
	if w.Game.Entities[v].Level >= 4 {
		price = 500
	}
	if g := hero(w, slot).Gold; g != 1000-price {
		t.Fatalf("gold after hiring %d, want %d", g, 1000-price)
	}
	s := w.Maps[7]
	m, e := s.Member(v)
	if m == nil || e.Group != slot || s.ByEntity[v] != nil || w.Game.Entities[v].Group != slot {
		t.Fatalf("hireling not a member of the party: member %v group %d", m != nil, w.Game.Entities[v].Group)
	}
	if w.PartySize(slot) != 2 {
		t.Fatalf("party size %d", w.PartySize(slot))
	}
	// The limit: the second villager refuses (349).
	if hireVia(t, w, slot, vs[1]) {
		t.Fatal("hired past «Макс. отряд» (2)")
	}
	// The hireling follows: the hero walks away, the hireling comes near.
	w.state(slot).talk = nil
	heroM, _ := s.Member(slot)
	goal := mapdata.Cell{Row: heroM.Cell.Row + 16, Col: heroM.Cell.Col}
	goal = mapdata.NearestFree(goal, s.Blocked, 8)
	w.Walk(slot, nil, goal.Row, goal.Col)
	for range 400 {
		w.Step()
	}
	heroM, _ = s.Member(slot)
	m, _ = s.Member(v)
	if d := gamestate.Dist(heroM.Cell.Row, heroM.Cell.Col, m.Cell.Row, m.Cell.Col); d > 12 {
		t.Fatalf("hireling at %v, hero at %v: distance %d", m.Cell, heroM.Cell, d)
	}
	// Selected alone, the hireling takes a walk order; the hero stays.
	at := heroM.Cell
	far := mapdata.NearestFree(mapdata.Cell{Row: at.Row - 10, Col: at.Col}, s.Blocked, 8)
	w.Walk(slot, []int{v}, far.Row, far.Col)
	for range 200 {
		w.Step()
	}
	heroM, _ = s.Member(slot)
	if heroM.Cell != at {
		t.Fatalf("the hero moved on the hireling's order: %v → %v", at, heroM.Cell)
	}
	if m, _ = s.Member(v); gamestate.Dist(m.Cell.Row, m.Cell.Col, far.Row, far.Col) > 2 {
		t.Fatalf("the selected hireling at %v, sent to %v", m.Cell, far)
	}
	// Ordered, it stays there (+2 bit 0x02) when the hero walks off …
	away := mapdata.NearestFree(mapdata.Cell{Row: at.Row + 20, Col: at.Col}, s.Blocked, 8)
	w.Walk(slot, nil, away.Row, away.Col)
	for range 300 {
		w.Step()
	}
	heroM, _ = s.Member(slot)
	if m, _ = s.Member(v); gamestate.Dist(m.Cell.Row, m.Cell.Col, far.Row, far.Col) > 2 {
		t.Fatalf("the ordered hireling left its place: %v, ordered to %v", m.Cell, far)
	}
	// … until «Все ко мне» calls it back.
	w.Gather(slot)
	for range 400 {
		w.Step()
	}
	m, _ = s.Member(v)
	if d := gamestate.Dist(heroM.Cell.Row, heroM.Cell.Col, m.Cell.Row, m.Cell.Col); d > 8 {
		t.Fatalf("after F the hireling is %d away (at %v, hero at %v)", d, m.Cell, heroM.Cell)
	}
	// The player cannot attack its own hireling.
	w.Stance(slot, nil, true)
	w.Right(slot, nil, v, m.Cell.Row, m.Cell.Col)
	if heroM.Target == v {
		t.Fatal("the hero attacks its own hireling")
	}
}

// The party goes with the hero: a jump and an exit take the living
// hirelings to the hero's new map; logging out sends them back to their
// village as villagers (the port's stand-in for the original's world
// reset), and they talk again.
func TestPartyTravelsAndGoesHome(t *testing.T) {
	w := load(t)
	const slot = 0
	if _, _, err := w.AddPlayer(slot, 2, nil); err != nil {
		t.Fatal(err)
	}
	v := villagers7(w)[0]
	nextTo(t, w, slot, v)
	chief(w, slot, 30, 5000)
	if !hireVia(t, w, slot, v) {
		t.Fatal("hire failed")
	}
	w.state(slot).talk = nil
	// A jump to map 8.
	if !w.MoveTo(slot, 8, mapdata.Cell{Row: 60, Col: 30}) {
		t.Fatal("MoveTo")
	}
	if m, _ := w.Maps[8].Member(v); m == nil || w.hirelingMap(v) != 8 {
		t.Fatal("the hireling did not come along on the jump")
	}
	if m, _ := w.Maps[7].Member(v); m != nil {
		t.Fatal("the hireling stayed on map 7 too")
	}
	// An exit back: walk the hero onto an exit of map 8.
	var x gamestate.MapExit
	for _, e := range w.exits {
		if e.From == 8 && e.To != 8 {
			x = e
			break
		}
	}
	if x.From == 0 {
		t.Fatal("no exit on map 8")
	}
	w.Maps[8].RemoveMember(slot)
	w.place(slot, 8, mapdata.Cell{Row: x.Row, Col: x.Col}, 0)
	w.moving = append(w.moving, move{slot: slot, x: x})
	w.Step()
	if w.PlayerMap(slot) != x.To || w.hirelingMap(v) != x.To {
		t.Fatalf("after the exit: hero on %d, hireling on %d, want %d", w.PlayerMap(slot), w.hirelingMap(v), x.To)
	}
	// Logout: the hireling goes home.
	w.RemovePlayer(slot)
	if w.hirelingMap(v) != 0 || len(w.Party(slot)) != 0 {
		t.Fatal("the hireling stayed a member after logout")
	}
	n := w.Maps[7].ByEntity[v]
	if n == nil || n.Mode != world.NPCVillager || w.Game.Entities[v].Group != 16 {
		t.Fatalf("the hireling is not back in village 7: npc %v group %d", n != nil, w.Game.Entities[v].Group)
	}
	if a, ok := w.VillageArea(7); !ok || n.Area != a {
		t.Fatalf("its area %+v, the village's %+v", n.Area, a)
	}
	// It talks again.
	w.AddPlayer(slot, 2, nil)
	nextTo(t, w, slot, v)
	w.Talk(slot, v)
	if w.state(slot).talk == nil {
		t.Fatal("the villager back home does not talk")
	}
}

// A dead hireling is dropped when the hero leaves the map; the session's
// end (GAME.RES again) has both back in their village.
func TestDeadHirelingAndSessionEnd(t *testing.T) {
	w := load(t)
	const slot = 0
	w.AddPlayer(slot, 2, nil)
	vs := villagers7(w)
	chief(w, slot, 30, 5000)
	for _, v := range vs[:2] {
		if !hireVia(t, w, slot, v) {
			t.Fatalf("hire %d failed", v)
		}
		w.state(slot).talk = nil
	}
	dead, alive := vs[0], vs[1]
	_, e := w.Maps[7].Member(dead)
	e.HP = 0
	e.Flags4 |= 0x80
	w.MoveTo(slot, 8, mapdata.Cell{Row: 60, Col: 30})
	if w.hireling(slot, dead) || w.hirelingMap(dead) != 0 || w.Game.Entities[dead].Flags3&0x80 == 0 {
		t.Fatal("the dead hireling was not dropped")
	}
	if w.hirelingMap(alive) != 8 {
		t.Fatal("the living hireling did not come along")
	}
	if err := w.Reset(); err != nil {
		t.Fatal(err)
	}
	// The session's end reads GAME.RES again: both are villagers at home
	// again, the dead one alive.
	if len(w.Party(slot)) != 0 || w.Maps[7].ByEntity[alive] == nil || w.Maps[7].ByEntity[dead] == nil ||
		w.Game.Entities[dead].Dead() || w.Game.Entities[alive].Group != w.Game.Entities[vs[2]].Group {
		t.Fatal("the session's end did not bring the hirelings back to their village")
	}
}

// Over the network: the player hires through the dialogue, gets its party
// (MsgParty) and the hireling's state in the view with its owner; a walk
// with the hireling selected moves only it; F gathers.
func TestHireOverNetwork(t *testing.T) {
	w0 := load(t)
	w0.AddPlayer(0, 2, nil)
	v := villagers7(w0)[0]
	nextTo(t, w0, 0, v)
	chief(w0, 0, 30, 5000)
	path, ok := explorePath(w0, 0, v, func() bool { return w0.hireling(0, v) })
	if !ok {
		t.Fatal("no path to the hire")
	}

	w := load(t)
	ss := startSession(t, w)
	a := enter(t, ss, "Ivar", 2)
	ss.host.mu.Lock()
	nextTo(t, w, a.at.Slot, v)
	chief(w, a.at.Slot, 30, 5000)
	ss.host.mu.Unlock()
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderTalk, Target: v})
	waitUntil(t, "the dialogue", func() bool { d := a.dialogue(); return d.Open && d.NPC == v })
	for _, k := range path {
		a.c.Order(netplay.OrderPayload{Kind: netplay.OrderReply, Index: k})
	}
	waitUntil(t, "the party", func() bool { p := a.party(); return len(p.Hirelings) == 1 && p.Hirelings[0] == v })
	waitUntil(t, "the hireling's owner in the view", func() bool {
		c, ok := a.char(v)
		return ok && c.Owner == a.at.Slot+1 && !c.Player
	})
	ss.host.mu.Lock()
	s := w.Maps[w.PlayerMap(a.at.Slot)]
	hm, _ := s.Member(a.at.Slot)
	at := hm.Cell
	far := mapdata.NearestFree(mapdata.Cell{Row: at.Row - 8, Col: at.Col}, s.Blocked, 8)
	ss.host.mu.Unlock()
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderWalk, Row: far.Row, Col: far.Col, Sel: []int{v}})
	waitUntil(t, "the hireling walked", func() bool {
		c, ok := a.char(v)
		return ok && gamestate.Dist(c.Row, c.Col, far.Row, far.Col) <= 2
	})
	if c, _ := a.char(a.at.Slot); c.Row != at.Row || c.Col != at.Col {
		t.Fatal("the hero walked on the hireling's order")
	}
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderGather})
	waitUntil(t, "the hireling back", func() bool {
		c, ok := a.char(v)
		return ok && gamestate.Dist(c.Row, c.Col, at.Row, at.Col) <= 12
	})

	// The hireling's items come with the inventory; the hero gives it a
	// bag item: lifted in the hero's window, dropped on its portrait.
	waitUntil(t, "the hireling's inventory", func() bool {
		inv, _, _ := a.items()
		return len(inv.Hirelings) == 1 && inv.Hirelings[0].Index == v
	})
	ss.host.mu.Lock()
	he := hero(w, a.at.Slot)
	gift := w.Game.SetItemKind(0, gamestate.CatBlade, 18)
	he.Bag = [32]int{gift}
	ss.host.mu.Unlock()
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderItem, Item: netplay.ItemBag, Index: 0, Member: a.at.Slot})
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderItem, Item: netplay.ItemGive, Index: v, Member: a.at.Slot})
	waitUntil(t, "the gift in the hireling's bag", func() bool {
		inv, _, _ := a.items()
		if len(inv.Hirelings) != 1 || inv.Hand.Item != 0 {
			return false
		}
		_, ok := inv.Items[gift]
		return ok && slices.Contains(inv.Hirelings[0].Bag[:], gift) && !slices.Contains(inv.Hero.Bag[:], gift)
	})
}

func (v *viewer) party() netplay.PartyPayload {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.pty
}

func (v *viewer) char(i int) (netplay.CharState, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	c, ok := v.chars[i]
	return c.Predict(v.since[i], v.tick), ok
}
