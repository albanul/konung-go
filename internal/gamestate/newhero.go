package gamestate

import (
	"encoding/binary"

	"konung1.5/internal/resfmt"
)

// Character creation «Выбор героя» (screen state 2, docs/konung_new_game.md).
// The original edits the hero entity 0x00A3E694 in place: «Новая игра»
// (0x00437B91) initialises it, the click switch 0x00437E86 (jump table
// 0x00438828, hotspots 0x004600FC) changes it, and the game start
// (0x00438F3F) copies it to 0x00A3E584, loads GAME.<model> and writes the
// chosen fields back over the file's hero (StartHero). The port keeps the
// template as a plain Entity.

// Hero models (+0xD6), in the order of the screen's portraits (hotspots
// 0–2): Драгомир, Михаил, Волк.
const (
	HeroVolk     = 0
	HeroDragomir = 1
	HeroMichail  = 2
)

// Classes as the +4 byte (portrait face; the class index is +4 − 1, see
// RaiseCost). Храбрец (1) cannot be chosen.
const (
	ClassMerchant = 2 // Купец
	ClassHunter   = 3 // Охотник
	ClassChief    = 4 // Вождь
	ClassWarrior  = 5 // Воин
)

// Creation hotspots (table 0x004600FC, 32 bytes per entry: 640×480 then
// 800×600 x1,y1,x2,y2; the index is the click switch case).
const (
	NewHeroPortrait  = 0  // 0–2: Драгомир, Михаил, Волк
	NewHeroClass     = 3  // 3–6: Купец, Охотник, Вождь, Воин
	NewHeroStat      = 7  // 7–10: Хар, Сил, Лов, Вын values
	NewHeroWeight    = 11 // «Макс. груз»
	NewHeroParty     = 12 // «Макс. отряд»
	NewHeroSmithing  = 13 // «Кузнечное дело»
	NewHeroBuilding  = 14 // «Плотницкое дело»
	NewHeroFencing   = 15 // «Фехтование»
	NewHeroArchery   = 16 // «Меткость»
	NewHeroIdentify  = 17 // «Идентификация»
	NewHeroTrade     = 18 // «Торговля»
	NewHeroHealing   = 19 // «Знахарство»
	NewHeroOK        = 20
	NewHeroCancel    = 21
	NewHeroPreview   = 22 // top-left of the hero preview (a point)
	NewHeroText      = 23 // description box
	NewHeroFreeExp   = 24 // free experience next to «Параметры»
	NewHeroMinus     = 25 // 25/26, 27/28, 29/30, 31/32: − and + for Хар, Сил, Лов, Вын
	NewHeroHotspots  = 33
	NewHeroTextCount = 20 // descriptions 0x00462F3C + i·512 (i < 20)
)

// NewHeroRects800 is the 800×600 half of the hotspot table 0x004600FC.
var NewHeroRects800 = [NewHeroHotspots][4]int{
	{44, 108, 120, 190}, {155, 108, 236, 190}, {265, 108, 346, 190}, // portraits
	{582, 81, 602, 101}, {549, 81, 569, 101}, {549, 49, 569, 69}, {582, 49, 602, 69}, // classes
	{545, 195, 575, 210}, {545, 218, 575, 233}, {545, 239, 575, 254}, {545, 259, 575, 274}, // stats
	{607, 311, 643, 326}, {607, 333, 643, 348}, // weight, party
	{695, 414, 725, 429}, {695, 436, 725, 451}, // smithing, building
	{542, 413, 562, 433}, {542, 434, 562, 454}, // fencing, archery
	{542, 466, 562, 486}, {542, 487, 562, 507}, {542, 509, 562, 529}, // identify, trade, healing
	{85, 527, 185, 560}, {195, 527, 295, 560}, // Ok, Отмена
	{570, 160, 570, 160},                       // preview origin
	{40, 266, 357, 566},                        // description
	{543, 164, 577, 179},                       // free experience
	{515, 195, 545, 210}, {575, 195, 605, 210}, // Хар −/+
	{515, 218, 545, 233}, {575, 218, 605, 233}, // Сил −/+
	{515, 239, 545, 254}, {575, 239, 605, 254}, // Лов −/+
	{515, 259, 545, 274}, {575, 259, 605, 274}, // Вын −/+
}

// NewHeroHit is 0x004398C4 on the 800×600 table: the first hotspot whose
// rectangle holds (x, y), or −1. The game's test includes x2 and y2.
func NewHeroHit(x, y int) int {
	for i, r := range NewHeroRects800 {
		if x >= r[0] && x <= r[2] && y >= r[1] && y <= r[3] {
			return i
		}
	}
	return -1
}

// HeroPalette is the palette byte offset (+0xD8) that hero selection writes
// per model (0x0043805A Волк 0x9400, 0x00437EDB Драгомир 0x4200,
// 0x00437FA0 Михаил 0x4E00): GRAPH.RES palettes 74, 33 and 39.
var HeroPalette = [3]uint32{74 * 512, 33 * 512, 39 * 512}

// HeroNameIndex is the first-name index (+0xD4) hero selection writes:
// model 0 Волк 10, 1 Драгомир 124, 2 Михаил 151.
var HeroNameIndex = [3]int{10, 124, 151}

// heroGear is the equipment (+0x66, items 1–11 of NewHeroItems) hero
// selection gives each model for the preview (0x00437F23, 0x00437FE8,
// 0x004380A2). The game start does not copy it: the hero keeps the gear from
// GAME.N.
var heroGear = [3][6]int{
	HeroVolk:     {0, 4, 5, 2, 6, 9},
	HeroDragomir: {3, 4, 0, 1, 8, 11},
	HeroMichail:  {0, 4, 5, 0, 7, 10},
}

// newHeroItems are the item records 1–11 that 0x00437D34 writes for the
// preview: category (+0), type (+3).
var newHeroItems = [12][2]int{
	{}, {3, 0x24}, {3, 0x22}, {0, 3}, {1, 6}, {2, 0},
	{4, 0x34}, {4, 0x2B}, {4, 0x37}, {5, 0x25}, {5, 0x26}, {5, 0x3D},
}

// NewHeroItems returns a Game holding only the preview items, for
// Game.Overlays on the template.
func NewHeroItems() *Game {
	g := &Game{items: make([]byte, len(newHeroItems)*itemStride)}
	for i, it := range newHeroItems[1:] {
		g.setItem(i+1, it[0], it[1])
	}
	return g
}

// classPreset is what a class click sets (0x00438102 …): base Хар, Сил,
// Лов, Вын (BaseStats order). Free experience goes to 0.
var classPreset = map[byte][4]int{
	ClassMerchant: {3, 10, 3, 12},
	ClassHunter:   {3, 3, 15, 10},
	ClassChief:    {25, 7, 3, 7},
	ClassWarrior:  {3, 15, 3, 10},
}

// NewHeroTemplate is «Новая игра» (0x00437B91): Драгомир, a level-10 Воин
// in the combat stance facing SE, with Фехтование and Идентификация chosen.
func NewHeroTemplate() Entity {
	e := Entity{
		Level:        10,
		Variant:      5,
		Flags3:       0x04,
		Fencing:      100,
		Marksmanship: 0,
	}
	e.Skills = [5]int{50, 10, 10, 1, 20} // healing, smithing, trade, building, curios
	SelectClass(&e, ClassWarrior)
	SelectHero(&e, HeroDragomir)
	return e
}

// SelectHero is a portrait click (hotspots 0–2, model = Драгомир 1,
// Михаил 2, Волк 0): name, model, palette, the preview gear and the weapon
// in hand (the bow with Меткость; else the axe for Драгомир, the blade for
// the others). Stats and skills stay.
func SelectHero(e *Entity, model int) {
	if model < 0 || model > 2 {
		return
	}
	e.Model, e.NameIndex, e.PaletteOffset = model, HeroNameIndex[model], HeroPalette[model]
	e.Equipment = heroGear[model]
	e.WeaponMode = WeaponBow
	if e.Fencing > e.Marksmanship {
		e.WeaponMode = WeaponBlade
		if model == HeroDragomir {
			e.WeaponMode = WeaponAxe
		}
	}
	e.Type, e.Step = 0, 0 // 0x004143DC(hero, 0): stance idle, step 0
}

// SelectClass is a class click (hotspots 3–6): the class byte and its
// preset base stats; the effective stats follow, free experience is 0.
func SelectClass(e *Entity, class byte) {
	p, ok := classPreset[class]
	if !ok {
		return
	}
	e.Flags4 = class
	e.BaseStats, e.Stats = p, p
	e.FreeExp = 0
}

// SelectCombatSkill is hotspot 15 (Фехтование, 0x00438352) or 16
// (Меткость, 0x004383C0): the chosen skill gets 100, the other 0, with
// building and smithing to match, and the weapon in hand follows.
func SelectCombatSkill(e *Entity, archery bool) {
	if archery {
		e.Fencing, e.Marksmanship = 0, 100
		e.Skills[3], e.Skills[1] = 2, 5
		e.WeaponMode = WeaponBow
		return
	}
	e.Fencing, e.Marksmanship = 100, 0
	e.Skills[3], e.Skills[1] = 1, 10
	e.WeaponMode = WeaponBlade
	if e.Model == HeroDragomir {
		e.WeaponMode = WeaponAxe
	}
}

// Peaceful skills chosen by hotspots 17–19.
const (
	SkillIdentify = 0 // Идентификация: curios 20
	SkillTrade    = 1 // Торговля: trade 20
	SkillHealing  = 2 // Знахарство: healing 100
)

// SelectSkill is hotspots 17–19 (0x00438410, 0x0043844C, 0x00438488): the
// chosen skill is raised, the other two get their base values (trade 10,
// healing 50, curios 10).
func SelectSkill(e *Entity, k int) {
	trade, healing, curios := 10, 50, 10
	switch k {
	case SkillIdentify:
		curios = 20
	case SkillTrade:
		trade = 20
	case SkillHealing:
		healing = 100
	default:
		return
	}
	e.Skills[2], e.Skills[0], e.Skills[4] = trade, healing, curios
}

// ChosenSkill is the peaceful skill the screen marks (0x0043016F): curios
// > 10, trade > 10, healing > 50. More than one can't be set by the screen.
func ChosenSkill(e Entity) int {
	switch {
	case e.Skills[4] > 10:
		return SkillIdentify
	case e.Skills[2] > 10:
		return SkillTrade
	case e.Skills[0] > 50:
		return SkillHealing
	}
	return -1
}

// TradeStat is a − or + click on stat s (BaseStats order: 0 Хар, 1 Сил,
// 2 Лов, 3 Вын; hotspots 25 + 2s and 26 + 2s, 0x0043850A … 0x004387D2).
// + pays RaiseCost for the next point out of free experience. − takes a
// point off a non-zero stat and refunds what that point cost: RaiseCost
// at the lowered value, evaluated with 1000 extra free experience so it is
// always affordable. Both fail at the class caps of the template's level
// (10), which RaiseCost applies. It reports whether the stat changed.
//
// The game moves the effective value along only for Хар and Вын; the
// screen shows the base values and the game start sets effective = base,
// so the port keeps both in step.
func TradeStat(e *Entity, t *resfmt.ExeTables, s int, plus bool) bool {
	if s < 0 || s > 3 {
		return false
	}
	id := RaiseIDs[s+1]
	g := new(Game) // RaiseCost reads only e and t
	if plus {
		c := g.RaiseCost(*e, t, id)
		if c == 0 {
			return false
		}
		e.BaseStats[s]++
		e.Stats[s] = e.BaseStats[s]
		e.FreeExp -= c
		return true
	}
	if e.BaseStats[s] == 0 {
		return false
	}
	e.BaseStats[s]--
	e.FreeExp += 1000
	c := g.RaiseCost(*e, t, id)
	e.FreeExp -= 1000
	if c == 0 {
		e.BaseStats[s]++
		return false
	}
	e.FreeExp += c
	e.Stats[s] = e.BaseStats[s]
	return true
}

// MaxParty is «Макс. отряд», 0x00411078: the number of the class's ten
// Хар thresholds (int32 table 0x0045BCB4 + class·40) that effective Хар
// reaches, plus one, at most 10.
func MaxParty(e Entity, t *resfmt.ExeTables) int {
	class := int(e.Flags4&7) - 1
	if class < 0 || class > 4 {
		return 0
	}
	b := t.Bytes(0x0045BCB4+class*40, 40)
	n := 0
	for n < 10 && int(int32(binary.LittleEndian.Uint32(b[n*4:]))) <= e.Stats[0] {
		n++
	}
	if n < 10 {
		n++
	}
	return n
}

// StartHero is the game start 0x00438F3F–0x004390D5 after GAME.<model> is
// loaded: the hero (group 0's first member) takes the template's skills,
// base stats (effective = base), free experience, name, model, palette and
// class byte, faces SE (+1 = 5) and starts at level 1; the party's map
// becomes model·3 + 1. Everything else (gear, HP, position) is the file's.
func (g *Game) StartHero(tmpl Entity) {
	if len(g.Groups) == 0 {
		return
	}
	i := g.Groups[0].First
	if i < 0 || i >= len(g.Entities) {
		return
	}
	e := &g.Entities[i]
	e.Fencing, e.Marksmanship = tmpl.Fencing, tmpl.Marksmanship
	e.Skills = tmpl.Skills
	e.BaseStats, e.Stats = tmpl.BaseStats, tmpl.BaseStats
	e.FreeExp = tmpl.FreeExp
	e.Variant = 5
	e.NameIndex, e.Model, e.PaletteOffset = tmpl.NameIndex, tmpl.Model, tmpl.PaletteOffset
	e.Flags4 = tmpl.Flags4
	e.Level = 1
	g.Groups[0].Map = HeroStartMap(tmpl.Model)
}

// HeroStartMap is the hero's start map, KONUNG.<model·3 + 1> (0x004390B4).
func HeroStartMap(model int) int { return model*3 + 1 }
