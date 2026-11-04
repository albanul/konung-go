package gamestate

import (
	"slices"
	"testing"

	"konung1.5/internal/resfmt"
)

func TestInventoryAndItemNames(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	name := func(item int) string { return resfmt.CP866ToUTF8(tb.ItemTypeName(g.ItemType(item))) }
	volk := g.Entities[0]
	if volk.Equipment != [6]int{0, 0, 1, 0, 0, 0} || name(1) != "Нож" || volk.WeaponMode != WeaponBlade {
		t.Fatalf("Volk equipment %v (%q), weapon %d", volk.Equipment, name(1), volk.WeaponMode)
	}
	if volk.Bag[0] != 2 || name(2) != "Клееный лук" {
		t.Fatalf("Volk bag %v, first %q", volk.Bag, name(volk.Bag[0]))
	}
	// Weapon slots hold the weapon kinds: 0 axe/club, 1 bow, 2 blade.
	for idx, want := range map[int][3]string{355: {"Дубина"}, 13: {1: "Пехотный лук"}, 10: {2: "Спата"}} {
		e := g.Entities[idx]
		for s, w := range want {
			if got := ""; e.Equipment[s] != 0 {
				got = name(e.Equipment[s])
				if got != w {
					t.Errorf("entity %d slot %d = %q, want %q", idx, s, got, w)
				}
			} else if w != "" {
				t.Errorf("entity %d slot %d empty, want %q", idx, s, w)
			}
		}
	}
}

func TestStanceToggleAndDrawnWeapon(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	overlay := func(e Entity, slot int) int { return tb.ItemOverlay(g.ItemType(e.Equipment[slot])) }

	volk := g.Entities[0]
	if volk.InStance() || volk.Type != 6 {
		t.Fatalf("Volk starts type %d stance %v", volk.Type, volk.InStance())
	}
	knife := overlay(volk, WeaponBlade)
	if got := g.Overlays(volk, tb); !slices.Equal(got, []int{knife}) {
		t.Fatalf("peaceful overlays %v, want [%d]", got, knife)
	}
	ToggleStance([]*Entity{&volk})
	if !volk.InStance() || volk.Type != 0 {
		t.Fatalf("after Space: type %d stance %v", volk.Type, volk.InStance())
	}
	if got := g.Overlays(volk, tb); !slices.Equal(got, []int{knife + 1}) {
		t.Fatalf("stance overlays %v, want drawn knife [%d]", got, knife+1)
	}
	// Only the blade slot is filled: other weapons can't be selected.
	SelectWeapon(&volk, WeaponAxe)
	SelectWeapon(&volk, WeaponCycle)
	if volk.WeaponMode != WeaponBlade {
		t.Fatalf("weapon mode %d, want blade", volk.WeaponMode)
	}
	volk.Flags2 = 0
	ToggleStance([]*Entity{&volk})
	if volk.InStance() || volk.Type != 6 || volk.Flags2 != 0x40 {
		t.Fatalf("after second Space: type %d stance %v flags2 %#x", volk.Type, volk.InStance(), volk.Flags2)
	}

	// Club and shield are drawn (k+1) in the stance; armour and helmet aren't.
	v := g.Entities[355]
	ToggleStance([]*Entity{&v})
	got := g.Overlays(v, tb)
	for _, k := range []int{overlay(v, WeaponAxe) + 1, overlay(v, EquipShield) + 1, overlay(v, EquipArmour), overlay(v, EquipHelmet)} {
		if !slices.Contains(got, k) {
			t.Errorf("villager 355 stance overlays %v lack %d", got, k)
		}
	}
	// A drawn bow: bow k+1 and no drawn shield (table 0x00462B1C).
	a := g.Entities[13]
	ToggleStance([]*Entity{&a})
	if got := g.Overlays(a, tb); !slices.Contains(got, overlay(a, WeaponBow)+1) {
		t.Errorf("archer stance overlays %v lack the drawn bow %d", got, overlay(a, WeaponBow)+1)
	}
	if a.Type = 5; true { // action 2: the bow is not drawn
		if got := g.Overlays(a, tb); slices.Contains(got, overlay(a, WeaponBow)+1) {
			t.Errorf("archer type 5 overlays %v draw the bow", got)
		}
	}
}

func TestToggleStanceFollowsFirstSelected(t *testing.T) {
	a := Entity{Type: 0, Flags3: stanceBit, Flags2: 0x44}
	b := Entity{Type: 7}
	dead := Entity{Type: 6, Flags4: 0x80}
	ToggleStance([]*Entity{&a, &b, &dead})
	if a.InStance() || a.Type != 6 || a.Flags2 != 0x40 || b.InStance() || b.Type != 7 || dead.InStance() {
		t.Fatalf("leave: a %+v b %+v dead %+v", a, b, dead)
	}
	ToggleStance([]*Entity{&b, &a, nil, &dead})
	if !a.InStance() || a.Type != 0 || !b.InStance() || b.Type != 1 {
		t.Fatalf("enter: a %+v b %+v", a, b)
	}
	attacking := Entity{Type: 5, Flags3: stanceBit}
	ToggleStance([]*Entity{&attacking})
	if attacking.Type != 6 {
		t.Fatalf("leaving mid-attack: type %d, want idle 6", attacking.Type)
	}
}

func TestSelectWeaponCycle(t *testing.T) {
	e := Entity{WeaponMode: WeaponAxe, Equipment: [6]int{1, 0, 3}}
	SelectWeapon(&e, WeaponCycle)
	if e.WeaponMode != WeaponBlade {
		t.Fatalf("axe → %d, want blade (bow slot empty)", e.WeaponMode)
	}
	SelectWeapon(&e, WeaponCycle)
	if e.WeaponMode != WeaponAxe {
		t.Fatalf("blade → %d, want axe", e.WeaponMode)
	}
	e.Equipment[1], e.Flags2 = 2, 0x04
	SelectWeapon(&e, WeaponBow)
	if e.WeaponMode != WeaponBow || e.Flags2 != 0x04 {
		t.Fatalf("select bow: mode %d flags2 %#x", e.WeaponMode, e.Flags2)
	}
	SelectWeapon(&e, WeaponBlade)
	if e.WeaponMode != WeaponBlade || e.Flags2 != 0 {
		t.Fatalf("leave bow: mode %d flags2 %#x", e.WeaponMode, e.Flags2)
	}
}
