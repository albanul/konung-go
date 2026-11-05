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
	Index     int
	First     int  // +0x00 int16: first entity index
	Row, Col  int  // +0x08, +0x10: group position on the map's fine grid
	Map       int  // +0x1B: KONUNG.N number
	Count     int  // +0x1C: number of consecutive entities
	Kind      byte // +0x1E: 0 = unused; bit 0x40 creature group, 0x80 village; other values not fully decoded
	Debt      int  // +0x02 int32: respawn debt; members are respawned on map entry while it is negative
	Countdown int  // +0x06 int16: upkeep passes until the population target grows
	Target    int  // +0x18 int8: population target (creature groups)
	MaxPop    int  // +0x19 int8: population maximum = reserved entity slots
	Enemy     int  // +0x1A int8: the group an alarmed group attacks (set by AlarmState's caller)
	Alarm     int  // +0x1D: AlarmCalm / AlarmAttack / AlarmReturn (creature.go); 0x80 in the files, cleared on map load
	Raw       [groupStride]byte
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
	Gold          int     // +0x34 int32: «Золото» (character window)
	FreeExp       int     // +0x38 int32: «Свободный опыт»
	Experience    int     // +0x3C int32: «Опыт»
	Fencing       int     // +0x40 word: «Фехтование» ×100
	Marksmanship  int     // +0x44 word: «Меткость» ×100
	BaseStats     [4]int  // words +0x5A, +0x60, +0x56, +0x64: base Хар, Сил, Лов, Вын («базовая» column)
	Quiver        int     // +0xCC word: ammunition item in the box next to the bow (0 = empty)
	HitBonus      int     // +0xCA int16: added to the hit chance while PotionTimer != 0 (0x0041B5EE)
	PotionTimer   int     // +0xCE word: potion timer in upkeep passes (Upkeep, UseHerb); while set, nothing can be raised and HitBonus applies
	SavedBase     [4]int  // words +0xBC, +0xBE, +0xC0, +0xC2: base Лов, Хар, Сил, Вын restored when the potion ends
	Throttle      int     // +0x11: walk throttle mask (walk steps only on ticks where time & m == m)
	PoisonDealt   int     // +0x52 int16: poison a creature adds to its target's +0x50 per hit (0x0041AF5D)
	ArmourBase    int     // +0x4E int16: armour without gear («Броня», 0x0041AC1C)
	Bonus         [3]int  // +0xC4, +0xC6, +0xC8 int16: armour, attack and stat-9 bonuses written by Game.Recompute
	BonusFlags    byte    // +0xD1: bits 0-2 = the three bonuses stack (1) or replace (0)
	Enchants      [2]int  // +0xD2, +0xD3 int8: enchantments on the character itself (-1 = none)
	Stats         [4]int  // words +0x58, +0x5E, +0x54, +0x62: Хар, Сил, Лов, Вын (status line, 0x00431A94)
	Skills        [5]int  // words +0x48, +0x4C, +0x42, +0x46, +0x4A: healing, smithing, trade, building, curios
	Dialogue      int     // +0xD0: dialogue opened on click (VA 0x004364F0); NoDialogue = none
	Group         int     // owning group index
	Equipment     [6]int  // +0x66 words: worn items (0 = empty), drawn as overlays; see EquipAxe..EquipShield
	Bag           [32]int // +0x72 words: carried items (0 = empty)
	Extra         [5]int  // +0xB2 words: five more item slots, meaning not traced (0 = empty)
	// Raw is the record as loaded; Encode writes the fields above over it,
	// so bytes the port does not decode survive a save.
	Raw [entityStride]byte
}

// Game holds the decoded groups (index 0..199) and entities.
type Game struct {
	Groups      []Group
	Entities    []Entity
	Settlements []Settlement // section 6: villages with their named-character posts
	Piles       []Pile       // section 3: items lying on the ground
	Exits       []MapExit    // section 4: map exits (teleports), see exits.go
	// Lit is 0x00A3E758: the visible piles glow (resfmt.LightsGlow). Set by
	// «Чистая слеза» (UseHerb) and the torch (UseMisc); the map loader
	// clears it (0x0043C11B).
	Lit bool
	// Not ported, but saved with the game (0x00424E34): the quest flags
	// 0x00625068 (QUESTR.RES tail, SetQuestFlags) and the misc-item
	// draw flags 0x00A3E0A8 (0x00414E64).
	Quests   [QuestFlagsSize]byte
	MiscUsed [miscUsedSize]byte
	// Options are the settings 0x00A3E71C and 0x00A3E734 that a save
	// stores; 1 without a config file (0x0042BB58).
	Options [2]int32
	items   []byte
	data    []byte // the GAME.N image the records came from (Marshal)
}

// ItemType returns byte +3 of item record i, the item type that selects the
// overlay graphic (VA 0x004282AC).
func (g *Game) ItemType(i int) int {
	if i <= 0 || (i+1)*itemStride > len(g.items) {
		return -1
	}
	return int(g.items[i*itemStride+3])
}

// Item is an item record (16 bytes, section 1) as far as it is understood.
type Item struct {
	Index    int
	Category int  // +0: selects the type table (resfmt.ItemTableOf); 0xFF = free (0x004227A0)
	Enchant  int  // +1 int8: enchantment (resfmt.Enchant), -1 = none
	Special  int  // +2 int8: special property (resfmt.ItemSpecial), -1 = none
	Type     int  // +3: record in that table
	Count    int  // +4 int32: stack size of arrows (category 12); other items hold floats here
	Flagged  bool // +0xE bit 0: unidentified («Неведомое…» description, enchantment ignored, cell tinted yellow)
}

// Item returns item record i; ok is false for 0 (empty slot) or out of range.
func (g *Game) Item(i int) (it Item, ok bool) {
	if i <= 0 || (i+1)*itemStride > len(g.items) {
		return Item{}, false
	}
	r := g.items[i*itemStride:]
	return Item{Index: i, Category: int(r[0]), Enchant: int(int8(r[1])), Special: int(int8(r[2])), Type: int(r[3]),
		Count: int(int32(binary.LittleEndian.Uint32(r[4:]))), Flagged: r[0xE]&1 != 0}, true
}

// Table returns the item's type table (resfmt.ItemTable*).
func (it Item) Table() int { return resfmt.ItemTableOf(it.Category) }

// Parse decodes a GAME.N file.
func Parse(data []byte) (*Game, error) {
	if len(data) != FileSize {
		return nil, fmt.Errorf("GAME file has size %d, want %d", len(data), FileSize)
	}
	data = append([]byte(nil), data...)
	g := &Game{items: data[itemOffset : itemOffset+0x20000], data: data, Options: [2]int32{1, 1}}
	owner := map[int]int{}
	for i := 0; i < groupCount; i++ {
		r := data[groupOffset+i*groupStride:][:groupStride]
		gr := Group{
			Index:     i,
			First:     int(int16(binary.LittleEndian.Uint16(r))),
			Row:       int(binary.LittleEndian.Uint16(r[0x08:])),
			Col:       int(binary.LittleEndian.Uint16(r[0x10:])),
			Map:       int(r[0x1B]),
			Count:     int(r[0x1C]),
			Kind:      r[0x1E],
			Enemy:     int(int8(r[0x1A])),
			Debt:      int(int32(binary.LittleEndian.Uint32(r[2:]))),
			Countdown: int(int16(binary.LittleEndian.Uint16(r[6:]))),
			Target:    int(int8(r[0x18])),
			MaxPop:    int(int8(r[0x19])),
			Alarm:     int(r[0x1D]),
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
			Gold:          int(int32(binary.LittleEndian.Uint32(e[0x34:]))),
			FreeExp:       int(int32(binary.LittleEndian.Uint32(e[0x38:]))),
			Experience:    int(int32(binary.LittleEndian.Uint32(e[0x3C:]))),
			Fencing:       u16(e, 0x40),
			Marksmanship:  u16(e, 0x44),
			BaseStats:     [4]int{u16(e, 0x5A), u16(e, 0x60), u16(e, 0x56), u16(e, 0x64)},
			Quiver:        u16(e, 0xCC),
			HitBonus:      s16(e, 0xCA),
			PotionTimer:   u16(e, 0xCE),
			SavedBase:     [4]int{u16(e, 0xBC), u16(e, 0xBE), u16(e, 0xC0), u16(e, 0xC2)},
			Throttle:      int(e[0x11]),
			PoisonDealt:   s16(e, 0x52),
			ArmourBase:    int(int16(binary.LittleEndian.Uint16(e[0x4E:]))),
			Bonus:         [3]int{s16(e, 0xC4), s16(e, 0xC6), s16(e, 0xC8)},
			BonusFlags:    e[0xD1],
			Enchants:      [2]int{int(int8(e[0xD2])), int(int8(e[0xD3]))},
			Stats:         [4]int{u16(e, 0x58), u16(e, 0x5E), u16(e, 0x54), u16(e, 0x62)},
			Skills:        [5]int{u16(e, 0x48), u16(e, 0x4C), u16(e, 0x42), u16(e, 0x46), u16(e, 0x4A)},
			Group:         grp,
			Equipment:     [6]int(words(e[0x66:], 6)),
			Bag:           [32]int(words(e[0x72:], 32)),
			Extra:         [5]int(words(e[0xB2:], 5)),
			Raw:           [entityStride]byte(e),
		})
	}
	g.Settlements = parseSettlements(data)
	g.Piles = parsePiles(data)
	g.Exits = parseExits(data)
	return g, nil
}

// NoDialogue is the +0xD0 value of entities that cannot be talked to.
const NoDialogue = 0xFF

func u16(b []byte, off int) int { return int(binary.LittleEndian.Uint16(b[off:])) }
func s16(b []byte, off int) int { return int(int16(binary.LittleEndian.Uint16(b[off:]))) }

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
