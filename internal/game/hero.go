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
	// lasts. One 12-step walk cycle moves the character exactly one fine cell
	// (the frames' anchors carry the displacement), so this sets the walking
	// speed: 4 gives 0.8 s per cell. The original tick rate is not traced.
	walkTicksPerStep = 4
	defaultWalkSteps = 12
	// pathSearchLimit caps A* exploration so a click on an unreachable spot
	// can't stall a frame.
	pathSearchLimit = 30000
	walkTypeOffset  = 1 // walk = idle type + 1 (type 6 idle, 7 walk), seen in the body records
)

// hero is the player character. Like the original, it moves one fine cell
// per walk cycle: while stepping it stays anchored on its current cell and
// the walk frames' per-step anchors carry the body toward the next cell; at
// the end of the cycle it snaps to that cell.
type hero struct {
	cell    mapdata.Cell
	path    []mapdata.Cell // path[0] is the cell being stepped into while walking
	dir     mapdata.Direction
	walking bool
	stepT   int // ticks into the current cell step
	// anims[0] = idle, anims[1] = walk; frames hold offsets from the cell centre.
	anims [2][8][]sceneFrame
}

// loadHero preloads idle and walk loops for every facing, with equipment.
func loadHero(game *gamestate.Game, p gamestate.Placed, objects, graph []byte, t *resfmt.ExeTables,
	upload func(*resfmt.EntitySprite) (*ebiten.Image, *ebiten.Image)) (*hero, error) {
	wx, wy := gamestate.WorldPos(p.Row, p.Col)
	h := &hero{cell: mapdata.Cell{Row: p.Row, Col: p.Col}, dir: mapdata.Direction(p.Variant & 7)}
	for kind := 0; kind < 2; kind++ {
		for dir := 0; dir < 8; dir++ {
			q := p
			q.Type, q.Variant, q.Step = p.Type+kind*walkTypeOffset, dir, 0
			anim, err := game.LoadAnimation(q, objects, graph, t)
			if err != nil {
				return nil, fmt.Errorf("hero type %d dir %d: %w", q.Type, dir, err)
			}
			for _, f := range anim {
				img, sh := upload(f.Sprite)
				h.anims[kind][dir] = append(h.anims[kind][dir], sceneFrame{
					img: img, shadow: sh, x: f.X - wx, y: f.Y - wy, sx: f.SX - wx, sy: f.SY - wy,
				})
			}
		}
	}
	return h, nil
}

// walkTo plans a path to the free cell nearest to goal. A hero already
// stepping finishes the step it is on before following the new path.
func (h *hero) walkTo(goal mapdata.Cell, blocked func(mapdata.Cell) bool) {
	goal = mapdata.NearestFree(goal, blocked, 6)
	start := h.cell
	if h.walking {
		start = h.path[0]
	}
	path := mapdata.FindPath(start, goal, blocked, pathSearchLimit)
	if h.walking {
		path = append([]mapdata.Cell{start}, path...)
	}
	if len(path) > 0 || start == goal {
		h.path = path
	}
}

// walkSteps is the length of the walk cycle for the current facing.
func (h *hero) walkSteps() int {
	if n := len(h.anims[1][h.dir]); n > 0 {
		return n
	}
	return defaultWalkSteps
}

// faceNext turns the hero toward path[0] using the grid's neighbour table.
func (h *hero) faceNext() {
	for _, n := range mapdata.Neighbors(h.cell) {
		if n.Cell == h.path[0] {
			h.dir = n.Dir
			return
		}
	}
	ax, ay := mapdata.CellCenter(h.cell)
	bx, by := mapdata.CellCenter(h.path[0])
	h.dir = mapdata.DirectionTo(float64(ax), float64(ay), float64(bx), float64(by))
}

// update advances the walk cycle by one tick.
func (h *hero) update() {
	if !h.walking {
		if len(h.path) == 0 {
			return
		}
		h.walking, h.stepT = true, 0
		h.faceNext()
	}
	h.stepT++
	if h.stepT < h.walkSteps()*walkTicksPerStep {
		return
	}
	h.cell, h.path, h.stepT = h.path[0], h.path[1:], 0
	if len(h.path) == 0 {
		h.walking = false
		return
	}
	h.faceNext()
}

// position is a smooth world position for the camera: the current cell's
// centre moved toward the next cell by the step's progress.
func (h *hero) position() (float64, float64) {
	ax, ay := mapdata.CellCenter(h.cell)
	if !h.walking {
		return float64(ax), float64(ay)
	}
	bx, by := mapdata.CellCenter(h.path[0])
	t := float64(h.stepT) / float64(h.walkSteps()*walkTicksPerStep)
	return float64(ax) + float64(bx-ax)*t, float64(ay) + float64(by-ay)*t
}

// frame returns the current animation frame, positioned in world pixels.
// Walk frames are drawn from the cell being left; their anchors carry the
// body toward the next cell.
func (h *hero) frame(tick int) sceneFrame {
	var f sceneFrame
	if h.walking {
		frames := h.anims[1][h.dir]
		f = frames[min(h.stepT/walkTicksPerStep, len(frames)-1)]
	} else {
		frames := h.anims[0][h.dir]
		f = frames[(tick/ticksPerStep)%len(frames)]
	}
	x, y := mapdata.CellCenter(h.cell)
	f.x, f.y, f.sx, f.sy = f.x+x, f.y+y, f.sx+x, f.sy+y
	return f
}
