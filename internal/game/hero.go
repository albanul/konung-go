package game

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
	"konung1.5/internal/world"
)

// Walkers, NPCs and party members are internal/world's; the game keeps
// their sprites (spriteCache in Walker.Sprites) and draws them.
type (
	walker  = world.Walker
	npc     = world.NPC
	member  = world.Member
	look    = world.Look
	npcMode = world.NPCMode
)

const (
	npcStill    = world.NPCStill
	npcVillager = world.NPCVillager
	npcCreature = world.NPCCreature
	npcRemote   = world.NPCRemote
)

func lookOf(e gamestate.Entity) look { return world.LookOf(e) }

func animBase(typ int) int { return world.AnimBase(typ) }

type animKey struct {
	look      look
	kind, dir int
}

// spriteCache is a walker's frames per look (stance, weapon), kind (0
// idle, 1 walk, an action) and facing, as offsets from the cell centre;
// load fills missing entries on first use.
type spriteCache struct {
	anims map[animKey][]sceneFrame
	load  func(kind, dir int) []sceneFrame
}

func (c *spriteCache) frames(l look, kind, dir int) []sceneFrame {
	k := animKey{l, kind, dir}
	f, ok := c.anims[k]
	if !ok && c.load != nil {
		if c.anims == nil {
			c.anims = map[animKey][]sceneFrame{}
		}
		f = c.load(kind, dir)
		c.anims[k] = f
	}
	return f
}

// withSprites gives w a lazy sprite loader: its frames are drawn by the
// game and counted by the simulation (Walker.Frames).
func withSprites(w *walker, load func(kind, dir int) []sceneFrame) {
	c := &spriteCache{load: load}
	w.Sprites = c
	w.Frames = func(l look, kind, dir int) int { return len(c.frames(l, kind, dir)) }
}

// spriteFrames returns w's frames for kind in its current look and facing.
func spriteFrames(w *walker, kind int) []sceneFrame {
	c, _ := w.Sprites.(*spriteCache)
	if c == nil {
		return nil
	}
	return c.frames(w.Look, kind, int(w.Dir))
}

// walkerFrame returns w's current animation frame in world pixels. Walk
// frames are drawn from the cell being left; their anchors carry the body
// forward.
func walkerFrame(w *walker, tick int) (sceneFrame, bool) {
	var f sceneFrame
	if frames := spriteFrames(w, w.Act); w.Act != 0 && len(frames) > 0 {
		f = frames[min(w.ActStep, len(frames)-1)]
	} else if frames := spriteFrames(w, 1); w.Walking && len(frames) > 0 {
		f = frames[min(w.StepT, len(frames)-1)]
	} else if frames := spriteFrames(w, 0); len(frames) > 0 {
		f = frames[max(w.Idle, 0)%len(frames)]
	} else {
		return f, false
	}
	x, y := mapdata.CellCenter(w.Anchor())
	f.x, f.y, f.sx, f.sy = f.x+x, f.y+y, f.sx+x, f.sy+y
	return f, true
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
		q.Type, q.Variant, q.Step = animBase(q.Type)+kind*world.WalkTypeOffset, dir, 0
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
