package game

import (
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
)

// MsgLooks puts the sent equipment on a world character's record as
// drawing-only items (from the item table's top) and takes it off again;
// the own party's records are left to MsgInventory.
func TestLANApplyLooks(t *testing.T) {
	s := viewScene(t)
	const v = 300 // a character of GAME.RES's world
	armour := netplay.EquipLook{Item: true, Category: gamestate.CatArmour, Type: 3}
	var eq [netplay.EquipSlots]netplay.EquipLook
	eq[gamestate.EquipArmour] = armour
	s.applyLooks(netplay.LooksPayload{Equip: map[int][netplay.EquipSlots]netplay.EquipLook{v: eq}})
	e := s.Game.Entities[v]
	i := e.Equipment[gamestate.EquipArmour]
	if c, typ := s.Game.ItemKind(i); c != gamestate.CatArmour || typ != 3 || i < len(s.Game.Entities) {
		t.Fatalf("armour item %d kind %d/%d", i, c, typ)
	}
	for k, it := range e.Equipment {
		if k != gamestate.EquipArmour && it != 0 {
			t.Errorf("slot %d holds %d, want empty", k, it)
		}
	}
	// Taken off: the slot empties and the record is free again.
	s.applyLooks(netplay.LooksPayload{Equip: map[int][netplay.EquipSlots]netplay.EquipLook{v: {}}})
	if s.Game.Entities[v].Equipment[gamestate.EquipArmour] != 0 {
		t.Fatal("the armour stayed on")
	}
	if c, _ := s.Game.ItemKind(i); c >= 0 {
		t.Fatal("its drawing record was not freed")
	}
	// The own hero is not touched.
	own := s.Party[0].Index
	before := s.Game.Entities[own].Equipment
	s.applyLooks(netplay.LooksPayload{Equip: map[int][netplay.EquipSlots]netplay.EquipLook{own: eq}})
	if s.Game.Entities[own].Equipment != before {
		t.Fatal("a look changed the own party")
	}
}
