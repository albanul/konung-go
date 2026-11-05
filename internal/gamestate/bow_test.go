package gamestate

import (
	"encoding/binary"
	"math"
	"testing"

	"konung1.5/internal/mapdata"
)

// giveBow equips e with a hunting bow (gear type 6, value 64, range 12) and
// a quiver of n arrows (type 30, value 1).
func giveBow(g *Game, e *Entity, n int) {
	bow, ammo := g.newItem(), 0
	g.setItem(bow, CatBow, 6)
	binary.LittleEndian.PutUint32(g.items[bow*itemStride+4:], math.Float32bits(100))
	ammo = g.newItem()
	g.setItem(ammo, CatArrows, 30)
	binary.LittleEndian.PutUint32(g.items[ammo*itemStride+4:], uint32(n))
	e.Equipment[WeaponBow], e.Quiver, e.WeaponMode = bow, ammo, WeaponBow
}

func TestBowShot(t *testing.T) {
	g, tb := game0(t)
	volk := g.Entities[g.Groups[0].First]
	giveBow(g, &volk, 5)
	if !CanShoot(volk) || tb.GearRange(6) != 12 || tb.GearValue(6) != 64 {
		t.Fatalf("bow: can shoot %v, range %d, value %d", CanShoot(volk), tb.GearRange(6), tb.GearValue(6))
	}
	from, to := mapdata.Cell{Row: 100, Col: 40}, mapdata.Cell{Row: 100, Col: 46} // 6 cells east
	dir := int(mapdata.DirectionTo(0, 0, 1, 0))
	free := func(mapdata.Cell) bool { return false }
	if r := LineOfFire(from, dir, to, 12, free, free); r != 1 {
		t.Fatalf("clear shot east: %d, want 1", r)
	}
	if r := LineOfFire(from, dir, mapdata.Cell{Row: 100, Col: 42}, 12, free, free); r != -1 {
		t.Errorf("2 cells away: %d, want -1 (too close)", r)
	}
	between := func(c mapdata.Cell) bool { return c.Col == 43 } // someone on the arrow's way
	if r := LineOfFire(from, dir, to, 12, free, between); r != 0 {
		t.Errorf("someone in between: %d, want 0", r)
	}
	a, ok := g.Shoot(&volk, from, dir, to, tb, 0)
	if !ok || a.Bow != 64 || a.Life != 12 || a.Shooter != volk.Index {
		t.Fatalf("shot %+v ok %v", a, ok)
	}
	if c := binary.LittleEndian.Uint32(g.items[volk.Quiver*itemStride+4:]); c != 4 {
		t.Errorf("quiver holds %d arrows, want 4", c)
	}
	hit := -1
	for range 20 {
		h, gone := a.Fly(free, func(c mapdata.Cell) int {
			if c == to {
				return 921
			}
			return -1
		})
		if gone {
			hit = h
			break
		}
	}
	if hit != 921 {
		t.Fatalf("arrow hit %d, want the ant 921 on %v", hit, to)
	}
	ant := g.Entities[921]
	atk := g.ArrowAttack(a, volk, ant, tb)
	if atk != 64 { // arrow 1 × bow 64
		t.Errorf("arrow attack %d, want 64", atk)
	}
	// x = float32(44.8 − 10) = 34.799999; trunc(((19.2 + x)/3 + 1)·16) = 303.
	if r, d := g.ArrowDamage(&ant, a, atk, tb, func(n int) int { return n - 1 }); r != StrikeHeavy || d != 303 {
		t.Errorf("arrow damage %d, result %d; want 303, heavy", d, r)
	}
}
