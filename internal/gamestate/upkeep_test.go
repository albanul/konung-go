package gamestate

import (
	"math/rand/v2"
	"testing"

	"konung1.5/internal/resfmt"
)

func game0(t *testing.T) (*Game, *resfmt.ExeTables) {
	t.Helper()
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	return g, tb
}

func TestUpkeepPoisonAndRespawn(t *testing.T) {
	g, tb := game0(t)
	if c, p := tb.RespawnRule(0x43); c != 4800 || p != 12000 {
		t.Fatalf("respawn rule 0x43 = %d/%d, want 4800/12000", c, p)
	}
	// Group 4 (ants, kind 0x41): target 5 of 12, countdown 3600, debt 720.
	gr := g.Groups[4]
	if gr.Target != 5 || gr.MaxPop != 12 || gr.Countdown != 3600 || gr.Debt != 720 {
		t.Fatalf("group 4 respawn fields %+v", gr)
	}
	g.Groups[4].Countdown = 0
	ant := gr.First
	g.Entities[ant].Poison = 1000
	var died []int
	g.Upkeep(tb, gr.Map, func(int) bool { return false }, func(i int) { died = append(died, i) })
	g2 := g.Groups[4]
	if g2.Target != 6 || g2.Countdown != 3600-1 || g2.Debt != 719 {
		t.Errorf("after the countdown: target %d countdown %d debt %d, want 6, 3599, 719", g2.Target, g2.Countdown, g2.Debt)
	}
	if e := g.Entities[ant]; e.HP != HPFull-1000 || e.Poison != 1000 {
		t.Errorf("poisoned ant: HP %d poison %d", e.HP, e.Poison)
	}
	g.Upkeep(tb, gr.Map, func(int) bool { return false }, func(i int) { died = append(died, i) })
	if len(died) != 1 || died[0] != ant || g.Entities[ant].HP != 0 {
		t.Errorf("second poison tick: died %v, HP %d", died, g.Entities[ant].HP)
	}
}

func TestEnterMapPlacesAndRespawns(t *testing.T) {
	g, tb := game0(t)
	m := g.Groups[4].Map
	// Make every creature group on the map owe one member; the first one is
	// skipped by the game's loop.
	var groups []int
	for gi := 1; gi < len(g.Groups); gi++ {
		if gr := &g.Groups[gi]; gr.CreatureGroup() && gr.Map == m && gr.Kind <= 0x50 {
			groups = append(groups, gi)
			gr.Target, gr.Debt = min(gr.Count+1, gr.MaxPop), -1
		}
	}
	if len(groups) < 2 {
		t.Skip("need two creature groups on the map")
	}
	before := map[int]int{}
	for _, gi := range groups {
		before[gi] = g.Groups[gi].Count
	}
	taken := map[[2]int]bool{}
	rng := rand.New(rand.NewPCG(1, 1)).IntN
	g.EnterMap(m, tb, rng, func(r, c int) bool { return taken[[2]int{r, c}] }, func(r, c int) { taken[[2]int{r, c}] = true })
	for i, gi := range groups {
		gr := g.Groups[gi]
		cost, _ := tb.RespawnRule(int(gr.Kind))
		want, debt := before[gi]+1, cost-1 // one member paid; a surplus stays
		if i == 0 {
			want, debt = before[gi], -1 // the first group on the map is skipped
		}
		if gr.Count != want || gr.Debt != debt || gr.Alarm != AlarmCalm {
			t.Errorf("group %d: count %d (was %d, want %d), debt %d, alarm %#x", gi, gr.Count, before[gi], want, gr.Debt, gr.Alarm)
		}
		a, _ := gr.WanderArea()
		for k := 0; k < gr.Count; k++ {
			if e := g.Entities[gr.First+k]; !a.Contains(e.Row, e.Col) && a.RowMax-a.RowMin >= 4 {
				t.Errorf("group %d member %d at (%d,%d), outside %+v", gi, k, e.Row, e.Col, a)
			}
		}
		if want > before[gi] {
			n := g.Entities[gr.First+gr.Count-1]
			if n.HP != HPFull || n.Flags4 != gr.Kind || n.Dialogue != NoDialogue || !n.InStance() {
				t.Errorf("group %d new member %+v", gi, n)
			}
		}
	}
}

func TestLootAndLevels(t *testing.T) {
	g, tb := game0(t)
	// A creature with gold: the gold and a bag item go to a new pile.
	v := g.Entities[921]
	v.Gold, v.Bag[0] = 7, 5
	curios := 0
	piles := len(g.Piles)
	at, ok := g.DropLoot(&v, 0, 0, 1, -1, true, &curios, tb, func(int) int { return 0 })
	if !ok || at != piles || g.Piles[at].Gold != 7 || g.Piles[at].Items[0] != 5 || v.Bag[0] != 0 {
		t.Fatalf("loot pile %d (%v): %+v; bag %v", at, ok, g.Piles[at], v.Bag[0])
	}
	// Nothing to drop and the roll above 30: no pile.
	empty := Entity{Index: 922, Flags4: 0x41}
	if _, ok := g.DropLoot(&empty, 0, 0, 1, -1, true, &curios, tb, func(n int) int { return n - 1 }); ok {
		t.Error("an empty creature dropped a pile")
	}
	// Level costs: Volk (race (kind 5 & 7) − 1 = 4, favoured stat id 5 Сил).
	volk := g.Entities[g.Groups[0].First]
	volk.FreeExp = 1000
	if c := g.RaiseCost(volk, tb, 5); c != 5 { // Сил 15 < 19 on the favoured row
		t.Errorf("Volk Сил cost %d, want 5", c)
	}
	if c := g.RaiseCost(volk, tb, 2); c != 0 { // Лов 10 is above the level-1 cap 7 of the other row
		t.Errorf("Volk Лов cost %d, want 0 (capped)", c)
	}
	volk.HitBonusOn = 1
	if g.RaiseCost(volk, tb, 5) != 0 {
		t.Error("raise allowed during a potion")
	}
}

func TestVillageAnger(t *testing.T) {
	g, tb := game0(t)
	g.InitSettlementFlags(0)
	st := g.SettlementOnMap(1)
	if st == nil || st.Flags != SettlementIndependent {
		t.Fatalf("KONUNG.1 settlement flags %+v", st)
	}
	gr := g.Groups[st.Group]
	a, _ := gr.WanderArea()
	volk := g.Entities[g.Groups[0].First]
	volk.Row, volk.Col = (a.RowMin+a.RowMax)/2, (a.ColMin+a.ColMax)/2
	g.VillageAlarm(st, []Entity{volk})
	if g.Groups[st.Group].Alarm != AlarmCalm {
		t.Fatal("a peaceful village alarmed by Volk")
	}
	g.Anger(st)
	if st.Flags != 0x15 {
		t.Fatalf("angry flags %#x, want 0x15", st.Flags)
	}
	g.Groups[st.Group].Alarm = AlarmCalm
	g.VillageAlarm(st, []Entity{volk})
	if g.Groups[st.Group].Alarm != AlarmAttack {
		t.Fatal("a hostile village ignores Volk inside it")
	}
	weak := volk
	if g.Surrenders(st, []Entity{weak}, tb) {
		t.Error("the village surrendered to Volk alone")
	}
	strong := volk
	strong.Stats, strong.BaseStats = [4]int{1000, 1000, 1000, 1000}, [4]int{1000, 1000, 1000, 1000}
	if !g.Surrenders(st, []Entity{strong}, tb) || st.Flags&SettlementSurrendered == 0 || st.Flags&0x15 != 0 {
		t.Errorf("no surrender to an overwhelming party: flags %#x", st.Flags)
	}
}
