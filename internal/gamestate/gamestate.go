// Package gamestate decodes the GAME.N start-of-game state files: entity
// groups (parties, villagers, monster packs) and the entities in them.
// Layout follows the loader at VA 0x0043BB6C; see docs/konung_game_format.md.
package gamestate

import (
	"encoding/binary"
	"fmt"

	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

const (
	FileSize = 709822 // sum of the six sections the loader reads

	itemOffset   = 0x00000 // 0x20000 bytes -> VA 0x0067AC34: 8192 x 16-byte items
	itemStride   = 0x10
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
	Type          int     // +0x00 (signed): action / animation set
	Variant       int     // +0x01 (signed): facing direction
	Flags2        byte    // +0x02: action state bits (0x40 idle, 0x48 walking to +0x0C/+0x0E); see stance.go
	Flags3        byte    // +0x03; bit 0x80 = not drawn, bit 0x04 = combat stance (InStance)
	Flags4        byte    // +0x04; bit 0x40 = animated OBJECTS.RES model
	Row, Col      int     // +0x08, +0x0A; 0,0 = placed at runtime around the group
	Step          int     // +0x10: animation step (the variant +0x01 is the facing direction)
	WeaponMode    int     // +0x12: active weapon = equipment slot WeaponAxe/WeaponBow/WeaponBlade (SelectWeapon)
	Poison        int     // +0x50 int16: non-zero while poisoned (HP bar colour, VA 0x0042D977)
	HP            int     // +0x5C int16: health; HPFull (1600) = 100 %
	Model         int     // +0xD6 (signed): OBJECTS slot or GRAPH Type 0 block
	PaletteOffset uint32  // +0xD8
	NameIndex     int     // +0xD4: first name (resfmt.FirstNamesVA); set by hero selection for the hero
	Nickname      int     // +0xD5: second name part (resfmt.NicknamesVA), 0 = none
	Level         int     // +0x33: level shown in the status line
	Stats         [4]int  // words +0x58, +0x5E, +0x54, +0x62: Хар, Сил, Лов, Вын (status line, 0x00431A94)
	Skills        [5]int  // words +0x48, +0x4C, +0x42, +0x46, +0x4A: healing, smithing, trade, building, curios
	Dialogue      int     // +0xD0: dialogue opened on click (VA 0x004364F0); NoDialogue = none
	Group         int     // owning group index
	Equipment     [6]int  // +0x66 words: worn items (0 = empty), drawn as overlays; see EquipAxe..EquipShield
	Bag           [32]int // +0x72 words: carried items (0 = empty)
	Extra         [5]int  // +0xB2 words: five more item slots, meaning not traced (0 = empty)
}

// Game holds the decoded groups (index 0..199) and entities.
type Game struct {
	Groups      []Group
	Entities    []Entity
	Settlements []Settlement // section 6: villages with their named-character posts
	items       []byte
}

// ItemType returns byte +3 of item record i, the item type that selects the
// overlay graphic (VA 0x004282AC).
func (g *Game) ItemType(i int) int {
	if i <= 0 || (i+1)*itemStride > len(g.items) {
		return -1
	}
	return int(g.items[i*itemStride+3])
}

// Parse decodes a GAME.N file.
func Parse(data []byte) (*Game, error) {
	if len(data) != FileSize {
		return nil, fmt.Errorf("GAME file has size %d, want %d", len(data), FileSize)
	}
	g := &Game{items: data[itemOffset : itemOffset+0x20000]}
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
			Flags2:        e[2],
			Flags3:        e[3],
			Flags4:        e[4],
			Row:           int(binary.LittleEndian.Uint16(e[0x08:])),
			Col:           int(binary.LittleEndian.Uint16(e[0x0A:])),
			Step:          int(e[0x10]),
			WeaponMode:    int(e[0x12]),
			Poison:        int(int16(binary.LittleEndian.Uint16(e[0x50:]))),
			HP:            int(int16(binary.LittleEndian.Uint16(e[0x5C:]))),
			Model:         int(int8(e[0xD6])),
			PaletteOffset: binary.LittleEndian.Uint32(e[0xD8:]),
			Dialogue:      int(e[0xD0]),
			NameIndex:     int(e[0xD4]),
			Nickname:      int(e[0xD5]),
			Level:         int(e[0x33]),
			Stats:         [4]int{u16(e, 0x58), u16(e, 0x5E), u16(e, 0x54), u16(e, 0x62)},
			Skills:        [5]int{u16(e, 0x48), u16(e, 0x4C), u16(e, 0x42), u16(e, 0x46), u16(e, 0x4A)},
			Group:         grp,
			Equipment:     [6]int(words(e[0x66:], 6)),
			Bag:           [32]int(words(e[0x72:], 32)),
			Extra:         [5]int(words(e[0xB2:], 5)),
		})
	}
	g.Settlements = parseSettlements(data)
	return g, nil
}

// NoDialogue is the +0xD0 value of entities that cannot be talked to.
const NoDialogue = 0xFF

func u16(b []byte, off int) int { return int(binary.LittleEndian.Uint16(b[off:])) }

// Animated reports whether the entity uses an animated OBJECTS.RES model.
func (e Entity) Animated() bool { return e.Flags4&0x40 != 0 }

// WorldPos converts a fine-grid row/col to the world pixel at the cell
// centre; see mapdata.CellCenter.
func WorldPos(row, col int) (x, y int) { return mapdata.CellCenter(mapdata.Cell{Row: row, Col: col}) }

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
func (e Entity) ModelSpec() resfmt.EntityModel {
	return resfmt.EntityModel{
		Animated: e.Animated(), Model: e.Model, Type: e.Type, Variant: e.Variant,
		Step: e.Step, PaletteOffset: e.PaletteOffset,
	}
}

// LoadSprite decodes the entity's current frame and returns it positioned in
// world pixels, with its depth key (image bottom edge).
// t may be nil to skip equipment overlays.
func (g *Game) LoadSprite(p Placed, objects, graph []byte, t *resfmt.ExeTables) (img *resfmt.EntitySprite, x, y, sx, sy, depth int, err error) {
	img, err = resfmt.LoadEntitySprite(objects, graph, p.ModelSpec())
	if err == nil {
		img, err = resfmt.ComposeEquipment(graph, img, p.ModelSpec(), g.Overlays(p.Entity, t), t)
	}
	if err != nil {
		return nil, 0, 0, 0, 0, 0, fmt.Errorf("entity %d: %w", p.Index, err)
	}
	wx, wy := WorldPos(p.Row, p.Col)
	x, y = wx+img.ImageDX, wy+img.ImageDY
	return img, x, y, wx + img.ShadowDX, wy + img.ShadowDY, y + img.Image.Bounds().Dy(), nil
}

// Frame is one animation step positioned in world pixels.
type Frame struct {
	Sprite       *resfmt.EntitySprite
	X, Y, SX, SY int
}

// LoadAnimation decodes every step of the entity's current sequence, with
// equipment overlays composed onto Type 0 bodies (t may be nil to skip them).
func (g *Game) LoadAnimation(p Placed, objects, graph []byte, t *resfmt.ExeTables) ([]Frame, error) {
	steps, err := resfmt.LoadEntityAnimation(objects, graph, p.ModelSpec())
	if err != nil {
		return nil, fmt.Errorf("entity %d: %w", p.Index, err)
	}
	if overlays := g.Overlays(p.Entity, t); len(overlays) > 0 {
		var prevBody *resfmt.EntitySprite
		var prevOut *resfmt.EntitySprite
		for i, body := range steps {
			if prevBody != nil && body.Image == prevBody.Image && body.ImageDX == prevBody.ImageDX && body.ImageDY == prevBody.ImageDY {
				steps[i] = prevOut // repeated step: reuse the composite
				continue
			}
			m := p.ModelSpec()
			m.Step = i
			out, err := resfmt.ComposeEquipment(graph, body, m, overlays, t)
			if err != nil {
				return nil, fmt.Errorf("entity %d: %w", p.Index, err)
			}
			prevBody, prevOut, steps[i] = body, out, out
		}
	}
	wx, wy := WorldPos(p.Row, p.Col)
	out := make([]Frame, len(steps))
	for i, s := range steps {
		out[i] = Frame{Sprite: s, X: wx + s.ImageDX, Y: wy + s.ImageDY, SX: wx + s.ShadowDX, SY: wy + s.ShadowDY}
	}
	return out, nil
}

// HeroNameIndex is the first-name index the hero-selection code writes to
// the hero's +0xD4 together with its body model +0xD6 (VA 0x00438042,
// 0x00437EC3, 0x00437F88): model 0 Волк, 1 Драгомир, 2 Михаил.
var HeroNameIndex = [3]int{10, 124, 151}

// PlaceholderHeroPalette is used for the hero until the character-creation
// palette is decoded: palette 72, one of the clean dyes for Type 0 model 0
// (70-74, 113-114 render cleanly; the file's 33 suits model 1 only).
const PlaceholderHeroPalette = 72 * 512

// HeroModelFromGame applies the new-game rule that ties the hero to GAME.N:
// the loader opens GAME.<hero model byte> (VA 0x0043BBB2 reads the template's
// +0xD6), and the start map is model*3+1 (VA 0x004390B4). So in GAME.N the
// player's entity is shown with Type 0 model N; the file's own value is a
// placeholder that the new-game code overwrites from the hero template.
// Model 0 = Volk (KONUNG.1), model 1 = Dragomir. The hero's palette (+0xD8)
// also comes from the template (VA 0x0043904E), set on the character-creation
// screen, which is not decoded: pass a non-zero paletteOffset to override the
// file's placeholder.
func (g *Game) HeroModelFromGame(n int, paletteOffset uint32) {
	if len(g.Groups) == 0 {
		return
	}
	if i := g.Groups[0].First; i >= 0 && i < len(g.Entities) {
		g.Entities[i].Model = n
		g.Entities[i].Flags4 &^= 0x40
		if n >= 0 && n < len(HeroNameIndex) {
			g.Entities[i].NameIndex = HeroNameIndex[n]
		}
		if paletteOffset != 0 {
			g.Entities[i].PaletteOffset = paletteOffset
		}
	}
}

func words(b []byte, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = int(binary.LittleEndian.Uint16(b[i*2:]))
	}
	return out
}

// Overlays returns the equipment overlay blocks to draw over a Type 0 body,
// in draw order, as 0x004281A4 picks them. Out of the combat stance the
// order is the facing's peaceful table (0x00428484). In the stance
// (0x004281B3) the order depends on the action (CombatAction) and on whether
// the bow is drawn; the active weapon's slot and, unless the bow is drawn, the
// shield slot use overlay k+1, the drawn version of the item.
func (g *Game) Overlays(e Entity, t *resfmt.ExeTables) []int {
	if t == nil || e.Animated() {
		return nil
	}
	order := t.DrawOrder(e.Variant)
	drawn, shield := -1, -1
	if e.InStance() {
		action := CombatAction(e.Type)
		bow := e.WeaponMode == WeaponBow && action != 2
		order = t.CombatDrawOrder(action, e.Variant, bow)
		if !bow {
			shield = EquipShield
			if e.WeaponMode != WeaponBow {
				drawn = e.WeaponMode
			}
		} else {
			drawn = WeaponBow
		}
	}
	var out []int
	for _, slot := range order {
		if slot < 0 || slot >= len(e.Equipment) || e.Equipment[slot] == 0 {
			continue
		}
		k := t.ItemOverlay(g.ItemType(e.Equipment[slot]))
		if k < 0 {
			continue
		}
		if slot == drawn || slot == shield {
			k++
		}
		out = append(out, k)
	}
	return out
}
