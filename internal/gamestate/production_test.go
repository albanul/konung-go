package gamestate

import (
	"math/rand/v2"
	"testing"
)

// TestHealerProduces: the first settlement pass in «Родники» starts the
// healer's cycle and makes one Лечебный бальзам, Яд and Масло (slots 5–7).
func TestHealerProduces(t *testing.T) {
	tk := talkSetup(t)
	g := tk.G
	s := g.SettlementOnMap(1)
	g.SettlementPass(tk.T, tk.Hero)
	if s.i32(healerTimerOff) != healerCycle {
		t.Errorf("timer %d", s.i32(healerTimerOff))
	}
	for i, want := range map[int]int{5: HerbBalm, 6: 6, 7: 7} {
		it, ok := g.Item(s.healer(i))
		if !ok || it.Category != CatHerb || it.Type != want {
			t.Errorf("slot %d: %+v", i, it)
		}
		if c := g.Concentration(s.healer(i)); c < 2 || c > 2.1 { // skill 201, +3 after the Яд
			t.Errorf("slot %d concentration %v (healing 201)", i, c)
		}
	}
	if s.healer(19) != 0 || s.healer(0)+s.healer(1)+s.healer(2) != 0 {
		t.Error("more than one of each in the first pass")
	}
}

// TestSmithProduces: the smith starts with arrows (W = 1 → 20 passes at
// smithing 9) and stocks a stack of 30.
func TestSmithProduces(t *testing.T) {
	tk := talkSetup(t)
	g := tk.G
	s := g.SettlementOnMap(1)
	g.SettlementPass(tk.T, tk.Hero)
	if int8(s.Raw[smithJobOff]) != 0 || s.smith(0) != 0 {
		t.Fatalf("job %d", int8(s.Raw[smithJobOff]))
	}
	passes := 1
	for ; s.smith(0) == 0 && passes < 100; passes++ {
		g.SettlementPass(tk.T, tk.Hero)
	}
	it, _ := g.Item(s.smith(0))
	if it.Category != CatArrows || it.Type != 30 || it.Count != arrowStack {
		t.Errorf("slot 0 after %d passes: %+v", passes, it)
	}
	if int8(s.Raw[smithJobOff]) != -1 {
		t.Error("smith not idle after the job")
	}
}

// TestMerchantRestock: a map load fills the merchant's regenerated half
// with 18 items and replaces them on the next load; the healer gets jars.
func TestMerchantRestock(t *testing.T) {
	tk := talkSetup(t)
	g := tk.G
	s := g.SettlementOnMap(1)
	r := rand.New(rand.NewPCG(1, 2))
	g.Restock(1, tk.Hero, tk.T, r.IntN)
	old := s.merchant(14)
	for i := 14; i < merchantSlots; i++ {
		if _, ok := g.Item(s.merchant(i)); !ok || s.merchant(i) == 0 {
			t.Fatalf("slot %d empty", i)
		}
	}
	if it, _ := g.Item(s.healer(3)); s.healer(3) == 0 || it.Type != HerbJar {
		t.Error("no jar in the healer's first slot")
	}
	g.Restock(1, tk.Hero, tk.T, r.IntN)
	if s.merchant(14) == 0 {
		t.Fatal("slot 14 not refilled")
	}
	if it, _ := g.Item(old); old != s.merchant(14) && it.Category != 0xFF {
		t.Error("the old item was not freed")
	}
	// The trade window shows it all.
	tr := g.OpenTrade(tk.Hero, tk.Hero, 357, 1)
	if tr.Lists[TradeGoods][17] == 0 {
		t.Error("merchant goods missing in the trade lists")
	}
}
