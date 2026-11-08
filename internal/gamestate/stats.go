package gamestate

import "konung1.5/internal/resfmt"

// Derived character values, ported from konung.exe: effective stats from
// enchantments (0x0041C750), the stat getter (0x0041B244), carried weight
// (0x0041B898), armour (0x0041AC1C), attack (0x0041AF48, 0x0041ADE4) and
// the "can use" check (0x00418364).

// Recompute is 0x0041C750: it sums the enchantments of the worn gear,
// jewellery and the character's own +0xD2/+0xD3 into nine bonuses and
// writes the effective stats from their bases (Stats from BaseStats, HP
// from itself, capped at HPFull) and the armour/attack/stat-9 bonuses. An
// effect with mask 1 stacks; mask 0 replaces the stat and locks it against
// later effects. The game applies it in place; because HP is its own base
// a stat-4 bonus is added again on every call (the port's callers apply it
// to a copy).
func (g *Game) Recompute(e Entity, t *resfmt.ExeTables) Entity {
	var b [9]int
	var stack [9]bool
	for i := range stack {
		stack[i] = true
	}
	apply := func(ench int) {
		if ench < 0 {
			return
		}
		for _, fx := range t.Enchant(ench).Effects {
			s := fx.Stat - 1
			if s < 0 || s >= len(b) || !stack[s] {
				continue
			}
			if stack[s] = fx.Mask&1 != 0; stack[s] {
				b[s] += fx.Value
			} else {
				b[s] = fx.Value
			}
		}
	}
	items := append(append([]int{}, e.Equipment[:]...), e.Extra[:]...)
	for _, i := range items {
		if it, ok := g.Item(i); ok && !it.Flagged {
			apply(it.Enchant)
		}
	}
	apply(e.Enchants[0])
	apply(e.Enchants[1])
	b[3] <<= 4 // health bonus in HP units (1600 = 100)
	eff := func(base, bonus int, stacks bool) int {
		switch {
		case bonus == 0:
			return base
		case stacks:
			return base + bonus
		}
		return bonus
	}
	// Stats/BaseStats order: Хар, Сил, Лов, Вын.
	e.Stats[2] = max(0, eff(e.BaseStats[2], b[1], stack[1]))
	e.Stats[0] = max(0, eff(e.BaseStats[0], b[2], stack[2]))
	e.HP = min(HPFull, eff(e.HP, b[3], stack[3]))
	e.Stats[1] = max(0, eff(e.BaseStats[1], b[4], stack[4]))
	e.Stats[3] = max(1, eff(e.BaseStats[3], b[5], stack[5]))
	e.Bonus = [3]int{b[6], b[7], b[8]}
	e.BonusFlags = 0
	for k := range 3 {
		if stack[6+k] {
			e.BonusFlags |= 1 << k
		}
	}
	return e
}

// StatValue is the stat getter 0x0041B244 used by item and enchantment
// requirements: 1, 3 Хар; 2 Лов; 4 health (HP/16); 5 Сил; 6 Вын; 7 armour;
// 8 attack; 9 hit chance at distance 1 (0x0041B4CC(e, 1), Game.HitChance).
func (g *Game) StatValue(e Entity, t *resfmt.ExeTables, id int) int {
	switch id {
	case 1, 3:
		return e.Stats[0]
	case 2:
		return e.Stats[2]
	case 4:
		return e.HP / 16
	case 5:
		return e.Stats[1]
	case 6:
		return e.Stats[3]
	case 7:
		return g.Armour(e, t)
	case 8:
		return g.Attack(e, t, -1)
	case 9:
		return g.HitChance(e, t, 1)
	}
	return 0
}

// CarriedWeight is 0x0041B898, in grams: worn gear, jewellery, the bag
// (by category; categories 10 and above 12 weigh nothing, arrows weigh per
// piece) and the ammunition stack.
func (g *Game) CarriedWeight(e Entity, t *resfmt.ExeTables) int {
	w := 0
	for _, i := range e.Equipment {
		if it, ok := g.Item(i); ok {
			w += t.ItemWeight(resfmt.ItemTableGear, it.Type)
		}
	}
	for _, i := range e.Extra {
		if it, ok := g.Item(i); ok {
			w += t.ItemWeight(resfmt.ItemTableJewel, it.Type)
		}
	}
	for _, i := range e.Bag {
		it, ok := g.Item(i)
		if !ok {
			continue
		}
		switch c := it.Category; {
		case c <= 5:
			w += t.ItemWeight(resfmt.ItemTableGear, it.Type)
		case c <= 8:
			w += t.ItemWeight(resfmt.ItemTableJewel, it.Type)
		case c == 9:
			w += t.ItemWeight(resfmt.ItemTableHerb, it.Type)
		case c == 11:
			w += t.ItemWeight(resfmt.ItemTableMisc, it.Type)
		case c == 12:
			w += t.ItemWeight(resfmt.ItemTableGear, it.Type) * it.Count
		}
	}
	if it, ok := g.Item(e.Quiver); ok {
		w += t.ItemWeight(resfmt.ItemTableGear, it.Type) * it.Count
	}
	return w
}

// bonus applies a Recompute bonus k (0 armour, 1 attack): when non-zero it
// is added if BonusFlags bit k is set, else it replaces the value.
func bonus(e Entity, k, v int) int {
	switch {
	case e.Bonus[k] == 0:
		return v
	case e.BonusFlags&(1<<k) != 0:
		return v + e.Bonus[k]
	}
	return e.Bonus[k]
}

// Armour is 0x0041AC1C: +0x4E plus the armour, helmet and shield values.
func (g *Game) Armour(e Entity, t *resfmt.ExeTables) int {
	e = g.Recompute(e, t)
	a := e.ArmourBase
	for _, slot := range []int{EquipArmour, EquipHelmet, EquipShield} {
		if it, ok := g.Item(e.Equipment[slot]); ok {
			a += t.GearValue(it.Type)
		}
	}
	return bonus(e, 0, a)
}

// strengthBonus is trunc(Сил·f·HP·0.0625) as the game computes it in
// doubles (f = 0.04 armed, 0.02 unarmed or with the bow).
func strengthBonus(e Entity, f float64) int {
	return int(float64(e.Stats[1]) * f * float64(e.HP) * 0.0625)
}

// Attack is 0x0041AF48(e, kind): creatures (animated models) hit with
// strength and health alone. Others use the active weapon (axe or blade)
// plus a strength bonus; a weapon's special property replaces its value
// against targets of its kind (entity +4). Unarmed or with the bow, only a
// halved strength bonus counts.
func (g *Game) Attack(e Entity, t *resfmt.ExeTables, kind int) int {
	if e.Animated() {
		return strengthBonus(e, 0.04)
	}
	e = g.Recompute(e, t)
	m := e.WeaponMode
	if m != WeaponAxe && m != WeaponBlade || e.Equipment[m] == 0 {
		return bonus(e, 1, strengthBonus(e, 0.02))
	}
	it, _ := g.Item(e.Equipment[m])
	base := t.GearValue(it.Type)
	if it.Special >= 0 {
		if v, k := t.ItemSpecial(it.Special); k == kind {
			base = v
		}
	}
	return bonus(e, 1, base+strengthBonus(e, 0.04))
}

// SheetDamage is «Удар» (0x0042F0C7): with the bow drawn and arrows in the
// quiver, bow value × arrow value (0x0041ADE4); otherwise Attack(e, 0).
func (g *Game) SheetDamage(e Entity, t *resfmt.ExeTables) int {
	if e.Quiver != 0 && e.WeaponMode == WeaponBow && e.Equipment[WeaponBow] != 0 {
		bow, _ := g.Item(e.Equipment[WeaponBow])
		ammo, _ := g.Item(e.Quiver)
		return bonus(e, 1, t.GearValue(bow.Type)*t.GearValue(ammo.Type))
	}
	return g.Attack(e, t, 0)
}

// CanUse is 0x00418364: in god mode (Game.GodMode) everything is usable.
// Otherwise gear and jewellery must meet their enchantment's requirement,
// gear also its own (stat id and minimum from the type record). Everything
// else is usable.
func (g *Game) CanUse(e Entity, t *resfmt.ExeTables, it Item) bool {
	if g.GodMode {
		return true
	}
	e = g.Recompute(e, t)
	tab := it.Table()
	if tab != resfmt.ItemTableGear && tab != resfmt.ItemTableJewel {
		return true
	}
	if it.Enchant >= 0 {
		if en := t.Enchant(it.Enchant); en.ReqMin != 0 && g.StatValue(e, t, en.ReqStat) < en.ReqMin {
			return false
		}
	}
	if tab == resfmt.ItemTableGear {
		if stat, min := t.GearRequirement(it.Type); min != 0 && g.StatValue(e, t, stat) < min {
			return false
		}
	}
	return true
}

// CellTint returns the palette tint of a bag cell holding it (0x0042C914):
// red 24 if e can't use it, red+green 15 if flagged, green 12 if enchanted.
func (g *Game) CellTint(e Entity, t *resfmt.ExeTables, it Item) (r, gr, b int, ok bool) {
	switch {
	case !g.CanUse(e, t, it):
		return 24, 0, 0, true
	case it.Flagged:
		return 15, 15, 0, true
	case it.Enchant >= 0:
		return 0, 12, 0, true
	}
	return 0, 0, 0, false
}
