package lanserver

import (
	"sync"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
)

func (v *viewer) items() (netplay.InventoryPayload, netplay.PilesPayload, netplay.PileWindowPayload) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.inv, v.piles, v.pileWin
}

func itemOrder(v *viewer, act string, index int, on bool) {
	v.c.Order(netplay.OrderPayload{Kind: netplay.OrderItem, Item: act, Index: index, On: on})
}

// pileItem reports the pile on cell c in p and whether it holds item i.
func pileItem(p netplay.PilesPayload, c mapdata.Cell, i int) bool {
	for _, ps := range p.Piles {
		if ps.Pile.Row == c.Row && ps.Pile.Col == c.Col {
			for _, it := range ps.Pile.Items {
				if it == i {
					return true
				}
			}
		}
	}
	return false
}

// Items over the network: the inventory arrives; taking the weapon off
// changes the look the other player sees; dropping it makes a pile both
// see; a right click picks it up into the bag; equipping it again; a full
// bag opens the pile window; leaving saves the items and entering again
// brings them back.
func TestItemsOverNetwork(t *testing.T) {
	w := load(t)
	ss := startSession(t, w)
	var mu sync.Mutex
	saved := map[string][]Item{}
	ss.host.Save = func(p netplay.PlayerInfo, _ *netplay.Hero, items []Item) {
		mu.Lock()
		saved[p.Name] = items
		mu.Unlock()
	}
	ss.host.Load = func(p netplay.PlayerInfo) ([]Item, bool) {
		mu.Lock()
		defer mu.Unlock()
		items, ok := saved[p.Name]
		return items, !ok // never saved: a new hero with the knife
	}
	a := enter(t, ss, "Ivar", 1)
	b := enter(t, ss, "Olaf", 1)

	// The inventory arrives with the starting knife.
	weapon := 0
	waitUntil(t, "the inventory", func() bool {
		inv, _, _ := a.items()
		weapon = inv.Hero.Equipment[gamestate.WeaponAxe] + inv.Hero.Equipment[gamestate.WeaponBlade]
		return weapon != 0
	})
	inv, _, _ := a.items()
	if _, ok := inv.Items[weapon]; !ok {
		t.Fatalf("no record for the weapon %d: %v", weapon, inv.Items)
	}
	slot := gamestate.WeaponBlade
	if inv.Hero.Equipment[gamestate.WeaponAxe] == weapon {
		slot = gamestate.WeaponAxe
	}
	looksEquipped := func(on bool) func() bool {
		return func() bool {
			b.mu.Lock()
			defer b.mu.Unlock()
			p, ok := b.ply[a.at.Slot]
			return ok && p.Look.Equip[slot].Item == on
		}
	}
	waitUntil(t, "Olaf sees Ivar's weapon", looksEquipped(true))

	// Taken off (the hand box, hotspot 9): in the hand, gone from the look.
	itemOrder(a, netplay.ItemEquip, 9, false)
	waitUntil(t, "the weapon in the hand", func() bool {
		inv, _, _ := a.items()
		return inv.Hand.Item == weapon && inv.Hero.Equipment[slot] == 0
	})
	waitUntil(t, "Olaf sees Ivar without it", looksEquipped(false))

	// Dropped: a pile on Ivar's cell for both.
	_, _, chars := a.state()
	here := mapdata.Cell{Row: chars[a.at.Slot].Row, Col: chars[a.at.Slot].Col}
	itemOrder(a, netplay.ItemDrop, 0, false)
	waitUntil(t, "the pile for Ivar", func() bool { _, p, _ := a.items(); return pileItem(p, here, weapon) })
	waitUntil(t, "the pile for Olaf", func() bool { _, p, _ := b.items(); return pileItem(p, here, weapon) })

	// A step away and a right click back on the pile: picked up.
	away := mapdata.Cell{Row: here.Row - 2, Col: here.Col}
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderWalk, Row: away.Row, Col: away.Col})
	waitUntil(t, "Ivar off the pile", func() bool {
		_, _, c := a.state()
		s := c[a.at.Slot]
		return !s.Walking && (s.Row != here.Row || s.Col != here.Col)
	})
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderRight, Target: -1, Row: here.Row, Col: here.Col})
	k := -1
	waitUntil(t, "the weapon in the bag", func() bool {
		inv, p, _ := a.items()
		for i, it := range inv.Hero.Bag {
			if it == weapon {
				k = i
			}
		}
		return k >= 0 && !pileItem(p, here, weapon)
	})

	// From the bag into the hand and onto the hand box: equipped again.
	itemOrder(a, netplay.ItemBag, k, false)
	waitUntil(t, "lifted from the bag", func() bool { inv, _, _ := a.items(); return inv.Hand.Item == weapon })
	itemOrder(a, netplay.ItemEquip, 9, false)
	waitUntil(t, "equipped again", func() bool { inv, _, _ := a.items(); return inv.Hero.Equipment[slot] == weapon })
	waitUntil(t, "Olaf sees it again", looksEquipped(true))

	// A full bag: a pick-up stops and the pile window opens; «Закрыть»
	// closes it.
	_, _, chars = a.state()
	at := mapdata.Cell{Row: chars[a.at.Slot].Row, Col: chars[a.at.Slot].Col}
	ss.host.mu.Lock()
	m := w.PlayerMap(a.at.Slot)
	_, e := w.Maps[m].Member(a.at.Slot)
	for i := range e.Bag {
		if e.Bag[i] == 0 {
			e.Bag[i] = w.Game.SetItemKind(0, gamestate.CatHerb, 1)
		}
	}
	extra := w.Game.SetItemKind(0, gamestate.CatHerb, 1)
	pileCell := mapdata.Cell{Row: at.Row + 2, Col: at.Col}
	pi, ok := w.Game.DropItem(-1, m, pileCell.Row, pileCell.Col, extra)
	if !ok {
		ss.host.mu.Unlock()
		t.Fatal("no pile")
	}
	w.Maps[m].PileAt[pileCell] = pi
	ss.host.mu.Unlock()
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderRight, Target: -1, Row: pileCell.Row, Col: pileCell.Col})
	waitUntil(t, "the pile window", func() bool { _, _, pw := a.items(); return pw.Open && pw.Pile == pi })
	a.mu.Lock()
	gotMsg := len(a.msgs) > 0
	a.mu.Unlock()
	if !gotMsg {
		t.Error("no message for the full bag")
	}
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderItem, Item: netplay.ItemPile, On: false})
	waitUntil(t, "the pile window closed", func() bool { _, _, pw := a.items(); return !pw.Open })

	// Leaving saves the items; entering again brings them back.
	a.c.Close()
	waitUntil(t, "the save", func() bool { mu.Lock(); defer mu.Unlock(); return len(saved["Ivar"]) > 0 })
	mu.Lock()
	var kept *Item
	for _, it := range saved["Ivar"] {
		if it.Slot == SlotEquip && it.Index == slot {
			it := it
			kept = &it
		}
	}
	mu.Unlock()
	if kept == nil {
		t.Fatalf("the weapon was not saved: %+v", saved["Ivar"])
	}
	a2 := enter(t, ss, "Ivar", 1)
	waitUntil(t, "the items back", func() bool {
		inv, _, _ := a2.items()
		i := inv.Hero.Equipment[slot]
		r, ok := inv.Items[i]
		return i != 0 && ok && int(r[3]) == kept.Type && int(int8(r[0])) == kept.Category
	})
	inv, _, _ = a2.items()
	if inv.Hero.Bag[31] == 0 {
		t.Error("the full bag did not come back")
	}
	a2.mu.Lock()
	t.Logf("inventory: %d messages, %d bytes", a2.invs, a2.invBytes)
	a2.mu.Unlock()
}

// SavedItems and LoadItems keep what SAVITEMS.DAT keeps: kinds,
// enchantment, condition and maximum (float32), the arrows' count.
func TestSavedItemsRoundTrip(t *testing.T) {
	w := load(t)
	if _, _, err := w.AddPlayer(0, 1, nil); err != nil {
		t.Fatal(err)
	}
	before := w.SavedItems(0)
	// A new hero carries only the knife (the original server).
	if len(before) != 1 || before[0].Slot != SlotEquip || before[0].Index != gamestate.WeaponBlade ||
		before[0].Category != gamestate.CatBlade || before[0].Type != 18 {
		t.Fatalf("starting gear %+v, want only the knife", before)
	}
	arrows := Item{Slot: SlotArrows, Category: gamestate.CatArrows, Type: 1, Count: 37}
	w.LoadItems(0, append(append([]Item{}, before...), arrows))
	after := w.SavedItems(0)
	if len(after) != len(before)+1 {
		t.Fatalf("saved %d items, loaded %d back", len(before)+1, len(after))
	}
	for i := range before {
		if after[i] != before[i] {
			t.Errorf("item %d: %+v, was %+v", i, after[i], before[i])
		}
	}
	if got := after[len(after)-1]; got.Slot != SlotArrows || got.Count != 37 || got.Type != 1 {
		t.Errorf("arrows: %+v", got)
	}
	// A gap the store's drop rules leave closes up.
	bag := []Item{
		{Slot: SlotBag, Index: 0, Category: gamestate.CatHerb, Type: 1},
		{Slot: SlotBag, Index: 1, Category: gamestate.CatHerb, Type: 2},
		{Slot: SlotBag, Index: 2, Category: gamestate.CatHerb, Type: 3},
	}
	w.LoadItems(0, []Item{bag[0], bag[2]})
	if e, _ := w.Hero(0); e.Bag[0] == 0 || e.Bag[1] == 0 || e.Bag[2] != 0 {
		t.Errorf("bag with a gap: %v", e.Bag[:4])
	}
}
