package gamestate

import (
	"math"

	"konung1.5/internal/resfmt"
)

// Trade is window 7 (0x00431D70): trading with a character (dialogue
// script 275, 0x00432FD0) or swapping with a pile on the ground (the pile
// window 0x004170A0, opened when a pick-up stops at the weight or bag
// limit). See docs/konung_trade.md.
//
// The four lists (0x0083DC4C, 4 × 32 words) are copies; nothing changes
// in the game until «Ok» applies the deal (Apply). A click on a cell moves
// the item between the paired lists 0 ↔ 1 and 2 ↔ 3.
type Trade struct {
	Lists  [4][TradeSlots]int // TradeBag, TradeOffer, TradeGoods, TradeTake
	Scroll [4]int             // 0x0083DD4C: first visible slot per list
	// Mode is 0x00A3E76C: the active pile's index in the pile window; for
	// a trade, the trader's post slot (2–4, from 0x0041182C), negated when
	// the village is the hero's (not independent), or 6 for a barter
	// with the trader's own bag.
	Mode  int
	Pile  int // index into Game.Piles in the pile window, −1 otherwise
	NPC   int // the trader (0x00A3E69C), −1 in the pile window
	Shown int // the character whose bag is traded (0x00A3E688)
	Hero  int // the hero (0x00A3E694), who pays
	Map   int
	orig  Pile // the pile as the window found it (for the herb respawn)
	// Money: what the hero can spend (0x00A3E7C8) and the other side's
	// (0x00A3E7CC); coins moved to balance the deal (0x00A3E82C hero,
	// 0x00A3E830 the other side).
	HeroMoney, OtherMoney int
	HeroPaid, OtherPaid   int
}

// TradeSlots is the size of each list.
const TradeSlots = 32

// The lists of window 7.
const (
	TradeBag   = iota // the shown character's bag
	TradeOffer        // what it gives
	TradeGoods        // the stock, the trader's bag or the pile
	TradeTake         // what it takes
)

// Trade messages.
const (
	MsgTooLittle = 0x0045BF7C // «Слишком мало даешь!»
)

// tradeShopStock is the stock of village post p (2 healer, 3 smith, 4
// merchant… by 0x0041182C's 1-based slot): settlement offset and length.
func tradeShopStock(p int) (off, n int) {
	switch p {
	case 2:
		return 0x412, 24
	case 3:
		return 0x444, 32
	}
	return 0x484, 23
}

// coin is «Неразменный пятак» (misc type 0x20): each in the hero's bag adds
// 10 to what the hero can spend; it is never paid away.
const coinType = 0x20

// OpenTrade is script 275 (0x00432FD0): the hero trades with npc. A posted
// village role (slots 2–4) sells the settlement's stock for post p (weapons
// are hidden while the village refuses armed strangers), with 99999 coins;
// anyone else barters with its own bag and gold.
func (g *Game) OpenTrade(hero, shown, npc, m int) *Trade {
	tr := &Trade{NPC: npc, Shown: shown, Hero: hero, Map: m, Pile: -1}
	h := g.Entities[hero]
	tr.HeroMoney = h.Gold
	for _, i := range h.Bag {
		if it, ok := g.Item(i); ok && i != 0 && it.Category == CatQuest && it.Type == coinType {
			tr.HeroMoney += 10
		}
	}
	copy(tr.Lists[TradeBag][:], g.Entities[shown].Bag[:])
	p, s := g.postSlot(npc), g.settlementOn(m)
	if p > 1 && p <= 4 && s != nil {
		tr.Mode = p
		if s.Flags&SettlementIndependent == 0 {
			tr.Mode = -p
		}
		tr.OtherMoney = 99999
		off, n := tradeShopStock(p)
		k := 0
		for j := range n {
			i := int(int16(uint16(s.Raw[off+2*j]) | uint16(s.Raw[off+2*j+1])<<8))
			if i == 0 {
				continue
			}
			if it, ok := g.Item(i); s.Flags&SettlementStrangers != 0 && ok && it.Category <= CatBlade {
				continue
			}
			tr.Lists[TradeGoods][k] = i
			if k++; k == TradeSlots {
				break
			}
		}
		return tr
	}
	tr.Mode = 6
	tr.OtherMoney = g.Entities[npc].Gold
	copy(tr.Lists[TradeGoods][:], g.Entities[npc].Bag[:])
	return tr
}

// postSlot is 0x0041182C: e's 1-based post slot in its settlement, 0 = none.
func (g *Game) postSlot(e int) int {
	if p, ok := g.PostOf(g.Entities[e]); ok {
		return p.Slot
	}
	return 0
}

// OpenPileTrade is the pile window 0x004170A0: shown swaps bag items with
// pile pile (its 22 items in the goods list); no money.
func (g *Game) OpenPileTrade(hero, shown, pile int) *Trade {
	tr := &Trade{NPC: -1, Shown: shown, Hero: hero, Pile: pile, Mode: pile, orig: g.Piles[pile]}
	tr.Map = g.Piles[pile].Map
	tr.HeroMoney = g.Entities[hero].Gold
	copy(tr.Lists[TradeBag][:], g.Entities[shown].Bag[:])
	copy(tr.Lists[TradeGoods][:], g.Piles[pile].Items[:])
	return tr
}

// SwitchShown is window 7's left click on the portrait of party member i
// (0x00422F03): nothing when i is already shown. Otherwise the offered
// items leave the old character's real bag (0x00421D24 each) and stay in
// the offer; i becomes the shown character, its bag the bag list, with the
// scroll at 0 (0x00422F5E). The caller checks that i can act.
func (g *Game) SwitchShown(tr *Trade, i int) bool {
	if i == tr.Shown || i < 0 || i >= len(g.Entities) {
		return false
	}
	old := &g.Entities[tr.Shown]
	for _, it := range tr.Lists[TradeOffer] {
		if it != 0 {
			bagRemove(old, it)
		}
	}
	tr.Shown = i
	tr.Lists[TradeBag] = g.Entities[i].Bag
	tr.Scroll[TradeBag] = 0
	return true
}

// Move is a click on slot s of list k (0x004232C0): a non-empty slot's item
// moves to the first free slot of the paired list (0 ↔ 1, 2 ↔ 3) and the
// list closes up. False when nothing moved.
func (tr *Trade) Move(k, s int) bool {
	if k < 0 || k > 3 {
		return false
	}
	s &= TradeSlots - 1
	src := &tr.Lists[k]
	if src[s] == 0 {
		return false
	}
	to := 1 - k
	if k >= 2 {
		to = 5 - k
	}
	dst := &tr.Lists[to]
	for j := range dst {
		if dst[j] == 0 {
			dst[j] = src[s]
			copy(src[s:], src[s+1:])
			src[TradeSlots-1] = 0
			return true
		}
	}
	return false
}

// ScrollBy moves list k's view one cell (hotspots k<<8 | 0x78/0x79): left
// stops at 0; right only while the slot just past the last visible cell
// holds an item (0x0041EB9C).
func (tr *Trade) ScrollBy(k, dir, visible int) {
	switch {
	case dir < 0:
		tr.Scroll[k] = max(0, tr.Scroll[k]-1)
	case tr.Scroll[k]+visible < TradeSlots && tr.Lists[k][tr.Scroll[k]+visible] != 0:
		tr.Scroll[k]++
	}
}

// ItemPrice is 0x0041B348: an item's price. Gear and jewellery: the type's
// price plus an identified enchantment's value, at least 1; arrows: price ×
// count; herbs: the type's price × 8, or (−price × concentration) × 8 for
// a negative price; other items: the type's price.
func (g *Game) ItemPrice(t *resfmt.ExeTables, i int) int {
	it, ok := g.Item(i)
	if !ok {
		return 0
	}
	v := t.ItemPrice(it.Table(), it.Type)
	switch c := it.Category; {
	case c <= 8:
		if it.Enchant >= 0 && !it.Flagged {
			v += t.Enchant(it.Enchant).Value
			if v <= 0 {
				v = 1
			}
		}
	case c == CatArrows:
		v *= it.Count
	case c == CatHerb:
		if v < 0 {
			v = int(math.Trunc(float64(-g.Concentration(i) * float32(v))))
		}
		v *= 8
	}
	return v
}

// sum adds the prices of list k (0x00A3E850 for the offer, 0x00A3E854 for
// the take).
func (g *Game) sum(t *resfmt.ExeTables, tr *Trade, k int) int {
	n := 0
	for _, i := range tr.Lists[k] {
		if i != 0 {
			n += g.ItemPrice(t, i)
		}
	}
	return n
}

// tradeDiff is the hero's trade skill minus the trader's (+0x42).
func (g *Game) tradeDiff(tr *Trade) int {
	return g.Entities[tr.Hero].Skills[2] - g.Entities[tr.NPC].Skills[2]
}

// roundPrice is the tail of 0x0041AEA0 / 0x0041B65C: 0 stays 0, anything
// below 1 becomes 1, then truncation.
func roundPrice(f float32) int {
	if f == 0 {
		return 0
	}
	if f < 1 {
		f = 1
	}
	return int(math.Trunc(float64(f)))
}

// SellPrice is 0x0041AEA0: what the trader pays for goods worth v:
// v · 0.5 · (1 + 0.002 · skill difference), × 2/3 in an independent
// village's shop or a barter (Mode > 0). The pile window takes v as is.
func (g *Game) SellPrice(tr *Trade, v int) int {
	if tr.NPC < 0 {
		return v
	}
	f := float32((float64(g.tradeDiff(tr))*0.002 + 1) * (float64(v) * 0.5))
	if tr.Mode > 0 {
		f = float32(float64(f) * (2.0 / 3.0))
	}
	return roundPrice(f)
}

// BuyPrice is 0x0041B65C: what the hero pays for goods worth v:
// v · (1 − 0.002 · skill difference), × 1.5 when Mode > 0.
func (g *Game) BuyPrice(tr *Trade, v int) int {
	if tr.NPC < 0 {
		return v
	}
	f := float32((1 - float64(g.tradeDiff(tr))*0.002) * float64(v))
	if tr.Mode > 0 {
		f = float32(float64(f) * 1.5)
	}
	return roundPrice(f)
}

// Totals are what each side puts in: the hero's offer at the sell price
// plus its coins, and the trader's goods at the buy price plus its coins.
func (g *Game) Totals(t *resfmt.ExeTables, tr *Trade) (hero, other int) {
	return g.SellPrice(tr, g.sum(t, tr, TradeOffer)) + tr.HeroPaid, g.BuyPrice(tr, g.sum(t, tr, TradeTake)) + tr.OtherPaid
}

// Balance is the coin balancing of the window draw (0x00432041): when the
// hero's side is worth more, the hero's coins come back first, then the
// trader pays coins until even or out of money; the other way round
// likewise. No money moves in the pile window.
func (g *Game) Balance(t *resfmt.ExeTables, tr *Trade) {
	if tr.NPC < 0 {
		return
	}
	a, b := g.Totals(t, tr)
	if a > b {
		for tr.HeroPaid != 0 && a > b {
			a--
			tr.HeroMoney++
			tr.HeroPaid--
		}
		for tr.OtherMoney != 0 && a > b {
			b++
			tr.OtherMoney--
			tr.OtherPaid++
		}
		return
	}
	for tr.OtherPaid != 0 && a < b {
		b--
		tr.OtherMoney++
		tr.OtherPaid--
	}
	for tr.HeroMoney != 0 && a < b {
		a++
		tr.HeroMoney--
		tr.HeroPaid++
	}
}

// Apply is 0x00420634: «Ok» (accept) or «Закрыть». The shown character's
// new bag (with accept: the take, then the kept bag; otherwise: the offer
// back, then the kept bag) must fit 32 slots and the weight limit
// («Мешок у тебя не резиновый!», «Превышен вес»); a trade must not
// leave the hero's side short («Слишком мало даешь!»). On failure nothing
// changes and the window stays (ok false).
//
// Accepting pays the hero the coin difference (gold never below 0) and:
//
//   - pile window: the pile gets the offer and the remaining goods,
//     interleaved, at most 22; an emptied pile is removed (0x0041093C).
//     It reports removed;
//   - trade: both trade skills grow by (sell price + coins) / 1024, at
//     most 100. An unposted trader (barter) gets the coin difference and
//     its new bag (offer and remaining goods, interleaved, at most 32;
//     closing gives it back the take instead of the offer). For a posted
//     trader the take leaves the post's stock and the offer is destroyed.
//     (The game takes this path for the elder and the voevoda too, whose
//     barter lists come from their bag; the port does the same.)
func (g *Game) Apply(t *resfmt.ExeTables, tr *Trade, accept bool, now int) (msg int, ok, removed bool) {
	shown := &g.Entities[tr.Shown]
	src := tr.Lists[TradeOffer]
	if accept {
		src = tr.Lists[TradeTake]
	}
	var bag []int
	for _, l := range [][TradeSlots]int{src, tr.Lists[TradeBag]} {
		for _, i := range l {
			if i != 0 {
				bag = append(bag, i)
			}
		}
	}
	if len(bag) > TradeSlots {
		return MsgBagFull, false, false
	}
	probe := *shown
	probe.Bag = [32]int{}
	copy(probe.Bag[:], bag)
	if g.CarriedWeight(probe, t) > MaxWeight(*shown) {
		return MsgTooHeavy, false, false
	}
	if accept && tr.NPC >= 0 {
		if a, b := g.Totals(t, tr); a < b {
			return MsgTooLittle, false, false
		}
	}
	if accept {
		h := &g.Entities[tr.Hero]
		h.Gold = max(0, h.Gold+tr.OtherPaid-tr.HeroPaid)
	}
	shown.Bag = probe.Bag
	interleave := func(a, b [TradeSlots]int, limit int) []int {
		var out []int
		for j := range TradeSlots {
			for _, i := range []int{a[j], b[j]} {
				if i != 0 && len(out) < limit {
					out = append(out, i)
				}
			}
		}
		return out
	}
	if tr.NPC < 0 {
		if !accept {
			return 0, true, false
		}
		p := &g.Piles[tr.Pile]
		p.Items = [pileMaxItems]int{}
		copy(p.Items[:], interleave(tr.Lists[TradeOffer], tr.Lists[TradeGoods], pileMaxItems))
		if p.Items[0] == 0 {
			g.removePile(tr.Pile, tr.orig, now)
			return 0, true, true
		}
		return 0, true, false
	}
	n := &g.Entities[tr.NPC]
	if accept {
		gain := (g.SellPrice(tr, g.sum(t, tr, TradeOffer)) + tr.HeroPaid) / 1024
		h := &g.Entities[tr.Hero]
		h.Skills[2] = min(h.Skills[2]+gain, 100)
		n.Skills[2] = min(n.Skills[2]+gain, 100)
	}
	p := g.postSlot(tr.NPC)
	if p == 0 {
		if accept {
			n.Gold += tr.HeroPaid - tr.OtherPaid
		}
		give := tr.Lists[TradeOffer]
		if !accept {
			give = tr.Lists[TradeTake]
		}
		n.Bag = [32]int{}
		copy(n.Bag[:], interleave(give, tr.Lists[TradeGoods], TradeSlots))
		return 0, true, false
	}
	if !accept {
		return 0, true, false
	}
	if s := g.settlementOn(tr.Map); s != nil {
		off, cnt := tradeShopStock(p)
		for _, i := range tr.Lists[TradeTake] {
			if i == 0 {
				continue
			}
			for j := range cnt {
				o := off + 2*j
				if int(int16(uint16(s.Raw[o])|uint16(s.Raw[o+1])<<8)) == i {
					s.Raw[o], s.Raw[o+1] = 0, 0
					break
				}
			}
		}
	}
	for _, i := range tr.Lists[TradeOffer] {
		if i != 0 {
			g.items[i*itemStride] = 0xFF
		}
	}
	return 0, true, false
}
