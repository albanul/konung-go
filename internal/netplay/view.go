package netplay

import "konung1.5/internal/mapdata"

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
	// Member is the party member an item click is for (OrderItem): the
	// character the window and the bag strip show, the original's 0x4A
	// member index (+3 & 3); 0 or the hero's index: the hero.
	Member int `json:"member,omitempty"`
}

// CharState is a character on the own hero's map at a tick: entity, cell
// (while walking the one being entered), the cell being left and the walk
// step, facing, action and its step, HP, death, stance and weapon. Short
// JSON keys: a busy map sends many.
//
// Steps, ActSteps and Throttle let the client advance the walk and action
// steps by itself between reports (Predict): the server sends a character
// only when its state differs from what the client predicts from the last
// one it was sent (lanserver.World.View).
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
	// Steps is the walk's frame count while walking (or about to: Path),
	// ActSteps the action's while acting, Throttle a creature's walk
	// throttle (it steps only on ticks t with t & Throttle == Throttle,
	// world.World.UpdateNPCs).
	Steps    int `json:"n,omitempty"`
	ActSteps int `json:"an,omitempty"`
	Throttle int `json:"m,omitempty"`
	// Path is the next cells of the walk (row, col pairs; at most
	// PredictCells of them).
	Path []int `json:"pa,omitempty"`
}

// PredictCells is how many cells of a walk's path a report carries.
const PredictCells = 3

// Predict is c, reported at tick from, advanced to tick to the way the
// server's walker advances one tick (world.Walker.Update) while nothing else
// changes: it starts the next step of Path, turns an octant toward the
// step's direction, shows the next walk frame, and after the last one enters
// the next cell of Path or stops (on the throttle's ticks, not while
// acting); an action advances a step per tick up to its last frame. The
// server reports a character when its state differs from this (Same), so a
// client applying Predict between reports has the server's state at every
// view tick.
func (c CharState) Predict(from, to int) CharState {
	if to <= from || c.Dead {
		return c
	}
	path := c.Path
	for t := from + 1; t <= to; t++ {
		if c.Act == 0 && (c.Walking || len(path) > 0) && (c.Throttle == 0 || t&c.Throttle == c.Throttle) {
			path = c.walkTick(path)
		}
		if c.Act != 0 && c.ActStep+1 < c.ActSteps {
			c.ActStep++
		}
	}
	c.Path = path
	return c
}

// walkTick is one tick of world.Walker.Update on c with path; it returns
// what is left of path.
func (c *CharState) walkTick(path []int) []int {
	enter := func() {
		c.FromRow, c.FromCol, c.Row, c.Col = c.Row, c.Col, path[0], path[1]
		path = path[2:]
	}
	if !c.Walking {
		enter()
		c.Walking, c.Step = true, 0
	}
	from, cell := mapdata.Cell{Row: c.FromRow, Col: c.FromCol}, mapdata.Cell{Row: c.Row, Col: c.Col}
	if d := mapdata.StepDirection(from, cell); mapdata.Direction(c.Facing) != d {
		c.Facing = int(mapdata.TurnToward(mapdata.Direction(c.Facing), d))
		return path
	}
	if c.Step+1 < c.Steps {
		c.Step++
		return path
	}
	c.Step = 0
	if len(path) == 0 {
		// The walk ends: a standing character's report has no walk fields.
		c.Walking, c.FromRow, c.FromCol, c.Steps, c.Throttle = false, 0, 0, 0, 0
		return nil
	}
	enter()
	return path
}

// Same reports whether the client's prediction p matches the server's state
// c: every field, and p's path the start of c's (a report carries only the
// next PredictCells cells of a walk).
func (p CharState) Same(c CharState) bool {
	if len(p.Path) > len(c.Path) {
		return false
	}
	for i, v := range p.Path {
		if c.Path[i] != v {
			return false
		}
	}
	return p.Entity == c.Entity && p.Player == c.Player && p.Owner == c.Owner && p.Row == c.Row && p.Col == c.Col &&
		p.FromRow == c.FromRow && p.FromCol == c.FromCol && p.Walking == c.Walking && p.Step == c.Step &&
		p.Facing == c.Facing && p.Act == c.Act && p.ActStep == c.ActStep && p.HP == c.HP && p.Dead == c.Dead &&
		p.Stance == c.Stance && p.Weapon == c.Weapon && p.Steps == c.Steps && p.ActSteps == c.ActSteps &&
		p.Throttle == c.Throttle
}

// Report is c as it is sent: its path cut to PredictCells cells.
func (c CharState) Report() CharState {
	if len(c.Path) > 2*PredictCells {
		c.Path = c.Path[: 2*PredictCells : 2*PredictCells]
	}
	return c
}

// ViewPayload is the own hero's map at a tick, sent for every tick the
// world ran. Full: every character on the map (after entering or a map
// change); else only those whose state differs from their last report
// advanced to Tick (CharState.Predict), and Gone the ones no longer on it.
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
	EventSound    = "s" // Sound at (Row, Col)
	EventAction   = "a" // Entity started action Act (its sound is the client's)
	EventMsg      = "m" // a status-line message for the own hero (Text, CP866)
	EventArrow    = "r" // an arrow released at Tick (Arrow): the client flies it (the original's 0x12)
	EventBuilding = "b" // a village building's state (Building; the original's 0x5E)
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
