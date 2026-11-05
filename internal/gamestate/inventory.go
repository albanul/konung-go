package gamestate

import "konung1.5/internal/resfmt"

// Moving items in the character window, ported from konung.exe. Clicking an
// item lifts it into the hand (the cursor changes to the item kind's
// sprite); clicking a fitting box or the bag strip puts it there; anything
// else, or an item the character can't use, sends it back where it came
// from. See docs/konung_character_window.md#moving-items.

// Messages shown in the status line (0x0042F9B8), CP866 strings in the exe.
const (
	MsgUnusable = 0x0045C17C // «Предмет не доступен для использования»
	MsgTooHeavy = 0x0045C57C // «Превышен вес»
	MsgBagFull  = 0x0045BD7C // «Мешок у тебя не резиновый!»
)

// Item sources of the hand (0x00A3E524 = the source array's length).
const (
	fromAmmo      = 1
	fromJewellery = 5
	fromEquipment = 6
	fromBag       = 32
)

// Cursor sprites (GRAPH.RES Type 5) by hand state, 0x00A3E790.
const (
	CursorPointer = 4 // empty hand: the plain blade
	// CursorDig is the shovel's mode (0x00415504 sets 0x00A3E790 = 7): the
	// hand is empty; the next left or right click resets it (0x0042296C,
	// 0x004238FB, 0x00423412) and continues as a normal click.
	CursorDig = 7
	// CursorHerb is the hand holding a herb or potion (category 9).
	CursorHerb = 8
	// cursorMisc is the hand holding jewellery or a misc item (starburst).
	cursorMisc = 9
)

// Map cursor sprites chosen by 0x00428A50 for the character under the mouse
// (0x00A3E6A0) or the cell under it.
const (
	CursorAttack  = 1 // crossed swords: a living selected member is in the combat stance
	CursorBlocked = 3 // red barred circle: a character without dialogue, or an occupied cell
	CursorTalk    = 5 // parchment: a character with a dialogue (+0xD0 ≠ 0xFF), no one in the stance
)

// EquipAmmo is the slot argument of the ammunition box (entity +0xCC) in
// ClickEquipment (0x0041E500 is called with 12, the arrows' category).
const EquipAmmo = 12

// Hand is the item being moved (0x00A3E520–0x00A3E5A0): the item, the
// cursor sprite, where it came from and a backup of that array so it can
// be returned. Box is the item in the window's in-hand box (hotspot 19,
// 0x00A3E7A0): it stays where it is (bag, equipment…), the box only
// points at it; herbs held in the hand are used on it (ClickBox).
type Hand struct {
	Item   int // 0x00A3E528
	Cursor int // 0x00A3E790; CursorPointer when empty
	Box    int // 0x00A3E7A0; 0 = empty, cleared when the window opens (0x004246D1)
	from   int
	equip  [6]int  // 0x00A3E52C
	bag    [32]int // 0x00A3E538: the bag without the item, the item appended
	extra  [5]int  // 0x00A3E578
}

// NewHand returns an empty hand.
func NewHand() Hand { return Hand{Cursor: CursorPointer} }

// Holding reports whether an item is in the hand (cursor mode ≠ 4, 7).
func (h *Hand) Holding() bool { return h.Cursor != CursorPointer && h.Cursor != CursorDig }

// Digging reports the shovel's dig mode.
func (h *Hand) Digging() bool { return h.Cursor == CursorDig }

// cursorFor is 0x00425104: the cursor sprite for an item category.
func cursorFor(category int) int {
	switch category {
	case 0, 2:
		return 10 // melee weapons: large sword
	case 1, 12:
		return 2 // bow, arrows
	case 3, 4, 5:
		return 0 // armour, helmet, shield
	case CatHerb:
		return CursorHerb // herbs: globe with hand
	}
	return cursorMisc // jewellery and the rest: starburst
}

// lift is 0x00425104 followed by 0x0041E210(e, from, item): set the cursor,
// remember the source and take the item out of it. A bag item is removed
// and the bag closed up; the backup keeps the item appended at the end.
func (g *Game) lift(h *Hand, e *Entity, from, item int) {
	if it, ok := g.Item(item); ok {
		h.Cursor = cursorFor(it.Category)
	}
	h.Item, h.from = item, from
	switch from {
	case fromEquipment:
		h.equip = e.Equipment
		for i, v := range e.Equipment {
			if v == item {
				e.Equipment[i] = 0
				break
			}
		}
	case fromJewellery:
		h.extra = e.Extra
		for i, v := range e.Extra {
			if v == item {
				e.Extra[i] = 0
				break
			}
		}
	case fromBag:
		i := 0
		for i < len(e.Bag)-1 && e.Bag[i] != item {
			i++
		}
		copy(e.Bag[i:], e.Bag[i+1:])
		e.Bag[len(e.Bag)-1] = 0
		h.bag = e.Bag
		last := len(h.bag) - 1
		for last > 0 && h.bag[last] == 0 {
			last--
		}
		if h.bag[last] != 0 {
			last++
		}
		if last < len(h.bag) {
			h.bag[last] = item
		}
	default:
		e.Quiver = 0
	}
}

// Return is 0x00429194: put the held item back where it came from. A bag
// item goes to the front of the bag.
//
// In dig mode the game's routine would restore the stale source of the last
// lifted item; the port only resets the cursor.
func (h *Hand) Return(e *Entity) {
	if !h.Holding() {
		h.Cursor = CursorPointer
		return
	}
	h.Cursor = CursorPointer
	switch h.from {
	case fromEquipment:
		e.Equipment = h.equip
	case fromBag:
		last := len(h.bag) - 1
		for last > 0 && h.bag[last] == 0 {
			last--
		}
		h.bag[last] = 0
		copy(e.Bag[1:], h.bag[:len(h.bag)-1])
		e.Bag[0] = h.Item
	case fromJewellery:
		e.Extra = h.extra
	default:
		e.Quiver = h.Item
	}
}

// BagInsert is 0x00424918: put item at the front of the bag, shifting the
// rest; false (and nothing stored) when all 32 slots are taken.
func BagInsert(e *Entity, item int) bool {
	full := true
	for _, v := range e.Bag {
		if v == 0 {
			full = false
			break
		}
	}
	if full {
		return false
	}
	copy(e.Bag[1:], e.Bag[:len(e.Bag)-1])
	e.Bag[0] = item
	return true
}

// ItemWeight is 0x0041B164: an item's weight in grams by category (arrows
// per piece; categories 10 and above 12 weigh nothing).
func (g *Game) ItemWeight(t *resfmt.ExeTables, item int) int {
	it, ok := g.Item(item)
	if !ok {
		return 0
	}
	switch c := it.Category; {
	case c <= 5:
		return t.ItemWeight(resfmt.ItemTableGear, it.Type)
	case c <= 8:
		return t.ItemWeight(resfmt.ItemTableJewel, it.Type)
	case c == 9:
		return t.ItemWeight(resfmt.ItemTableHerb, it.Type)
	case c == 11:
		return t.ItemWeight(resfmt.ItemTableMisc, it.Type)
	case c == 12:
		return t.ItemWeight(resfmt.ItemTableGear, it.Type) * it.Count
	}
	return 0
}

// HandSlot is the slot the hand box (hotspot 9, shared by the axe and the
// blade) stands for (0x00422CB4). Holding an item: slot 0 for category 0,
// else 2. Empty hand: the blade if there is no axe, the axe in axe mode,
// otherwise the blade when present.
func HandSlot(h *Hand, e Entity, g *Game) int {
	if h.Holding() {
		if it, _ := g.Item(h.Item); it.Category == 0 {
			return WeaponAxe
		}
		return WeaponBlade
	}
	switch {
	case e.Equipment[WeaponAxe] == 0:
		return WeaponBlade
	case e.WeaponMode == WeaponAxe:
		return WeaponAxe
	case e.Equipment[WeaponBlade] != 0:
		return WeaponBlade
	}
	return WeaponAxe
}

// ClickEquipment is 0x0041E500 for equipment slot 0–5 or EquipAmmo. With an
// empty hand it lifts the slot's item. Holding an item, the item goes in if
// its category equals the slot and the character can use it; the previous
// item goes to the front of the bag (when the bag is full it is lost, as in
// the game). Otherwise the item returns to its source. It returns a message
// VA or 0.
func (g *Game) ClickEquipment(h *Hand, e *Entity, t *resfmt.ExeTables, slot int) int {
	if !h.Holding() {
		item := e.Quiver
		from := fromAmmo
		if slot != EquipAmmo {
			item, from = e.Equipment[slot], fromEquipment
		}
		if item != 0 {
			g.lift(h, e, from, item)
			g.sound(g.equipSound(item))
		}
		return 0
	}
	it, _ := g.Item(h.Item)
	if e.Animated() || it.Category != slot {
		h.Return(e)
		return 0
	}
	if !g.CanUse(*e, t, it) {
		h.Return(e)
		return MsgUnusable
	}
	h.Cursor = CursorPointer
	g.sound(g.equipSound(h.Item))
	if slot == EquipAmmo {
		if e.Quiver != 0 {
			BagInsert(e, e.Quiver)
		}
		e.Quiver = h.Item
	} else {
		if old := e.Equipment[slot]; old != 0 {
			BagInsert(e, old)
		}
		e.Equipment[slot] = h.Item
	}
	return 0
}

// ClickJewellery is 0x0041E778 for jewellery box i (hotspots 14–18): box 0
// takes category 6, boxes 1–2 category 7, boxes 3–4 category 8 (by their
// positions a necklace, bracelets and rings). Otherwise like ClickEquipment.
func (g *Game) ClickJewellery(h *Hand, e *Entity, t *resfmt.ExeTables, box int) int {
	if !h.Holding() {
		if item := e.Extra[box]; item != 0 {
			g.lift(h, e, fromJewellery, item)
			g.sound(SoundJewellery)
		}
		return 0
	}
	it, _ := g.Item(h.Item)
	fits := map[int]bool{6: box == 0, 7: box == 1 || box == 2, 8: box == 3 || box == 4}[it.Category]
	if !fits || !g.CanUse(*e, t, it) {
		h.Return(e)
		return 0
	}
	h.Cursor = CursorPointer
	g.sound(SoundJewellery)
	if old := e.Extra[box]; old != 0 {
		BagInsert(e, old)
	}
	e.Extra[box] = h.Item
	return 0
}

// ClickBag is the bag strip click (0x0042342A) on bag index i: an empty
// hand lifts the item there; a held item is dropped into the bag (0x004246F8)
// at the front, if the weight limit and the bag allow, else it returns.
// The item in the in-hand box can't be lifted from the bag (0x004234A0).
func (g *Game) ClickBag(h *Hand, e *Entity, t *resfmt.ExeTables, i int) int {
	if !h.Holding() {
		if item := e.Bag[i&31]; item != 0 && item != h.Box {
			g.lift(h, e, fromBag, item)
		}
		return 0
	}
	return g.give(h, e, e, t)
}

// give is 0x004246F8(to): the held item goes to the front of to's bag if
// to can carry it and the bag has room; otherwise it returns to holder.
func (g *Game) give(h *Hand, holder, to *Entity, t *resfmt.ExeTables) int {
	if g.CarriedWeight(*to, t)+g.ItemWeight(t, h.Item) > MaxWeight(*to) {
		h.Return(holder)
		return MsgTooHeavy
	}
	if !BagInsert(to, h.Item) {
		h.Return(holder)
		return MsgBagFull
	}
	h.Cursor = CursorPointer
	return 0
}

// ClickBox is the in-hand box click (hotspot 19, 0x00422D50). With an empty
// box or an empty hand it is boxSwap (0x0041E96C). Holding a herb, the herb
// is used on the boxed item (UseHerb: mixing, or poison/oil on a weapon or
// arrows) and the hand empties; a held misc item goes to 0x00414E64 (UseMisc:
// the torch only). Anything else (and a refusal) shows «Предмет не доступен
// для использования» and returns the held item.
func (g *Game) ClickBox(h *Hand, e *Entity, t *resfmt.ExeTables) int {
	if h.Box == 0 || !h.Holding() {
		g.boxSwap(h, e)
		return 0
	}
	if h.Cursor == CursorHerb && g.UseHerb(e, t, h.Box, h.Item) ||
		h.Cursor == cursorMisc && g.UseMisc(e, h.Box, h.Item) {
		h.Cursor = CursorPointer
		return 0
	}
	h.Return(e)
	return MsgUnusable
}

// boxSwap is 0x0041E96C. Empty hand: the boxed item is lifted from where it
// is (ammunition, equipment, jewellery, else the bag) and the box empties.
// Holding an item: its source is restored from the backup (a bag item goes
// back to the end of the bag, not the front) and the box points at it.
func (g *Game) boxSwap(h *Hand, e *Entity) {
	if !h.Holding() {
		item := h.Box
		if item == 0 {
			return
		}
		from := fromBag
		switch {
		case e.Quiver == item:
			from = fromAmmo
		case contains(e.Equipment[:], item):
			from = fromEquipment
		case contains(e.Extra[:], item):
			from = fromJewellery
		}
		g.lift(h, e, from, item)
		h.Box = 0
		g.sound(SoundClick) // 0x0041EAA2 → 0x0041EB84
		return
	}
	h.Cursor = CursorPointer
	switch h.from {
	case fromEquipment:
		e.Equipment = h.equip
	case fromBag:
		e.Bag = h.bag
	case fromJewellery:
		e.Extra = h.extra
	default:
		e.Quiver = h.Item
	}
	h.Box = h.Item
	g.sound(SoundClick) // 0x0041EB55 → 0x0041EB84
}

func contains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// ClickPortrait is a left click on party member m's portrait (hotspots
// 30–39, 0x00422E21) while holder holds an item: on a dead member or on
// holder itself the item is used on m (useOn); another member gets it in
// the bag (give). With an empty hand the click selects (not handled here).
func (g *Game) ClickPortrait(h *Hand, holder, m *Entity, t *resfmt.ExeTables) int {
	if !h.Holding() {
		return 0
	}
	if m.Dead() || m == holder {
		return g.useOn(h, holder, m, t)
	}
	return g.give(h, holder, m, t)
}

// useOn is 0x0042054C(e): a held herb is used on character e (UseHerb with
// no target), a misc item through UseMisc (the torch); what is left (the
// empty jar, the torch) returns to the holder's bag front. A refusal, or
// any other item, shows «Предмет не доступен для использования» and returns
// the item.
func (g *Game) useOn(h *Hand, holder, e *Entity, t *resfmt.ExeTables) int {
	if h.Cursor == CursorHerb && g.UseHerb(e, t, 0, h.Item) || h.Cursor == cursorMisc && g.UseMisc(e, 0, h.Item) {
		if it, ok := g.Item(h.Item); ok && int8(it.Category) != -1 {
			h.Return(holder)
		}
		h.Cursor = CursorPointer
		return 0
	}
	h.Return(holder)
	return MsgUnusable
}

// UseBagItem is the right click on bag slot i (0x00423E26, jump table
// 0x00423F28 by category): gear (0–5) and arrows (12) are equipped at once
// (lift, then ClickEquipment with the category as slot); a necklace (6) goes
// to jewellery box 0; the shovel (misc type 0x18, 0x00415504) switches the
// hand to CursorDig and the caller closes the window. A herb or potion (9)
// is used on e at once (UseHerb, 0x0041DB64(e, 0, item)): it stays in the
// bag as «Пустая банка»; a refusal is silent. The torch lights the piles
// (UseMisc). Other misc items (0x00414E64) are not ported; bracelets, rings
// and category 10 do nothing. It returns a message VA or 0.
func (g *Game) UseBagItem(h *Hand, e *Entity, t *resfmt.ExeTables, i int) int {
	item := e.Bag[i&31]
	it, ok := g.Item(item)
	if !ok {
		return 0
	}
	switch c := it.Category; {
	case c <= 5 || c == 12:
		g.lift(h, e, fromBag, item)
		return g.ClickEquipment(h, e, t, c)
	case c == 6:
		g.lift(h, e, fromBag, item)
		return g.ClickJewellery(h, e, t, 0)
	case c == CatHerb:
		g.UseHerb(e, t, 0, item)
	case c == 11 && it.Type == torchType:
		g.UseMisc(e, 0, item)
	case c == 11 && it.Type == shovelType:
		h.Cursor = CursorDig
		g.sound(SoundUse) // 0x00415520
	}
	return 0
}
