package game

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

const (
	// defaultWalkSteps is the walk frame count when a look has no walk
	// frames. A step into the next cell shows one walk frame per game tick
	// (0x00413DE4), so a 12-frame walk takes 12 ticks = 0.72 s per cell.
	defaultWalkSteps = 12
	walkTypeOffset   = 1 // walk = idle type + 1 (6 idle, 7 walk; 0/1 in the combat stance), seen in the body records
)

// walker is a character that walks the fine grid like the original
// (docs/konung_movement.md): update runs once per game tick; a step first
// turns one octant per tick toward the next cell, then shows one walk frame
// per tick. Like the game (0x0041492E) it enters the next cell at the start
// of a step: cell becomes that cell and the one left (from) is freed at
// once; the body is drawn from the centre of from and the walk frames'
// anchors carry it across. The hero and the NPCs share it.
type walker struct {
	entity  int            // GAME.N entity index (name, status line)
	cell    mapdata.Cell   // the cell held; while walking, the one being entered
	from    mapdata.Cell   // while walking: the cell being left (the drawing anchor)
	path    []mapdata.Cell // the cells after the step in progress
	goal    mapdata.Cell
	dir     mapdata.Direction
	walking bool
	stepT   int // walk frame of the current cell step (one per tick)
	// idle is the idle loop's frame (+0x10 while idle), −1 while busy;
	// see startScene.idleLoop.
	idle int

	// Frames per look (stance, weapon), kind (0 idle, 1 walk) and facing,
	// as offsets from the cell centre. load fills missing entries on first
	// use for the current look; change look when the entity's appearance
	// changes (combat stance, weapon).
	look  look
	anims map[animKey][]sceneFrame
	load  func(kind, dir int) []sceneFrame

	// act is an action animation other than idle/walk (gamestate.ActFlinch,
	// ActDeath, ActAttack; 0 = none), played one step per game tick from
	// actStep 0 and held on its last step (0x00413DE4).
	act, actStep int
	// stopAt, when set, ends the walk before stepping into a cell it
	// accepts (the hero's attack target, 0x004149B0).
	stopAt func(mapdata.Cell) bool

	blocked func(mapdata.Cell) bool // cells this walker may not path through
	reserve func(mapdata.Cell) bool // claim the next cell; false if taken
	release func(mapdata.Cell)      // free the cell just left
	// hold, when set, is asked once the step's frames have played: true
	// keeps the walker there this tick (the pauses of creature kinds
	// 0x4D/0x4F, 0x004127BD).
	hold func() bool
	// sound plays the action sound (0x004143DC).
	sound func(w *walker, act int)
}

// look is what selects an entity's frames besides kind and facing: its
// animation set (+0, combat 0–5 or peaceful 6–11), stance, weapon and the
// worn equipment.
type look struct {
	typ, weapon int
	stance      bool
	equipment   [6]int
}

func lookOf(e gamestate.Entity) look {
	return look{typ: animBase(e.Type), weapon: e.WeaponMode, stance: e.InStance(), equipment: e.Equipment}
}

// animBase is the animation set an action type belongs to: 0 (combat
// stance, also animated models) or 6.
func animBase(typ int) int {
	if typ >= 6 {
		return 6
	}
	return 0
}

type animKey struct {
	look      look
	kind, dir int
}

// framesFor returns the frames for kind (an action type: 0 idle, 1 walk,
// gamestate.ActFlinch, ActDeath, ActAttack) in the current
// look and facing, loading them on first use.
func (w *walker) framesFor(kind int) []sceneFrame {
	k := animKey{w.look, kind, int(w.dir)}
	f, ok := w.anims[k]
	if !ok && w.load != nil {
		if w.anims == nil {
			w.anims = map[animKey][]sceneFrame{}
		}
		f = w.load(kind, int(w.dir))
		w.anims[k] = f
	}
	return f
}

// walkTo plans a path to the free cell nearest to goal. A walker already
// stepping finishes the step it is on before following the new path.
func (w *walker) walkTo(goal mapdata.Cell) {
	goal = mapdata.NearestFree(goal, w.blocked, 6)
	start := w.cell // while walking, the cell being entered
	path := mapdata.FindPath(start, goal, w.blocked)
	if len(path) > 0 || start == goal {
		w.path, w.goal = path, goal
		if len(path) > 0 && w.sound != nil {
			w.sound(w, 1) // 0x004142D4 starts the walk (action 1)
		}
	}
}

func (w *walker) walkSteps() int {
	if n := len(w.framesFor(1)); n > 0 {
		return n
	}
	return defaultWalkSteps
}

// stepDir is the direction of the step in progress, from w.from to w.cell.
func (w *walker) stepDir() mapdata.Direction {
	for _, n := range mapdata.Neighbors(w.from) {
		if n.Cell == w.cell {
			return n.Dir
		}
	}
	ax, ay := mapdata.CellCenter(w.from)
	bx, by := mapdata.CellCenter(w.cell)
	return mapdata.DirectionTo(float64(ax), float64(ay), float64(bx), float64(by))
}

// startStep enters path[0] (0x0041492E): the walker takes the cell and
// frees the one it leaves at once. If someone holds the cell the walk
// stops and the path is dropped; the AI or the party planner decides again
// on a later tick.
func (w *walker) startStep() bool {
	next := w.path[0]
	if w.stopAt != nil && w.stopAt(next) {
		w.path = nil
		return false
	}
	if w.reserve != nil && !w.reserve(next) {
		w.path = nil
		return false
	}
	if w.release != nil {
		w.release(w.cell)
	}
	w.from, w.cell, w.path = w.cell, next, w.path[1:]
	return true
}

// update is one game tick of the walk case (0x0042218D, 0x00412717): a
// walker not facing the step's direction turns one octant (0x004391BC) and
// does nothing else; otherwise it shows the next walk frame, and after the
// last one it enters the next cell and starts the next step.
func (w *walker) update() {
	if !w.walking {
		if len(w.path) == 0 || !w.startStep() {
			return
		}
		w.walking, w.stepT = true, 0
	}
	if d := w.stepDir(); w.dir != d {
		w.turnToward(d)
		return
	}
	if w.stepT+1 < w.walkSteps() {
		w.stepT++
		return
	}
	if w.hold != nil && w.hold() {
		return
	}
	w.stepT = 0
	if len(w.path) == 0 || !w.startStep() {
		w.walking = false
	}
}

// position is a smooth world position (for the camera): the current cell's
// centre moved toward the next cell by the step's progress.
func (w *walker) position() (float64, float64) {
	if !w.walking {
		x, y := mapdata.CellCenter(w.cell)
		return float64(x), float64(y)
	}
	ax, ay := mapdata.CellCenter(w.from)
	bx, by := mapdata.CellCenter(w.cell)
	t := float64(w.stepT) / float64(w.walkSteps())
	return float64(ax) + float64(bx-ax)*t, float64(ay) + float64(by-ay)*t
}

// frame returns the current animation frame in world pixels. Walk frames are
// drawn from the cell being left; their anchors carry the body forward.
func (w *walker) frame(tick int) (sceneFrame, bool) {
	var f sceneFrame
	if frames := w.framesFor(w.act); w.act != 0 && len(frames) > 0 {
		f = frames[min(w.actStep, len(frames)-1)]
	} else if frames := w.framesFor(1); w.walking && len(frames) > 0 {
		f = frames[min(w.stepT, len(frames)-1)]
	} else if frames := w.framesFor(0); len(frames) > 0 {
		f = frames[max(w.idle, 0)%len(frames)]
	} else {
		return f, false
	}
	x, y := mapdata.CellCenter(w.anchor())
	f.x, f.y, f.sx, f.sy = f.x+x, f.y+y, f.sx+x, f.sy+y
	return f, true
}

// startAct starts action animation act at step 0 (0x004143DC).
func (w *walker) startAct(act int) {
	w.act, w.actStep = act, 0
	if w.sound != nil {
		w.sound(w, act)
	}
}

// advanceAct is 0x00413DE4: the next step of the action, or false (staying
// on the last step) when the action has played out.
func (w *walker) advanceAct() bool {
	if w.actStep+1 >= len(w.framesFor(w.act)) {
		return false
	}
	w.actStep++
	return true
}

// actSteps is the action's step count (+0xD7).
func (w *walker) actSteps() int { return len(w.framesFor(w.act)) }

// turnToward turns one octant toward d, the shorter way (0x004391BC).
func (w *walker) turnToward(d mapdata.Direction) {
	cur, t := int(w.dir), int(d)
	if cur > t {
		t += 8
	}
	if t-cur > 4 {
		cur = (cur + 7) % 8
	} else {
		cur = (cur + 1) % 8
	}
	w.dir = mapdata.Direction(cur)
}

// anchor is the cell the body is drawn from: while walking the cell being
// left (+0xE8/+0xF0 keep its centre until the step ends), else the cell.
func (w *walker) anchor() mapdata.Cell {
	if w.walking {
		return w.from
	}
	return w.cell
}

// halt ends a walk at once. The walker stays in the cell it was entering
// (it took it at the step's start), so the body jumps there, as when the
// game starts another action mid-step (0x004143DC sets the position from
// the cell).
func (w *walker) halt() {
	w.walking, w.stepT, w.path = false, 0, nil
}

// stopWalk ends a walk after the step in progress.
func (w *walker) stopWalk() { w.path = nil }

// chase plans a path that ends on goal even though goal is occupied (the
// target's cell); stopAt keeps the walker from entering it.
func (w *walker) chase(goal mapdata.Cell) {
	blocked := func(c mapdata.Cell) bool { return c != goal && w.blocked(c) }
	w.path, w.goal = mapdata.FindPath(w.cell, goal, blocked), goal
}

// liveEntity returns the current GAME.N record of entity i, for loaders that
// must see runtime changes (the stance moves the action type to the set
// 0–5, re-equipping changes the weapon).
func liveEntity(game *gamestate.Game, i int) func() gamestate.Entity {
	return func() gamestate.Entity { return game.Entities[i] }
}

// entityLoader returns a lazy loader of idle/walk frames for an entity, with
// equipment, as offsets from its cell centre. cur returns the entity's
// current state (stance, weapon); its type is the idle type of the current
// animation set. Errors leave the entry empty.
func entityLoader(game *gamestate.Game, p gamestate.Placed, cur func() gamestate.Entity, objects, graph []byte, t *resfmt.ExeTables,
	upload func(*resfmt.EntitySprite) (*ebiten.Image, *ebiten.Image), onErr func(error)) func(kind, dir int) []sceneFrame {
	return func(kind, dir int) []sceneFrame {
		q := p
		q.Entity = cur()
		q.Type, q.Variant, q.Step = animBase(q.Type)+kind*walkTypeOffset, dir, 0
		anim, err := game.LoadAnimation(q, objects, graph, t)
		if err != nil {
			onErr(fmt.Errorf("entity %d type %d dir %d: %w", p.Index, q.Type, dir, err))
			return []sceneFrame{}
		}
		wx, wy := gamestate.WorldPos(p.Row, p.Col)
		out := make([]sceneFrame, 0, len(anim))
		for _, f := range anim {
			img, sh := upload(f.Sprite)
			out = append(out, sceneFrame{img: img, shadow: sh, x: f.X - wx, y: f.Y - wy, sx: f.SX - wx, sy: f.SY - wy})
		}
		return out
	}
}

// npcMode selects an NPC's behaviour, following the group dispatch of the AI
// tick at VA 0x00421DA4.
type npcMode int

const (
	npcStill    npcMode = iota // creatures while calm, and groups without a ported AI: stay put
	npcVillager                // village group (kind 0x80): ported AI 0x00416934
	npcCreature                // creature group (kind 0x40): combat AI 0x004123BC (combat.go)
)

// npc is a non-player character.
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
type npc struct {
	walker
	mode     npcMode
	liveMode npcMode // the mode to return to when revived (0x00414B3C)
	area     gamestate.Area
	post     gamestate.Post
	posted   bool
	moveAt   int // village roll threshold (gamestate.VillageMoveThreshold)

	// Creature combat state (npcCreature): group index, the attack target
	// (entity +6, -1 = none) and its cell (+0x0C/+0x0E), and the +2 intent
	// bits (intentMelee, intentRetaliate, intentWalk).
	group      int
	target     int
	targetCell mapdata.Cell
	intent     byte
	rng        func(int) int
	// pause is +0x12 of creature kinds 0x4D/0x4F: ticks still to wait at
	// the end of a walk step (pauseHold).
	pause int
}

// pauseHold is 0x004127BD for creature kinds 0x4D/0x4F, asked when a walk
// step's frames have played: with no pause running it starts one of
// (rand() % 2) · (rand() % frames, 0 below 2 frames) + 1 ticks and waits;
// otherwise it counts down and lets the walk go on when it reaches 0.
func (n *npc) pauseHold() bool {
	if n.pause == 0 {
		k := 0
		if f := n.walkSteps(); f >= 2 {
			k = n.rng(f)
		}
		n.pause = n.rng(2)*k + 1
		return true
	}
	n.pause--
	return n.pause != 0
}

func (n *npc) idle() bool { return !n.walking && len(n.path) == 0 && n.act == 0 }

// villageTick is one AI tick of 0x00416934 for an idle villager. budget is
// the tick's remaining path searches; it returns the number used. apprentice
// is gamestate.Game.ApprenticePost for the villager.
func (n *npc) villageTick(budget int, apprentice func() (gamestate.Post, bool)) int {
	if !n.idle() || budget <= 0 {
		return 0
	}
	if p, ok := apprentice(); ok {
		return n.apprenticeTick(p)
	}
	if n.posted {
		if at := (mapdata.Cell{Row: n.post.Row, Col: n.post.Col}); n.cell != at {
			n.walkTo(at)
			return 1
		}
		n.dir = mapdata.Direction(n.post.Facing & 7)
		return 0
	}
	if n.rng(100) < n.moveAt {
		return 0
	}
	blocked := func(r, c int) bool { return n.blocked(mapdata.Cell{Row: r, Col: c}) }
	r, c, ok := gamestate.VillageStepTarget(n.cell.Row, n.cell.Col, n.area,
		gamestate.VillageRowRange, gamestate.VillageColRange, n.rng, blocked)
	if !ok {
		return 0
	}
	n.walkTo(mapdata.Cell{Row: r, Col: c})
	return 1
}

// apprenticeTick is the apprentice branch of 0x00416934 (0x00416A94): the
// apprentice stands on a cell next to its master's post (0x0041165C: one of
// the post cell's 8 neighbours) and turns to the post's facing. Elsewhere,
// the post cell included, it walks to the post's first free neighbour
// (0x00410FFC); with none it stays idle.
func (n *npc) apprenticeTick(p gamestate.Post) int {
	at := mapdata.Cell{Row: p.Row, Col: p.Col}
	nb := mapdata.Neighbors(at)
	for _, c := range nb {
		if c.Cell == n.cell {
			n.dir = mapdata.Direction(p.Facing & 7)
			return 0
		}
	}
	for _, c := range nb {
		if c.Row >= 0 && c.Row < mapdata.WalkRows && c.Col >= 0 && c.Col < mapdata.WalkCols && !n.blocked(c.Cell) {
			n.walkTo(c.Cell)
			return 1
		}
	}
	return 0
}
