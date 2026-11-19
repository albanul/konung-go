package lanserver

import (
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

// creatureOn is a creature of map m's world.
func creatureOn(t *testing.T, w *World, m int) int {
	t.Helper()
	for _, n := range w.Maps[m].NPCs {
		if n.Mode == world.NPCCreature {
			return n.Entity
		}
	}
	t.Fatalf("no creature on map %d", m)
	return -1
}

// The session's end builds the world anew from GAME.RES (World.Reset): a
// creature killed is back, items dropped are gone and the GAME.RES piles
// whole again, an angry village is calm, the shops' stocks are new (the
// heroes' merchants restocked); every hero in the world is on its type's
// start, alive with full HP, no poison or enchantments, out of the stance,
// with its items but the ones the load drops (an amulet part), its quest
// flags kept; a hero that was elsewhere is reported moving.
func TestSessionReset(t *testing.T) {
	w := load(t)
	if _, _, err := w.AddPlayer(0, 2, nil); err != nil { // map 7
		t.Fatal(err)
	}
	if _, _, err := w.AddPlayer(1, 0, nil); err != nil { // map 1
		t.Fatal(err)
	}
	w.MoveTo(1, 8, mapdata.Cell{Row: 60, Col: 30})
	for range 30 { // some time of day
		w.Step()
	}
	w.TakeEvents()

	// A creature on map 8 killed.
	cr := creatureOn(t, w, 8)
	w.Game.Entities[cr].HP = 0
	w.Game.Entities[cr].Flags4 |= 0x80
	// A GAME.RES pile on map 7 emptied, an item dropped there.
	pile := -1
	for i, p := range w.Game.Piles {
		if p.Map == 7 && p.Index >= 0 && p.Items[0] > 0 {
			pile = i
			break
		}
	}
	if pile < 0 {
		t.Fatal("no GAME.RES pile with items on map 7")
	}
	want := w.Game.Piles[pile]
	w.Game.Piles[pile].Items = [len(want.Items)]int{}
	dropped := w.Game.SetItemKind(0, gamestate.CatQuest, gamestate.MiscWords)
	w.Game.DropItem(-1, 7, 70, 20, dropped)
	piles := len(w.Game.Piles)
	// Village 7 angry.
	si := w.settlementOf(7)
	w.Game.Settlements[si].Flags |= gamestate.SettlementHostile
	vg := w.Game.Settlements[si].Group
	w.Game.Groups[vg].Alarm = gamestate.AlarmAttack
	// Hero 0: hurt, poisoned, enchanted, in the stance, an amulet part and
	// «Слова Воскрешения» in the bag, a quest flag set.
	_, e := w.Maps[7].Member(0)
	e.HP, e.Poison, e.Enchants = 300, 5, [2]int{3, -1}
	gamestate.EnterStance(e)
	amulet := w.Game.SetItemKind(0, gamestate.CatQuest, gamestate.MiscAmuletRight)
	words := w.Game.SetItemKind(0, gamestate.CatQuest, gamestate.MiscWords)
	e.Bag[0], e.Bag[1] = amulet, words
	w.state(0).quests[0] |= 0x80
	knife, _ := w.Game.Item(e.Equipment[gamestate.WeaponBlade])
	// Hero 1 dead on map 8, its bag dropped.
	_, e1 := w.Maps[8].Member(1)
	e1.HP = 0
	e1.Flags4 |= 0x80
	e1.Bag = [32]int{}
	tick7 := w.Maps[7].Tick
	before := w.Game

	if err := w.Reset(); err != nil {
		t.Fatal(err)
	}
	if w.Game == before {
		t.Fatal("the world was not read again")
	}
	if w.Maps[7].Tick != tick7 {
		t.Errorf("map 7's time %d, want %d (the time of day goes on)", w.Maps[7].Tick, tick7)
	}
	if c := w.Game.Entities[cr]; c.Dead() || w.Maps[8].ByEntity[cr] == nil {
		t.Errorf("creature %d not back on map 8 (dead %v)", cr, c.Dead())
	}
	if got := w.Game.Piles[pile]; got.Items != want.Items {
		t.Errorf("pile %d: items %v, want GAME.RES's %v", pile, got.Items, want.Items)
	}
	if len(w.Game.Piles) >= piles {
		t.Errorf("%d piles after the reset, the dropped one still there (%d before)", len(w.Game.Piles), piles)
	}
	for _, p := range w.Game.Piles {
		if p.Index < 0 && w.amuletKind(p.Items[0]) == 0 { // the amulet parts are put out anew
			t.Errorf("a dropped pile is still there: %+v", p)
		}
	}
	if s := w.Game.Settlements[si]; s.Flags&gamestate.SettlementHostile != 0 || w.Game.Groups[vg].Alarm == gamestate.AlarmAttack {
		t.Errorf("village 7 still angry: flags %#x, alarm %d", s.Flags, w.Game.Groups[vg].Alarm)
	}

	for slot, model := range []int{2, 0} {
		start, _ := w.Start(model)
		h, ok := w.Hero(slot)
		if !ok || w.PlayerMap(slot) != start.To {
			t.Fatalf("hero %d on map %d, want its start %d", slot, w.PlayerMap(slot), start.To)
		}
		if h.Dead() || h.HP != gamestate.HPFull || h.Poison != 0 || h.InStance() || h.Enchants != [2]int{-1, -1} {
			t.Errorf("hero %d after the reset: dead %v HP %d poison %d stance %v enchants %v", slot, h.Dead(), h.HP, h.Poison, h.InStance(), h.Enchants)
		}
		if k, ok := w.Game.Item(h.Equipment[gamestate.WeaponBlade]); !ok || k.Category != knife.Category || k.Type != knife.Type {
			t.Errorf("hero %d lost its knife", slot)
		}
	}
	h0, _ := w.Hero(0)
	var bag []gamestate.Item
	for _, i := range h0.Bag {
		if it, ok := w.Game.Item(i); ok && i > 0 {
			bag = append(bag, it)
		}
	}
	if len(bag) != 1 || bag[0].Category != gamestate.CatQuest || bag[0].Type != gamestate.MiscWords {
		t.Errorf("hero 0's bag %+v, want only «Слова Воскрешения» (the amulet part does not come back)", bag)
	}
	if w.state(0).quests[0]&0x80 == 0 {
		t.Error("hero 0's quest flag was lost")
	}
	// Hero 1 moved 8 → 1, hero 0 stayed on 7.
	var moved []Event
	for _, ev := range w.TakeEvents() {
		if ev.Kind == EventMapChange {
			moved = append(moved, ev)
		}
	}
	if len(moved) != 1 || moved[0].Entity != 1 || moved[0].From != 8 || moved[0].Map != 1 {
		t.Errorf("map changes %+v, want hero 1 from 8 to 1", moved)
	}
	// The shops: the heroes' merchants restocked, an empty slot's empty.
	if w.shops[si][0].Merchant == [0x40]byte{} {
		t.Error("hero 0's merchant was not restocked")
	}
	if w.shops[si][5].Merchant != [0x40]byte{} {
		t.Error("slot 5's merchant stock was not emptied")
	}
	// The world runs on.
	for range 200 {
		w.Step()
	}
}

// Over the network: at the session's end a player whose hero was elsewhere
// gets the map change to its start and the whole map; the creature it
// killed is alive again when it comes back.
func TestSessionEndOverNetwork(t *testing.T) {
	w := load(t)
	ss := startSession(t, w)
	a := enter(t, ss, "Ivar", 2)
	ss.host.mu.Lock()
	w.MoveTo(a.at.Slot, 8, mapdata.Cell{Row: 60, Col: 30})
	cr := creatureOn(t, w, 8)
	w.Game.Entities[cr].HP = 0
	w.Game.Entities[cr].Flags4 |= 0x80
	ss.host.mu.Unlock()
	waitUntil(t, "the view of map 8 with the dead creature", func() bool {
		m, _, chars := a.state()
		c, ok := chars[cr]
		return m == 8 && ok && c.Dead
	})
	if err := ss.host.EndSession(); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the start map after the session's end", func() bool {
		m, _, chars := a.state()
		h, ok := chars[a.at.Slot]
		return m == 7 && ok && !h.Dead && h.HP == gamestate.HPFull
	})
	a.mu.Lock()
	changes := append([]netplay.EnterPayload(nil), a.changes...)
	a.mu.Unlock()
	if n := len(changes); n == 0 || changes[n-1].Map != 7 {
		t.Errorf("map changes %+v, want the last to map 7", changes)
	}
	ss.host.mu.Lock()
	w.MoveTo(a.at.Slot, 8, mapdata.Cell{Row: 60, Col: 30})
	ss.host.mu.Unlock()
	waitUntil(t, "the creature alive again", func() bool {
		m, _, chars := a.state()
		c, ok := chars[cr]
		return m == 8 && ok && !c.Dead
	})
	a.mu.Lock()
	inv := a.inv
	a.mu.Unlock()
	if inv.Items == nil {
		t.Error("no inventory after the session's end")
	}
}
