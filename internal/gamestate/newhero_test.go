package gamestate

import (
	"bytes"
	"testing"

	"konung1.5/internal/resfmt"
)

// TestNewHero checks the character-creation rules against konung.exe's
// tables and GAME.1: the template, the stat trade, «Макс. отряд» and the
// game start.
func TestNewHero(t *testing.T) {
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	e := NewHeroTemplate()
	if e.Model != HeroDragomir || e.PaletteOffset != 33*512 || e.Level != 10 || e.Flags4 != ClassWarrior ||
		e.BaseStats != [4]int{3, 15, 3, 10} || e.WeaponMode != WeaponAxe || ChosenSkill(e) != SkillIdentify {
		t.Fatalf("template = %+v", e)
	}
	if NewHeroHit(60, 120) != 0 || NewHeroHit(590, 90) != NewHeroClass || NewHeroHit(10, 10) != -1 {
		t.Error("hit test")
	}

	// Without free experience nothing can be bought; lowering Хар 3 → 2
	// refunds the cost of that point (2), which buys Лов 3 → 4 but not
	// the favoured Сил 15 → 16 (5).
	if TradeStat(&e, tb, 1, true) {
		t.Error("raised Сил with no free experience")
	}
	if !TradeStat(&e, tb, 0, false) || e.BaseStats[0] != 2 || e.Stats[0] != 2 || e.FreeExp != 2 {
		t.Fatalf("Хар − : %v free %d", e.BaseStats, e.FreeExp)
	}
	if TradeStat(&e, tb, 1, true) {
		t.Error("raised Сил for 2")
	}
	if !TradeStat(&e, tb, 2, true) || e.BaseStats[2] != 4 || e.FreeExp != 0 {
		t.Fatalf("Лов + : %v free %d", e.BaseStats, e.FreeExp)
	}
	for TradeStat(&e, tb, 0, false) {
	}
	if e.BaseStats[0] != 0 || e.FreeExp != 4 {
		t.Errorf("Хар lowered to %d, free %d", e.BaseStats[0], e.FreeExp)
	}
	// The level-10 cap (index 9) holds: a Воин's Сил stops at 95.
	e.FreeExp = 1 << 20
	for TradeStat(&e, tb, 1, true) {
	}
	if e.BaseStats[1] != 95 {
		t.Errorf("Сил capped at %d, want 95", e.BaseStats[1])
	}

	SelectClass(&e, ClassChief)
	if e.BaseStats != [4]int{25, 7, 3, 7} || e.FreeExp != 0 || MaxParty(e, tb) != 3 {
		t.Errorf("Вождь: %v free %d party %d", e.BaseStats, e.FreeExp, MaxParty(e, tb))
	}
	SelectCombatSkill(&e, true)
	SelectSkill(&e, SkillHealing)
	SelectHero(&e, HeroMichail)
	if e.WeaponMode != WeaponBow || e.Skills != [5]int{100, 5, 10, 2, 10} || e.NameIndex != 151 || e.PaletteOffset != 39*512 {
		t.Errorf("choices: %+v", e)
	}
	if got := NewHeroItems().Overlays(e, tb); len(got) == 0 {
		t.Error("no preview overlays")
	}

	data := load(t, "GAME.2")
	g, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	g.StartHero(e)
	h := g.Entities[g.Groups[0].First]
	if h.Level != 1 || h.Model != HeroMichail || h.Flags4 != ClassChief || h.Stats != e.BaseStats ||
		h.Marksmanship != 100 || h.Skills != e.Skills || g.Groups[0].Map != 7 {
		t.Errorf("started hero = %+v, map %d", h, g.Groups[0].Map)
	}
	out, err := g.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	g2, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if h2 := g2.Entities[h.Index]; h2.Stats != h.Stats || h2.Level != 1 || h2.Flags4 != ClassChief || bytes.Equal(out, data) {
		t.Errorf("encoded hero = %+v", h2)
	}
}
