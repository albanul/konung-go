package gamestate

import (
	"encoding/binary"
	"math"
	"slices"

	"konung1.5/internal/resfmt"
)

// Herbs and potions (category 9): mixing (0x0041BC24) and use (0x0041DB64).
// See docs/konung_potions.md.

// Herb types: records of the herb table 0x004594B0. Types 0–3 are
// ingredients and can't be used on their own; 4–13 are potions with a
// concentration (float at item +4).
const (
	HerbSting       = iota // «Ядовитое жало»
	HerbNut                // «Земляной орех»
	HerbRoot               // «Белый корень»
	HerbJar                // «Пустая банка»
	HerbMess               // «Непонятная смесь»: a failed mix
	HerbBalm               // «Лечебный бальзам»
	HerbPoison             // «Яд»
	HerbOil                // «Масло»
	HerbAntidote           // «Противоядие»
	HerbBraga              // «Брага»
	HerbPotion             // «Зелье»
	HerbTear               // «Чистая слеза»
	HerbWisdom             // «Элексир Мудрости»
	HerbLivingWater        // «Живая Вода»
)

const (
	maxHealing       = 500 // cap of the healing skill +0x48 (0x0041BDC3)
	maxConcentration = 15  // 0x00450134
	potionPass       = 30  // timer +0xCE per unit of a potion, in upkeep passes
)

// Concentration is item i's float at +4 (herbs and potions).
func (g *Game) Concentration(i int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(g.items[i*itemStride+4:]))
}

func (g *Game) setConcentration(i int, c float32) {
	binary.LittleEndian.PutUint32(g.items[i*itemStride+4:], math.Float32bits(c))
}

// Mix is 0x0041BC24(e, target, held): the herb held in the hand is poured
// onto the herb target. The first recipe (resfmt.Recipes, 0x00450E54)
// matching both types sets the target's concentration, turns the target and
// the held herb into the recipe's results (a held result of 0xFF frees it)
// and trains e's healing skill (+0x48, at most 500). Without a recipe the
// target becomes «Непонятная смесь» and the held herb is used up.
func (g *Game) Mix(e *Entity, t *resfmt.ExeTables, target, held int) {
	tr, hr := g.items[target*itemStride:], g.items[held*itemStride:]
	for _, r := range t.Recipes() {
		if int(tr[3]) != r.Target || int(hr[3]) != r.Held {
			continue
		}
		skill := float64(uint16(e.Skills[0])) * 0.01
		c, h := g.Concentration(target), g.Concentration(held)
		switch r.Mode {
		case resfmt.RecipeSet:
			c = float32(skill)
		case resfmt.RecipeAddSkill:
			c = float32(skill + float64(c))
		case resfmt.RecipeAdd:
			c = float32(float64(h) + float64(c))
		case resfmt.RecipeMin:
			if h < c {
				c = h
			}
		case resfmt.RecipeHeld:
			c = float32(skill + float64(h))
		}
		if c > maxConcentration {
			c = maxConcentration
		}
		g.setConcentration(target, c)
		tr[3], hr[3] = byte(r.NewTarget), byte(r.NewHeld)
		if int8(hr[3]) < 0 {
			hr[0] = 0xFF
		}
		e.Skills[0] = int(min(uint16(e.Skills[0]+r.Skill), maxHealing))
		return
	}
	tr[3], hr[0] = HerbMess, 0xFF
}

// UseHerb is 0x0041DB64(e, target, item): use herb item on character e, or
// on item target (the in-hand box) when target ≠ 0. A herb target means
// mixing (Mix). Otherwise the potion's type decides (jump table
// 0x0041E110), with f its concentration and n = trunc(f):
//
//   - «Непонятная смесь»: HP halved (at least 1).
//   - «Лечебный бальзам»: HP += f·160 (at most 1600).
//   - «Яд»: on e, poison +0x50 += f; on a blade, axe or arrows (categories
//     0, 2, 12), their poisoning +0xC += f; other targets refuse.
//   - «Масло»: only on arrows with f ≥ 10: special property +2 = 0.
//   - «Противоядие»: poison −= f·10, not below 0.
//   - «Брага»: n = min(Лов/3, f); refused while a potion timer runs. Base
//     Лов −3n, Хар, Сил and Вын +n for 30n upkeep passes.
//   - «Зелье»: refused while a timer runs. Base Лов, Сил, Вын +n for 30n
//     passes and HP += f·160.
//   - «Чистая слеза»: timer 30n, hit bonus +0xCA = trunc(√n·0.1), and the
//     visible piles glow until the next map load (Game.Lit, 0x00A3E758).
//   - «Элексир Мудрости»: refused below n = 6. Free experience
//     trunc(√(n−5)·100); base Сил −5n (at least 1) and HP −160n (at least
//     160) for 60n passes.
//   - «Живая Вода»: refused below f = 10; HP = 1600 (and a dead e is revived
//     by 0x00414B3C, not ported: the port's party has no dead members).
//
// Starting a timer first saves the base stats for Upkeep to restore (when
// no timer runs). A used potion becomes «Пустая банка». With a target, the
// held herb (if not used up) then goes to the front of e's bag. Ingredients
// (types 0–3) and refusals return false and change nothing.
func (g *Game) UseHerb(e *Entity, t *resfmt.ExeTables, target, item int) bool {
	if tgt, ok := g.Item(target); ok && tgt.Category == CatHerb {
		g.Mix(e, t, target, item)
	} else if !g.drink(e, t, target, item) {
		return false
	}
	if it, ok := g.Item(item); target != 0 && ok && int8(it.Category) >= 0 {
		BagInsert(e, item)
	}
	g.sound(SoundDrink) // 0x0041E19D, after mixing and drinking alike
	return true
}

func (g *Game) drink(e *Entity, t *resfmt.ExeTables, target, item int) bool {
	it, ok := g.Item(item)
	if !ok || it.Type < HerbMess || it.Type > HerbLivingWater {
		return false
	}
	f := float64(g.Concentration(item))
	n := truncInt(f)
	heal := func(add float64) { e.HP = min(truncInt(add+float64(e.HP)), HPFull) }
	save := func() {
		if e.PotionTimer == 0 {
			e.SavedBase = [4]int{e.BaseStats[2], e.BaseStats[0], e.BaseStats[1], e.BaseStats[3]}
		}
	}
	switch it.Type {
	case HerbMess:
		if e.HP /= 2; e.HP == 0 {
			e.HP = 1
		}
	case HerbBalm:
		heal(f * 160)
	case HerbPoison:
		if target == 0 {
			e.Poison = int(int16(truncInt(float64(float32(float32(e.Poison) + float32(f))))))
			break
		}
		tgt, _ := g.Item(target)
		if tgt.Category != CatAxe && tgt.Category != CatBlade && tgt.Category != CatArrows {
			return false
		}
		p := int16(g.itemWord(target, 0xC))
		g.setItemWord(target, 0xC, truncInt(float64(float32(float32(p)+float32(f)))))
	case HerbOil:
		tgt, _ := g.Item(target)
		if target == 0 || f < 10 || tgt.Category != CatArrows {
			return false
		}
		g.items[target*itemStride+2] = 0
	case HerbAntidote:
		if e.Poison != 0 {
			e.Poison = max(0, truncInt(float64(e.Poison)-f*10))
		}
	case HerbBraga:
		n = g.Recompute(*e, t).Stats[2] / 3
		if float64(n) > f {
			n = truncInt(f)
		}
		if e.PotionTimer != 0 {
			return false
		}
		save()
		e.PotionTimer, e.HitBonus = n*potionPass, 0
		e.BaseStats[2] -= 3 * n
		e.BaseStats[0] += n
		e.BaseStats[1] += n
		e.BaseStats[3] += n
	case HerbPotion:
		if e.PotionTimer != 0 {
			return false
		}
		save()
		e.PotionTimer, e.HitBonus = n*potionPass, 0
		e.BaseStats[2] += n
		e.BaseStats[1] += n
		e.BaseStats[3] += n
		heal(f * 160)
	case HerbTear:
		g.Lit = true
		save()
		e.PotionTimer = n * potionPass
		e.HitBonus = truncInt(math.Sqrt(float64(n)) * 0.1)
	case HerbWisdom:
		if n < 6 {
			return false
		}
		g.GainExperience(e, t, truncInt(math.Sqrt(float64(n-5))*100))
		save()
		e.PotionTimer, e.HitBonus = n*2*potionPass, 0
		if e.BaseStats[1] -= 5 * n; e.BaseStats[1] < 1 {
			e.BaseStats[1] = 1
		}
		if e.HP -= 160 * n; e.HP < 160 {
			e.HP = 160
		}
	case HerbLivingWater:
		if f < 10 {
			return false
		}
		e.HP = HPFull
	}
	g.items[item*itemStride+3] = HerbJar
	return true
}

// torchType is the misc item «Факел» (misc table 0x0046053C).
const torchType = 31

// UseMisc is the part of 0x00414E64(e, target, item) the port has:
//   - case 31, the torch, sets Game.Lit (0x004156E6);
//   - case 20, «Шар магического прыжка», hides the bag strip and opens the
//     map window (0x004153B0, Game.OpenMap);
//   - case 0, «Потерянная карта», marks a random location known
//     (0x00414E95, RevealMap) and is used up (item +0 = 0xFF).
//
// The common tail 0x00415BD3 then puts a kept item with a target at the
// front of e's bag, removes a used-up one from e's bag (0x00421D24) and
// plays sound 10 (0x00415C6C; case 20 has already played it once,
// 0x004153CC). Other misc items return false (the shovel's case is handled
// by UseBagItem).
func (g *Game) UseMisc(e *Entity, target, item int) bool {
	it, ok := g.Item(item)
	if !ok || it.Category != 11 {
		return false
	}
	switch it.Type {
	case torchType:
		g.Lit = true
	case jumpOrbType:
		g.OpenMap = true
	case lostMapType:
		g.RevealMap()
		g.items[item*itemStride] = 0xFF
		if k := slices.Index(e.Bag[:], item); k >= 0 {
			copy(e.Bag[k:], e.Bag[k+1:])
			e.Bag[len(e.Bag)-1] = 0
		}
		g.sound(SoundUse)
		return true
	default:
		return false
	}
	if target != 0 {
		BagInsert(e, item)
	}
	g.sound(SoundUse)
	return true
}
