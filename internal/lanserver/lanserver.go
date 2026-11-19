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
	Quests  []byte                 // QUESTR.RES: dialogues (nil: nobody talks)
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

	// Quests is QUESTR.RES (nil: no dialogues); shops and loans are the
	// LAN settlements' per-slot stocks and loan bits (talk.go).
	Quests *resfmt.Quests
	shops  [gamestate.LANSettlements][world.LANPlayers]gamestate.LANShop
	loans  [gamestate.LANSettlements]uint16

	// TeamSafe: heroes of one type (the same model) cannot hurt each other
	// (the server's team-damage option).
	TeamSafe bool

	hired map[int][]hireling // by slot: its hirelings (party.go)
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
	w := &World{Game: lw.Game, Tables: tables, exits: lw.Game.Exits, players: map[int]int{}, hired: map[int][]hireling{},
		frames: &frameCache{graph: f.Graph, objects: f.Objects, n: map[frameKey]int{}}}
	if f.Quests != nil {
		if w.Quests, err = resfmt.ParseQuests(f.Quests); err != nil {
			return nil, err
		}
	}
	for si := range gamestate.LANSettlements {
		for k := range world.LANPlayers {
			w.shops[si][k] = lw.Shop(si, k)
		}
		w.loans[si] = lw.Loans(si)
	}
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
		villageBounds(s)
		w.Maps[n] = s
	}
	return w, nil
}

// LAN village AI (0x00437E28, the single-player 0x00413A84 with fixed
// ranges): random steps of rand()%40 rows and rand()%20 columns.
const (
	lanVillageRows = 20
	lanVillageCols = 10
)

// villageMargin is how far (rows, columns) a village's wander area reaches
// beyond its villagers' and posts' cells.
const villageRowMargin, villageColMargin = 4, 2

// villageBounds keeps the villagers of map s inside their village (a port
// rule). The original server steps them inside their group's rectangle as
// single player does, but GAME.RES gives every LAN village the whole map
// (rows 0–128, columns 0–64: a MAPS.RES map's fine grid), so there they
// roam the map. Here the area is the box around the village's villagers
// and its posts as GAME.RES places them, widened by villageRowMargin and
// villageColMargin; the step ranges are the LAN server's.
func villageBounds(s *world.World) {
	boxes := map[int]*gamestate.Area{}
	grow := func(g, r, c int) {
		a := boxes[g]
		if a == nil {
			a = &gamestate.Area{RowMin: r, RowMax: r, ColMin: c, ColMax: c}
			boxes[g] = a
		}
		a.RowMin, a.RowMax = min(a.RowMin, r), max(a.RowMax, r)
		a.ColMin, a.ColMax = min(a.ColMin, c), max(a.ColMax, c)
	}
	for _, n := range s.NPCs {
		if n.Mode == world.NPCVillager {
			grow(n.Group, n.Cell.Row, n.Cell.Col)
		}
	}
	for _, st := range s.Game.Settlements {
		if boxes[st.Group] == nil {
			continue
		}
		for _, p := range st.Posts {
			if p.Entity != 0 {
				grow(st.Group, p.Row, p.Col)
			}
		}
	}
	for _, n := range s.NPCs {
		a := boxes[n.Group]
		if n.Mode != world.NPCVillager || a == nil {
			continue
		}
		n.Area = gamestate.Area{RowMin: max(a.RowMin-villageRowMargin, 0), RowMax: a.RowMax + villageRowMargin,
			ColMin: max(a.ColMin-villageColMargin, 0), ColMax: a.ColMax + villageColMargin}
		n.StepRows, n.StepCols = lanVillageRows, lanVillageCols
	}
}

// VillageArea is map m's village area (villageBounds), false without a
// village.
func (w *World) VillageArea(m int) (gamestate.Area, bool) {
	if m < 1 || m > Maps {
		return gamestate.Area{}, false
	}
	for _, n := range w.Maps[m].NPCs {
		if n.Mode == world.NPCVillager {
			return n.Area, true
		}
	}
	return gamestate.Area{}, false
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
			// A player's hero's kills (its hirelings' do not count, the
			// port's rule); the victim a player's hero or a hireling.
			if a.Index >= 0 && a.Index < world.LANPlayers {
				player := v.Group >= 0 && v.Group < world.LANPlayers
				w.events = append(w.events, Event{Kind: EventKill, Map: n, Entity: a.Index,
					Victim: v.Index, VictimPlayer: player && v.Index < world.LANPlayers,
					VictimHireling: player && v.Index >= world.LANPlayers})
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

// protected is the team-damage option: two players' heroes of one type,
// or their hirelings (a hireling is of its hero's team).
func (w *World) protected(a, t *gamestate.Entity) bool {
	ta, tt := w.team(a), w.team(t)
	return w.TeamSafe && ta >= 0 && ta == tt
}

// team is a player's hero's type (model), its hirelings' hero's; −1 for
// the world's characters.
func (w *World) team(e *gamestate.Entity) int {
	switch {
	case e.Index >= 0 && e.Index < world.LANPlayers:
		return e.Model
	case e.Group >= 0 && e.Group < world.LANPlayers:
		if _, ok := w.players[e.Group]; ok {
			return w.Game.Entities[e.Group].Model
		}
	}
	return -1
}

// Step runs one game tick (60 ms at normal speed) of every map, then moves
// the heroes that ended a walk on an exit to the exit's target.
func (w *World) Step() {
	w.Tick++
	for n := 1; n <= Maps; n++ {
		w.Maps[n].Step()
	}
	if (w.Tick+gamestate.SettlementTickOffset)%gamestate.UpkeepTicks == 0 {
		w.shopPass()
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
		w.moveParty(mv.slot)
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
// out of the stance, with the health it left with (full without an
// account), on the type's start (the nearest free cell),
// carrying a new hero's starting gear: only the knife, the item the
// template carries in the blade slot (category 2, type 18 in all three
// GAME.N; the original server: a new hero gets a knife). A player with saved
// items gets them instead (LoadItems). It returns the map and cell.
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
	knife := e.Equipment[gamestate.WeaponBlade]
	e.Equipment, e.Extra, e.Bag, e.Quiver = [6]int{}, [5]int{}, [32]int{}, 0
	e.Equipment[gamestate.WeaponBlade] = w.Game.CopyItemFrom(src, knife)
	e.WeaponMode = gamestate.WeaponBlade
	gamestate.LeaveStance(&e)
	e.Flags4 &^= 0x80
	e.Flags3 &^= 0x80
	if hero == nil {
		e.HP, e.Poison = gamestate.HPFull, 0
	} // else the health it left with (lanfight.ApplyHero)
	e.Index, e.Group = slot, slot
	w.Game.Entities[slot] = e
	g := &w.Game.Groups[slot]
	g.First, g.Count = slot, 1
	m := w.place(slot, start.To, mapdata.Cell{Row: start.Row, Col: start.Col}, start.Facing)
	w.startTalk(slot)
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
	w.sendHome(slot)      // its hirelings go back to their villages
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
	w.moveParty(slot)
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

// The orders go to the player's selection sel (its hero and hirelings on
// the hero's map; none: the hero), as single player's go to the selection
// (orderMembers).

// Walk is the left click on the map: walk to cell (row, col).
func (w *World) Walk(slot int, sel []int, row, col int) {
	s, ms := w.orderMembers(slot, sel)
	for _, m := range ms {
		s.WalkOrder(m, mapdata.Cell{Row: row, Col: col}, false)
	}
}

// Right is the right click (world.RightOrder): with target ≥ 0 (an entity
// on the hero's map, not of the player's party) a shot with the bow ready,
// a melee attack in the stance, nothing out of it; with target −1 a walk
// with a pick-up on (row, col).
func (w *World) Right(slot int, sel []int, target, row, col int) {
	s, ms := w.orderMembers(slot, sel)
	if s == nil {
		return
	}
	if target >= 0 {
		tw, t := s.Combatant(target)
		if tw == nil || target == slot || w.hireling(slot, target) {
			return // not on this map, or the player's own (0x446264 ignores its group)
		}
		if _, me := s.Member(slot); me != nil && w.protected(me, t) {
			return // a teammate the server protects
		}
	}
	for _, m := range ms {
		s.RightOrder(m, target, mapdata.Cell{Row: row, Col: col}, 0)
	}
}

// Stance puts the selection into the combat stance or out of it.
func (w *World) Stance(slot int, sel []int, on bool) {
	s, ms := w.orderMembers(slot, sel)
	for _, m := range ms {
		s.SetStance(m, on)
	}
}

// Weapon selects the selection's weapon (gamestate.WeaponAxe, WeaponBow,
// WeaponBlade or WeaponCycle; the client sends its first member, or all
// with Shift).
func (w *World) Weapon(slot int, sel []int, mode int) {
	s, ms := w.orderMembers(slot, sel)
	for _, m := range ms {
		s.SetWeapon(m, mode)
	}
}

// Attack is the A key: every living member of the party on the hero's map
// (single player's AttackOrder) takes the stance and the nearest enemy
// (creatures and hostile villagers; not other players).
func (w *World) Attack(slot int) {
	s, ms := w.partyMembers(slot)
	for _, m := range ms {
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

// reviveHeroes is «Слова Воскрешения» read on map m in a LAN world: every
// dead player's hero there comes back (world.World.Revive, the single-player
// 0x00414B3C: its cell or a free one next to it, its HP rule), wearing what
// it wore; its bag already lies where it fell (the original server: "кто-то
// из живых игроков прочтет на той же локации слова воскрешения"). Unlike
// single player's 0x00414FFB, dead creatures and villagers stay dead. It
// returns the slots revived.
func (w *World) reviveHeroes(m int) []int {
	if m < 1 || m > Maps {
		return nil
	}
	s := w.Maps[m]
	var out []int
	for _, mem := range s.Members {
		e := &s.Party[mem.K]
		if e.Dead() && e.Index >= 0 && e.Index < world.LANPlayers && s.Revive(e) {
			out = append(out, e.Index)
		}
	}
	return out
}

// ToStart puts slot's hero back at its type's start (the session's end,
// 0x00448BB8): alive (a dead hero revived, wearing what it wore), full HP,
// no poison, out of the stance, idle; a map change is reported
// (EventMapChange) when it was elsewhere.
func (w *World) ToStart(slot, model int) bool {
	from, ok := w.players[slot]
	if !ok {
		return false
	}
	start, err := w.Start(model)
	if err != nil {
		return false
	}
	w.closeItems(slot)
	w.sendHome(slot) // the session's end: the hirelings go back to their villages
	e, ok := w.Maps[from].RemoveMember(slot)
	if !ok {
		return false
	}
	e.Flags4 &^= 0x80
	e.HP, e.Poison = gamestate.HPFull, 0
	gamestate.LeaveStance(&e)
	w.Game.Entities[slot] = e
	mem := w.place(slot, start.To, mapdata.Cell{Row: start.Row, Col: start.Col}, start.Facing)
	if start.To != from {
		w.events = append(w.events, Event{Kind: EventMapChange, Map: start.To, From: from, Entity: slot,
			Row: mem.Cell.Row, Col: mem.Cell.Col, Facing: start.Facing})
	}
	return true
}
