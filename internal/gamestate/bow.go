package gamestate

import (
	"encoding/binary"
	"math"
	"slices"

	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

// Bows; see docs/konung_combat.md#bows-verified.

const (
	// ArrowLimit is the projectile list's size (0x0041BFF8).
	ArrowLimit = 100
	// ArrowSubSteps is the sub-steps per tick and line-of-fire range unit;
	// each sub-step moves 4 px along the major axis (32 px per tick).
	ArrowSubSteps = 8
	// arrowAimLift: the aim point is the target cell's pixel 30 px up, and
	// every point is looked up 30 px lower.
	arrowAimLift = 30
	// ArrowImage is the INTR*R.RES slot of the arrow pointing west; +2 per
	// facing, +1 on odd game ticks (0x00426F4D).
	ArrowImage = 0xB8
	// ShotStep is how many steps before the end of the shooting animation
	// the arrow leaves (count − 6).
	ShotStep = 6
)

// arrowLaunch is the per-facing offset from the shooter's cell pixel where
// arrows start (int8 pairs at 0x0045AD30), arrowSprite the draw offset of the
// arrow image (0x00458100).
var (
	arrowLaunch = [8][2]float64{{-70, -65}, {-48, -99}, {12, -85}, {68, -90}, {80, -58}, {52, -26}, {-8, -10}, {-60, -31}}
	arrowSprite = [8][2]int{{0, -3}, {0, 0}, {-3, 0}, {-29, 0}, {-32, -3}, {-29, -17}, {-3, -23}, {0, -17}}
)

// CanShoot is the planner's check (0x0041ED8C): bow active (+0x12 = 1),
// a bow equipped and arrows in the quiver.
func CanShoot(e Entity) bool {
	return e.WeaponMode == WeaponBow && e.Equipment[WeaponBow] != 0 && e.Quiver != 0
}

// HasBow is the AI's test (0x004130EE, 0x00416F65, 0x00410E84): a bow
// equipped and arrows in the quiver, whatever the active weapon; the AI then
// makes the bow active (+0x12 = 1) before trying a shot.
func HasBow(e Entity) bool { return e.Equipment[WeaponBow] != 0 && e.Quiver != 0 }

// Arrow is a flying projectile (32-byte record of 0x0082C1F8).
type Arrow struct {
	X, Y, VX, VY float64 // position and per-sub-step velocity (floats in the game)
	Shooter      int     // +0x14: entity index
	Bow          int     // +0x16: bow gear value
	Poison       int     // +0x18: the arrow item's poison charges
	Facing       int     // image: ArrowImage + 2·Facing
	Life         int     // +0x1D: ticks left (bow range)
	Special      int     // +0x1E: arrow special (0 incendiary, 1–11 slayer, -1 none)
	Type         int     // +0x1F: arrow gear type
}

// SpriteOffset is the arrow image's offset from its position.
func (a Arrow) SpriteOffset() (int, int) {
	return arrowSprite[a.Facing&7][0], arrowSprite[a.Facing&7][1]
}

// ray returns the start point and per-sub-step velocity from a shooter on
// cell from facing dir toward cell to.
func ray(from mapdata.Cell, dir int, to mapdata.Cell) (x, y, vx, vy float64) {
	sx, sy := mapdata.CellCenter(from)
	x, y = float64(sx)+arrowLaunch[dir&7][0], float64(sy)+arrowLaunch[dir&7][1]
	tx, ty := mapdata.CellCenter(to)
	dx, dy := float64(tx)-x, float64(ty-arrowAimLift)-y
	major := math.Max(math.Abs(dx), math.Abs(dy))
	k := major*0.25 + 0.001
	return x, y, dx / k, dy / k
}

// pointCell is the cell an arrow point is checked against (0x00439BDC at
// y + 30; the port uses the nearest cell centre).
func pointCell(x, y float64) mapdata.Cell {
	return mapdata.CellAt(int(x), int(y)+arrowAimLift)
}

// LineOfFire is 0x0041130C: −1 when the target is closer than 3 (grid
// distance), 1 when a ray from the shooter reaches the target's cell within
// range·8 sub-steps, 0 when it meets an arrow-blocking cell, another
// occupant first, or falls short. occupied reports cells someone stands on.
func LineOfFire(from mapdata.Cell, dir int, to mapdata.Cell, rangeCells int, blocked, occupied func(mapdata.Cell) bool) int {
	r, _ := LineOfFireBuilding(from, dir, to, rangeCells, blocked, occupied, nil)
	return r
}

// LineOfFireBuilding is 0x0041130C with a building as the target (+6 < 0):
// at an arrow-blocking cell the ray hits when building(x, y) reports an
// opaque pixel of the building's image at the point (x, y + 30)
// (0x0043F89C); the aim then moves to that cell (+0x0C/+0x0E = aim). A nil
// building is the character case: blocking cells stop the ray.
func LineOfFireBuilding(from mapdata.Cell, dir int, to mapdata.Cell, rangeCells int, blocked, occupied func(mapdata.Cell) bool, building func(x, y int) bool) (int, mapdata.Cell) {
	if Dist(to.Row, to.Col, from.Row, from.Col) < 3 {
		return -1, to
	}
	x, y, vx, vy := ray(from, dir, to)
	for range rangeCells * ArrowSubSteps {
		c := pointCell(x, y)
		switch {
		case c == to:
			return 1, to
		case blocked(c):
			if building != nil {
				if px, py := ArrowPoint(x, y); building(px, py) {
					return 1, c
				}
			}
			return 0, to
		case occupied(c):
			return 0, to
		}
		x, y = x+vx, y+vy
	}
	return 0, to
}

// ArrowPoint is the pixel an arrow point is tested at (0x00592CE4 and
// 0x00592CF0: the truncated position, 30 px lower).
func ArrowPoint(x, y float64) (int, int) { return int(x), int(y) + arrowAimLift }

// Shoot is 0x0041BFF8 at the shot step: an arrow from shooter s (on cell
// from, facing dir) toward cell to. The quiver loses one arrow (the item is
// freed at 0) and the bow 0.1 condition (destroyed at ≤ 0). ok is false
// without a bow, arrows or room in the list (n is the list's length).
func (g *Game) Shoot(s *Entity, from mapdata.Cell, dir int, to mapdata.Cell, t *resfmt.ExeTables, n int) (a Arrow, ok bool) {
	bow, okB := g.Item(s.Equipment[WeaponBow])
	ammo, okA := g.Item(s.Quiver)
	if !okB || !okA || n >= ArrowLimit {
		return Arrow{}, false
	}
	x, y, vx, vy := ray(from, dir, to)
	a = Arrow{X: x, Y: y, VX: vx, VY: vy, Shooter: s.Index, Bow: t.GearValue(bow.Type),
		Poison: g.itemWord(ammo.Index, 0xC), Facing: dir & 7, Life: t.GearRange(bow.Type),
		Special: ammo.Special, Type: ammo.Type}
	r := g.items[ammo.Index*itemStride:]
	if c := int32(binary.LittleEndian.Uint32(r[4:])) - 1; c > 0 {
		binary.LittleEndian.PutUint32(r[4:], uint32(c))
	} else {
		r[0] = 0xFF
		s.Quiver = 0
	}
	if g.wear(bow.Index, 0.1) {
		s.Equipment[WeaponBow] = 0
	}
	return a, true
}

// Fly moves an arrow through one game tick (0x00421170): life − 1, then up to
// 8 sub-steps. It returns the entity hit (or -1) and whether the arrow is
// gone (expired, blocked or hit). victim(c) returns the living occupant of
// cell c other than the shooter, or -1; dead occupants let arrows pass.
func (a *Arrow) Fly(blocked func(mapdata.Cell) bool, victim func(mapdata.Cell) int) (hit int, gone bool) {
	if a.Life--; a.Life <= 0 {
		return -1, true
	}
	for range ArrowSubSteps {
		c := pointCell(a.X, a.Y)
		if blocked(c) {
			return -1, true
		}
		if v := victim(c); v >= 0 {
			return v, true
		}
		a.X, a.Y = a.X+a.VX, a.Y+a.VY
	}
	return -1, false
}

// ArrowAttack is 0x0041ACE0: the arrow's value (a slayer special's value
// against its target kind, else the arrow's gear value) times the bow's,
// with the shooter's attack bonus (+0xC6) rule.
func (g *Game) ArrowAttack(a Arrow, shooter, victim Entity, t *resfmt.ExeTables) int {
	base := t.GearValue(a.Type)
	if a.Special > 0 {
		if v, kind := t.ItemSpecial(a.Special); kind == int(victim.Flags4) {
			base = v
		}
	}
	return bonus(shooter, 1, base*a.Bow)
}

// ArrowDamage is 0x0041C29C(victim, attack): Damage without the god mode,
// the facing condition and weapon wear; the arrow's poison is added.
func (g *Game) ArrowDamage(victim *Entity, a Arrow, attack int, t *resfmt.ExeTables, rng func(int) int) (result, damage int) {
	tr := g.Recompute(*victim, t)
	if rng(101) <= int(float64(tr.Stats[2])*0.5) {
		return StrikeDodged, 0
	}
	armour := g.Armour(*victim, t)
	result, damage = applyDamage(victim, tr, attack, armour)
	victim.Poison += a.Poison
	g.wearArmour(victim, attack, armour, t)
	return result, damage
}

// KillByArrow is the arrow's kill path: as Kill, but the shooter's
// Меткость (+0x44) grows by the experience.
func (g *Game) KillByArrow(shooter, victim *Entity, t *resfmt.ExeTables) int {
	victim.HP, victim.Flags2 = 0, 0
	xp := g.KillExperience(*victim, t)
	g.GainExperience(shooter, t, xp/4)
	shooter.Marksmanship = int(uint16(shooter.Marksmanship + xp))
	return xp
}

// ReEquip is 0x004105A0, run by the AI before a man enters the stance:
//   - an empty quiver takes the first arrows (category 12) in the bag that e
//     can use (CanUse);
//   - every empty equipment slot, in the order axe, bow, armour, helmet,
//     shield, blade, takes the first usable bag item of its category
//     (0x004104D4, which picks the first, not the best);
//   - the active weapon becomes the bow with bow and arrows, else the
//     axe/club when one is worn, else the blade.
//
// Items taken from the bag leave it, the rest shifting left (0x00421D24).
// The routine also counts one path search (0x00A3E720); the caller charges
// its budget.
func (g *Game) ReEquip(e *Entity, t *resfmt.ExeTables) {
	takeFirst := func(cat int) int {
		for _, i := range e.Bag {
			if it, ok := g.Item(i); ok && i != 0 && it.Category == cat && g.CanUse(*e, t, it) {
				bagRemove(e, i)
				return i
			}
		}
		return 0
	}
	if e.Quiver == 0 {
		e.Quiver = takeFirst(CatArrows)
	}
	for _, slot := range []int{WeaponAxe, WeaponBow, EquipArmour, EquipHelmet, EquipShield, WeaponBlade} {
		if e.Equipment[slot] == 0 {
			e.Equipment[slot] = takeFirst(slot) // categories 0–5 are the slots
		}
	}
	switch {
	case e.Quiver != 0 && e.Equipment[WeaponBow] != 0:
		e.WeaponMode = WeaponBow
	case e.Equipment[WeaponAxe] != 0:
		e.WeaponMode = WeaponAxe
	default:
		e.WeaponMode = WeaponBlade
	}
}

// bagRemove is 0x00421D24: item leaves e's bag, the items after it
// shifting left.
func bagRemove(e *Entity, item int) {
	if k := slices.Index(e.Bag[:], item); k >= 0 {
		copy(e.Bag[k:], e.Bag[k+1:])
		e.Bag[len(e.Bag)-1] = 0
	}
}
