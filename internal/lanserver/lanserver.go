// Package lanserver runs the LAN world on the dedicated server, as
// LAN_KONG.EXE's server does (tick 0x445988 over every group): the 20
// MAPS.RES maps with GAME.RES's villagers, creatures and piles, each map an
// internal/world World (World.LAN) over one shared gamestate.Game, and the
// players' heroes as party members of the map they are on (slot k = group
// and entity k, 0–15). Players give orders (Walk, Right, Stance, Weapon,
// Attack); every tick the runner steps all maps, moves heroes that took an
// exit to the target map, and collects events; Snapshot gives a map's
// characters for the clients (docs/konung_lan.md#server-world).
//
// Engine-free: frame counts (the walk steps' and actions' lengths) come
// from decoding GRAPH.RES and OBJECTS.RES (resfmt), cached; no images.
package lanserver

import (
	"fmt"
	"math/rand/v2"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/lanfight"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
	"konung1.5/internal/world"
)

// Files are the game files the world is built from.
type Files struct {
	GameRes []byte                 // the LAN world (gamestate.ParseLANWorld)
	MapsRes []byte                 // the 20 maps (mapdata.SplitMapsRes)
	Graph   []byte                 // GRAPH.RES: character sprites, for frame counts
	Objects []byte                 // OBJECTS.RES: animated models, for frame counts
	Exe     []byte                 // konung.exe: the rule tables
	Games   [netplay.Models][]byte // GAME.0–GAME.2: the hero types' gear
}

// Maps is the number of LAN maps.
const Maps = gamestate.LANMaps

// World is the server's world.
type World struct {
	Game   *gamestate.Game
	Tables *resfmt.ExeTables
	Maps   [Maps + 1]*world.World // by map number, 1–20
	Tick   int

	exits  []gamestate.MapExit
	tmpl   [netplay.Models]*gamestate.Game
	frames *frameCache

	players map[int]int // slot → map
	moving  []move      // heroes that took an exit this tick
	events  []Event
	items   map[int]*itemState // by slot: the hand, the pile window (items.go)

	// TeamSafe: heroes of one type (the same model) cannot hurt each other
	// (the server's team-damage option).
	TeamSafe bool
}

type move struct {
	slot int
	x    gamestate.MapExit
}

// Load builds the world: GAME.RES's records, every map's walk grid and
// size, and the map entry (creatures placed in their territory, villagers
// on their cells; posts stay where GAME.RES puts them, inside their
// buildings' footprints).
func Load(f Files) (*World, error) {
	lw, err := gamestate.ParseLANWorld(f.GameRes)
	if err != nil {
		return nil, err
	}
	tables, err := resfmt.NewExeTables(f.Exe)
	if err != nil {
		return nil, err
	}
	maps, err := mapdata.SplitMapsRes(f.MapsRes)
	if err != nil {
		return nil, err
	}
	if len(maps) < Maps {
		return nil, fmt.Errorf("MAPS.RES has %d maps, want %d", len(maps), Maps)
	}
	w := &World{Game: lw.Game, Tables: tables, exits: lw.Game.Exits, players: map[int]int{},
		frames: &frameCache{graph: f.Graph, objects: f.Objects, n: map[frameKey]int{}}}
	for m, data := range f.Games {
		if w.tmpl[m], err = gamestate.Parse(data); err != nil {
			return nil, fmt.Errorf("GAME.%d: %w", m, err)
		}
	}
	for n := 1; n <= Maps; n++ {
		mp, err := mapdata.Parse(maps[n-1].Map)
		if err != nil {
			return nil, fmt.Errorf("MAPS.RES map %d: %w", n, err)
		}
		walk, err := mapdata.ParseWalkGrid(maps[n-1].Map)
		if err != nil {
			return nil, fmt.Errorf("MAPS.RES map %d: %w", n, err)
		}
		s := &world.World{
			Game: w.Game, Tables: tables, MapNumber: n, Walk: walk, LAN: true,
			ByEntity: map[int]*world.NPC{}, Occ: map[mapdata.Cell]bool{}, PileAt: map[mapdata.Cell]int{},
			Rng: rand.New(rand.NewPCG(1, uint64(n))),
		}
		s.WorldW, s.WorldH = mp.WorldSize()
		s.Hooks = w.hooks(n)
		s.Populate(false, w.attach)
		w.Maps[n] = s
	}
	return w, nil
}

// attach gives a walker its frame counts (world.Attach).
func (w *World) attach(wk *world.Walker, _ gamestate.Placed, live func() gamestate.Entity) {
	wk.Frames = w.frames.counter(live)
}

// hooks are map n's world hooks: sounds become events, a hero on an exit
// is moved after the step. Windows and trade do not exist on the server;
// building hits (incendiary arrows) are not ported here (nil: arrows fly
// through buildings).
func (w *World) hooks(n int) world.Hooks {
	return world.Hooks{
		PlaySound: func(id int, c mapdata.Cell) {
			w.events = append(w.events, Event{Kind: EventSound, Map: n, Sound: id, Row: c.Row, Col: c.Col, Entity: -1})
		},
		ActionSound: func(wk *world.Walker, act int) {
			w.events = append(w.events, Event{Kind: EventAction, Map: n, Entity: wk.Entity, Act: act, Row: wk.Cell.Row, Col: wk.Cell.Col})
		},
		MemberExit: func(m *world.Member, x gamestate.MapExit) {
			w.moving = append(w.moving, move{slot: m.Entity, x: x})
		},
		Killed: func(a, v *gamestate.Entity) {
			if a.Group >= 0 && a.Group < world.LANPlayers {
				w.events = append(w.events, Event{Kind: EventKill, Map: n, Entity: a.Index,
					Victim: v.Index, VictimPlayer: v.Group >= 0 && v.Group < world.LANPlayers})
			}
		},
		Protected: w.protected,
		// A pick-up's message and pile window go to the hero picking up.
		Message: func(text []byte) {
			if a := w.Maps[n].Arriving; a != nil {
				w.say(a.Entity, text)
			}
		},
		OpenPileWindow: func(pile int) {
			if a := w.Maps[n].Arriving; a != nil {
				w.openPile(n, a.Entity, pile)
			}
		},
	}
}

// protected is the team-damage option: two players' heroes of one type.
func (w *World) protected(a, t *gamestate.Entity) bool {
	player := func(e *gamestate.Entity) bool { return e.Group >= 0 && e.Group < world.LANPlayers }
	return w.TeamSafe && player(a) && player(t) && a.Model == t.Model
}

// Step runs one game tick (60 ms at normal speed) of every map, then moves
// the heroes that ended a walk on an exit to the exit's target.
func (w *World) Step() {
	w.Tick++
	for n := 1; n <= Maps; n++ {
		w.Maps[n].Step()
	}
	moving := w.moving
	w.moving = nil
	for _, mv := range moving {
		from, ok := w.players[mv.slot]
		if !ok {
			continue
		}
		if _, ok := w.Maps[from].RemoveMember(mv.slot); !ok {
			continue
		}
		w.place(mv.slot, mv.x.To, mapdata.Cell{Row: mv.x.Row, Col: mv.x.Col}, mv.x.Facing)
		m := w.Maps[mv.x.To]
		c := mv.x.Row
		col := mv.x.Col
		if mem, _ := m.Member(mv.slot); mem != nil {
			c, col = mem.Cell.Row, mem.Cell.Col
		}
		w.events = append(w.events, Event{Kind: EventMapChange, Map: mv.x.To, From: from, Entity: mv.slot, Row: c, Col: col, Facing: mv.x.Facing})
	}
}

// place puts slot's hero on map m at cell c (nearest free).
func (w *World) place(slot, m int, c mapdata.Cell, facing int) *world.Member {
	w.Game.Groups[slot].Map = m
	w.players[slot] = m
	return w.Maps[m].AddMember(slot, c, facing, w.attach)
}

// Start is where a hero of type model enters (gamestate.LANStart).
func (w *World) Start(model int) (gamestate.MapExit, error) {
	return gamestate.LANStart(w.exits, model)
}

// AddPlayer puts slot's hero into the world: the hero type's GAME.N hero
// with the account's values (lanfight.Hero; hero nil: the type's default),
// its gear copied into the world's items, out of the stance, full HP, on
// the type's start (the nearest free cell). It returns the map and cell.
func (w *World) AddPlayer(slot, model int, hero *netplay.Hero) (int, mapdata.Cell, error) {
	if slot < 0 || slot >= world.LANPlayers {
		return 0, mapdata.Cell{}, fmt.Errorf("player slot %d", slot)
	}
	model = min(max(model, 0), netplay.Models-1)
	start, err := w.Start(model)
	if err != nil {
		return 0, mapdata.Cell{}, err
	}
	w.RemovePlayer(slot)
	src := w.tmpl[model]
	e := lanfight.Hero(src, model, hero)
	for k, i := range e.Equipment {
		e.Equipment[k] = w.Game.CopyItemFrom(src, i)
	}
	e.Quiver = w.Game.CopyItemFrom(src, e.Quiver)
	for k, i := range e.Bag {
		e.Bag[k] = w.Game.CopyItemFrom(src, i)
	}
	gamestate.LeaveStance(&e)
	e.Flags4 &^= 0x80
	e.Flags3 &^= 0x80
	e.HP = gamestate.HPFull
	e.Index, e.Group = slot, slot
	w.Game.Entities[slot] = e
	g := &w.Game.Groups[slot]
	g.First, g.Count = slot, 1
	m := w.place(slot, start.To, mapdata.Cell{Row: start.Row, Col: start.Col}, start.Facing)
	return start.To, m.Cell, nil
}

// RemovePlayer takes slot's hero out of the world (logout): its items are
// freed and the slot's group leaves every map.
func (w *World) RemovePlayer(slot int) {
	m, ok := w.players[slot]
	if !ok {
		return
	}
	w.closeItems(slot)
	delete(w.items, slot) // a fresh hand and no pile window next time
	delete(w.players, slot)
	e, _ := w.Maps[m].RemoveMember(slot)
	for _, i := range append(append(e.Equipment[:], e.Quiver), e.Bag[:]...) {
		if i > 0 {
			w.Game.SetItemKind(i, -1, 0)
		}
	}
	e.Equipment, e.Bag, e.Quiver = [6]int{}, [32]int{}, 0
	e.Flags3 |= 0x80
	w.Game.Entities[slot] = e
	w.Game.Groups[slot].Map = 0
}

// PlayerMap is the map slot's hero is on (0 when not in the world).
func (w *World) PlayerMap(slot int) int { return w.players[slot] }

// MoveTo puts slot's hero on map m at cell c at once (a jump: the
// original's orb or the server's own placement).
func (w *World) MoveTo(slot, m int, c mapdata.Cell) bool {
	from, ok := w.players[slot]
	if !ok || m < 1 || m > Maps {
		return false
	}
	e, _ := w.Maps[from].RemoveMember(slot)
	w.place(slot, m, c, e.Variant)
	return true
}

// member is slot's hero's member and record, nil when it is not in the
// world or cannot act.
func (w *World) member(slot int) (*world.World, *world.Member, *gamestate.Entity) {
	m, ok := w.players[slot]
	if !ok {
		return nil, nil, nil
	}
	s := w.Maps[m]
	mem, e := s.Member(slot)
	if mem == nil || !world.Alive(&mem.Walker, e) {
		return nil, nil, nil
	}
	return s, mem, e
}

// Walk is the left click on the map: walk to cell (row, col).
func (w *World) Walk(slot, row, col int) {
	if s, m, _ := w.member(slot); m != nil {
		s.WalkOrder(m, mapdata.Cell{Row: row, Col: col}, false)
	}
}

// Right is the right click (world.RightOrder): with target ≥ 0 (an entity
// on the hero's map) a shot with the bow ready, a melee attack in the
// stance, nothing out of it; with target −1 a walk with a pick-up on
// (row, col).
func (w *World) Right(slot, target, row, col int) {
	s, m, _ := w.member(slot)
	if m == nil {
		return
	}
	if target >= 0 {
		tw, t := s.Combatant(target)
		if tw == nil || target == slot {
			return // not on this map
		}
		if _, me := s.Member(slot); me != nil && w.protected(me, t) {
			return // a teammate the server protects
		}
	}
	s.RightOrder(m, target, mapdata.Cell{Row: row, Col: col}, 0)
}

// Stance puts slot's hero into the combat stance or out of it.
func (w *World) Stance(slot int, on bool) {
	if s, m, _ := w.member(slot); m != nil {
		s.SetStance(m, on)
	}
}

// Weapon selects slot's hero's weapon (gamestate.WeaponAxe, WeaponBow,
// WeaponBlade or WeaponCycle).
func (w *World) Weapon(slot, mode int) {
	if s, m, _ := w.member(slot); m != nil {
		s.SetWeapon(m, mode)
	}
}

// Attack is the A key: the stance and the nearest enemy (creatures and
// hostile villagers; not other players).
func (w *World) Attack(slot int) {
	if s, m, _ := w.member(slot); m != nil {
		s.AttackNearest(m)
	}
}

// Hero is slot's hero's record as it is now (cell and stance included).
func (w *World) Hero(slot int) (gamestate.Entity, bool) {
	m, ok := w.players[slot]
	if !ok {
		return gamestate.Entity{}, false
	}
	mem, e := w.Maps[m].Member(slot)
	if mem == nil {
		return gamestate.Entity{}, false
	}
	out := *e
	out.Row, out.Col, out.Variant = mem.Cell.Row, mem.Cell.Col, int(mem.Dir)
	return out, true
}

// TakeEvents returns the events since the last call.
func (w *World) TakeEvents() []Event {
	ev := w.events
	w.events = nil
	return ev
}
