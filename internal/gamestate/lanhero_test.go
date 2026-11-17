package gamestate

import (
	"encoding/binary"
	"os"
	"testing"
)

// The hard-coded «Новый герой» tables match LAN_KONG.EXE.
func TestLANHeroTables(t *testing.T) {
	exe, err := os.ReadFile("../../assets/Konung/LAN_KONG.EXE")
	if err != nil {
		t.Skip("LAN_KONG.EXE not available:", err)
	}
	at := func(va int) []byte { return exe[va-lanDGroupVA+lanDGroupFile:] }
	i32 := func(va int) int { return int(int32(binary.LittleEndian.Uint32(at(va)))) }
	for i, r := range lanRaiseCosts {
		for k := range 3 {
			if got := i32(0x00481E24 + 12*i + 4*k); got != r[k] {
				t.Errorf("cost row %d[%d] = %d, exe %d", i, k, r[k], got)
			}
		}
	}
	ids := [4]int{3, 5, 2, 6} // raise ids of Хар, Сил, Лов, Вын
	capVA := [4]int{0x00481ED9, 0x00481EF3, 0x00481EBF, 0x00481F0D}
	for class := ClassMerchant; class <= ClassWarrior; class++ {
		c := int(class) - 1
		fav := int(at(0x00481DDC + c)[0])
		for s := range 4 {
			if (ids[s] == fav) != (lanFavourite[byte(class)] == s) {
				t.Errorf("class %d: favourite id %d, port stat %d", class, fav, lanFavourite[byte(class)])
			}
			if got := int(at(capVA[s] + c*0x68 + LANHeroEditLevel*2)[0]); got != LANStatCap(byte(class), s) {
				t.Errorf("class %d stat %d: cap %d, exe %d", class, s, LANStatCap(byte(class), s), got)
			}
		}
		for k, v := range lanPartyThresholds[byte(class)] {
			if got := i32(0x00482190 + 12*c + 4*k); got != v {
				t.Errorf("class %d party threshold %d = %d, exe %d", class, k, v, got)
			}
		}
	}
	for m, va := range []int{0x004821CC, 0x004821EC, 0x0048220C} {
		for l := range 8 {
			if got := i32(va+4*l) / 512; got != LANLooks[m][l] {
				t.Errorf("look %d/%d = %d, exe %d", m, l, LANLooks[m][l], got)
			}
		}
	}
}

// Build agrees with trading click by click, and refuses what the screen
// cannot produce.
func TestLANHeroBuild(t *testing.T) {
	e := NewLANHeroTemplate()
	SelectClass(&e, ClassChief) // Хар 25 Сил 7 Лов 3 Вын 7, favourite Хар
	SelectCombatSkill(&e, true)
	SelectSkill(&e, SkillTrade)
	for range 5 { // Сил 7 → 2: refunds 5 each (other column: below 9)
		if !LANTradeStat(&e, 1, false) {
			t.Fatal("lower Сил refused")
		}
	}
	if e.FreeExp != 25 {
		t.Fatalf("points after lowering = %d, want 25", e.FreeExp)
	}
	if !LANTradeStat(&e, 0, true) || e.FreeExp != 0 { // Хар 25 → 26 costs 25 (favourite column: 19 ≤ 25 < 26)
		t.Fatalf("raise Хар: points %d", e.FreeExp)
	}
	if LANTradeStat(&e, 0, true) {
		t.Error("raise without points accepted")
	}
	got, err := ChoiceOf(e).Build()
	if err != nil {
		t.Fatal(err)
	}
	if got.BaseStats != e.BaseStats || got.FreeExp != 0 || got.Level != 1 || got.Flags4&7 != ClassChief ||
		got.Marksmanship != 100 || got.Skills != e.Skills {
		t.Errorf("built %+v", got)
	}
	// Left-over points are kept.
	h, err := LANHeroChoice{Class: ClassWarrior, Skill: SkillHealing, Stats: [4]int{3, 15, 0, 10}}.Build()
	if err != nil || h.FreeExp != 15 || h.Skills[0] != 100 {
		t.Errorf("Лов 3 → 0: %v, points %d", err, h.FreeExp)
	}
	for name, c := range map[string]LANHeroChoice{
		"too dear":      {Class: ClassWarrior, Skill: SkillIdentify, Stats: [4]int{3, 16, 3, 10}},
		"over cap":      {Class: ClassWarrior, Skill: SkillIdentify, Stats: [4]int{43, 0, 0, 0}},
		"negative":      {Class: ClassWarrior, Skill: SkillIdentify, Stats: [4]int{3, 15, -1, 10}},
		"bad class":     {Class: 1, Skill: SkillIdentify, Stats: [4]int{3, 15, 3, 10}},
		"bad skill":     {Class: ClassWarrior, Skill: 3, Stats: [4]int{3, 15, 3, 10}},
		"level 50 hack": {Class: ClassWarrior, Skill: SkillIdentify, Stats: [4]int{30, 30, 30, 30}},
	} {
		if _, err := c.Build(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if LANMaxParty(ClassWarrior, 3) != 1 || LANMaxParty(ClassChief, 25) != 3 || LANMaxParty(ClassChief, 28) != 3 || LANMaxParty(ClassChief, 15) != 2 {
		t.Error("LANMaxParty")
	}
}
