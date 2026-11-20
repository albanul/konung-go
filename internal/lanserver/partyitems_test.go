package lanserver

import (
	"slices"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
)

// itemClick sends slot's item click for party member mem.
func itemClick(w *World, slot, mem int, act string, index int) {
	w.Item(slot, netplay.OrderPayload{Kind: netplay.OrderItem, Item: act, Index: index, Member: mem})
}

// A hireling's items are the player's to arrange (LAN_KONG.EXE 0x4A with
// the member index, 0x4D give between two members of the group, 0x5D a
// raise for any member): the hero gives it an item from the bag (its
// portrait), the hireling's window equips it, its raises spend its free
// experience; while an item is held only its holder's window takes clicks;
// another player's hireling, a dead one and another player's give are
// refused; «Живая Вода» on a dead hireling's portrait revives it;
// MsgInventory carries the hirelings' records and items.
func TestHirelingItems(t *testing.T) {
	w := load(t)
	const slot, other = 0, 1
	for _, k := range []int{slot, other} {
		if _, _, err := w.AddPlayer(k, 2, nil); err != nil {
			t.Fatal(err)
		}
	}
	v := villagers7(w)[0]
	nextTo(t, w, slot, v)
	if !w.hire(slot, v) {
		t.Fatal("hire")
	}
	s := w.Maps[7]
	he := hero(w, slot)
	sword := w.Game.SetItemKind(0, gamestate.CatBlade, 18)
	he.Bag = [32]int{sword}

	// Give: lift from the hero's bag, drop on the hireling's portrait.
	itemClick(w, slot, 0, netplay.ItemBag, 0)
	if w.state(slot).hand.Item != sword {
		t.Fatal("the bag item was not lifted")
	}
	// The hireling's window does not take a click while the hero's item
	// is held.
	_, ve := s.Member(v)
	vbag := ve.Bag
	itemClick(w, slot, v, netplay.ItemBag, 0)
	if _, ve = s.Member(v); ve.Bag != vbag || w.state(slot).hand.Item != sword {
		t.Fatal("another member's window took a click while an item was held")
	}
	if inv, _ := w.Inventory(slot); inv.Holder != slot {
		t.Fatalf("holder %d, want the hero", inv.Holder)
	}
	itemClick(w, slot, 0, netplay.ItemGive, v)
	if _, ve = s.Member(v); !slices.Contains(ve.Bag[:], sword) || slices.Contains(he.Bag[:], sword) || w.state(slot).hand.Holding() {
		t.Fatalf("give: hireling bag %v, hero bag %v", ve.Bag[:4], he.Bag[:4])
	}

	// The hireling's window: lift the sword from its bag and put it in the
	// hand slot (hotspot 9).
	k := slices.Index(ve.Bag[:], sword)
	itemClick(w, slot, v, netplay.ItemBag, k)
	if w.state(slot).hand.Item != sword {
		t.Fatal("the hireling's bag item was not lifted")
	}
	itemClick(w, slot, 0, netplay.ItemEquip, 9) // the hero's window: refused while the hireling's item is held
	if w.state(slot).hand.Item != sword {
		t.Fatal("the hero's window took the hireling's item")
	}
	itemClick(w, slot, v, netplay.ItemEquip, 9)
	if _, ve = s.Member(v); !slices.Contains(ve.Equipment[:], sword) {
		t.Fatalf("not equipped on the hireling: %v", ve.Equipment)
	}
	w.returnHand(slot) // what the swap put in the hand goes back to the hireling

	// The inventory message has the hireling's record and items.
	inv, _ := w.Inventory(slot)
	if len(inv.Hirelings) != 1 || inv.Hirelings[0].Index != v || inv.Hirelings[0].Equipment != ve.Equipment {
		t.Fatalf("inventory hirelings %+v", inv.Hirelings)
	}
	if _, ok := inv.Items[sword]; !ok {
		t.Fatal("the hireling's item record is missing")
	}

	// A stat raise for the hireling.
	ve.FreeExp = 100000
	ve.Flags4 = ve.Flags4&^7 | gamestate.ClassChief // a villager has no class: no raise buttons
	base := ve.BaseStats[0]
	itemClick(w, slot, v, netplay.ItemRaise, 1) // Хар
	if _, ve = s.Member(v); ve.BaseStats[0] != base+1 {
		t.Fatalf("hireling Хар %d, want %d", ve.BaseStats[0], base+1)
	}

	// Another player: its clicks for this hireling and its gives to it are
	// refused.
	oe := hero(w, other)
	axe := w.Game.SetItemKind(0, gamestate.CatBlade, 18)
	oe.Bag = [32]int{axe}
	_, ve = s.Member(v)
	before := *ve
	itemClick(w, other, v, netplay.ItemEquip, 9)
	itemClick(w, other, 0, netplay.ItemBag, 0)
	itemClick(w, other, 0, netplay.ItemGive, v)
	if _, ve = s.Member(v); ve.Bag != before.Bag || ve.Equipment != before.Equipment || w.state(other).hand.Item != axe {
		t.Fatal("another player changed the hireling's items")
	}
	w.returnHand(other)

	// A dead hireling's window takes no clicks.
	ve.Flags4 |= 0x80
	itemClick(w, slot, v, netplay.ItemBag, 0)
	if w.state(slot).hand.Holding() {
		t.Fatal("a dead hireling's bag item was lifted")
	}
	// «Живая Вода» dropped on the dead hireling's portrait revives it on
	// this map (Game.Reviver is the hero's map's).
	ve.HP = 0
	water := w.Game.SetItemKind(0, gamestate.CatHerb, gamestate.HerbLivingWater)
	he.Bag = [32]int{water}
	itemClick(w, slot, 0, netplay.ItemBag, 0)
	itemClick(w, slot, 0, netplay.ItemGive, v)
	if m, e := s.Member(v); m == nil || e.Dead() || e.HP <= 0 {
		t.Fatal("«Живая Вода» did not revive the hireling")
	}
}
