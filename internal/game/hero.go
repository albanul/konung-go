package game

import (
	"fmt"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

const (
	// heroSpeed is the walking speed in world pixels per 60 Hz update. The
	// original speed is not traced; 1.6 px (96 px/s) keeps the 12-step walk
	// cycle roughly in step with the ground.
	heroSpeed = 1.6
	// pathSearchLimit caps A* exploration so a click on an unreachable spot
	// can't stall a frame.
	pathSearchLimit = 30000
	walkTypeOffset  = 1 // walk = idle type + 1 (type 6 idle, 7 walk), seen in the body records
)

// hero is the player character: position, path and animations.
type hero struct {
	cell    mapdata.Cell
	x, y    float64 // world position (cell-centre coordinates)
	path    []mapdata.Cell
	dir     mapdata.Direction
	walking bool
	walkT   int // ticks spent walking, drives the walk cycle
	// anims[0] = idle, anims[1] = walk; frames hold offsets from (x, y).
	anims [2][8][]sceneFrame
}

// loadHero preloads idle and walk loops for every facing, with equipment.
func loadHero(game *gamestate.Game, p gamestate.Placed, objects, graph []byte, t *resfmt.ExeTables,
	upload func(*resfmt.EntitySprite) (*ebiten.Image, *ebiten.Image)) (*hero, error) {
	wx, wy := gamestate.WorldPos(p.Row, p.Col)
	h := &hero{cell: mapdata.Cell{Row: p.Row, Col: p.Col}, x: float64(wx), y: float64(wy), dir: mapdata.Direction(p.Variant & 7)}
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
// walking finishes the step it is on before following the new path.
func (h *hero) walkTo(goal mapdata.Cell, blocked func(mapdata.Cell) bool) {
	goal = mapdata.NearestFree(goal, blocked, 6)
	start := h.cell
	if len(h.path) > 0 {
		start = h.path[0]
	}
	path := mapdata.FindPath(start, goal, blocked, pathSearchLimit)
	if len(h.path) > 0 {
		path = append([]mapdata.Cell{start}, path...)
	}
	if len(path) > 0 || start == goal {
		h.path = path
	}
}

// update advances the hero along its path by heroSpeed pixels.
func (h *hero) update() {
	budget := heroSpeed
	for budget > 0 && len(h.path) > 0 {
		cx, cy := mapdata.CellCenter(h.path[0])
		tx, ty := float64(cx), float64(cy)
		dx, dy := tx-h.x, ty-h.y
		d := math.Hypot(dx, dy)
		if d > 0 {
			h.dir = mapdata.DirectionTo(h.x, h.y, tx, ty)
		}
		if d <= budget {
			h.x, h.y, h.cell, h.path = tx, ty, h.path[0], h.path[1:]
			budget -= d
			continue
		}
		h.x += dx / d * budget
		h.y += dy / d * budget
		budget = 0
	}
	h.walking = len(h.path) > 0
	if h.walking {
		h.walkT++
	} else {
		h.walkT = 0
	}
}

// frame returns the current animation frame, positioned in world pixels.
func (h *hero) frame(tick int) sceneFrame {
	kind, t := 0, tick
	if h.walking {
		kind, t = 1, h.walkT
	}
	frames := h.anims[kind][h.dir]
	f := frames[(t/ticksPerStep)%len(frames)]
	x, y := int(math.Round(h.x)), int(math.Round(h.y))
	f.x, f.y, f.sx, f.sy = f.x+x, f.y+y, f.sx+x, f.sy+y
	return f
}
