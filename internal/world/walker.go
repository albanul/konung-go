// Package world is the game simulation without the engine: walkers on the
// fine grid, the party, the creature and village AI, fights, arrows, ground
// items and map exits, one game tick (60 ms) at a time. Single-player
// (internal/game) draws it; the dedicated server runs it for the LAN world.
// See docs/konung_movement.md, docs/konung_combat.md, docs/konung_party.md.
package world

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

const (
	// DefaultWalkSteps is the walk frame count when a look has no walk
	// frames. A step into the next cell shows one walk frame per game tick
	// (0x00413DE4), so a 12-frame walk takes 12 ticks = 0.72 s per cell.
	DefaultWalkSteps = 12
	// WalkTypeOffset: walk = idle type + 1 (6 idle, 7 walk; 0/1 in the
	// combat stance), seen in the body records.
	WalkTypeOffset = 1
)

// FrameCounter is the number of frames of an animation: kind (0 idle,
// 1 walk, or an action gamestate.ActFlinch, ActDeath, ActAttack, ActShoot)
// in look l facing dir. Frame counts drive the walk steps and the actions'
// durations (0x00413DE4); the game counts its loaded sprites, the server
// decodes them without drawing.
type FrameCounter func(l Look, kind, dir int) int

// Walker is a character that walks the fine grid like the original
// (docs/konung_movement.md): Update runs once per game tick; a step first
// turns one octant per tick toward the next cell, then shows one walk frame
// per tick. Like the game (0x0041492E) it enters the next cell at the start
// of a step: Cell becomes that cell and the one left (From) is freed at
// once; the body is drawn from the centre of From and the walk frames'
// anchors carry it across. The hero and the NPCs share it.
type Walker struct {
	Entity  int            // GAME.N entity index (name, status line)
	Cell    mapdata.Cell   // the cell held; while walking, the one being entered
	From    mapdata.Cell   // while walking: the cell being left (the drawing anchor)
	Path    []mapdata.Cell // the cells after the step in progress
	Goal    mapdata.Cell
	Dir     mapdata.Direction
	Walking bool
	StepT   int // walk frame of the current cell step (one per tick)
	// Idle is the idle loop's frame (+0x10 while idle), −1 while busy;
	// see World.IdleLoop.
	Idle int

	// Look selects the frames besides kind and facing; change it when the
	// entity's appearance changes (combat stance, weapon). Frames counts
	// them (nil: no frames); Sprites is the drawing side's own data.
	Look    Look
	Frames  FrameCounter
	Sprites any

	// Act is an action animation other than idle/walk (gamestate.ActFlinch,
	// ActDeath, ActAttack; 0 = none), played one step per game tick from
	// ActStep 0 and held on its last step (0x00413DE4).
	Act, ActStep int
	// StopAt, when set, ends the walk before stepping into a cell it
	// accepts (the hero's attack target, 0x004149B0).
	StopAt func(mapdata.Cell) bool

	Blocked func(mapdata.Cell) bool // cells this walker may not path through
	Reserve func(mapdata.Cell) bool // claim the next cell; false if taken
	Release func(mapdata.Cell)      // free the cell just left
	// Hold, when set, is asked once the step's frames have played: true
	// keeps the walker there this tick (the pauses of creature kinds
	// 0x4D/0x4F, 0x004127BD).
	Hold func() bool
	// Sound plays the action sound (0x004143DC).
	Sound func(w *Walker, act int)
}

// Look is what selects an entity's frames besides kind and facing: its
// animation set (+0, combat 0–5 or peaceful 6–11), stance, weapon and the
// worn equipment.
type Look struct {
	Typ, Weapon int
	Stance      bool
	Equipment   [6]int
}

// LookOf is e's look.
func LookOf(e gamestate.Entity) Look {
	return Look{Typ: AnimBase(e.Type), Weapon: e.WeaponMode, Stance: e.InStance(), Equipment: e.Equipment}
}

// AnimBase is the animation set an action type belongs to: 0 (combat
// stance, also animated models) or 6.
func AnimBase(typ int) int {
	if typ >= 6 {
		return 6
	}
	return 0
}

// FrameCount is the number of frames of kind in the current look and
// facing (0 without a FrameCounter).
func (w *Walker) FrameCount(kind int) int {
	if w.Frames == nil {
		return 0
	}
	return w.Frames(w.Look, kind, int(w.Dir))
}

// WalkTo plans a path to the free cell nearest to goal. A walker already
// stepping finishes the step it is on before following the new path.
func (w *Walker) WalkTo(goal mapdata.Cell) {
	goal = mapdata.NearestFree(goal, w.Blocked, 6)
	start := w.Cell // while walking, the cell being entered
	path := mapdata.FindPath(start, goal, w.Blocked)
	if len(path) > 0 || start == goal {
		w.Path, w.Goal = path, goal
		if len(path) > 0 && w.Sound != nil {
			w.Sound(w, 1) // 0x004142D4 starts the walk (action 1)
		}
	}
}

// WalkSteps is the walk's frame count, DefaultWalkSteps without frames.
func (w *Walker) WalkSteps() int {
	if n := w.FrameCount(1); n > 0 {
		return n
	}
	return DefaultWalkSteps
}

// StepDir is the direction of the step in progress, from From to Cell.
func (w *Walker) StepDir() mapdata.Direction { return mapdata.StepDirection(w.From, w.Cell) }

// startStep enters Path[0] (0x0041492E): the walker takes the cell and
// frees the one it leaves at once. If someone holds the cell the walk
// stops and the path is dropped; the AI or the party planner decides again
// on a later tick.
func (w *Walker) startStep() bool {
	next := w.Path[0]
	if w.StopAt != nil && w.StopAt(next) {
		w.Path = nil
		return false
	}
	if w.Reserve != nil && !w.Reserve(next) {
		w.Path = nil
		return false
	}
	if w.Release != nil {
		w.Release(w.Cell)
	}
	w.From, w.Cell, w.Path = w.Cell, next, w.Path[1:]
	return true
}

// Update is one game tick of the walk case (0x0042218D, 0x00412717): a
// walker not facing the step's direction turns one octant (0x004391BC) and
// does nothing else; otherwise it shows the next walk frame, and after the
// last one it enters the next cell and starts the next step.
func (w *Walker) Update() {
	if !w.Walking {
		if len(w.Path) == 0 || !w.startStep() {
			return
		}
		w.Walking, w.StepT = true, 0
	}
	if d := w.StepDir(); w.Dir != d {
		w.TurnToward(d)
		return
	}
	if w.StepT+1 < w.WalkSteps() {
		w.StepT++
		return
	}
	if w.Hold != nil && w.Hold() {
		return
	}
	w.StepT = 0
	if len(w.Path) == 0 || !w.startStep() {
		w.Walking = false
	}
}

// Position is a smooth world position (for the camera): the current cell's
// centre moved toward the next cell by the step's progress.
func (w *Walker) Position() (float64, float64) {
	if !w.Walking {
		x, y := mapdata.CellCenter(w.Cell)
		return float64(x), float64(y)
	}
	ax, ay := mapdata.CellCenter(w.From)
	bx, by := mapdata.CellCenter(w.Cell)
	t := float64(w.StepT) / float64(w.WalkSteps())
	return float64(ax) + float64(bx-ax)*t, float64(ay) + float64(by-ay)*t
}

// StartAct starts action animation act at step 0 (0x004143DC).
func (w *Walker) StartAct(act int) {
	w.Act, w.ActStep = act, 0
	if w.Sound != nil {
		w.Sound(w, act)
	}
}

// AdvanceAct is 0x00413DE4: the next step of the action, or false (staying
// on the last step) when the action has played out.
func (w *Walker) AdvanceAct() bool {
	if w.ActStep+1 >= w.FrameCount(w.Act) {
		return false
	}
	w.ActStep++
	return true
}

// ActSteps is the action's step count (+0xD7).
func (w *Walker) ActSteps() int { return w.FrameCount(w.Act) }

// TurnToward turns one octant toward d, the shorter way (0x004391BC).
func (w *Walker) TurnToward(d mapdata.Direction) { w.Dir = mapdata.TurnToward(w.Dir, d) }

// Anchor is the cell the body is drawn from: while walking the cell being
// left (+0xE8/+0xF0 keep its centre until the step ends), else the cell.
func (w *Walker) Anchor() mapdata.Cell {
	if w.Walking {
		return w.From
	}
	return w.Cell
}

// Halt ends a walk at once. The walker stays in the cell it was entering
// (it took it at the step's start), so the body jumps there, as when the
// game starts another action mid-step (0x004143DC sets the position from
// the cell).
func (w *Walker) Halt() {
	w.Walking, w.StepT, w.Path = false, 0, nil
}

// StopWalk ends a walk after the step in progress.
func (w *Walker) StopWalk() { w.Path = nil }

// Chase plans a path that ends on goal even though goal is occupied (the
// target's cell); StopAt keeps the walker from entering it.
func (w *Walker) Chase(goal mapdata.Cell) {
	blocked := func(c mapdata.Cell) bool { return c != goal && w.Blocked(c) }
	w.Path, w.Goal = mapdata.FindPath(w.Cell, goal, blocked), goal
}

// NPCMode selects an NPC's behaviour, following the group dispatch of the
// AI tick at VA 0x00421DA4.
type NPCMode int

const (
	NPCStill    NPCMode = iota // creatures while calm, and groups without a ported AI: stay put
	NPCVillager                // village group (kind 0x80): ported AI 0x00416934
	NPCCreature                // creature group (kind 0x40): combat AI 0x004123BC (combat.go)
	NPCRemote                  // another player's hero in a LAN world: walks where the server says
)

// NPC is a non-player character.
//
// Villagers run the game's village AI once per AI tick while idle: a posted
// villager (elder, merchant, healer…) returns to its post cell and turns to
// the post's facing; any other villager rolls rand()%100 and, at or above its
// threshold, walks to a gamestate.VillageStepTarget.
//
// Creatures (group kind 0x40, AI 0x004123BC) stand still and play their idle
// loop while their group is calm: the calm case of 0x00412E00 only restarts
// the idle animation, and the idle case of the type switch (0x0041291C) only
// advances it. Their group rectangle is a territory: 0x00412C30 alarms the
// group when a party member is inside it and sends members outside it home;
// alarmed members chase and fight (combat.go).
type NPC struct {
	Walker
	Mode     NPCMode
	LiveMode NPCMode // the mode to return to when revived (0x00414B3C)
	Area     gamestate.Area
	Post     gamestate.Post
	Posted   bool
	MoveAt   int // village roll threshold (gamestate.VillageMoveThreshold)
	// StepRows, StepCols are a villager's random-step ranges; 0 takes the
	// single-player view-size globals (gamestate.VillageRowRange,
	// VillageColRange). The LAN server's 0x00437E28 uses 20 and 10.
	StepRows, StepCols int

	// Creature combat state (NPCCreature): group index, the attack target
	// (entity +6, -1 = none) and its cell (+0x0C/+0x0E), and the +2 intent
	// bits (IntentMelee, IntentRetaliate, IntentWalk).
	Group      int
	Target     int
	TargetCell mapdata.Cell
	Intent     byte
	Rng        func(int) int
	// Pause is +0x12 of creature kinds 0x4D/0x4F: ticks still to wait at
	// the end of a walk step (PauseHold).
	Pause int
}

// PauseHold is 0x004127BD for creature kinds 0x4D/0x4F, asked when a walk
// step's frames have played: with no pause running it starts one of
// (rand() % 2) · (rand() % frames, 0 below 2 frames) + 1 ticks and waits;
// otherwise it counts down and lets the walk go on when it reaches 0.
func (n *NPC) PauseHold() bool {
	if n.Pause == 0 {
		k := 0
		if f := n.WalkSteps(); f >= 2 {
			k = n.Rng(f)
		}
		n.Pause = n.Rng(2)*k + 1
		return true
	}
	n.Pause--
	return n.Pause != 0
}

// IsIdle reports an NPC neither walking nor acting.
func (n *NPC) IsIdle() bool { return !n.Walking && len(n.Path) == 0 && n.Act == 0 }

// VillageTick is one AI tick of 0x00416934 for an idle villager. budget is
// the tick's remaining path searches; it returns the number used. apprentice
// is gamestate.Game.ApprenticePost for the villager.
func (n *NPC) VillageTick(budget int, apprentice func() (gamestate.Post, bool)) int {
	if !n.IsIdle() || budget <= 0 {
		return 0
	}
	if p, ok := apprentice(); ok {
		return n.apprenticeTick(p)
	}
	if n.Posted {
		if at := (mapdata.Cell{Row: n.Post.Row, Col: n.Post.Col}); n.Cell != at {
			n.WalkTo(at)
			return 1
		}
		n.Dir = mapdata.Direction(n.Post.Facing & 7)
		return 0
	}
	if n.Rng(100) < n.MoveAt {
		return 0
	}
	blocked := func(r, c int) bool { return n.Blocked(mapdata.Cell{Row: r, Col: c}) }
	rr, cr := gamestate.VillageRowRange, gamestate.VillageColRange
	if n.StepRows > 0 {
		rr, cr = n.StepRows, n.StepCols
	}
	r, c, ok := gamestate.VillageStepTarget(n.Cell.Row, n.Cell.Col, n.Area, rr, cr, n.Rng, blocked)
	if !ok {
		return 0
	}
	n.WalkTo(mapdata.Cell{Row: r, Col: c})
	return 1
}

// apprenticeTick is the apprentice branch of 0x00416934 (0x00416A94): the
// apprentice stands on a cell next to its master's post (0x0041165C: one of
// the post cell's 8 neighbours) and turns to the post's facing. Elsewhere,
// the post cell included, it walks to the post's first free neighbour
// (0x00410FFC); with none it stays idle.
func (n *NPC) apprenticeTick(p gamestate.Post) int {
	at := mapdata.Cell{Row: p.Row, Col: p.Col}
	nb := mapdata.Neighbors(at)
	for _, c := range nb {
		if c.Cell == n.Cell {
			n.Dir = mapdata.Direction(p.Facing & 7)
			return 0
		}
	}
	for _, c := range nb {
		if c.Row >= 0 && c.Row < mapdata.WalkRows && c.Col >= 0 && c.Col < mapdata.WalkCols && !n.Blocked(c.Cell) {
			n.WalkTo(c.Cell)
			return 1
		}
	}
	return 0
}

// Member is a party member's walker (party.go).
type Member struct {
	Walker
	K int // index in World.Party

	// Intent mirrors entity +2 (gamestate.Intent* bits): melee, strike
	// (retaliating; set on members other than the hero by a walk order,
	// which keeps them from following), shoot, walk, follow, idle.
	Intent byte
	// Target is +6 (entity index, -1 = none) and TargetCell its cell when
	// last checked (+0x0C/+0x0E); BurnTarget is placement + 1 of a building
	// shot at with incendiary arrows (+6 < 0), 0 = none.
	Target     int
	TargetCell mapdata.Cell
	BurnTarget int
	PickIntent bool // +3 bit 0x40: pick up where the walk ends
	// Resume is the walk order's goal kept through a hit reaction: the
	// planner (0x0041ED8C, +2 bit 0x08) walks on afterwards.
	Resume    mapdata.Cell
	ResumeSet bool
}

// Melee reports the melee intent.
func (m *Member) Melee() bool { return m.Intent&gamestate.IntentMelee != 0 }

// Shoot reports the shoot intent.
func (m *Member) Shoot() bool { return m.Intent&gamestate.IntentShoot != 0 }
