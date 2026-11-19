package netplay

// The world view. The server runs the world (internal/lanserver, as
// LAN_KONG.EXE's server, tick 0x445988); a client sends orders for its hero
// (MsgOrder, the original's 0x55) and draws what the server reports about
// the hero's map: the characters' states (MsgView, the original's 0x19),
// the players' heroes as they look (MsgPlayer, 0x5A), sounds (MsgEvents)
// and the own hero's map changes (MsgMapChange). Characters other than the
// players' heroes are GAME.RES's records, which the client reads itself;
// a character is named by its entity index in the world.

// EquipSlots is a hero's equipment slots (gamestate: axe, bow, blade,
// armour, helmet, shield).
const EquipSlots = 6

// EquipLook is an equipped item as far as drawing needs it; without Item
// the slot is empty (the zero value: a look without equipment).
type EquipLook struct {
	Item     bool `json:"i,omitempty"`
	Category int  `json:"c,omitempty"`
	Type     int  `json:"t,omitempty"`
}

// Look is how a hero looks: stance (the entity word's bit 15), weapon mode
// (bits 12–13) and the equipment (0x5A).
type Look struct {
	Stance bool                  `json:"stance"`
	Weapon int                   `json:"weapon"` // 0 axe, 1 bow, 2 blade
	Equip  [EquipSlots]EquipLook `json:"equip"`
}

// Order kinds (OrderPayload.Kind).
const (
	OrderWalk   = "walk"   // left click on the map: walk to (Row, Col)
	OrderRight  = "right"  // right click: Target ≥ 0 a character (attack in the stance, shoot with the bow ready), −1 a walk with a pick-up on (Row, Col)
	OrderStance = "stance" // Space: into the stance (On) or out of it
	OrderWeapon = "weapon" // 1/2/3, the HUD button: Mode (gamestate.Weapon*, −1 the next)
	OrderAttack = "attack" // A: the stance and the nearest enemy
	OrderItem   = "item"   // an item click (Item: ItemBag…), applied on the server (items.go)
	OrderTalk   = "talk"   // right click on a character with a dialogue out of the stance: Target (the original's 0x43)
	OrderReply  = "reply"  // a click on reply Index of the open dialogue (talk.go)
	OrderGather = "gather" // F, the HUD button «Все ко мне»: the hirelings follow the hero again (party.go)
)

// OrderPayload is an order for the own hero.
type OrderPayload struct {
	Kind   string `json:"kind"`
	Row    int    `json:"row,omitempty"`
	Col    int    `json:"col,omitempty"`
	Target int    `json:"target,omitempty"`
	On     bool   `json:"on,omitempty"`
	Mode   int    `json:"mode,omitempty"`
	// OrderItem: the action, the cell or slot it names (Index) and, for
	// the pile window's «Ok»/«Закрыть» (On = «Ok»), its four lists.
	Item  string                         `json:"item,omitempty"`
	Index int                            `json:"index,omitempty"`
	Lists *[PileLists][PileListSlots]int `json:"lists,omitempty"`
	// Sel is the player's selection (entity indices: its hero and
	// hirelings) for walk, right, stance and weapon; empty: the hero.
	Sel []int `json:"sel,omitempty"`
}

// CharState is a character on the own hero's map at a tick: entity, cell
// (while walking the one being entered), the cell being left and the walk
// step, facing, action and its step, HP, death, stance and weapon. Short
// JSON keys: a busy map sends many.
type CharState struct {
	Entity  int  `json:"e"`
	Player  bool `json:"p,omitempty"`
	Owner   int  `json:"o,omitempty"` // a hireling: its player's slot + 1
	Row     int  `json:"r"`
	Col     int  `json:"c"`
	FromRow int  `json:"fr,omitempty"`
	FromCol int  `json:"fc,omitempty"`
	Walking bool `json:"w,omitempty"`
	Step    int  `json:"s,omitempty"`
	Facing  int  `json:"f,omitempty"`
	Act     int  `json:"a,omitempty"`
	ActStep int  `json:"as,omitempty"`
	HP      int  `json:"hp"`
	Dead    bool `json:"d,omitempty"`
	Stance  bool `json:"st,omitempty"`
	Weapon  int  `json:"wp,omitempty"`
}

// ViewPayload is the own hero's map at a tick. Full: every character on
// the map (after entering or a map change); else only those that changed
// since the last view, and Gone the ones no longer on it.
type ViewPayload struct {
	Map   int         `json:"map"`
	Tick  int         `json:"tick"`
	Full  bool        `json:"full,omitempty"`
	Chars []CharState `json:"chars,omitempty"`
	Gone  []int       `json:"gone,omitempty"`
}

// PlayerPayload is a player's hero as others draw it (the original's 0x5A
// with name and clan): its slot (= entity), the player, the account's hero
// (model, palette, level, stats; nil without accounts) and its look.
type PlayerPayload struct {
	Slot   int        `json:"slot"`
	Player PlayerInfo `json:"player"`
	Hero   *Hero      `json:"hero,omitempty"`
	Look   Look       `json:"look"`
}

// Event kinds.
const (
	EventSound    = "sound"    // Sound at (Row, Col)
	EventAction   = "action"   // Entity started action Act (its sound is the client's)
	EventMsg      = "msg"      // a status-line message for the own hero (Text, CP866)
	EventArrow    = "arrow"    // an arrow released at Tick (Arrow): the client flies it (the original's 0x12)
	EventBuilding = "building" // a village building's state (Building; the original's 0x5E)
)

// Arrow is an arrow as it leaves the bow (gamestate.Arrow's flight
// values): position, per-sub-step velocity, shooter, image facing, ticks
// left and the special (0 incendiary: it glows). The client flies it with
// the single-player flight from the tick after Tick; the server alone
// decides what it hits.
type Arrow struct {
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	VX      float64 `json:"vx"`
	VY      float64 `json:"vy"`
	Shooter int     `json:"sh"`
	Facing  int     `json:"f,omitempty"`
	Life    int     `json:"l"`
	Special int     `json:"sp"`
}

// Event is something on the own hero's map.
type Event struct {
	Kind   string `json:"k"`
	Entity int    `json:"e,omitempty"`
	Sound  int    `json:"s,omitempty"`
	Act    int    `json:"a,omitempty"`
	Row    int    `json:"r,omitempty"`
	Col    int    `json:"c,omitempty"`
	Text   []byte `json:"x,omitempty"`
	Tick   int    `json:"t,omitempty"`
	Arrow  *Arrow `json:"ar,omitempty"`
	// Building: EventBuilding's building.
	Building *Building `json:"b,omitempty"`
}

// Building is a village building's state on the receiver's map (the
// original's 0x5E): Placement is its KONUNG.N/MAPS.RES placement, State
// 0–2 building, 3 standing, 4–5 burning, 6 a ruin.
type Building struct {
	Placement int `json:"p"`
	State     int `json:"s"`
}

// EventsPayload is a tick's events.
type EventsPayload struct {
	Events []Event `json:"events"`
}

// Order sends an order for the own hero.
func (c *Client) Order(o OrderPayload) error { return c.Send(mustMessage(MsgOrder, o)) }
