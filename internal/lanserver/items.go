package lanserver

import (
	"encoding/binary"
	"math"
	"slices"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

// Items on the server (netplay/items.go): every player's hand, its pile
// window and the messages of its item actions; the item clicks a client
// sends (OrderItem) applied with the single-player rules; what the client
// is sent (its hero's items, the piles on its map, the pile window); and
// the items kept between sessions (SAVITEMS.DAT, lanstore).

// itemState is a player's side of the items.
type itemState struct {
	hand gamestate.Hand
	pile *gamestate.Trade // the open pile window, nil when closed
	msgs [][]byte         // status-line messages for the client
}

// state is slot's item state (made on first use).
func (w *World) state(slot int) *itemState {
	if w.items == nil {
		w.items = map[int]*itemState{}
	}
	st := w.items[slot]
	if st == nil {
		st = &itemState{hand: gamestate.NewHand()}
		w.items[slot] = st
	}
	return st
}

// say queues a status-line message for slot's client.
func (w *World) say(slot int, text []byte) {
	if len(text) > 0 {
		st := w.state(slot)
		st.msgs = append(st.msgs, append([]byte(nil), text...))
	}
}

// TakeMessages returns slot's queued messages.
func (w *World) TakeMessages(slot int) [][]byte {
	st := w.state(slot)
	m := st.msgs
	st.msgs = nil
	return m
}

// openPile is the pile window for pile i, opened by slot's pick-up on map
// n (world.Arrive: over the weight or bag limit, 0x0041122A).
func (w *World) openPile(n, slot, i int) {
	w.Maps[n].StoreWorld()
	w.state(slot).pile = w.Game.OpenPileTrade(slot, slot, i)
}

// closeItems ends what slot holds: the held item goes back, the pile window
// closes (logout, death, a map change).
func (w *World) closeItems(slot int) {
	st := w.state(slot)
	st.pile = nil
	if m, ok := w.players[slot]; ok {
		if _, e := w.Maps[m].Member(slot); e != nil {
			st.hand.Return(e)
		}
	}
	st.hand.Box = 0
}

// Item applies an item click of slot's client (OrderItem) with the
// single-player rules: the bag strip (ClickBag, UseBagItem), the character
// window (equipment, jewellery, the in-hand box, the raises, «АВТО»), the
// own portrait (use or drink the held item), a drop on the ground, the
// hand's return and the pile window's «Ok»/«Закрыть». Misc items that need
// the single-player world (orbs, keys, the whistle, the doll, the lamp, the
// magnet, the seer's scroll) do nothing here (gamestate.Game.World is nil),
// and the shovel's dig mode is not offered.
func (w *World) Item(slot int, o netplay.OrderPayload) {
	m, ok := w.players[slot]
	if !ok {
		return
	}
	s := w.Maps[m]
	mem, e := s.Member(slot)
	if mem == nil || e.Dead() {
		return
	}
	st := w.state(slot)
	h, t := &st.hand, w.Tables
	msg := 0
	switch o.Item {
	case netplay.ItemBag:
		if o.Index < 0 || o.Index >= len(e.Bag) {
			return
		}
		if o.On {
			msg = w.Game.UseBagItem(h, e, t, o.Index)
		} else {
			msg = w.Game.ClickBag(h, e, t, o.Index)
		}
	case netplay.ItemEquip:
		if o.Index < 8 || o.Index > 13 {
			return
		}
		// The window's hotspots 8–13 (game.windowClick): helmet, the hand
		// (the held item's own slot), shield, armour, bow, ammunition.
		k := [6]int{gamestate.EquipHelmet, -1, gamestate.EquipShield, gamestate.EquipArmour,
			gamestate.WeaponBow, gamestate.EquipAmmo}[o.Index-8]
		if k < 0 {
			k = gamestate.HandSlot(h, *e, w.Game)
		}
		msg = w.Game.ClickEquipment(h, e, t, k)
	case netplay.ItemJewel:
		if o.Index < 0 || o.Index >= len(e.Extra) {
			return
		}
		msg = w.Game.ClickJewellery(h, e, t, o.Index)
	case netplay.ItemBox:
		msg = w.Game.ClickBox(h, e, t)
	case netplay.ItemSelf:
		if h.Holding() {
			msg = w.Game.ClickPortrait(h, e, e, t)
		}
	case netplay.ItemDrop:
		if h.Holding() {
			w.drop(s, mem, h)
		}
	case netplay.ItemReturn:
		h.Return(e)
	case netplay.ItemBoxClear:
		h.Box = 0
	case netplay.ItemRaise:
		if o.Index >= 0 && o.Index < gamestate.RaiseRows {
			w.Game.Raise(e, t, o.Index)
		}
	case netplay.ItemAuto:
		w.Game.ToggleAutoLevel(e, t)
	case netplay.ItemPile:
		msg = w.pileOrder(s, slot, o.On, o.Lists)
	default:
		return
	}
	if h.Digging() {
		h.Cursor = gamestate.CursorPointer // no dig mode on the server
	}
	mem.Look = world.LookOf(*e) // a weapon or armour changed: frames and look follow
	if msg != 0 {
		w.say(slot, t.CString(msg))
	}
}

// drop puts the held item on the ground under the hero (game.dropHeld):
// into the pile there, or a new pile while the active list has room;
// otherwise it is lost, as in the game.
func (w *World) drop(s *world.World, mem *world.Member, h *gamestate.Hand) {
	item := h.Item
	h.Cursor = gamestate.CursorPointer
	at, on := s.PileAt[mem.Cell]
	if !on {
		if len(s.PileAt) >= gamestate.MaxActivePiles {
			return
		}
		at = -1
	}
	if i, ok := w.Game.DropItem(at, s.MapNumber, mem.Cell.Row, mem.Cell.Col, item); ok {
		s.PileAt[mem.Cell] = i
	}
}

// pileOrder is the pile window's «Ok» (accept) or «Закрыть» with the
// client's lists (game.applyTrade): the lists must hold the window's own
// items, the bag's on the left and the pile's on the right; then the deal
// applies (gamestate.Game.Apply checks the bag and weight limits). After
// «Ok» the window shows the pile again unless it is gone.
func (w *World) pileOrder(s *world.World, slot int, accept bool, lists *[netplay.PileLists][netplay.PileListSlots]int) int {
	st := w.state(slot)
	tr := st.pile
	if tr == nil {
		return 0
	}
	cand := *tr
	if lists != nil {
		if !samePileItems(tr.Lists, *lists) {
			return 0
		}
		cand.Lists = *lists
	}
	p := w.Game.Piles[tr.Pile]
	cell := mapdata.Cell{Row: p.Row, Col: p.Col}
	s.StoreWorld()
	msg, ok, removed := w.Game.Apply(w.Tables, &cand, accept, s.GameTime())
	s.SyncParty()
	if !ok && accept {
		return msg
	}
	if !ok {
		// «Закрыть» over a limit: nothing changed (the lists are copies);
		// the window closes as with Escape (game.escape).
		st.pile = nil
		return msg
	}
	if removed {
		delete(s.PileAt, cell)
	}
	if !accept || removed {
		st.pile = nil
	} else {
		st.pile = w.Game.OpenPileTrade(slot, slot, tr.Pile)
	}
	return 0
}

// samePileItems reports whether lists move only the window's items: the
// bag's between lists 0 and 1, the pile's between lists 2 and 3.
func samePileItems(orig, got [netplay.PileLists][netplay.PileListSlots]int) bool {
	set := func(a, b [netplay.PileListSlots]int) []int {
		var out []int
		for _, l := range [][netplay.PileListSlots]int{a, b} {
			for _, i := range l {
				if i != 0 {
					out = append(out, i)
				}
			}
		}
		slices.Sort(out)
		return out
	}
	return slices.Equal(set(orig[0], orig[1]), set(got[0], got[1])) &&
		slices.Equal(set(orig[2], orig[3]), set(got[2], got[3]))
}

// Inventory is slot's hero as its client is sent it (MsgInventory): the
// record (its raw bytes left out: the client keeps its own), the records
// of every item it holds or has in the hand and the box, and the hand.
func (w *World) Inventory(slot int) (netplay.InventoryPayload, bool) {
	e, ok := w.Hero(slot)
	if !ok {
		return netplay.InventoryPayload{}, false
	}
	st := w.state(slot)
	items := map[int]netplay.ItemRecord{}
	add := func(i int) {
		if i > 0 {
			items[i] = netplay.ItemRecord(w.Game.ItemRecord(i))
		}
	}
	for _, l := range [][]int{e.Equipment[:], e.Bag[:], e.Extra[:]} {
		for _, i := range l {
			add(i)
		}
	}
	add(e.Quiver)
	hand := netplay.HandState{Cursor: st.hand.Cursor, Box: st.hand.Box}
	if st.hand.Holding() {
		hand.Item = st.hand.Item
		add(st.hand.Item)
	}
	add(st.hand.Box)
	e.Raw = [len(e.Raw)]byte{}
	return netplay.InventoryPayload{Hero: e, Items: items, Hand: hand}, true
}

// Piles are the active piles on map m and their items' records
// (MsgPiles), by pile index.
func (w *World) Piles(m int) netplay.PilesPayload {
	out := netplay.PilesPayload{Map: m, Items: map[int]netplay.ItemRecord{}}
	if m < 1 || m > Maps {
		return out
	}
	for _, i := range w.Maps[m].PileAt {
		p := w.Game.Piles[i]
		out.Piles = append(out.Piles, netplay.PileState{Index: i, Pile: p})
		for _, it := range p.Items {
			if it > 0 {
				out.Items[it] = netplay.ItemRecord(w.Game.ItemRecord(it))
			}
		}
	}
	slices.SortFunc(out.Piles, func(a, b netplay.PileState) int { return a.Index - b.Index })
	return out
}

// PileWindow is slot's pile window (MsgPileWindow).
func (w *World) PileWindow(slot int) netplay.PileWindowPayload {
	st := w.state(slot)
	if st.pile == nil {
		return netplay.PileWindowPayload{}
	}
	return netplay.PileWindowPayload{Open: true, Pile: st.pile.Pile, Map: st.pile.Map, Lists: st.pile.Lists}
}

// Saved item slots (lanstore's SAVITEMS.DAT slots).
const (
	SlotEquip  = "equip"
	SlotJewel  = "jewel"
	SlotBag    = "bag"
	SlotArrows = "arrows"
)

// Item is an item as SAVITEMS.DAT keeps it (lanstore.Item): category,
// enchantment, type, the identified flag (guess), condition and maximum;
// for the boxed arrows the count.
type Item struct {
	Slot         string
	Index        int
	Category     int
	Enchantment  int
	Type         int
	Flag         bool
	Condition    float64
	MaxCondition float64
	Count        int
}

// itemFields reads an item record.
func itemFields(r [gamestate.ItemStride]byte) (cat, ench, typ int, flag bool, cond, max float64, count int) {
	cat, ench, typ = int(int8(r[0])), int(int8(r[1])), int(r[3])
	flag = r[0xE]&1 != 0
	// Herbs and misc items keep other data there; what is not a number
	// is stored as 0 (SAVITEMS.DAT keeps only the words' high halves).
	num := func(b []byte) float64 {
		f := float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0
		}
		return f
	}
	cond, max = num(r[4:]), num(r[8:])
	count = int(int32(binary.LittleEndian.Uint32(r[4:])))
	return
}

// SavedItems is slot's hero's items to keep (0x00444918): equipment,
// jewellery, the bag and the boxed arrows. The held item goes back first.
func (w *World) SavedItems(slot int) []Item {
	m, ok := w.players[slot]
	if !ok {
		return nil
	}
	_, e := w.Maps[m].Member(slot)
	if e == nil {
		return nil
	}
	w.state(slot).hand.Return(e)
	var out []Item
	put := func(sl string, k, i int) {
		if i <= 0 {
			return
		}
		cat, ench, typ, flag, cond, mx, count := itemFields(w.Game.ItemRecord(i))
		if cat < 0 {
			return
		}
		it := Item{Slot: sl, Index: k, Category: cat, Enchantment: ench, Type: typ, Flag: flag}
		switch sl {
		case SlotArrows:
			it.Count = count
		case SlotJewel:
		default:
			it.Condition, it.MaxCondition = cond, mx
			if cat == gamestate.CatArrows {
				it.Condition, it.MaxCondition, it.Count = 0, 0, count // arrows in the bag keep their count
			}
		}
		out = append(out, it)
	}
	for k, i := range e.Equipment {
		put(SlotEquip, k, i)
	}
	for k, i := range e.Extra {
		put(SlotJewel, k, i)
	}
	for k, i := range e.Bag {
		put(SlotBag, k, i)
	}
	put(SlotArrows, 0, e.Quiver)
	return out
}

// LoadItems gives slot's hero the saved items (0x0044444C, after the
// store's drop rules), in place of the items it has.
func (w *World) LoadItems(slot int, items []Item) {
	m, ok := w.players[slot]
	if !ok {
		return
	}
	mem, e := w.Maps[m].Member(slot)
	if e == nil {
		return
	}
	for _, i := range append(append(append([]int{}, e.Equipment[:]...), e.Extra[:]...), append(e.Bag[:], e.Quiver)...) {
		if i > 0 {
			w.Game.SetItemKind(i, -1, 0)
		}
	}
	e.Equipment, e.Extra, e.Bag, e.Quiver = [6]int{}, [5]int{}, [32]int{}, 0
	// The bag is packed from the front (the game shifts it left on every
	// removal): the saved order, closing the gaps the store's drop rules
	// leave.
	slices.SortStableFunc(items, func(a, b Item) int {
		if a.Slot == SlotBag && b.Slot == SlotBag {
			return a.Index - b.Index
		}
		return 0
	})
	bag := 0
	for _, it := range items {
		i := w.Game.SetItemKind(0, it.Category, it.Type)
		if i == 0 {
			break // the item table is full
		}
		r := w.Game.ItemRecord(i)
		r[1] = byte(int8(it.Enchantment))
		if it.Flag {
			r[0xE] |= 1
		}
		switch {
		case it.Slot == SlotArrows || it.Category == gamestate.CatArrows:
			binary.LittleEndian.PutUint32(r[4:], uint32(int32(it.Count)))
		case it.Slot != SlotJewel:
			binary.LittleEndian.PutUint32(r[4:], math.Float32bits(float32(it.Condition)))
			binary.LittleEndian.PutUint32(r[8:], math.Float32bits(float32(it.MaxCondition)))
		}
		w.Game.SetItemRecord(i, r)
		switch it.Slot {
		case SlotEquip:
			if it.Index >= 0 && it.Index < len(e.Equipment) {
				e.Equipment[it.Index] = i
			}
		case SlotJewel:
			if it.Index >= 0 && it.Index < len(e.Extra) {
				e.Extra[it.Index] = i
			}
		case SlotBag:
			if bag < len(e.Bag) {
				e.Bag[bag] = i
				bag++
			}
		case SlotArrows:
			e.Quiver = i
		}
	}
	// The active weapon must be one the hero holds (SelectWeapon's rule).
	if e.Equipment[e.WeaponMode] == 0 {
		gamestate.SelectWeapon(e, gamestate.WeaponCycle)
	}
	mem.Look = world.LookOf(*e)
	w.Maps[m].StoreWorld()
}
