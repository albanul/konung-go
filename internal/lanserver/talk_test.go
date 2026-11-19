package lanserver

import (
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
)

// Village 7's posts in GAME.RES: healer 17 (dialogue 101), merchant 18
// (103), smith 19 (102).
const (
	healer7   = 17
	merchant7 = 18
)

// nextTo puts slot's hero next to entity npc on its map.
func nextTo(t *testing.T, w *World, slot, npc int) {
	t.Helper()
	m := w.Game.Groups[w.Game.Entities[npc].Group].Map
	n := w.Maps[m].ByEntity[npc]
	if n == nil {
		t.Fatalf("entity %d not on map %d", npc, m)
	}
	if !w.MoveTo(slot, m, mapdata.Cell{Row: n.Cell.Row + 2, Col: n.Cell.Col}) {
		t.Fatal("MoveTo")
	}
}

func hero(w *World, slot int) *gamestate.Entity {
	_, e := w.Maps[w.players[slot]].Member(slot)
	return e
}

// explore follows reply paths (depth ≤ 4) of npc's dialogue from a fresh
// start until done reports true.
func explore(w *World, slot, npc int, done func() bool) bool {
	_, ok := explorePath(w, slot, npc, done)
	return ok
}

// explorePath is explore returning the replies chosen.
func explorePath(w *World, slot, npc int, done func() bool) ([]int, bool) {
	var path []int
	var try func(depth int) bool
	try = func(depth int) bool {
		st := w.state(slot)
		st.talk, st.trade = nil, nil
		w.Talk(slot, npc)
		for _, k := range path {
			if st.talk == nil {
				return false
			}
			w.Reply(slot, k)
		}
		if done() {
			return true
		}
		if st.talk == nil || depth == 4 {
			return false
		}
		n := len(st.talk.Replies)
		for k := range n {
			path = append(path, k)
			if try(depth + 1) {
				return true
			}
			path = path[:len(path)-1]
		}
		return false
	}
	ok := try(0)
	return path, ok
}

// A dialogue opens only for a character of the world on the hero's map,
// out of the stance, within range; a reply not offered is ignored; the
// merchant's dialogue opens the trade with the slot's stock; buying and
// selling move items and gold by the trade rules; lists with items the
// window does not hold change nothing; the removed items are not sold.
func TestTalkAndTrade(t *testing.T) {
	w := load(t)
	const slot = 0
	if _, _, err := w.AddPlayer(slot, 2, nil); err != nil {
		t.Fatal(err)
	}
	st := w.state(slot)

	// Far away: nothing.
	w.Talk(slot, merchant7)
	if st.talk != nil {
		t.Fatal("talked from the start")
	}
	nextTo(t, w, slot, merchant7)
	// Another player's hero, a missing character: nothing.
	w.Talk(slot, slot)
	w.Talk(slot, 9999)
	if st.talk != nil {
		t.Fatal("talked to a hero or nobody")
	}
	// In the stance: nothing.
	w.Stance(slot, true)
	w.Talk(slot, merchant7)
	if st.talk != nil {
		t.Fatal("talked in the stance")
	}
	w.Stance(slot, false)
	w.Talk(slot, merchant7)
	if st.talk == nil || len(st.talk.Replies) == 0 {
		t.Fatal("no dialogue with the merchant")
	}
	if d := w.Dialogue(slot); !d.Open || d.NPC != merchant7 || len(d.Replies) != len(st.talk.Replies) {
		t.Fatalf("dialogue payload %+v", d)
	}
	before := st.talk.Node
	w.Reply(slot, 99)
	if st.talk == nil || st.talk.Node != before {
		t.Fatal("a reply not offered changed the dialogue")
	}

	// The slot's merchant stock was made on entry.
	si := w.settlementOf(7)
	stocked := 0
	for j := range 32 {
		if st := w.shops[si][slot].Merchant[2*j:]; st[0] != 0 || st[1] != 0 {
			stocked++
		}
	}
	if stocked == 0 {
		t.Fatal("the merchant has no stock")
	}
	// A removed item in the stock is not offered.
	dup := w.Game.SetItemKind(0, gamestate.CatQuest, gamestate.MiscDuplicator)
	w.shops[si][slot].Merchant[0], w.shops[si][slot].Merchant[1] = byte(dup), byte(dup>>8)

	hero(w, slot).Gold = 50000
	if !explore(w, slot, merchant7, func() bool { return st.trade != nil }) {
		t.Fatal("no reply of the merchant opens the trade")
	}
	tr := st.trade
	if tr.Mode != 3 && tr.Mode != -3 {
		t.Fatalf("trade mode %d, want the merchant's ±3", tr.Mode)
	}
	for _, i := range tr.Lists[gamestate.TradeGoods] {
		if i == dup {
			t.Fatal("the duplicator is for sale")
		}
	}
	pw := w.PileWindow(slot)
	if !pw.Open || pw.Trader == nil || pw.Trader.NPC != merchant7 || len(pw.Items) == 0 {
		t.Fatalf("trade payload %+v", pw)
	}

	// Buy the first good.
	good := tr.Lists[gamestate.TradeGoods][0]
	if good == 0 {
		t.Fatal("no goods")
	}
	buy := *tr
	buy.Move(gamestate.TradeGoods, 0)
	w.Game.Balance(w.Tables, &buy)
	cost := buy.HeroPaid
	gold := hero(w, slot).Gold
	lists := buy.Lists
	w.Item(slot, netplay.OrderPayload{Kind: netplay.OrderItem, Item: netplay.ItemTrade, On: true, Lists: &lists})
	e := hero(w, slot)
	if e.Bag[0] != good || e.Gold != gold-cost || cost <= 0 {
		t.Fatalf("bought: bag %v gold %d → %d, cost %d", e.Bag[:3], gold, e.Gold, cost)
	}
	if st.trade == nil {
		t.Fatal("the trade closed after «Ok»")
	}
	for _, i := range st.trade.Lists[gamestate.TradeGoods] {
		if i == good {
			t.Fatal("the bought item is still in the stock")
		}
	}

	// Lists with a foreign item: nothing happens.
	bad := st.trade.Lists
	bad[gamestate.TradeTake][0] = dup
	gold = e.Gold
	w.Item(slot, netplay.OrderPayload{Kind: netplay.OrderItem, Item: netplay.ItemTrade, On: true, Lists: &bad})
	if e := hero(w, slot); e.Gold != gold || e.Bag[1] != 0 {
		t.Fatalf("a foreign list applied: gold %d → %d, bag %v", gold, e.Gold, e.Bag[:3])
	}

	// Sell it back: the merchant pays; the item leaves the bag (destroyed;
	// the restock after the deal may reuse its record).
	sell := *st.trade
	sell.Move(gamestate.TradeBag, 0)
	w.Game.Balance(w.Tables, &sell)
	paid := sell.OtherPaid
	lists = sell.Lists
	gold = hero(w, slot).Gold
	w.Item(slot, netplay.OrderPayload{Kind: netplay.OrderItem, Item: netplay.ItemTrade, On: true, Lists: &lists})
	if e := hero(w, slot); e.Bag[0] != 0 || e.Gold != gold+paid || paid <= 0 {
		t.Fatalf("sold: bag %v gold %d → %d, paid %d", e.Bag[:3], gold, e.Gold, paid)
	}
	// «Закрыть» closes.
	w.Item(slot, netplay.OrderPayload{Kind: netplay.OrderItem, Item: netplay.ItemTrade})
	if st.trade != nil {
		t.Fatal("«Закрыть» left the trade open")
	}
}

// The healer heals for money (334); the quest flags are the player's own.
func TestHealerHeals(t *testing.T) {
	w := load(t)
	const slot = 0
	if _, _, err := w.AddPlayer(slot, 2, nil); err != nil {
		t.Fatal(err)
	}
	nextTo(t, w, slot, healer7)
	e := hero(w, slot)
	e.Gold, e.HP = 50000, 300
	if !explore(w, slot, healer7, func() bool { return hero(w, slot).HP == gamestate.HPFull }) {
		t.Fatal("no reply of the healer heals")
	}
	if g := hero(w, slot).Gold; g >= 50000 {
		t.Fatalf("healed for free: gold %d", g)
	}
}

// The shops produce per player slot: after some settlement passes the
// healer of village 7 has potions in slot 0's and slot 5's stock alike.
func TestShopsPerSlot(t *testing.T) {
	w := load(t)
	for range 16 * 40 {
		w.Step()
	}
	si := w.settlementOf(7)
	count := func(k int) int {
		n := 0
		for j := 3; j < 28; j++ {
			if b := w.shops[si][k].Healer[2*j:]; b[0] != 0 || b[1] != 0 {
				n++
			}
		}
		return n
	}
	if count(0) == 0 || count(5) == 0 {
		t.Fatalf("healer stock: slot 0 %d, slot 5 %d", count(0), count(5))
	}
}

// Over the network: OrderTalk opens the dialogue (MsgDialogue), replies
// lead to the merchant's trade (MsgPileWindow with the trader), «Ok» with a
// bought item puts it into the bag (MsgInventory).
func TestTalkOverNetwork(t *testing.T) {
	// The reply path to the trade, found on a world of its own.
	w0 := load(t)
	w0.AddPlayer(0, 2, nil)
	nextTo(t, w0, 0, merchant7)
	hero(w0, 0).Gold = 50000
	path, ok := explorePath(w0, 0, merchant7, func() bool { return w0.state(0).trade != nil })
	if !ok {
		t.Fatal("no path to the trade")
	}

	w := load(t)
	ss := startSession(t, w)
	a := enter(t, ss, "Ivar", 2)
	ss.host.mu.Lock()
	nextTo(t, w, a.at.Slot, merchant7)
	hero(w, a.at.Slot).Gold = 50000
	ss.host.mu.Unlock()

	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderTalk, Target: merchant7})
	waitUntil(t, "the dialogue", func() bool { d := a.dialogue(); return d.Open && d.NPC == merchant7 })
	for _, k := range path {
		a.c.Order(netplay.OrderPayload{Kind: netplay.OrderReply, Index: k})
	}
	var pw netplay.PileWindowPayload
	waitUntil(t, "the trade window", func() bool {
		_, _, pw = a.items()
		return pw.Open && pw.Trader != nil
	})
	if d := a.dialogue(); d.Open {
		t.Fatal("the dialogue stayed open under the trade")
	}
	good := pw.Lists[gamestate.TradeGoods][0]
	if _, ok := pw.Items[good]; !ok || good == 0 {
		t.Fatalf("no record of the good %d", good)
	}
	lists := pw.Lists
	lists[gamestate.TradeTake][0], lists[gamestate.TradeGoods] = good, [gamestate.TradeSlots]int{}
	copy(lists[gamestate.TradeGoods][:], pw.Lists[gamestate.TradeGoods][1:])
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderItem, Item: netplay.ItemTrade, On: true, Lists: &lists})
	waitUntil(t, "the bought item", func() bool {
		inv, _, _ := a.items()
		return inv.Hero.Bag[0] == good
	})
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderItem, Item: netplay.ItemClose})
	waitUntil(t, "the window closed", func() bool { _, _, pw := a.items(); return !pw.Open })
}

func (v *viewer) dialogue() netplay.DialoguePayload {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.dlg
}
