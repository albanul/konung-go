package gamestate

import (
	"encoding/binary"
	"math"

	"konung1.5/internal/resfmt"
)

// Loot and experience after a kill; see docs/konung_combat.md#loot-verified
// and #experience-and-levels-verified.

// Item categories used by the loot generator.
const (
	CatAxe      = 0
	CatBow      = 1
	CatBlade    = 2
	CatArmour   = 3
	CatHelmet   = 4
	CatShield   = 5
	CatNecklace = 6
	CatBracelet = 7
	CatRing     = 8
	CatHerb     = 9
	CatArrows   = 12
)

// newItem is 0x004399AC: the first free record (index ≥ 1, negative
// category) with +0xE = 0, no enchantment or special (+1 = +2 = 0xFF) and
// +0xC = 0. 0 when none is free.
func (g *Game) newItem() int {
	for i := 1; (i+1)*itemStride <= len(g.items); i++ {
		r := g.items[i*itemStride:]
		if int8(r[0]) < 0 {
			r[0xE], r[1], r[2] = 0, 0xFF, 0xFF
			binary.LittleEndian.PutUint16(r[0xC:], 0)
			return i
		}
	}
	return 0
}

func (g *Game) setItem(i, cat, typ int) {
	r := g.items[i*itemStride:]
	r[0], r[3] = byte(cat), byte(typ)
}

func (g *Game) setCondition(i int, t *resfmt.ExeTables) {
	r := g.items[i*itemStride:]
	c := math.Float32bits(float32(t.ItemField16(resfmt.ItemTableGear, int(r[3]), 0x2E)))
	binary.LittleEndian.PutUint32(r[4:], c)
	binary.LittleEndian.PutUint32(r[8:], c)
}

// lootLevelBand is 0x004171D4.
func lootLevelBand(l int) int {
	switch {
	case l <= 3:
		return 3
	case l <= 6:
		return 6
	case l <= 9:
		return 8
	}
	return 10
}

// DropLoot is 0x00418510(victim, a, l) on map m: gold, the bag, maybe a
// random item (by the killer's level l) and the poison sting go into the
// pile on the victim's cell (index at, or -1 to start a new one when newOK).
// curios is the hero's curios skill (+0x4A), which identifying a weapon
// raises. It returns the pile's index and whether one exists afterwards.
// rng(n) must return a value in [0, n); calls follow the game's order.
func (g *Game) DropLoot(victim *Entity, a, l, m, at int, newOK bool, curios *int, t *resfmt.ExeTables, rng func(int) int) (int, bool) {
	bagged := 0
	for _, it := range victim.Bag {
		if it != 0 {
			bagged++
		}
	}
	if bagged == 0 && victim.Gold == 0 && rng(101) > 30 && victim.PoisonDealt == 0 {
		return -1, false
	}
	var p *Pile
	if at >= 0 {
		p = &g.Piles[at]
	} else {
		if !newOK {
			return -1, false
		}
		g.Piles = append(g.Piles, Pile{Index: -1, Map: m, Row: victim.Row, Col: victim.Col})
		at = len(g.Piles) - 1
		p = &g.Piles[at]
	}
	slot := 0
	for slot < pileMaxItems && p.Items[slot] != 0 {
		slot++
	}
	g2 := func(n int) int { // rand()%(n+1), no call below 1
		if n+1 < 2 {
			return 0
		}
		return rng(n + 1)
	}
	bonus := 0
	if a >= 2 {
		bonus = rng(a)
	}
	p.Gold += victim.Gold + bonus
	for k, it := range victim.Bag {
		if slot >= pileMaxItems {
			break
		}
		if it != 0 {
			p.Items[slot] = it
			slot++
			victim.Bag[k] = 0
		}
	}
	ench := func() int {
		v := rng(lootLevelBand(l))
		return v + 10*rng(5)
	}
	identify := func(it int, learn bool) {
		if rng(100) > *curios {
			g.items[it*itemStride+0xE] |= 1
		} else if learn && *curios < 100 {
			*curios++
		}
		g.items[it*itemStride+1] = byte(ench())
	}
	if slot < pileMaxItems && l != 0 {
		if r := rng(101); r > 85 {
			if it := g.newItem(); it != 0 {
				p.Items[slot] = it
				slot++
				switch {
				case r == 86:
					typ := g2(l/5) + 3*rng(9)
					cat := CatBow
					switch typ % 9 {
					case 0, 1, 2:
						cat = CatBlade
					case 3, 4, 5:
						cat = CatAxe
					}
					g.setItem(it, cat, typ)
					g.setCondition(it, t)
					if rng(101) < 10 {
						identify(it, true)
					}
				case r == 87:
					switch rng(3) {
					case 0:
						tier := g2(l / 5)
						g.setItem(it, CatArmour, 0x22+11*rng(3)+tier)
					case 1:
						g.setItem(it, CatShield, 0x25+11*rng(3)+g2(l/4))
					default:
						g.setItem(it, CatHelmet, 0x29+11*rng(3)+g2(l/4))
					}
					g.setCondition(it, t)
					if rng(100) < 7 {
						identify(it, false)
					}
				case r <= 94:
					g.setItem(it, CatRing, 6+g2(l/5))
					identify(it, false)
				case r <= 97:
					g.setItem(it, CatBracelet, 3+g2(l/5))
					identify(it, false)
				default:
					g.setItem(it, CatNecklace, g2(l/5))
					identify(it, false)
				}
			}
		}
	}
	if slot < pileMaxItems && victim.PoisonDealt != 0 {
		if it := g.newItem(); it != 0 {
			p.Items[slot] = it
			g.setItem(it, CatHerb, 0) // «Ядовитое жало»
			r := g.items[it*itemStride:]
			binary.LittleEndian.PutUint32(r[4:], math.Float32bits(1))
			binary.LittleEndian.PutUint16(r[0xC:], uint16(victim.PoisonDealt))
		}
	}
	if p.Gold == 0 && p.Items[0] == 0 {
		if p.Index == -1 && at == len(g.Piles)-1 {
			g.Piles = g.Piles[:at]
		}
		return -1, false
	}
	return at, true
}

// RaiseCost is 0x00410B08(e, id): for id 0 it reports (as 1) whether e may
// go up a level; for ids 2/3/5/6 (Лов, Хар, Сил, Вын) the free-experience
// cost of raising that stat, or 0 when it is capped or unaffordable.
func (g *Game) RaiseCost(e Entity, t *resfmt.ExeTables, id int) int {
	if e.PotionTimer != 0 {
		return 0
	}
	// The class is the window title's index (+4 & 7) − 1: 0 Храбрец,
	// 1 Купец, 2 Охотник, 3 Вождь, 4 Воин; animated creatures use 1.
	class := int(e.Flags4&7) - 1
	if e.Animated() {
		class = 1
	}
	if class < 0 || class > 4 {
		return 0
	}
	stat := map[int]int{2: 0, 3: 1, 5: 2, 6: 3}
	// The game reads the high word of the dwords at +0x54, +0x58, +0x5E,
	// +0x62 (sar 0x10): the base values +0x56, +0x5A, +0x60, +0x64, not the
	// effective ones. Enchantments and potions do not count.
	value := [4]int{e.BaseStats[2], e.BaseStats[0], e.BaseStats[1], e.BaseStats[3]} // Лов, Хар, Сил, Вын
	pair := func(s, level int) (minV, capV int) {
		b := t.Bytes(0x0045B10C+class*0x68+s*0x1A+level*2, 2)
		return int(b[0]), int(b[1])
	}
	if id == 0 {
		if e.Level >= 13 {
			return 0
		}
		for s := range 4 {
			if mn, _ := pair(s, e.Level); value[s] < mn {
				return 0
			}
		}
		return 1
	}
	s, ok := stat[id]
	if !ok || e.Level < 1 {
		return 0
	}
	if _, cp := pair(s, e.Level-1); cp <= value[s] {
		return 0
	}
	col := 1
	if int(t.Bytes(0x0045AE88+class, 1)[0]) == id {
		col = 0
	}
	for row := range 13 {
		b := t.Bytes(0x0045B010+row*12, 12)
		thr := int(int32(binary.LittleEndian.Uint32(b[col*4:])))
		cost := int(int32(binary.LittleEndian.Uint32(b[8:])))
		if value[s] < thr {
			if e.FreeExp >= cost {
				return cost
			}
			return 0
		}
	}
	return 0
}

// GainExperience is 0x0041079C(e, xp): free experience += xp; entities with
// the auto-level toggle (+3 bit 0x02) then level up and buy Вын, Сил, Лов,
// Хар in turn while they can. RaiseCost reads the base stats this loop
// raises, so every purchase sees the previous one and the caps hold; the
// effective stats are recomputed only by the window redraw at the end.
func (g *Game) GainExperience(e *Entity, t *resfmt.ExeTables, xp int) {
	e.FreeExp += xp
	if e.Flags3&0x02 == 0 {
		return
	}
	for changed := true; changed; {
		changed = false
		if g.RaiseCost(*e, t, 0) != 0 {
			e.Level++
			changed = true
		}
		for _, id := range []int{6, 5, 2, 3} {
			if c := g.RaiseCost(*e, t, id); c != 0 {
				e.FreeExp -= c
				e.Experience += c
				switch id {
				case 6:
					e.BaseStats[3]++
				case 5:
					e.BaseStats[1]++
				case 2:
					e.BaseStats[2]++
				case 3:
					e.BaseStats[0]++
				}
				changed = true
			}
		}
	}
}
