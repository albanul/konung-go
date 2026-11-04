package gamestate

import (
	"testing"

	"konung1.5/internal/resfmt"
)

func TestCharSheetGame0(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	// Volk: knife (200 g) + amulet half (100 g) + five balms (500 g each);
	// armour +0x4E only; attack = knife 60 + trunc(Сил 15 · 0.04 · 1600 · 0.0625).
	v := g.CharSheet(g.Entities[0], tb)
	for k, want := range map[int]string{
		SheetWeight: " 2.8", SheetMaxWeight: "13.3", SheetArmour: "10", SheetDamage: "120",
		SheetHealth: "100", SheetLevel: "1", SheetStatsNow + 1: "15", SheetGold: "100",
	} {
		if v[k] != want {
			t.Errorf("Volk value %d = %q, want %q", k, v[k], want)
		}
	}
	// Villager 355: club, leather armour, helmet and shield.
	v = g.CharSheet(g.Entities[355], tb)
	if v[SheetWeight] != " 6.8" || v[SheetArmour] != "52" || v[SheetDamage] != "120" {
		t.Errorf("villager 355 weight %q armour %q damage %q", v[SheetWeight], v[SheetArmour], v[SheetDamage])
	}
	// Nothing in GAME.0's start kit is enchanted, flagged or unusable for its owner.
	for _, i := range g.Entities[0].Bag {
		if it, ok := g.Item(i); ok {
			if _, _, _, tint := g.CellTint(g.Entities[0], tb, it); tint {
				t.Errorf("Volk's bag item %d is tinted", i)
			}
		}
	}
}

func TestSheetEquipmentHandBox(t *testing.T) {
	e := Entity{Equipment: [6]int{1, 0, 3}, WeaponMode: WeaponAxe}
	if s := SheetEquipment(e); !s[0] || s[2] {
		t.Fatalf("axe active: %v", s)
	}
	e.WeaponMode = WeaponBlade
	if s := SheetEquipment(e); s[0] || !s[2] {
		t.Fatalf("blade active: %v", s)
	}
	e.Equipment[0] = 0
	e.WeaponMode = WeaponAxe
	if s := SheetEquipment(e); !s[2] {
		t.Fatalf("blade only: %v", s)
	}
}
