package gamestate

import "testing"

func TestRaiseGame0(t *testing.T) {
	g, tb := game0(t)
	volk := g.Entities[g.Groups[0].First]
	if c := g.RaiseCosts(volk, tb); c != [RaiseRows]int{} {
		t.Errorf("start costs %v, want none (no free experience)", c)
	}
	// With free experience: Сил (favoured, 15 < 19) costs 5; Лов 10 is
	// above the level-1 cap.
	volk.FreeExp = 100
	c := g.RaiseCosts(volk, tb)
	if c[2] != 5 || c[3] != 0 {
		t.Fatalf("costs %v", c)
	}
	base, stat := volk.BaseStats[1], volk.Stats[1]
	if !g.Raise(&volk, tb, 2) {
		t.Fatal("Сил not raised")
	}
	if volk.BaseStats[1] != base+1 || volk.Stats[1] != stat+1 || volk.FreeExp != 95 || volk.Experience != 5 {
		t.Errorf("after Сил: base %d stat %d free %d exp %d", volk.BaseStats[1], volk.Stats[1], volk.FreeExp, volk.Experience)
	}
	if g.Raise(&volk, tb, 3) {
		t.Error("capped Лов raised")
	}
	// The level row: allowed only when every stat meets the next level's
	// minimum; raising it is free.
	if lv := volk.Level; g.RaiseCosts(volk, tb)[0] != 0 {
		free := volk.FreeExp
		g.Raise(&volk, tb, 0)
		if volk.Level != lv+1 || volk.FreeExp != free {
			t.Errorf("level %d→%d, free %d→%d", lv, volk.Level, free, volk.FreeExp)
		}
	} else if g.Raise(&volk, tb, 0) {
		t.Error("level raised although RaiseCost(0) is 0")
	}
	// The toggle: on spends the free experience at once (GainExperience(e, 0)),
	// off leaves it.
	volk.FreeExp = 50
	g.ToggleAutoLevel(&volk, tb)
	if !volk.AutoLevel() || volk.FreeExp >= 50 {
		t.Errorf("toggle on: auto %v, free %d", volk.AutoLevel(), volk.FreeExp)
	}
	free := volk.FreeExp
	g.ToggleAutoLevel(&volk, tb)
	if volk.AutoLevel() || volk.FreeExp != free {
		t.Errorf("toggle off: auto %v, free %d", volk.AutoLevel(), volk.FreeExp)
	}
}

// TestRaiseUsesBaseStats checks that 0x00410B08 reads the base values (the
// high words at +0x56…+0x64): an enchantment does not move the caps, and
// auto-level never pushes a stat past the cap of its level.
func TestRaiseUsesBaseStats(t *testing.T) {
	g, tb := game0(t)
	volk := g.Entities[g.Groups[0].First]
	volk.FreeExp = 100
	want := g.RaiseCost(volk, tb, 5)
	volk.Stats[1] = 200 // an effective Сил far above every cap
	if c := g.RaiseCost(volk, tb, 5); c != want || c == 0 {
		t.Errorf("Сил cost %d with effective 200, want %d (base %d)", c, want, volk.BaseStats[1])
	}
	// Auto-level with plenty of free experience: Воин (class 4).
	volk = g.Entities[g.Groups[0].First]
	start := volk.BaseStats
	volk.Flags3 |= 0x02
	g.GainExperience(&volk, tb, 1_000_000)
	if volk.Level < 2 || volk.Level > 13 {
		t.Fatalf("level %d", volk.Level)
	}
	for i, s := range [4]int{1, 2, 0, 3} { // BaseStats Хар, Сил, Лов, Вын → table stats Лов 0, Хар 1, Сил 2, Вын 3
		capV := int(tb.Bytes(0x0045B10C+4*0x68+s*0x1A+(volk.Level-1)*2, 2)[1])
		if v := volk.BaseStats[i]; v > max(capV, start[i]) {
			t.Errorf("base stat %d = %d past the level-%d cap %d", i, v, volk.Level, capV)
		}
	}
	t.Logf("level %d, base %v → %v, free %d", volk.Level, start, volk.BaseStats, volk.FreeExp)
}
