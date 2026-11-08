package gamestate

import "testing"

// fillBag fills e's empty bag slots with weightless items.
func fillBag(g *Game, e *Entity) {
	for k := range e.Bag {
		if e.Bag[k] == 0 {
			i := g.newItem()
			g.setItem(i, 10, 0) // category 10 weighs nothing (0x0041B164)
			e.Bag[k] = i
		}
	}
}

// TestPileTradeSwap: the hero's bag is full, so the pick-up stops and the
// pile window swaps one bag item with one pile item.
func TestPileTradeSwap(t *testing.T) {
	tk := talkSetup(t)
	g := tk.G
	h := &g.Entities[tk.Hero]
	fillBag(g, h)
	a := g.newItem()
	g.setItem(a, CatQuest, 0)
	b := g.newItem()
	g.setItem(b, CatQuest, 1)
	pile, _ := g.DropItem(-1, 1, 50, 30, a)
	g.DropItem(pile, 1, 50, 30, b)
	msg, removed := g.PickUp(h, tk.T, pile, 0)
	if msg != MsgBagFull || removed {
		t.Fatalf("pick-up into a full bag: msg %x removed %v", msg, removed)
	}
	tr := g.OpenPileTrade(tk.Hero, tk.Hero, pile)
	if tr.Lists[TradeGoods][0] != a || tr.Lists[TradeGoods][1] != b {
		t.Fatalf("goods %v", tr.Lists[TradeGoods][:3])
	}
	nut := h.Bag[5]
	if !tr.Move(TradeGoods, 0) {
		t.Fatal("take failed")
	}
	if msg, ok, _ := g.Apply(tk.T, tr, true, 0); ok || msg != MsgBagFull {
		t.Fatalf("33 items accepted: %x", msg)
	}
	tr.Move(TradeBag, 5)
	if tr.Lists[TradeOffer][0] != nut {
		t.Fatal("offer")
	}
	msg, ok, removed := g.Apply(tk.T, tr, true, 0)
	if !ok || removed {
		t.Fatalf("apply: %x %v %v", msg, ok, removed)
	}
	if h.Bag[0] != a {
		t.Errorf("taken item not at the bag front: %v", h.Bag[:3])
	}
	if p := g.Piles[pile]; p.Items[0] != nut || p.Items[1] != b || p.Items[2] != 0 {
		t.Errorf("pile %v", p.Items[:4])
	}
}

// TestPileTradeEmpties: taking everything removes the pile; «Закрыть»
// changes nothing.
func TestPileTradeEmpties(t *testing.T) {
	tk := talkSetup(t)
	g := tk.G
	a := g.newItem()
	g.setItem(a, CatQuest, 0)
	pile, _ := g.DropItem(-1, 1, 50, 30, a)
	tr := g.OpenPileTrade(tk.Hero, tk.Hero, pile)
	bag := g.Entities[tk.Hero].Bag
	tr.Move(TradeGoods, 0)
	if _, ok, _ := g.Apply(tk.T, tr, false, 0); !ok || g.Entities[tk.Hero].Bag != bag || g.Piles[pile].Items[0] != a {
		t.Fatal("close changed the bag or the pile")
	}
	if _, ok, removed := g.Apply(tk.T, tr, true, 0); !ok || !removed || g.Entities[tk.Hero].Bag[0] != a {
		t.Fatalf("take all: ok %v removed %v", ok, removed)
	}
}

// TestBarterMoney: buying from an unposted trader with coins; the balance
// moves the hero's coins and Apply pays them.
func TestBarterMoney(t *testing.T) {
	tk := talkSetup(t)
	g := tk.G
	npc := 536 // Путята: no post, barters with his own bag
	n := &g.Entities[npc]
	i := g.newItem()
	g.setItem(i, CatHerb, HerbNut)
	BagInsert(n, i)
	h := &g.Entities[tk.Hero]
	h.Gold, n.Gold = 100, 0
	tr := g.OpenTrade(tk.Hero, tk.Hero, npc, 1)
	if tr.Mode != 6 || tr.Lists[TradeGoods][0] != i {
		t.Fatalf("mode %d goods %v", tr.Mode, tr.Lists[TradeGoods][:2])
	}
	tr.Move(TradeGoods, 0)
	g.Balance(tk.T, tr)
	price := g.BuyPrice(tr, g.ItemPrice(tk.T, i))
	if price <= 0 || tr.HeroPaid != price || tr.HeroMoney != 100-price {
		t.Fatalf("price %d paid %d money %d", price, tr.HeroPaid, tr.HeroMoney)
	}
	if _, ok, _ := g.Apply(tk.T, tr, true, 0); !ok {
		t.Fatal("apply failed")
	}
	if h.Gold != 100-price || n.Gold != price || h.Bag[0] != i || n.Bag[0] == i {
		t.Errorf("gold %d/%d bags %v %v", h.Gold, n.Gold, h.Bag[0], n.Bag[0])
	}
}

// TestTradeTooLittle: taking more than the hero can pay is refused.
func TestTradeTooLittle(t *testing.T) {
	tk := talkSetup(t)
	g := tk.G
	n := &g.Entities[536]
	i := g.newItem()
	g.setItem(i, CatHerb, HerbNut)
	BagInsert(n, i)
	g.Entities[tk.Hero].Gold = 0
	tr := g.OpenTrade(tk.Hero, tk.Hero, 536, 1)
	tr.Move(TradeGoods, 0)
	g.Balance(tk.T, tr)
	if msg, ok, _ := g.Apply(tk.T, tr, true, 0); ok || msg != MsgTooLittle {
		t.Fatalf("msg %x ok %v", msg, ok)
	}
}

// TestSellToShop: the healer's shop pays for the hero's balm and destroys
// it; the hero's trade skill gains nothing for a small deal.
func TestSellToShop(t *testing.T) {
	tk := talkSetup(t)
	g := tk.G
	h := &g.Entities[tk.Hero]
	h.Gold = 0
	tr := g.OpenTrade(tk.Hero, tk.Hero, 356, 1)
	if tr.Mode != 2 || tr.OtherMoney != 99999 {
		t.Fatalf("mode %d money %d", tr.Mode, tr.OtherMoney)
	}
	balm := h.Bag[1]
	tr.Move(TradeBag, 1)
	g.Balance(tk.T, tr)
	// Independent village: 112 · 0.5 · (1 + 0.002 · (hero − healer trade)) · 2/3.
	d := float64(h.Skills[2] - g.Entities[356].Skills[2])
	want := int(float32(float32((d*0.002+1)*56) * 2 / 3))
	if got := g.SellPrice(tr, g.ItemPrice(tk.T, balm)); got != want || tr.OtherPaid != want {
		t.Fatalf("sell price %d, want %d, paid %d", got, want, tr.OtherPaid)
	}
	if _, ok, _ := g.Apply(tk.T, tr, true, 0); !ok {
		t.Fatal("apply failed")
	}
	if it, _ := g.Item(balm); h.Gold != want || it.Category != 0xFF || h.Bag[1] == balm {
		t.Errorf("gold %d, balm category %d", h.Gold, it.Category)
	}
}

// TestSwitchShown: a portrait click in window 7 (0x00422F03) takes the
// offered items out of the old character's bag, keeps them offered and
// shows the new character's bag.
func TestSwitchShown(t *testing.T) {
	tk := talkSetup(t)
	g := tk.G
	h := &g.Entities[tk.Hero]
	m := &g.Entities[tk.Hero+1]
	h.Bag, m.Bag = [32]int{}, [32]int{}
	a, b, c := g.newItem(), g.newItem(), g.newItem()
	h.Bag[0], h.Bag[1], m.Bag[0] = a, b, c
	pile, _ := g.DropItem(-1, 1, 50, 30, g.newItem())
	tr := g.OpenPileTrade(tk.Hero, tk.Hero, pile)
	tr.Move(TradeBag, 0) // offer a
	tr.Scroll[TradeBag] = 1
	if g.SwitchShown(tr, tk.Hero) {
		t.Error("switched to the shown character")
	}
	if !g.SwitchShown(tr, m.Index) {
		t.Fatal("switch refused")
	}
	if h.Bag[0] != b || h.Bag[1] != 0 {
		t.Errorf("old bag %v, want the offer removed", h.Bag[:2])
	}
	if tr.Shown != m.Index || tr.Lists[TradeBag][0] != c || tr.Lists[TradeOffer][0] != a || tr.Scroll[TradeBag] != 0 {
		t.Errorf("shown %d bag %v offer %v scroll %d", tr.Shown, tr.Lists[TradeBag][:2], tr.Lists[TradeOffer][:2], tr.Scroll[TradeBag])
	}
}
