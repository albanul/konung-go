package netplay

import "konung1.5/internal/gamestate"

// Items in the LAN world live on the server (internal/lanserver, as
// LAN_KONG.EXE's server with items 0xC4E1B8): a client sends its item
// clicks as orders (OrderItem: the character window, the bag strip, the
// in-hand box, the portrait, a drop on the map, the pile window), the
// server applies them with the single-player rules (gamestate inventory,
// potions, levels, trade's pile window) and sends back the own hero's
// items (MsgInventory, the original's 0x00/0x40 item records), the piles on
// its map (MsgPiles) and the pile window (MsgPileWindow). The client
// mirrors them and changes nothing itself.

// Item actions (OrderPayload.Item).
const (
	ItemBag      = "bag"      // a bag cell (Index): left click ClickBag, right click (On) UseBagItem
	ItemEquip    = "equip"    // the character window's equipment hotspot Index (8–13)
	ItemJewel    = "jewel"    // jewellery box Index (0–4)
	ItemBox      = "box"      // the in-hand box (hotspot 19)
	ItemSelf     = "self"     // the held item dropped on the own portrait (use or drink)
	ItemDrop     = "drop"     // the held item put on the ground under the hero
	ItemReturn   = "return"   // the held item goes back where it came from
	ItemBoxClear = "boxclear" // the window opened: the in-hand box empties (0x004246D1)
	ItemRaise    = "raise"    // a raise button (Index: the row)
	ItemAuto     = "auto"     // «АВТО»
	ItemPile     = "pile"     // the pile window's «Ok» (On) or «Закрыть», with Lists
	ItemTrade    = "trade"    // a trade's «Ok» (On) or «Закрыть», with Lists (talk.go)
	ItemClose    = "close"    // Escape: window 7 closes without a deal (0x00437447)
)

// The pile window's lists (gamestate.Trade.Lists).
const (
	PileLists     = 4
	PileListSlots = gamestate.TradeSlots
)

// ItemRecord is an item's 16-byte record (gamestate's item table).
type ItemRecord [gamestate.ItemStride]byte

// HandState is the hand as the client draws it: the held item, the cursor
// and the in-hand box's item (gamestate.Hand).
type HandState struct {
	Item   int `json:"item,omitempty"`
	Cursor int `json:"cursor"`
	Box    int `json:"box,omitempty"`
}

// InventoryPayload is the own hero as the server keeps it: its record
// (bag, equipment, jewellery, quiver, gold, level, stats, skills; the
// client keeps its own position, action and HP, which MsgView carries),
// every item record it holds, and the hand.
type InventoryPayload struct {
	Hero  gamestate.Entity   `json:"hero"`
	Items map[int]ItemRecord `json:"items"`
	Hand  HandState          `json:"hand"`
}

// PileState is a pile on the map: its index in the server's pile table and
// the pile (cell, gold, items).
type PileState struct {
	Index int            `json:"index"`
	Pile  gamestate.Pile `json:"pile"`
}

// PilesPayload is every active pile on the own hero's map and their items'
// records.
type PilesPayload struct {
	Map   int                `json:"map"`
	Piles []PileState        `json:"piles"`
	Items map[int]ItemRecord `json:"items"`
}

// PileWindowPayload is window 7: the pile window (0x004170A0) or a trade
// (script 275), open with its lists (the bag, the offer, the pile or the
// goods, the take) or closed.
type PileWindowPayload struct {
	Open  bool                          `json:"open"`
	Pile  int                           `json:"pile"`
	Map   int                           `json:"map"`
	Lists [PileLists][PileListSlots]int `json:"lists"`
	// Trader is set for a trade (dialogue script 275): the window's lists
	// are then the bag, the offer, the trader's goods and the take, and
	// Items has the goods' records.
	Trader *TraderState       `json:"trader,omitempty"`
	Items  map[int]ItemRecord `json:"items,omitempty"`
}

// TraderState is a trade's other side as the window needs it for the
// prices (gamestate.Trade): the trader, the mode (a shop's post slot, ±; 6
// a barter), both sides' money and the trader's trade skill.
type TraderState struct {
	NPC        int `json:"npc"`
	Mode       int `json:"mode"`
	HeroMoney  int `json:"heroMoney"`
	OtherMoney int `json:"otherMoney"`
	Skill      int `json:"skill"`
}
