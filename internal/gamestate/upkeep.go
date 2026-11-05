package gamestate

import "konung1.5/internal/resfmt"

// Upkeep and respawn; see docs/konung_combat.md#upkeep-poison-and-respawn-verified.

// UpkeepTicks is the period of the upkeep pass 0x0041D0FC (time & 0xF == 0).
const UpkeepTicks = 16

// Upkeep is one pass of 0x0041D0FC over every active group on every map:
// creature respawn counters (0x0041BDF0), poison (HP −= +0x50; the poison
// stays) and potion timers (+0xCE). skip(i) reports members not to touch
// (dying, already dead in the scene). A member on map current whose poison
// killed it gets HP 0 and +2 = 0 and is passed to died, which starts its death
// action and drops its loot; elsewhere it just stays at HP 0. There is no
// HP regeneration.
func (g *Game) Upkeep(t *resfmt.ExeTables, current int, skip func(i int) bool, died func(i int)) {
	for gi := range g.Groups {
		gr := &g.Groups[gi]
		if gr.Kind == 0 && gi != 0 {
			continue
		}
		if gr.CreatureGroup() && gr.Kind <= 0x50 {
			if gr.Target < gr.MaxPop {
				if gr.Countdown--; gr.Countdown < 0 {
					_, period := t.RespawnRule(int(gr.Kind))
					gr.Target++
					gr.Countdown += period
				}
			}
			if gr.Target > gr.Count {
				gr.Debt--
			}
		}
		for k := 0; k < gr.Count; k++ {
			i := gr.First + k
			if i < 0 || i >= len(g.Entities) || g.Entities[i].Dead() || skip(i) {
				continue
			}
			e := &g.Entities[i]
			if e.Poison != 0 {
				e.HP -= e.Poison
				if e.HP <= 0 {
					e.HP, e.Flags2 = 0, 0
					if gr.Map == current {
						died(i)
					}
				}
			}
			if e.PotionTimer != 0 {
				if e.PotionTimer--; e.PotionTimer == 0 {
					e.HitBonus = 0
					e.BaseStats[2], e.BaseStats[0], e.BaseStats[1], e.BaseStats[3] =
						e.SavedBase[0], e.SavedBase[1], e.SavedBase[2], e.SavedBase[3]
				}
			}
		}
	}
}

// EnterMap is the map loader's group pass (0x0043C612, 0x0043C878) for map
// m: creature groups calm down; every member of a non-village group with
// kind < 0x51 gets a random cell in its territory (PlaceInTerritory; the
// group is cut where that fails); then creature groups with negative debt
// respawn members (NewMember) up to their target, paying the kind's cost
// each, and leftover debt is cleared. The first eligible group on the map is
// skipped, as the game's loop does. blocked reports walls and cells already
// taken; placed cells are added to it through take.
func (g *Game) EnterMap(m int, t *resfmt.ExeTables, rng func(int) int, blocked func(r, c int) bool, take func(r, c int)) {
	for gi := 1; gi < len(g.Groups); gi++ {
		gr := &g.Groups[gi]
		if gr.Kind == 0 || gr.Map != m {
			continue
		}
		if gr.CreatureGroup() {
			gr.Alarm = AlarmCalm
		}
		if gr.Kind&0x80 != 0 || int8(gr.Kind) >= 0x51 {
			continue
		}
		for k := 0; k < gr.Count; k++ {
			e := &g.Entities[gr.First+k]
			if !g.PlaceInTerritory(e, *gr, rng, blocked) {
				gr.Count = k
				break
			}
			take(e.Row, e.Col)
		}
	}
	skipFirst := true
	for gi := 1; gi < len(g.Groups); gi++ {
		gr := &g.Groups[gi]
		if !gr.CreatureGroup() || gr.Map != m || gr.Kind > 0x50 {
			continue
		}
		if skipFirst {
			skipFirst = false
			continue
		}
		cost, _ := t.RespawnRule(int(gr.Kind))
		for gr.Debt < 0 && gr.Count < gr.Target {
			e := g.NewMember(gi, rng, blocked)
			if e == nil {
				break
			}
			take(e.Row, e.Col)
			gr.Debt += cost
		}
		gr.Debt = max(gr.Debt, 0)
	}
}

// PlaceInTerritory is 0x00411DE4: up to 100 random cells around the
// territory's centre (± rand()%(half span) per axis, random sign; no offset
// when the half span is below 2) until one is free.
func (g *Game) PlaceInTerritory(e *Entity, gr Group, rng func(int) int, blocked func(r, c int) bool) bool {
	a, _ := gr.WanderArea()
	halfR, halfC := (a.RowMax-a.RowMin)/2, (a.ColMax-a.ColMin)/2
	cr, cc := (a.RowMin+a.RowMax+1)/2, (a.ColMin+a.ColMax+1)/2
	off := func(half int) int {
		if half < 2 {
			return 0
		}
		d := rng(half)
		if rng(2) != 0 {
			return d
		}
		return -d
	}
	for range 100 {
		e.Row, e.Col = cr+off(halfR), cc+off(halfC)
		if e.Row >= 0 && e.Col >= 0 && !blocked(e.Row, e.Col) {
			return true
		}
	}
	return false
}

// NewMember is 0x00420CCC: a new member at slot first+count with the
// group's average Лов, Хар, Сил, Вын, armour, poison, level, gold and walk
// throttle (member 0's values when count ≤ 1), a random member's model and
// palette, full HP, empty gear and bag, the stance, a random facing and the
// dormant pose for kinds 0x46/0x4C/0x55; placed with PlaceInTerritory. Nil
// when the group has no room or no free cell. Fields the game leaves
// stale in the slot (skills, experience, enchantments) start empty here.
func (g *Game) NewMember(gi int, rng func(int) int, blocked func(r, c int) bool) *Entity {
	gr := &g.Groups[gi]
	slot := gr.First + gr.Count
	if gr.Count >= gr.MaxPop || slot >= len(g.Entities) || gr.Count == 0 {
		return nil
	}
	var sum [9]int
	n := gr.Count // integer averages over every member, the dead included
	for k := 0; k < n; k++ {
		m := g.Entities[gr.First+k]
		for j, v := range [9]int{m.Stats[2], m.Stats[0], m.Stats[1], m.Stats[3], m.ArmourBase, m.PoisonDealt, m.Level, m.Gold, m.Throttle} {
			sum[j] += v
		}
	}
	for j := range sum {
		sum[j] /= n
	}
	donor := g.Entities[gr.First]
	if gr.Count >= 2 {
		donor = g.Entities[gr.First+rng(gr.Count)]
	}
	e := &g.Entities[slot]
	*e = Entity{Index: slot, Group: gi, Flags4: gr.Kind, Flags2: 0x40, Flags3: 0x04, HP: HPFull,
		Variant: rng(8), Model: donor.Model, PaletteOffset: donor.PaletteOffset, Dialogue: NoDialogue,
		Stats:      [4]int{sum[1], sum[2], sum[0], sum[3]},
		BaseStats:  [4]int{sum[1], sum[2], sum[0], sum[3]},
		ArmourBase: sum[4], PoisonDealt: sum[5], Level: sum[6], Gold: sum[7], Throttle: sum[8],
		Enchants: [2]int{-1, -1}}
	if TacticOf(gr.Kind) == TacticDormant {
		e.Type = ActShoot
	}
	if gr.Kind == 0x4C {
		e.WeaponMode = rng(5) + 1
	}
	if !g.PlaceInTerritory(e, *gr, rng, blocked) {
		return nil
	}
	gr.Count++
	return e
}
