package game

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

const (
	// walkTicksPerStep is how many 60 Hz updates each walk animation step
	// lasts. One 12-step walk cycle moves a character exactly one fine cell
	// (the frames' anchors carry the displacement), so this sets the walking
	// speed: 4 gives 0.8 s per cell. The original tick rate is not traced.
	walkTicksPerStep = 4
	defaultWalkSteps = 12
	// pathSearchLimit caps A* exploration so a click on an unreachable spot
	// can't stall a frame.
	pathSearchLimit = 30000
	walkTypeOffset  = 1 // walk = idle type + 1 (6 idle, 7 walk; 0/1 in the combat stance), seen in the body records
)

// walker is a character that walks the fine grid like the original: one cell
// per walk cycle. While stepping it stays anchored on its current cell and the
// walk frames' per-step anchors carry the body toward the next cell; at the
// end of the cycle it snaps to that cell. The hero and wandering NPCs share it.
type walker struct {
	entity  int // GAME.N entity index (name, status line)
	cell    mapdata.Cell
	path    []mapdata.Cell // path[0] is the cell being stepped into while walking
	goal    mapdata.Cell
	dir     mapdata.Direction
	walking bool
	stepT   int // ticks into the current cell step
	phase   int // idle animation offset

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

	blocked   func(mapdata.Cell) bool // cells this walker may not path through
	reserve   func(mapdata.Cell) bool // claim the next cell; false if taken
	release   func(mapdata.Cell)      // free the cell just left
	searchCap int
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
	start := w.cell
	if w.walking {
		start = w.path[0]
	}
	path := mapdata.FindPath(start, goal, w.blocked, w.searchCap)
	if w.walking {
		path = append([]mapdata.Cell{start}, path...)
	}
	if len(path) > 0 || start == goal {
		w.path, w.goal = path, goal
	}
}

func (w *walker) walkSteps() int {
	if n := len(w.framesFor(1)); n > 0 {
		return n
	}
	return defaultWalkSteps
}

// faceNext turns toward path[0] using the grid's neighbour table.
func (w *walker) faceNext() {
	for _, n := range mapdata.Neighbors(w.cell) {
		if n.Cell == w.path[0] {
			w.dir = n.Dir
			return
		}
	}
	ax, ay := mapdata.CellCenter(w.cell)
	bx, by := mapdata.CellCenter(w.path[0])
	w.dir = mapdata.DirectionTo(float64(ax), float64(ay), float64(bx), float64(by))
}

// startStep claims path[0]. If someone took it, it replans once toward the
// goal; if that fails too, the walker stops.
func (w *walker) startStep() bool {
	if w.stopAt != nil && w.stopAt(w.path[0]) {
		w.path = nil
		return false
	}
	if w.reserve == nil || w.reserve(w.path[0]) {
		w.faceNext()
		return true
	}
	if p := mapdata.FindPath(w.cell, w.goal, w.blocked, w.searchCap); len(p) > 0 && w.reserve(p[0]) {
		w.path = p
		w.faceNext()
		return true
	}
	w.path = nil
	return false
}

// update advances the walk cycle by one tick.
func (w *walker) update() {
	if !w.walking {
		if len(w.path) == 0 || !w.startStep() {
			return
		}
		w.walking, w.stepT = true, 0
	}
	w.stepT++
	if w.stepT < w.walkSteps()*walkTicksPerStep {
		return
	}
	old := w.cell
	w.cell, w.path, w.stepT = w.path[0], w.path[1:], 0
	if w.release != nil {
		w.release(old)
	}
	if len(w.path) == 0 || !w.startStep() {
		w.walking = false
	}
}

// position is a smooth world position (for the camera): the current cell's
// centre moved toward the next cell by the step's progress.
func (w *walker) position() (float64, float64) {
	ax, ay := mapdata.CellCenter(w.cell)
	if !w.walking {
		return float64(ax), float64(ay)
	}
	bx, by := mapdata.CellCenter(w.path[0])
	t := float64(w.stepT) / float64(w.walkSteps()*walkTicksPerStep)
	return float64(ax) + float64(bx-ax)*t, float64(ay) + float64(by-ay)*t
}

// frame returns the current animation frame in world pixels. Walk frames are
// drawn from the cell being left; their anchors carry the body forward.
func (w *walker) frame(tick int) (sceneFrame, bool) {
	var f sceneFrame
	if frames := w.framesFor(w.act); w.act != 0 && len(frames) > 0 {
		f = frames[min(w.actStep, len(frames)-1)]
	} else if frames := w.framesFor(1); w.walking && len(frames) > 0 {
		f = frames[min(w.stepT/walkTicksPerStep, len(frames)-1)]
	} else if frames := w.framesFor(0); len(frames) > 0 {
		f = frames[(tick/ticksPerStep+w.phase)%len(frames)]
	} else {
		return f, false
	}
	x, y := mapdata.CellCenter(w.cell)
	f.x, f.y, f.sx, f.sy = f.x+x, f.y+y, f.sx+x, f.sy+y
	return f, true
}

// startAct starts action animation act at step 0 (0x004143DC).
func (w *walker) startAct(act int) { w.act, w.actStep = act, 0 }

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

// halt ends a walk at once, releasing the cell being stepped into.
func (w *walker) halt() {
	if w.walking && w.release != nil && len(w.path) > 0 {
		w.release(w.path[0])
	}
	w.walking, w.stepT, w.path = false, 0, nil
}

// stopWalk ends a walk after the step in progress.
func (w *walker) stopWalk() {
	if w.walking {
		w.path = w.path[:1]
		return
	}
	w.path = nil
}

// chase plans a path that ends on goal even though goal is occupied (the
// target's cell); stopAt keeps the walker from entering it.
func (w *walker) chase(goal mapdata.Cell) {
	start := w.cell
	if w.walking {
		start = w.path[0]
	}
	blocked := func(c mapdata.Cell) bool { return c != goal && w.blocked(c) }
	path := mapdata.FindPath(start, goal, blocked, w.searchCap)
	if w.walking {
		path = append([]mapdata.Cell{start}, path...)
	}
	w.path, w.goal = path, goal
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
	mode   npcMode
	area   gamestate.Area
	post   gamestate.Post
	posted bool
	moveAt int // village roll threshold (gamestate.VillageMoveThreshold)

	// Creature combat state (npcCreature): group index, the attack target
	// (entity +6, -1 = none) and its cell (+0x0C/+0x0E), and the +2 intent
	// bits (intentMelee, intentRetaliate, intentWalk).
	group      int
	target     int
	targetCell mapdata.Cell
	intent     byte
	rng        func(int) int
}

const (
	npcSearchCap = 4000
)

func (n *npc) idle() bool { return !n.walking && len(n.path) == 0 && n.act == 0 }

// villageTick is one AI tick of 0x00416934 for an idle villager. budget is
// the tick's remaining path searches; it returns the number used.
func (n *npc) villageTick(budget int) int {
	if !n.idle() || budget <= 0 {
		return 0
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
