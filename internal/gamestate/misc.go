package gamestate

import (
	"slices"

	"konung1.5/internal/resfmt"
)

// Misc items (category 11, type table 0x0046053C): their use is
// 0x00414E64(e, target, item), a jump table at 0x00415ADC over types 0–53
// (0x00415BBA: larger types do nothing). See docs/konung_misc_items.md.

// Misc item types (records of the misc table).
const (
	MiscLostMap      = 0  // «Потерянная карта»
	MiscHippocrates  = 1  // «Труды Гиппократа»
	MiscPyramids     = 2  // «Чертежи древних пирамид»
	MiscSmithScroll  = 3  // «Свиток кузнеца»
	MiscTradeTract   = 4  // «Трактат о торговле»
	MiscThirdEye     = 5  // «Ягода Третий Глаз»
	MiscWords        = 6  // «Слова Воскрешения»
	MiscDuplicator   = 7  // «Дупликатор»
	MiscFalconFoot   = 8  // «Соколиная лапка»
	MiscSeerScroll   = 9  // «Свиток ведуна»
	MiscApple        = 10 // «Яблоко познания»
	MiscWine         = 12 // «Вино»
	MiscMagnet       = 13 // «Великий магнит»
	MiscOrbFirst     = 14 // «Магический шар волхва Бережков» … 19 «… Лесного лагеря»
	MiscJumpOrb      = 20 // «Шар магического прыжка»
	MiscOrbCaves     = 21 // «Магический шар Подземного города»
	MiscHilt         = 22 // «Рукоять древнего меча»
	MiscBlade        = 23 // «Лезвие древнего меча»
	MiscShovel       = 24 // «Лопата»
	MiscAmuletRight  = 25 // «Правая часть амулета Дракона»
	MiscAmuletLeft   = 26 // «Левая часть амулета Дракона»
	MiscAmuletBottom = 27 // «Нижняя часть амулета Дракона»
	MiscBirchBark    = 28 // «Берестяная грамота»
	MiscSkeletonLamp = 30 // «Медная лампа с именем Короля скелетов»
	MiscTorch        = 31 // «Факел»
	MiscDoll         = 33 // «Кукла»
	MiscWhistle      = 35 // «Свисток»
	MiscGoblet       = 36 // «Чаша воеводы»
	MiscPipe         = 37 // «Дудочка»
	MiscRelics       = 38 // «Святые мощи»
	MiscToys         = 39 // «Детские игрушки»
	MiscAmulet       = 40 // «Амулет Дракона»
	MiscAmuletLR     = 41 // «Левая и правая части амулета Дракона»
	MiscAmuletRB     = 42 // «Правая и нижняя части амулета Дракона»
	MiscAmuletLB     = 43 // «Левая и нижняя части амулета Дракона»
	MiscWaterKey     = 44 // «Ключ Воды»
	MiscMirror       = 48 // «Медное зеркало колдуна»
	MiscMoonKey      = 52 // «Ключ Луны»
	MiscFireKey      = 53 // «Ключ Огня»
)

// Status-line messages of the books (0x0042F9B8).
const (
	MsgHippocrates = 0x0045C77C // «Ты открыл для себя секреты древних целителей»
	MsgPyramids    = 0x0045C97C // «Ты познал секреты строительного искусства»
	MsgSmithScroll = 0x0045CB7C // «Ты познал секреты кузнечного дела»
	MsgTradeTract  = 0x0045CD7C // «Ты узнал много нового об искусстве торговли»
	MsgThirdEye    = 0x0045CF7C // «Ты узнал много интересного о неведомых вещах»
	MsgBirchBark   = 0x0045D17C // «Ты прочитал Берестяную Грамоту»
)

// Enchantments the misc items put on characters (entity +0xD2 / +0xD3).
const (
	enchantDoll   = 19 // «Кукла» on every character of the other groups (+0xD2)
	enchantRelics = 70 // «Святые мощи» on the user (+0xD3)
)

// World is the part of the scene the misc items reach beyond their user and
// its item records: the game keeps one set of entity records, the port keeps
// the party in copies and every position in the scene's walkers.
type World interface {
	// Hero is the hero's live record (0x00A3E694).
	Hero() *Entity
	// Entity is the live record of entity i (a party copy or the GAME.N
	// record).
	Entity(i int) *Entity
	// Cell is entity i's fine-grid cell (+0x08, +0x0A).
	Cell(i int) (row, col int)
	// Alive: not dead (+4 bit 0x80) and not playing its death (action 3/9).
	Alive(i int) bool
	// Kill starts entity i's death action (0x004143DC with 3 in the stance,
	// 9 out of it) and drops its loot (0x00418510(e, 0, level)). The record
	// is already dead (UseMisc set HP 0).
	Kill(i, level int)
	// Walk orders entity i to walk to (row, col) (0x004142D4(e, row, col,
	// 8)).
	Walk(i, row, col int)
	// Blocked tests a fine-grid cell's occupant and wall bits (& 0xFFF).
	Blocked(row, col int) bool
	// EnemyGroup is 0x0041FD74 for the party: the group (on this map,
	// other than the party, not a calm village, not kind 0x53) with the
	// living member nearest to the leader inside the leader's cell ±
	// (W + W/2) rows and (C + C/2) columns; -1 = none.
	EnemyGroup() int
	// Jump is 0x004106E0: a map change after this click, as a map exit
	// (0x00A3E810 = −x.To, party group +0x08/+0x10/+0x0C).
	Jump(x MapExit)
}

// orbJumps are the magic orbs' destinations (0x00415329 … 0x004153D9:
// 0x004106E0(map, row, col, facing)), each to its magus.
var orbJumps = map[int]MapExit{
	14: {To: 28, Row: 184, Col: 18, Facing: 1}, // Волхв у Бережков
	15: {To: 5, Row: 60, Col: 68, Facing: 2},   // Волхв у Старого лагеря
	16: {To: 10, Row: 147, Col: 66, Facing: 1}, // Волхв у Византийского Лагеря
	17: {To: 20, Row: 49, Col: 41, Facing: 2},  // Волхв у Черного Бора
	18: {To: 3, Row: 141, Col: 54, Facing: 1},  // Волхв у Родников
	19: {To: 40, Row: 42, Col: 31, Facing: 1},  // Волхв у Лесного лагеря
	21: {To: 43, Row: 18, Col: 24, Facing: 2},  // Пещера Зверя
}

// keyDoors are the keys of the dragon's nest (0x0041594F, 0x00415A34,
// 0x00415A7F): on map Map with the user on cell (Row, Col) exactly, the key
// jumps the party; anywhere else it does nothing (and is kept).
var keyDoors = map[int]struct {
	Map, Row, Col int
	To            MapExit
}{
	MiscWaterKey: {50, 34, 11, MapExit{To: 46, Row: 68, Col: 13, Facing: 2}},
	MiscMoonKey:  {48, 23, 9, MapExit{To: 49, Row: 106, Col: 17, Facing: 2}},
	MiscFireKey:  {49, 15, 17, MapExit{To: 50, Row: 134, Col: 12, Facing: 2}},
}

// amuletParts is 0x00410010: two parts (in either order) and what the
// target becomes.
var amuletParts = []struct{ a, b, to int }{
	{MiscAmuletRight, MiscAmuletLeft, MiscAmuletLR},
	{MiscAmuletRight, MiscAmuletBottom, MiscAmuletRB},
	{MiscAmuletRight, MiscAmuletLB, MiscAmulet},
	{MiscAmuletLeft, MiscAmuletBottom, MiscAmuletLB},
	{MiscAmuletLeft, MiscAmuletRB, MiscAmulet},
	{MiscAmuletBottom, MiscAmuletLR, MiscAmulet},
}

// Kinds (entity +4) the whistle and the pipe leave alone (0x0041587E).
var unscared = []byte{0x44, 0x53, 0x57, 0x54}

// UseMisc is 0x00414E64(e, target, item): misc item item is used by e, on
// the boxed item target (0 = none: the bag's right click, a portrait). It
// reports whether the item was used, and a status-line message (0 = none).
// A refusal changes nothing.
//
// After a use the common tail 0x00415BD3 runs: with a target, a kept item
// goes to the front of e's bag (0x00424918) and a used-up one (item +0 =
// 0xFF) leaves it (0x00421D24); without one, a used-up item leaves e's bag.
// Then sound 10 plays (0x00415C6C).
func (g *Game) UseMisc(e *Entity, t *resfmt.ExeTables, target, item int) (bool, int) {
	it, ok := g.Item(item)
	if !ok || it.Category != CatQuest {
		return false, 0
	}
	w := g.World
	curMap := g.Groups[0].Map // 0x00A3E7FC
	used := false             // item +0 = 0xFF
	msg := 0
	// skill adds to a skill word, capped when limit > 0 (unsigned compare).
	skill := func(k, add, limit int) {
		e.Skills[k] = int(uint16(e.Skills[k] + add))
		if limit > 0 && e.Skills[k] > limit {
			e.Skills[k] = limit
		}
	}
	tgt, hasTgt := g.Item(target)
	if target == 0 {
		hasTgt = false
	}
	switch typ := it.Type; typ {
	case MiscLostMap:
		g.RevealMap()
		used = true
	case MiscHippocrates:
		skill(0, 100, maxHealing)
		msg, used = MsgHippocrates, true
	case MiscPyramids:
		skill(3, 5, 0)
		msg, used = MsgPyramids, true
	case MiscSmithScroll:
		skill(1, 4, 0)
		msg, used = MsgSmithScroll, true
	case MiscTradeTract:
		skill(2, 10, 100)
		msg, used = MsgTradeTract, true
	case MiscThirdEye:
		skill(4, 100, 100)
		msg, used = MsgThirdEye, true
	case MiscWords:
		g.ReviveAll = true // 0x00414FFB; the scene revives (it holds the party copies)
		used = true
	case MiscDuplicator:
		// 0x004150AD: the duplicator becomes a copy of the boxed item and
		// goes to the bag by the tail.
		if !hasTgt {
			return false, 0
		}
		copy(g.items[item*itemStride:][:itemStride], g.items[target*itemStride:][:itemStride])
	case MiscFalconFoot:
		e.BaseStats[1] = int(uint16(e.BaseStats[1] + 3)) // base Сил +0x60
		used = true
	case MiscSeerScroll:
		// 0x004150E1 → 0x0041BABC: e's unidentified items are identified,
		// each training the hero's curios.
		if w == nil {
			return false, 0
		}
		g.identifyAll(e, w.Hero())
		used = true
	case MiscApple:
		g.GainExperience(e, t, 200)
		used = true
	case MiscWine:
		g.wine(e)
		used = true
	case MiscMagnet:
		if w == nil {
			return false, 0
		}
		g.magnet(w, e, curMap)
		used = true
	case 14, 15, 16, 17, 18, 19, MiscOrbCaves:
		if w == nil {
			return false, 0
		}
		w.Jump(orbJumps[typ]) // the orb is kept
	case MiscJumpOrb:
		g.OpenMap = true
		g.sound(SoundUse) // 0x004153CC, besides the tail's
	case MiscHilt, MiscBlade:
		// 0x004153EE / 0x00415479: hilt on blade (or blade on hilt) makes
		// the boxed part «Меч-кладенец».
		other := MiscBlade
		if typ == MiscBlade {
			other = MiscHilt
		}
		if !hasTgt || tgt.Category != CatQuest || tgt.Type != other {
			return false, 0
		}
		g.GainExperience(e, t, 30)
		g.setItem(target, CatBlade, ancientSwordType)
		g.items[target*itemStride+1], g.items[target*itemStride+2] = 0xFF, ancientSwordSpecial
		g.setCondition(target, t)
		g.setItemWord(target, 0xC, 0)
		used = true
	case MiscShovel:
		// 0x00415504: window 0 (closed) and the dig cursor 7; the callers
		// set the cursor (the box and portrait clicks reset it to 4).
		g.CloseWindow = true
		g.sound(SoundUse) // 0x00415520, besides the tail's
	case MiscAmuletRight, MiscAmuletLeft, MiscAmuletBottom, MiscAmuletLR, MiscAmuletRB, MiscAmuletLB:
		if !hasTgt || !g.joinAmulet(e, t, typ, target, tgt) {
			return false, 0
		}
		used = true
	case MiscBirchBark:
		g.Quests[0] |= questFlagBit // quest flag 0 (0x00625068 bit 0x80)
		msg, used = MsgBirchBark, true
	case MiscSkeletonLamp:
		if w == nil {
			return false, 0
		}
		g.skeletonLamp(w, e, curMap)
		used = true
	case MiscTorch:
		g.Lit = true // 0x004156E6: 0x00A3E758 = 1; the torch is kept
	case MiscDoll:
		if w == nil {
			return false, 0
		}
		g.othersOnMap(w, e, curMap, func(i int, m *Entity) {
			if w.Alive(i) {
				m.Enchants[0] = enchantDoll
			}
		})
		used = true
	case MiscWhistle, MiscPipe:
		if w == nil {
			return false, 0
		}
		g.scare(w) // kept
	case MiscGoblet:
		e.BaseStats[0] = int(uint16(e.BaseStats[0] + 10)) // base Хар +0x5A
		used = true
	case MiscRelics:
		e.Enchants[1] = enchantRelics
		used = true
	case MiscToys:
		e.BaseStats[0] = int(uint16(e.BaseStats[0] + 5))
		used = true
	case MiscWaterKey, MiscMoonKey, MiscFireKey:
		if w == nil {
			return false, 0
		}
		d := keyDoors[typ]
		if r, c := w.Cell(e.Index); curMap == d.Map && r == d.Row && c == d.Col {
			w.Jump(d.To)
		} // kept either way
	case MiscMirror:
		// 0x0041599A: the buried piles of this map (gold < 0) come up.
		for i := range g.Piles {
			if p := &g.Piles[i]; p.Gold < 0 && p.Map == curMap {
				p.Gold = -p.Gold
			}
		}
		used = true
	default:
		// 0x00415ACD: 11 «Сын Луны», 29 «Медная лампа», 32 «Неразменный
		// пятак», 34, 40, 45–47, 49–51 and anything above 53.
		return false, 0
	}
	if used {
		g.items[item*itemStride] = 0xFF
	}
	switch {
	case target != 0 && !used:
		BagInsert(e, item)
	case used:
		if k := slices.Index(e.Bag[:], item); k >= 0 {
			copy(e.Bag[k:], e.Bag[k+1:])
			e.Bag[len(e.Bag)-1] = 0
		}
	}
	g.sound(SoundUse)
	return true, msg
}

// «Меч-кладенец», what the hilt and the blade make: gear type 27 with
// special property 14 (0x0041542E–0x00415468).
const (
	ancientSwordType    = 27
	ancientSwordSpecial = 14
)

// wine is 0x00415156, a drink like «Брага» from base Лов: n = base Лов / 3,
// at most 5. A first potion saves the base stats; the timer becomes 30n
// (no refusal while another potion runs); Лов −3n, Хар, Сил, Вын +n.
func (g *Game) wine(e *Entity) {
	n := min(int(int16(e.BaseStats[2]))/3, 5)
	if e.PotionTimer == 0 {
		e.SavedBase = [4]int{e.BaseStats[2], e.BaseStats[0], e.BaseStats[1], e.BaseStats[3]}
	}
	e.PotionTimer, e.HitBonus = n*potionPass, 0
	e.BaseStats[2] = int(uint16(e.BaseStats[2] - 3*n))
	for _, k := range []int{0, 1, 3} {
		e.BaseStats[k] = int(uint16(e.BaseStats[k] + n))
	}
}

// joinAmulet is 0x00410010(e, target, item): two parts of the dragon's
// amulet (target a misc item) join in the target; e gains 100 experience.
func (g *Game) joinAmulet(e *Entity, t *resfmt.ExeTables, typ, target int, tgt Item) bool {
	if tgt.Category != CatQuest {
		return false
	}
	to := 0
	for _, p := range amuletParts {
		if typ == p.a && tgt.Type == p.b || typ == p.b && tgt.Type == p.a {
			to = p.to
		}
	}
	if to == 0 {
		return false
	}
	g.items[target*itemStride+3] = byte(to)
	g.GainExperience(e, t, 100)
	return true
}

// othersOnMap calls f for every member of every group on map m other than
// e's (group order, then member order), as the map-wide cases loop.
func (g *Game) othersOnMap(w World, e *Entity, m int, f func(i int, member *Entity)) {
	for gi, gr := range g.Groups {
		if gi == e.Group || gr.Map != m {
			continue
		}
		for k := 0; k < gr.Count; k++ {
			if i := gr.First + k; i >= 0 && i < len(g.Entities) {
				f(i, w.Entity(i))
			}
		}
	}
}

// magnet is 0x0041521A: from every character of the other groups on the
// map, dead or alive, the gold goes to the hero, and the bag items from the
// last slot down to the front of e's bag until it is full.
func (g *Game) magnet(w World, e *Entity, m int) {
	hero := w.Hero()
	g.othersOnMap(w, e, m, func(_ int, v *Entity) {
		hero.Gold += v.Gold
		v.Gold = 0
		for k := len(v.Bag) - 1; k >= 0; k-- {
			if v.Bag[k] == 0 {
				continue
			}
			if !BagInsert(e, v.Bag[k]) {
				break
			}
			v.Bag[k] = 0
		}
	})
}

// skeletonLamp is 0x0041558E: every living character of the other groups on
// the map that are neither villages (+0x1E bit 0x80) nor kind 0x53 dies,
// with no experience; its loot drops with the hero's level.
func (g *Game) skeletonLamp(w World, e *Entity, m int) {
	level := w.Hero().Level
	g.othersOnMap(w, e, m, func(i int, v *Entity) {
		gr := g.Groups[v.Group]
		if gr.Kind&0x80 != 0 || gr.Kind == 0x53 || !w.Alive(i) {
			return
		}
		v.WeaponMode, v.Flags2, v.HP = 0, 0, 0
		w.Kill(i, level)
	})
}

// scare is the whistle and the pipe (0x004157D2): the animals (+4 bit 0x40,
// not kinds 0x44, 0x53, 0x54, 0x57) of the party's enemy group (unless it
// is a village) run away (FleeOffset).
func (g *Game) scare(w World) {
	gi := w.EnemyGroup()
	if gi < 0 || gi >= len(g.Groups) || g.Groups[gi].Kind&0x80 != 0 {
		return
	}
	gr := g.Groups[gi]
	for k := 0; k < gr.Count; k++ {
		i := gr.First + k
		if i < 0 || i >= len(g.Entities) || !w.Alive(i) {
			continue
		}
		if kind := w.Entity(i).Flags4; kind&0x40 == 0 || slices.Contains(unscared, kind) {
			continue
		}
		r, c := w.Cell(i)
		if dr, dc, ok := FleeOffset(g.rand, r, c, VillageRowRange, VillageColRange, w.Blocked); ok {
			w.Walk(i, r+dr, c+dc)
		}
	}
}

// FleeOffset is 0x004101E4: a cell far off one side of (row, col), picked
// by rand() % 4: two view heights (2·rows) up or down with the first free
// column offset from −cols on, or two view widths (2·cols) left or right
// with the first free row offset from −rows on. Cells with a negative
// coordinate count as blocked. ok is false when the scanned line has no
// free cell.
func FleeOffset(rng func(int) int, row, col, rows, cols int, blocked func(r, c int) bool) (dr, dc int, ok bool) {
	free := func(r, c int) bool { return r >= 0 && c >= 0 && !blocked(r, c) }
	side := rng(4)
	if side < 2 { // 0x0041020B up, 0x00410287 down
		dr = -2 * rows
		if side == 1 {
			dr = 2 * rows
		}
		for dc = -cols; dc < cols && !free(row+dr, col+dc); dc++ {
		}
	} else { // 0x00410300 left, 0x0041037C right
		dc = -2 * cols
		if side == 3 {
			dc = 2 * cols
		}
		for dr = -rows; dr < rows && !free(row+dr, col+dc); dr++ {
		}
	}
	return dr, dc, dr != rows && dc != cols // 0x00410417
}
