package gamestate

import "fmt"

// The LAN version's «Новый герой» (state 4, LAN_KONG.EXE 0x00427D14 draw,
// 0x0042E3CA clicks, docs/konung_lan.md#new-hero). It is the single-player
// creation screen's handlers (class, combat skill, peaceful skill, the stat
// trade) on the LAN tables below, with a clan picker and a look switch in
// place of the hero portraits; the hero model is chosen at login. The
// tables are hard-coded so the dedicated server, which has no exe, applies
// the same rules; TestLANHeroTables checks them against LAN_KONG.EXE.

// «Новый герой» hotspots of the LAN's own (LANExe.NewHeroRects; 3–21 and
// 24–32 are the single-player screen's: NewHeroClass, NewHeroStat, …).
const (
	LANNewClan     = 0 // clan text field (31 bytes with the cursor)
	LANNewClanUp   = 1 // clan list scroll
	LANNewClanDown = 2
	LANNewPortrait = 22 // the hero (160×160 from the top-left); a click: next look
	LANNewClanList = 23 // clan rows, 15 px each from +5
	LANNewClanRowH = 15
)

// LANHeroEditLevel is the template's level while the screen edits it
// (0x0042E0DB; single-player 10): the stat caps are level 13's. «Ok» sets 1.
const LANHeroEditLevel = 13

// lanRaiseCosts is 0x00481E24: rows of (threshold for the class's
// favourite stat, threshold for the others, cost); a raise from value v
// costs the first row whose threshold is above v (0x0044CDF0). The last
// row (999999, 999999, 999) is left out: values never reach it below the
// caps.
var lanRaiseCosts = [12][3]int{
	{12, 9, 5}, {19, 14, 15}, {26, 20, 25}, {34, 28, 35}, {43, 36, 60}, {53, 44, 100},
	{63, 52, 200}, {74, 60, 300}, {80, 65, 400}, {90, 70, 500}, {95, 75, 600}, {100, 80, 800},
}

// lanFavourite is 0x00481DDC by class byte: the favourite stat in
// BaseStats order (0 Хар, 1 Сил, 2 Лов, 3 Вын): Купец Вын, Охотник Лов,
// Вождь Хар, Воин Сил.
var lanFavourite = map[byte]int{ClassMerchant: 3, ClassHunter: 2, ClassChief: 0, ClassWarrior: 1}

// LANStatCap is a stat's cap at the edit level (0x00481EC0 + class·0x68 +
// stat·0x1A, level 13): a raise needs value < cap. 105 for the class's
// favourite stat, 42 for the others (every class).
func LANStatCap(class byte, stat int) int {
	if lanFavourite[class] == stat {
		return 105
	}
	return 42
}

// LANRaiseCost is what raising stat (BaseStats order) from value to
// value+1 costs a hero of class, 0 when the raise is not possible (cap, or
// not a creation class).
func LANRaiseCost(class byte, stat, value int) int {
	fav, ok := lanFavourite[class]
	if !ok || stat < 0 || stat > 3 || value < 0 || value >= LANStatCap(class, stat) {
		return 0
	}
	col := 1
	if stat == fav {
		col = 0
	}
	for _, r := range lanRaiseCosts {
		if value < r[col] {
			return r[2]
		}
	}
	return 0
}

// LANTradeStat is a − or + click (hotspots 25–32), like TradeStat on the
// LAN costs: + pays the raise cost out of the free points if they cover
// it; − takes a point off a non-zero stat and refunds that point's cost.
func LANTradeStat(e *Entity, s int, plus bool) bool {
	if s < 0 || s > 3 {
		return false
	}
	class := e.Flags4 & 7
	if plus {
		c := LANRaiseCost(class, s, e.BaseStats[s])
		if c == 0 || c > e.FreeExp {
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
	c := LANRaiseCost(class, s, e.BaseStats[s]-1)
	if c == 0 {
		return false
	}
	e.BaseStats[s]--
	e.Stats[s] = e.BaseStats[s]
	e.FreeExp += c
	return true
}

// lanPartyThresholds is 0x00482190 by class byte: the base Хар a hero
// needs for each further party member.
var lanPartyThresholds = map[byte][3]int{
	ClassMerchant: {15, 21, 28}, ClassHunter: {17, 23, 30}, ClassChief: {15, 21, 28}, ClassWarrior: {20, 28, 36},
}

// LANMaxParty is «Макс. отряд» (0x0044D798): the number of the class's
// three thresholds base Хар reaches, plus one unless it reaches all three:
// 1 to 3.
func LANMaxParty(class byte, charisma int) int {
	t := lanPartyThresholds[class]
	n := 0
	for n < 3 && t[n] <= charisma {
		n++
	}
	if n < 3 {
		n++
	}
	return n
}

// LANMaxWeight is «Макс. груз» in grams (0x00427D14: 10 kg + base Вын·1000/3).
func LANMaxWeight(endurance int) int { return 10000 + endurance*1000/3 }

// LANLooks are the palettes (GRAPH.RES palette index) of the 8 looks per
// hero model (0x004821CC, 0x004821EC, 0x0048220C; the exe stores index ×
// 512). The screen's portrait click steps the look (0x0042E3CA hotspot 22).
var LANLooks = [3][8]int{
	{70, 71, 72, 70, 73, 74, 113, 114},
	{28, 29, 30, 31, 32, 33, 109, 110},
	{34, 35, 36, 37, 38, 39, 111, 112},
}

// LANLookPalette is model's palette index for look (0–7).
func LANLookPalette(model, look int) int {
	if model < 0 || model > 2 {
		model = 0
	}
	if look < 0 || look > 7 {
		look = 0
	}
	return LANLooks[model][look]
}

// NewLANHeroTemplate is the template the screen opens with (0x0042E0DB):
// the single-player one (Воин, Фехтование, Идентификация) at level 13
// with no free points.
func NewLANHeroTemplate() Entity {
	e := NewHeroTemplate()
	e.Level = LANHeroEditLevel
	return e
}

// LANHeroChoice is what a player picks on «Новый герой»: the class byte
// (2–5), the combat skill, the peaceful skill (SkillIdentify, SkillTrade,
// SkillHealing) and the four base stats after trading (BaseStats order:
// Хар, Сил, Лов, Вын).
type LANHeroChoice struct {
	Class   byte
	Archery bool
	Skill   int
	Stats   [4]int
}

// Build applies c to the template like the screen's clicks would: the
// class preset, the skills, then the stat trade from the preset to c.Stats.
// The trade is path independent (a point's cost depends only on its value),
// so it is valid when every stat is within 0 … cap and the points left are
// not negative. It returns the hero (level 1, as «Ok» sets) or why c is
// not something the screen can produce.
func (c LANHeroChoice) Build() (Entity, error) {
	e := NewLANHeroTemplate()
	if _, ok := classPreset[c.Class]; !ok {
		return e, fmt.Errorf("class %d", c.Class)
	}
	if c.Skill < SkillIdentify || c.Skill > SkillHealing {
		return e, fmt.Errorf("skill %d", c.Skill)
	}
	SelectClass(&e, c.Class)
	SelectCombatSkill(&e, c.Archery)
	SelectSkill(&e, c.Skill)
	points := 0
	for s, want := range c.Stats {
		cap := LANStatCap(c.Class, s)
		if want < 0 || want > cap {
			return e, fmt.Errorf("stat %d = %d outside 0…%d", s, want, cap)
		}
		for v := e.BaseStats[s]; v < want; v++ {
			points -= LANRaiseCost(c.Class, s, v)
		}
		for v := e.BaseStats[s] - 1; v >= want; v-- {
			points += LANRaiseCost(c.Class, s, v)
		}
	}
	if points < 0 {
		return e, fmt.Errorf("stats cost %d points more than traded", -points)
	}
	e.BaseStats, e.Stats = c.Stats, c.Stats
	e.FreeExp = points
	e.Level = 1
	return e, nil
}

// ChoiceOf reads the choice back from a hero the screen edited.
func ChoiceOf(e Entity) LANHeroChoice {
	skill := ChosenSkill(e)
	if skill < 0 {
		skill = SkillIdentify
	}
	return LANHeroChoice{Class: e.Flags4 & 7, Archery: e.Marksmanship > e.Fencing, Skill: skill, Stats: e.BaseStats}
}
