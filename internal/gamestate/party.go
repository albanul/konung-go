package gamestate

import (
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

// Party play: the selection list, following the leader, the placement after
// a map change and revival. See docs/konung_party.md.

// MaxSelection is the size of the selection list 0x0083B93C.
const MaxSelection = 10

// Party intent bits (entity +2) set by the orders (0x00422A3A, 0x00423C1D,
// 0x00423CC2, 0x004226C0) and read by the planner 0x0041ED8C.
const (
	IntentMelee  = 0x01 // go for +6 (or the nearest enemy)
	IntentStrike = 0x02 // retaliating; also set on a walk order for members other than the hero
	IntentShoot  = 0x04 // shoot +6
	IntentWalk   = 0x08 // walk to +0x0C/+0x0E
	IntentFollow = 0x20 // follow the leader (0x0041F5D8 → 0x0041FA5D)
	IntentIdle   = 0x40 // the AI may decide
	// IntentGather is «Все ко мне» (0x00422762): idle, following.
	IntentGather = IntentIdle | IntentFollow
)

// FollowDistance is the grid distance (Dist) from the leader at which a
// following member walks back to it (0x0041FAAE).
const FollowDistance = 10

// gridRows and gridCols bound the cells the formation routines accept
// (strictly inside: 0 < row < 0x13F, 0 < col < 0x9F).
const gridRows, gridCols = 0x13F, 0x9F

func insideGrid(r, c int) bool { return r > 0 && r < gridRows && c > 0 && c < gridCols }

// Select is 0x00425084: without add (Shift up) entity i becomes the whole
// selection; with add it is appended to the first free slot.
func Select(sel []int, i int, add bool) []int {
	if !add {
		return []int{i}
	}
	if len(sel) >= MaxSelection {
		return sel
	}
	return append(sel, i)
}

// Deselect is 0x00439230: entity i leaves the selection; the entries after
// it move up.
func Deselect(sel []int, i int) []int {
	for k, v := range sel {
		if v == i {
			return append(sel[:k:k], sel[k+1:]...)
		}
	}
	return sel
}

// Selected is 0x00411708: entity i is in the selection.
func Selected(sel []int, i int) bool {
	for _, v := range sel {
		if v == i {
			return true
		}
	}
	return false
}

// FormationDir is 0x00439AA4(r1, c1, r2, c2), the direction index the
// formation table is read with: from cell 1 toward cell 2, with
// dr = r2 − r1 and dc = c2 − c1:
//   - |dr| ≤ |dc|/2: 0 when c2 > c1, else 4;
//   - r2 < r1: 6 when |dc| ≤ |dr|/4, 5 when c2 < c1, else 7;
//   - otherwise: 2 when |dc| ≤ |dr|/4, 3 when c2 < c1, else 1.
func FormationDir(r1, c1, r2, c2 int) int {
	dr, dc := abs(r2-r1), abs(c2-c1)
	switch {
	case dr <= dc/2:
		if c2 > c1 {
			return 0
		}
		return 4
	case r2 < r1:
		switch {
		case dc <= dr/4:
			return 6
		case c2 < c1:
			return 5
		}
		return 7
	}
	switch {
	case dc <= dr/4:
		return 2
	case c2 < c1:
		return 3
	}
	return 1
}

// FollowTarget is 0x0041FA5D for a party member on (r, c) following the
// leader on (lr, lc). Closer than FollowDistance it returns false. Otherwise
// it searches the formation table (parity lr & 1) for a free cell around
// the leader: direction d = FormationDir(member → leader) and d − 1; four
// rounds, each starting at slot rand()%12 and trying the 12 slots in turn
// (d's slot, then d − 1's), then d + 1 and d − 2. free reports a cell with
// no occupant (grid bits 0xFFF). The caller walks there (+2 |= 0x48,
// +0x14 = 0xFF).
func FollowTarget(t *resfmt.ExeTables, lr, lc, r, c int, rng func(int) int, free func(r, c int) bool) (int, int, bool) {
	if Dist(lr, lc, r, c) < FollowDistance {
		return 0, 0, false
	}
	parity := lr & 1
	d := FormationDir(r, c, lr, lc)
	d2 := (d + 7) % 8
	try := func(dir, k int) (int, int, bool) {
		dr, dc := t.FormationOffset(parity, dir, k)
		rr, cc := lr+dr, lc+dc
		return rr, cc, insideGrid(rr, cc) && free(rr, cc)
	}
	for range 4 {
		k := rng(resfmt.FormationSlots)
		for range resfmt.FormationSlots {
			if rr, cc, ok := try(d, k); ok {
				return rr, cc, true
			}
			if rr, cc, ok := try(d2, k); ok {
				return rr, cc, true
			}
			k = (k + 1) % resfmt.FormationSlots
		}
		d = (d + 1) % 8
		d2 = (d2 + 7) % 8
	}
	return 0, 0, false
}

// placeSlots is how many formation slots the party placement uses.
const placeSlots = 6

// PlaceParty is 0x004118DC, after the party arrived on its map (a map exit,
// the jump orb): the hero goes to the group cell (+0x08/+0x10) with the
// arrival facing (+0x0C) when that cell is inside the grid and free; every
// other living member (the hero too when its cell was taken) gets a free
// formation cell around the group cell: direction = the hero's facing
// turned around (f ± 4), one octant less on a coin flip; eight directions
// from there, each trying the first 6 slots from slot rand()%6. A member
// placed takes the arrival facing and drops its intents (+2 &= 0xF0).
// Members left unplaced keep their cell (0, 0 after TakeExit: OnMap puts
// them next to the group cell). take marks a cell occupied. It returns the
// entity indices of the members placed.
func (g *Game) PlaceParty(t *resfmt.ExeTables, rng func(int) int, free func(r, c int) bool, take func(r, c int)) []int {
	gr := g.Groups[0]
	if gr.Count == 0 || gr.First < 0 || gr.First >= len(g.Entities) {
		return nil
	}
	var placed []int
	facing := int(int8(gr.Raw[0x0C]))
	hero := &g.Entities[gr.First]
	place := func(e *Entity, r, c int) {
		e.Variant, e.Row, e.Col = facing, r, c
		e.Flags2 &= 0xF0
		take(r, c)
		placed = append(placed, e.Index)
	}
	start := 0
	if insideGrid(gr.Row, gr.Col) && free(gr.Row, gr.Col) {
		place(hero, gr.Row, gr.Col)
		start = 1
	}
	parity := gr.Row & 1
	for k := start; k < gr.Count; k++ {
		e := &g.Entities[gr.First+k]
		if !e.canAct() {
			continue
		}
		d := hero.Variant + 4
		if hero.Variant >= 4 {
			d = hero.Variant - 4
		}
		if rng(2) != 0 {
			d = (d + 7) % 8
		}
	search:
		for range 8 {
			j := rng(placeSlots)
			for range placeSlots {
				dr, dc := t.FormationOffset(parity, d, j)
				if r, c := gr.Row+dr, gr.Col+dc; insideGrid(r, c) && free(r, c) {
					place(e, r, c)
					break search
				}
				j = (j + 1) % placeSlots
			}
			d = (d + 1) % 8
		}
	}
	return placed
}

// reviveScan is how many cells the revival's fallback search tries.
const reviveScan = 100

// Revive is 0x00414B3C, a dead member brought back by «Живая Вода»: it
// stays on its cell when that is free, else takes the first free neighbour
// (0x00410FFC, in Neighbors order); failing that it scans up to 100 cells
// from a random start in a rectangle (its group's when the group kind has
// bit 0x40 or 0x80, else the hero's cell ± 2·VillageRowRange rows and
// ± 2·VillageColRange cols), column first, wrapping. Without a free cell it
// fails. Otherwise: HP 1600, poison 0, kind 0x4C gets +0x12 = rand()%5 + 1
// (its revivals), the dead bit clears and the member idles (+2 |= 0x40, type 0 or
// 6 by the stance).
func (g *Game) Revive(e *Entity, heroRow, heroCol int, rng func(int) int, free func(r, c int) bool) bool {
	r, c := e.Row, e.Col
	if !free(r, c) {
		found := false
		for _, n := range mapdata.Neighbors(mapdata.Cell{Row: r, Col: c}) {
			if n.Row >= 0 && n.Row < mapdata.WalkRows && n.Col >= 0 && n.Col < mapdata.WalkCols && free(n.Row, n.Col) {
				r, c, found = n.Row, n.Col, true
				break
			}
		}
		if !found {
			var a Area
			if e.Group >= 0 && e.Group < len(g.Groups) && g.Groups[e.Group].Kind&0xC0 != 0 {
				a, _ = g.Groups[e.Group].WanderArea()
			} else {
				a = Area{heroRow - 2*VillageRowRange, heroRow + 2*VillageRowRange,
					heroCol - 2*VillageColRange, heroCol + 2*VillageColRange}
			}
			span := func(lo, hi int) int {
				if hi-lo < 2 {
					return lo
				}
				return lo + rng(hi-lo)
			}
			r, c = span(a.RowMin, a.RowMax), span(a.ColMin, a.ColMax)
			k := 0
			for ; k < reviveScan; k++ {
				if r >= 0 && r < mapdata.WalkRows && c >= 0 && c < mapdata.WalkCols && free(r, c) {
					break
				}
				if c++; c > a.ColMax {
					c = a.ColMin
					if r++; r > a.RowMax {
						r = a.RowMin
					}
				}
			}
			if k == reviveScan {
				return false
			}
		}
	}
	e.Row, e.Col = r, c
	e.HP, e.Poison = HPFull, 0
	if e.Flags4 == 0x4C {
		e.WeaponMode = rng(5) + 1
	}
	e.Flags4 &^= 0x80
	e.Flags2 |= IntentIdle // 0x00414AB4; the scene adds IntentFollow for members
	if e.InStance() {
		e.Type = ActIdle
	} else {
		e.Type = ActIdle + 6
	}
	return true
}
