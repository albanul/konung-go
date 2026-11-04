package gamestate

import "encoding/binary"

// Area is a group's wander rectangle on the fine grid: rows RowMin..RowMax,
// cols ColMin..ColMax (group words +0x08, +0x0C, +0x10, +0x14).
type Area struct{ RowMin, RowMax, ColMin, ColMax int }

// WanderArea returns the group's rectangle and whether it is usable. For
// the player's group (0) the same words hold other data.
func (g Group) WanderArea() (Area, bool) {
	w := func(off int) int { return int(binary.LittleEndian.Uint16(g.Raw[off:])) }
	a := Area{w(0x08), w(0x0C), w(0x10), w(0x14)}
	return a, g.Index != 0 && a.RowMax > a.RowMin+1 && a.ColMax > a.ColMin+1
}

// Contains reports whether a cell lies inside the rectangle.
func (a Area) Contains(row, col int) bool {
	return row >= a.RowMin && row <= a.RowMax && col >= a.ColMin && col <= a.ColMax
}

// WanderTarget picks a destination inside a like the game's wander routine
// (VA 0x0041F5D8): start at the rectangle's centre and, when a quarter span
// is at least 2 cells, shift both axes by rand()%(span/4) in the same
// direction, chosen by a coin flip. Then scan forward (column first,
// wrapping to the next row) for the first cell the walk grid marks free. The
// target is stored in entity words +0x0C/+0x0E and the entity's +2 byte gets
// 0x48. intn(n) must return a value in [0, n).
func WanderTarget(a Area, intn func(int) int, blocked func(row, col int) bool) (row, col int, ok bool) {
	row, col = (a.RowMin+a.RowMax)/2, (a.ColMin+a.ColMax)/2
	spanR, spanC := a.RowMax-a.RowMin-1, a.ColMax-a.ColMin-1
	add := intn(2) == 0
	shift := func(span int) int {
		if span/4 < 2 {
			return 0
		}
		d := intn(span / 4)
		if add {
			return d
		}
		return -d
	}
	row += shift(spanR)
	col += shift(spanC)
	for i := 0; i < spanR*spanC; i++ {
		if !blocked(row, col) {
			return row, col, true
		}
		if col++; col >= a.ColMax-1 {
			col = a.ColMin + 1
			if row++; row >= a.RowMax-1 {
				row = a.RowMin + 1
			}
		}
	}
	return 0, 0, false
}
