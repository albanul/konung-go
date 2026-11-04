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
)

// EquipAmmo is the slot argument of the ammunition box (entity +0xCC) in
// ClickEquipment (0x0041E500 is called with 12, the arrows' category).
const EquipAmmo = 12

// Hand is the item being moved (0x00A3E520–0x00A3E5A0): the item, the
// cursor sprite, where it came from and a backup of that array so it can
// be returned.
type Hand struct {
	Item   int // 0x00A3E528
	Cursor int // 0x00A3E790; CursorPointer when empty
	from   int
	equip  [6]int  // 0x00A3E52C
	bag    [32]int // 0x00A3E538: the bag without the item, the item appended
	extra  [5]int  // 0x00A3E578
}

// NewHand returns an empty hand.
func NewHand() Hand { return Hand{Cursor: CursorPointer} }

// Holding reports whether an item is in the hand (cursor mode ≠ 4).
func (h *Hand) Holding() bool { return h.Cursor != CursorPointer }

// cursorFor is 0x00425104: the cursor sprite for an item category.
func cursorFor(category int) int {
	switch category {
	case 0, 2:
		return 10 // melee weapons: large sword
	case 1, 12:
		return 2 // bow, arrows
	case 3, 4, 5:
		return 0 // armour, helmet, shield
	case 9:
		return 8 // herbs: globe with hand
	}
	return 9 // jewellery and the rest: starburst
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
func (h *Hand) Return(e *Entity) {
	if !h.Holding() {
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
	if old := e.Extra[box]; old != 0 {
		BagInsert(e, old)
	}
	e.Extra[box] = h.Item
	return 0
}

// ClickBag is the bag strip click (0x0042342A) on bag index i: an empty
// hand lifts the item there; a held item is dropped into the bag (0x004246F8)
// at the front, if the weight limit and the bag allow, else it returns.
func (g *Game) ClickBag(h *Hand, e *Entity, t *resfmt.ExeTables, i int) int {
	if !h.Holding() {
		if item := e.Bag[i&31]; item != 0 {
			g.lift(h, e, fromBag, item)
		}
		return 0
	}
	if g.CarriedWeight(*e, t)+g.ItemWeight(t, h.Item) > MaxWeight(*e) {
		h.Return(e)
		return MsgTooHeavy
	}
	if !BagInsert(e, h.Item) {
		h.Return(e)
		return MsgBagFull
	}
	h.Cursor = CursorPointer
	return 0
}
