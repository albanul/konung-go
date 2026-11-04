// Package konunggame decodes the GAME.N start-of-game state files: entity
// groups (parties, villagers, monster packs) and the entities in them.
// Layout follows the loader at VA 0x0043BB6C; see docs/konung_game_format.md.
package konunggame

import (
	"encoding/binary"
	"fmt"

	"konung1.5/internal/resource"
)

const (
	FileSize = 709822 // sum of the six sections the loader reads

	groupOffset  = 0x20000 // 0x1900 bytes -> VA 0x0069D1B4
	groupCount   = 200
	groupStride  = 0x20
	entityOffset = 0x2A69A // 0x7D000 bytes -> VA 0x007AB378
	entityCount  = 2000
	entityStride = 0x100
)

// Group is a 32-byte entity group record. Group 0 is the player's party.
type Group struct {
	Index    int
	First    int  // +0x00 int16: first entity index
	Row, Col int  // +0x08, +0x10: group position on the map's fine grid
	Map      int  // +0x1B: KONUNG.N number
	Count    int  // +0x1C: number of consecutive entities
	Kind     byte // +0x1E: 0 = unused; other values not fully decoded
	Raw      [groupStride]byte
}

// Entity is a 256-byte entity record (fields that are understood).
type Entity struct {
	Index         int
	Type, Variant int    // +0x00, +0x01 (signed)
	Flags3        byte   // +0x03; bit 0x80 = not drawn
	Flags4        byte   // +0x04; bit 0x40 = animated OBJECTS.RES model
	Row, Col      int    // +0x08, +0x0A; 0,0 = placed at runtime around the group
	Direction     int    // +0x10
	Model         int    // +0xD6 (signed): OBJECTS slot or GRAPH Type 0 block
	PaletteOffset uint32 // +0xD8
	Group         int    // owning group index
}

// Game holds the decoded groups (index 0..199) and entities.
type Game struct {
	Groups   []Group
	Entities []Entity
}

// Parse decodes a GAME.N file.
func Parse(data []byte) (*Game, error) {
	if len(data) != FileSize {
		return nil, fmt.Errorf("GAME file has size %d, want %d", len(data), FileSize)
	}
	g := &Game{}
	owner := map[int]int{}
	for i := 0; i < groupCount; i++ {
		r := data[groupOffset+i*groupStride:][:groupStride]
		gr := Group{
			Index: i,
			First: int(int16(binary.LittleEndian.Uint16(r))),
			Row:   int(binary.LittleEndian.Uint16(r[0x08:])),
			Col:   int(binary.LittleEndian.Uint16(r[0x10:])),
			Map:   int(r[0x1B]),
			Count: int(r[0x1C]),
			Kind:  r[0x1E],
		}
		copy(gr.Raw[:], r)
		g.Groups = append(g.Groups, gr)
		if i == 0 || gr.Kind != 0 {
			for k := 0; k < gr.Count; k++ {
				owner[gr.First+k] = i
			}
		}
	}
	for i := 0; i < entityCount; i++ {
		e := data[entityOffset+i*entityStride:][:entityStride]
		grp, ok := owner[i]
		if !ok {
			grp = -1
		}
		g.Entities = append(g.Entities, Entity{
			Index:         i,
			Type:          int(int8(e[0])),
			Variant:       int(int8(e[1])),
			Flags3:        e[3],
			Flags4:        e[4],
			Row:           int(binary.LittleEndian.Uint16(e[0x08:])),
			Col:           int(binary.LittleEndian.Uint16(e[0x0A:])),
			Direction:     int(e[0x10]),
			Model:         int(int8(e[0xD6])),
			PaletteOffset: binary.LittleEndian.Uint32(e[0xD8:]),
			Group:         grp,
		})
	}
	return g, nil
}

// Animated reports whether the entity uses an animated OBJECTS.RES model.
func (e Entity) Animated() bool { return e.Flags4&0x40 != 0 }

// WorldPos converts a fine-grid row/col to the world pixel at the cell
// centre (VA 0x00439BA0): x = col*58 + (row odd ? 29 : 58), y = row*16 + 16.
func WorldPos(row, col int) (x, y int) {
	x = col * 0x3A
	if row&1 != 0 {
		x += 0x1D
	} else {
		x += 0x3A
	}
	return x, row*0x10 + 0x10
}

// Placed is an entity with its resolved fine-grid position.
type Placed struct {
	Entity
	Row, Col int
	Spawned  bool // true when the position was chosen by this package (entity had 0,0)
}

// OnMap returns the drawable entities of map n: the player's party (group 0)
// when it is on that map, and every active group (Kind != 0) whose map byte
// matches, like the map loader at VA 0x0043C612. Entities with a zero
// position are spread over free cells around their group position: the game
// does this at runtime with random formation offsets, so these spots are an
// approximation. blocked reports cells the walk grid marks as obstacles.
func (g *Game) OnMap(n int, blocked func(row, col int) bool) []Placed {
	var out []Placed
	used := map[[2]int]bool{}
	var pending []Placed
	for _, gr := range g.Groups {
		if gr.Map != n || (gr.Index != 0 && gr.Kind == 0) {
			continue
		}
		for k := 0; k < gr.Count; k++ {
			idx := gr.First + k
			if idx < 0 || idx >= len(g.Entities) {
				continue
			}
			e := g.Entities[idx]
			if e.Flags3&0x80 != 0 || e.Model < 0 {
				continue
			}
			p := Placed{Entity: e, Row: e.Row, Col: e.Col}
			if e.Row == 0 && e.Col == 0 {
				p.Row, p.Col, p.Spawned = gr.Row, gr.Col, true
				pending = append(pending, p)
				continue
			}
			used[[2]int{p.Row, p.Col}] = true
			out = append(out, p)
		}
	}
	for _, p := range pending {
		p.Row, p.Col = freeCellNear(p.Row, p.Col, used, blocked)
		used[[2]int{p.Row, p.Col}] = true
		out = append(out, p)
	}
	return out
}

// freeCellNear searches rings of growing radius (rows step 2 to keep the
// same stagger parity spacing visually even) for an unused, unblocked cell.
func freeCellNear(row, col int, used map[[2]int]bool, blocked func(int, int) bool) (int, int) {
	for r := 0; r < 12; r++ {
		for dr := -2 * r; dr <= 2*r; dr++ {
			for dc := -r; dc <= r; dc++ {
				if max(abs(dr+1)/2, abs(dc)) != r && r != 0 {
					continue
				}
				rr, cc := row+dr, col+dc
				if rr < 0 || cc < 0 || used[[2]int{rr, cc}] || (blocked != nil && blocked(rr, cc)) {
					continue
				}
				return rr, cc
			}
		}
	}
	return row, col
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// ModelSpec returns the art selector for an entity.
func (e Entity) ModelSpec() resource.EntityModel {
	return resource.EntityModel{
		Animated: e.Animated(), Model: e.Model, Type: e.Type, Variant: e.Variant,
		Direction: e.Direction, PaletteOffset: e.PaletteOffset,
	}
}

// LoadSprite decodes the entity's current frame and returns it positioned in
// world pixels, with its depth key (image bottom edge).
func (p Placed) LoadSprite(objects, graph []byte) (img *resource.EntitySprite, x, y, sx, sy, depth int, err error) {
	img, err = resource.LoadEntitySprite(objects, graph, p.ModelSpec())
	if err != nil {
		return nil, 0, 0, 0, 0, 0, fmt.Errorf("entity %d: %w", p.Index, err)
	}
	wx, wy := WorldPos(p.Row, p.Col)
	x, y = wx+img.ImageDX, wy+img.ImageDY
	return img, x, y, wx + img.ShadowDX, wy + img.ShadowDY, y + img.Image.Bounds().Dy(), nil
}
