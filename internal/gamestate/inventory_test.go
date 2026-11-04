package gamestate

import (
	"testing"

	"konung1.5/internal/resfmt"
)

func TestMoveItemsGame0(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	volk := g.Entities[0]
	h := NewHand()

	// Lift the knife: the slot empties, the cursor becomes the large sword.
	if msg := g.ClickEquipment(&h, &volk, tb, WeaponBlade); msg != 0 || h.Item != 1 || h.Cursor != 10 || volk.Equipment[WeaponBlade] != 0 {
		t.Fatalf("lift knife: msg %#x hand %+v equipment %v", msg, h, volk.Equipment)
	}
	// The helmet box doesn't take it: it goes back.
	g.ClickEquipment(&h, &volk, tb, EquipHelmet)
	if h.Holding() || volk.Equipment[WeaponBlade] != 1 {
		t.Fatalf("wrong box: hand %+v equipment %v", h, volk.Equipment)
	}
	// Knife into the bag: front of the bag, weight allows it.
	g.ClickEquipment(&h, &volk, tb, WeaponBlade)
	if msg := g.ClickBag(&h, &volk, tb, 3); msg != 0 || h.Holding() || volk.Bag[0] != 1 || volk.Bag[1] != 2 {
		t.Fatalf("drop in bag: msg %#x bag %v", msg, volk.Bag[:8])
	}
	// Lift a balm from the middle, then cancel: it returns to the front.
	g.ClickBag(&h, &volk, tb, 3)
	if h.Item != 4 || h.Cursor != 8 || volk.Bag[3] != 5 {
		t.Fatalf("lift balm: hand %+v bag %v", h, volk.Bag[:8])
	}
	h.Return(&volk)
	if want := [8]int{4, 1, 2, 3, 5, 6, 7, 0}; [8]int(volk.Bag[:8]) != want {
		t.Fatalf("returned bag %v, want %v", volk.Bag[:8], want)
	}
	// Re-equip the knife from the bag.
	g.ClickBag(&h, &volk, tb, 1)
	if msg := g.ClickEquipment(&h, &volk, tb, WeaponBlade); msg != 0 || volk.Equipment[WeaponBlade] != 1 || h.Holding() {
		t.Fatalf("equip: msg %#x equipment %v", msg, volk.Equipment)
	}
	// A balm can't be worn; a full bag refuses.
	full := volk
	for i := range full.Bag {
		full.Bag[i] = 3
	}
	g.ClickEquipment(&h, &full, tb, WeaponBlade)
	if msg := g.ClickBag(&h, &full, tb, 0); msg != MsgTooHeavy && msg != MsgBagFull {
		t.Fatalf("full bag: msg %#x", msg)
	}
	if h.Holding() || full.Equipment[WeaponBlade] != 1 {
		t.Fatalf("refused drop must return the knife: %+v %v", h, full.Equipment)
	}
	if got := resfmt.CP866ToUTF8(tb.CString(MsgBagFull)); got != "Мешок у тебя не резиновый!" {
		t.Fatalf("message %q", got)
	}
}

func TestHandSlot(t *testing.T) {
	h := NewHand()
	e := Entity{Equipment: [6]int{1, 0, 3}, WeaponMode: WeaponBlade}
	if HandSlot(&h, e, nil) != WeaponBlade {
		t.Fatal("blade mode with both: want blade")
	}
	e.WeaponMode = WeaponAxe
	if HandSlot(&h, e, nil) != WeaponAxe {
		t.Fatal("axe mode: want axe")
	}
}

func TestRightClickUseGame0(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	volk := g.Entities[0]
	h := NewHand()
	// Take the shovel (pile 101) and the shield (pile 106) off the ground.
	g.PickUp(&volk, tb, 101, 0)
	g.PickUp(&volk, tb, 106, 0)
	shield := volk.Bag[0]
	// Right click on the shield: worn at once.
	if msg := g.UseBagItem(&h, &volk, tb, 0); msg != 0 || volk.Equipment[EquipShield] != shield || h.Holding() {
		t.Fatalf("equip shield: msg %#x equipment %v hand %+v", msg, volk.Equipment, h)
	}
	// Right click on the shovel (now first in the bag): dig mode, nothing lifted.
	if g.UseBagItem(&h, &volk, tb, 0); !h.Digging() || h.Holding() || volk.Bag[0] == 0 {
		t.Fatalf("shovel: hand %+v bag %v", h, volk.Bag[:4])
	}
	h.Return(&volk)
	if h.Digging() || h.Cursor != CursorPointer {
		t.Fatalf("dig mode not ended: %+v", h)
	}
	// Drop a balm on the ground: a runtime pile that never respawns.
	balm := volk.Bag[2]
	at, ok := g.DropItem(-1, 1, 86, 33, balm)
	if !ok || g.Piles[at].Index != -1 || g.Piles[at].Items[0] != balm || !g.Piles[at].Active(1, 0) {
		t.Fatalf("drop: %+v", g.Piles[at])
	}
	if _, removed := g.PickUp(&volk, tb, at, 10); !removed || g.Piles[at].Active(1, 1<<20) {
		t.Fatal("runtime pile must vanish after pick-up")
	}
}
