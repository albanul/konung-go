package gamestate

import (
	"encoding/binary"
	"math"

	"konung1.5/internal/resfmt"
)

// Melee combat rules, traced from the party AI's attack case (action type 5,
// 0x00421E8C) and the routines it calls; see docs/konung_combat.md.

// Action types (entity +0x00) relative to the animation set's base: 0 in the
// combat stance, 6 otherwise (jump table 0x00422654).
const (
	ActIdle   = 0
	ActWalk   = 1
	ActFlinch = 2 // hit reaction
	ActDeath  = 3
	ActShoot  = 4
	ActAttack = 5 // melee swing; always type 5, the swing enters the stance
)

// Strike results, the return values of 0x0041C480 plus a miss.
const (
	StrikeMiss   = -3 // 0x0041B100 returned 0: the roll was above the hit chance
	StrikeDodged = -2 // dodged (or the god-mode hero)
	StrikeLight  = -1 // damage below 0x30: no hit reaction
	StrikeHeavy  = 0  // damage of 0x30 or more: the target plays ActFlinch
	StrikeKill   = 1  // HP would drop to 0 or below: the target dies
)

// HeavyHit is the damage from which the target flinches (0x0041C59A).
const HeavyHit = 0x30

// HitChance is 0x0041B4CC(e, distance), a percentage compared with
// rand()%101. Creatures: 100 − (100 − Лов·0.5)·0.5. Others, in melee
// (distance 0) or without a bow drawn: 100 − (100 − Лов·0.5) /
// (Фехтование·0.00015 + 1.5). With the bow drawn and distance > 0:
// 100 − (distance/range + 1)·0.7·(100 − Лов·0.5) / (Меткость·0.00015 + 1.5),
// range = the bow's gear +0x34. Timed (+0xCA while +0xCE ≠ 0) and stat-9
// (+0xC8) bonuses apply last. All steps truncate (0x0043FE4C).
func (g *Game) HitChance(e Entity, t *resfmt.ExeTables, distance int) int {
	dex := float64(e.Stats[2])
	if e.Animated() {
		return int(100 - (100-dex*0.5)*0.5)
	}
	e = g.Recompute(e, t)
	dex = float64(e.Stats[2])
	var hit int
	if bow, ok := g.Item(e.Equipment[WeaponBow]); ok && e.WeaponMode == WeaponBow && distance != 0 {
		r := float64(t.GearRange(bow.Type))
		hit = int(100 - (float64(distance)/r+1)*0.7*(100-dex*0.5)/(float64(e.Marksmanship)*0.00015+1.5))
	} else {
		hit = int(100 - (100-dex*0.5)/(float64(e.Fencing)*0.00015+1.5))
	}
	if e.HitBonusOn != 0 {
		hit += e.HitBonus
	}
	return bonus(e, 2, hit)
}

// Strike is one melee blow at the end of the swing (0x00421FEC): the hit
// roll 0x0041B100 (rand()%101 > HitChance misses), then the attack value
// 0x0041AF48 against the target's kind (+4) and the damage routine
// 0x0041C480. rng(n) must return a value in [0, n). It returns a Strike*
// result and the damage dealt (0 unless StrikeLight or StrikeHeavy).
func (g *Game) Strike(attacker, target *Entity, t *resfmt.ExeTables, rng func(int) int) (result, damage int) {
	if rng(101) > g.HitChance(*attacker, t, 0) {
		return StrikeMiss, 0
	}
	atk := g.Attack(*attacker, t, int(target.Flags4))
	return g.Damage(target, attacker, t, atk, rng)
}

// Damage is 0x0041C480(target, attacker, attack):
//   - the target dodges when the facing table (resfmt.FacingDodge) allows it
//     and rand()%101 ≤ trunc(Лов·0.5);
//   - x = max(0, attack·0.7 − Armour), stored as a float;
//     damage = trunc(((attack·0.3 + x) / Вын + 1)·16);
//   - HP ≤ damage kills (the caller's kill path sets HP to 0); otherwise HP
//     drops by the damage;
//   - then the target's poison +0x50 grows by the attacker's poison, the
//     worn armour, helmet and shield wear by gear value·0.001·min(1,
//     attack/Armour), and the attacker's melee weapon loses one poison charge
//     (+0xC) and attack·0.00025 condition. Worn-out items are destroyed.
//
// The god-mode check (0x00A3E768 with the hero as target) is not ported.
func (g *Game) Damage(target, attacker *Entity, t *resfmt.ExeTables, attack int, rng func(int) int) (result, damage int) {
	tr := g.Recompute(*target, t)
	if t.FacingDodge(target.Variant, attacker.Variant) {
		if rng(101) <= int(float64(tr.Stats[2])*0.5) {
			return StrikeDodged, 0
		}
	}
	armour := g.Armour(*target, t)
	x := float32(float64(attack)*0.7 - float64(armour))
	if x < 0 {
		x = 0
	}
	dmg := truncInt(((float64(attack)*0.3+float64(x))/float64(tr.Stats[3]) + 1) * 16)
	if target.HP <= dmg {
		result = StrikeKill
	} else {
		target.HP -= dmg
		damage = dmg
		result = StrikeLight
		if dmg >= HeavyHit {
			result = StrikeHeavy
		}
	}
	f := float32(1)
	if armour != 0 {
		if r := float32(attack) / float32(armour); r <= 1 {
			f = r
		}
	}
	target.Poison += g.poisonOf(*attacker)
	for _, slot := range []int{EquipArmour, EquipHelmet, EquipShield} {
		if it, ok := g.Item(target.Equipment[slot]); ok {
			if g.wear(it.Index, float64(t.GearValue(it.Type))*0.001*float64(f)) {
				target.Equipment[slot] = 0
			}
		}
	}
	if m := attacker.WeaponMode; !attacker.Animated() && (m == WeaponAxe || m == WeaponBlade) && attacker.Equipment[m] != 0 {
		i := attacker.Equipment[m]
		if c := g.itemWord(i, 0xC); c != 0 {
			g.setItemWord(i, 0xC, c-1)
		}
		if g.wear(i, float64(attack)*0.00025) {
			attacker.Equipment[m] = 0
		}
	}
	return result, damage
}

// poisonOf is the global 0x00A3E828 that 0x0041AF48 leaves behind: a
// creature's +0x52, the active melee weapon's poison charges (+0xC), or 0.
func (g *Game) poisonOf(e Entity) int {
	if e.Animated() {
		return e.PoisonDealt
	}
	if m := e.WeaponMode; (m == WeaponAxe || m == WeaponBlade) && e.Equipment[m] != 0 {
		return g.itemWord(e.Equipment[m], 0xC)
	}
	return 0
}

// KillExperience is 0x0041B70C(victim): creatures trunc(Лов·0.2 + Сил·0.3 +
// Вын·0.3 + Armour·0.1); others add Фехтование·0.002 + Меткость·0.003 to the
// sum, and with a melee weapon drawn trunc(sum + its gear value·0.2).
func (g *Game) KillExperience(v Entity, t *resfmt.ExeTables) int {
	armour := float64(g.Armour(v, t))
	if v.Animated() {
		return int(armour*0.1 + (float64(v.Stats[2])*0.2 + float64(v.Stats[1])*0.3 + float64(v.Stats[3])*0.3))
	}
	r := g.Recompute(v, t)
	sum := float64(r.Stats[2])*0.2 + float64(r.Stats[1])*0.3 + float64(r.Stats[3])*0.3 +
		float64(r.Fencing)*0.002 + float64(r.Marksmanship)*0.003
	xp := int(armour*0.1 + sum)
	if m := r.WeaponMode; (m == WeaponAxe || m == WeaponBlade) && r.Equipment[m] != 0 {
		it, _ := g.Item(r.Equipment[m])
		xp = int(float64(t.GearValue(it.Type))*0.2 + float64(xp))
	}
	return xp
}

// Kill applies the kill path of the attack case (0x00422082): the victim's
// HP becomes 0 and its action state 0; the killer gains xp/4 free experience
// (0x0041079C, which also raises stats for entities with +3 bit 0x02 — not
// ported) and xp on Фехтование (+0x40). It returns xp.
func (g *Game) Kill(killer, victim *Entity, t *resfmt.ExeTables) int {
	victim.HP = 0
	victim.Flags2 = 0
	xp := g.KillExperience(*victim, t)
	killer.FreeExp += xp / 4
	killer.Fencing = int(uint16(killer.Fencing + xp))
	return xp
}

// wear subtracts amount from item i's condition (float at +4) and destroys
// the item (category 0xFF) when it reaches 0. It reports destruction.
func (g *Game) wear(i int, amount float64) bool {
	r := g.items[i*itemStride:]
	c := float32(float64(math.Float32frombits(binary.LittleEndian.Uint32(r[4:]))) - amount)
	binary.LittleEndian.PutUint32(r[4:], math.Float32bits(c))
	if c <= 0 {
		r[0] = 0xFF
		return true
	}
	return false
}

func (g *Game) itemWord(i, off int) int {
	return int(binary.LittleEndian.Uint16(g.items[i*itemStride+off:]))
}

func (g *Game) setItemWord(i, off, v int) {
	binary.LittleEndian.PutUint16(g.items[i*itemStride+off:], uint16(v))
}

// truncInt converts like fistp after the truncating rounding mode
// (0x0043FE4C); out-of-range values (a 0 endurance) give math.MinInt32.
func truncInt(f float64) int {
	if math.IsNaN(f) || f >= math.MaxInt32+1 || f < math.MinInt32 {
		return math.MinInt32
	}
	return int(f)
}
