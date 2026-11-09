package mapdata

import (
	"math"
	"sync"
)

// Cell is a position on the fine grid (WalkRows x WalkCols) that static
// objects, entities and the walk grid share.
type Cell struct{ Row, Col int }

// CellCenter returns the world pixel at a fine cell's centre (VA 0x00439BA0):
// x = col*58 + (row odd ? 29 : 58), y = row*16 + 16.
func CellCenter(c Cell) (x, y int) {
	x = c.Col * FineCellWidth
	if c.Row&1 != 0 {
		x += FineCellWidth / 2
	} else {
		x += FineCellWidth
	}
	return x, c.Row*FineCellHeight + FineCellHeight
}

// CellAt is the game's pixel-to-cell routine 0x00439BDC: the fine cell
// whose diamond (58 × 32 px around CellCenter) holds world pixel (x, y).
// row = y/16 and the column by row parity give a cell whose centre cx lies
// on the band's lower edge; a point above that diamond's upper-left or
// upper-right edge belongs to the cell in the row above. The result is
// clamped to the grid. Clicks, the hover and arrows use it.
func CellAt(x, y int) Cell {
	const w, h, half = FineCellWidth, FineCellHeight, FineCellWidth / 2
	row := y / h
	col, cx := (x-half)/w, 0
	if row&1 != 0 {
		col = x / w
		cx = col*w + half
	} else {
		cx = col*w + w
	}
	bottom := (row + 1) * h
	top := bottom - h
	switch {
	case x < cx:
		if left := cx - half; (y-top)*(x-left) < (bottom-y)*(cx-x) {
			row--
			if row&1 == 0 {
				col--
			}
		}
	case x > cx:
		if right := cx + half; (bottom-y)*(cx-x) < (y-top)*(x-right) {
			if row&1 == 0 {
				col++
			}
			row--
		}
	}
	return Cell{min(max(row, 0), WalkRows-1), min(max(col, 0), WalkCols-1)}
}

// Direction is a facing as used by entity variants (byte +1): 0 W, 1 NW,
// 2 N, 3 NE, 4 E, 5 SE, 6 S, 7 SW. The mapping was identified visually from
// the walk frames of body model 0.
type Direction int

// Neighbors returns the 8 adjacent fine cells with their facing. On the
// staggered grid, same-row neighbours are 58 px apart (E/W), rows +-2 are
// 32 px apart (N/S) and rows +-1 are the diagonals (+-29, +-16 px).
func Neighbors(c Cell) [8]struct {
	Cell
	Dir Direction
} {
	even := c.Row&1 == 0
	right, left := c.Col, c.Col-1 // diagonal columns from an odd row
	if even {
		right, left = c.Col+1, c.Col
	}
	return [8]struct {
		Cell
		Dir Direction
	}{
		{Cell{c.Row, c.Col - 1}, 0},
		{Cell{c.Row - 1, left}, 1},
		{Cell{c.Row - 2, c.Col}, 2},
		{Cell{c.Row - 1, right}, 3},
		{Cell{c.Row, c.Col + 1}, 4},
		{Cell{c.Row + 1, right}, 5},
		{Cell{c.Row + 2, c.Col}, 6},
		{Cell{c.Row + 1, left}, 7},
	}
}

// DirectionTo returns the facing for moving from world point a to b.
func DirectionTo(ax, ay, bx, by float64) Direction {
	// Screen angle, with y down; 0 = east. Map to the W-first clockwise order.
	a := math.Atan2(by-ay, bx-ax)                // -pi..pi
	sector := int(math.Round(a/(math.Pi/4))) & 7 // 0 E, 1 SE, 2 S, 3 SW, 4 W, 5 NW, 6 N, 7 NE
	return [8]Direction{4, 5, 6, 7, 0, 1, 2, 3}[sector]
}

// PathSteps is how many steps a path search keeps: the entity's path
// buffer +0x13 holds 31 directions and an end byte (0x0043F740).
const PathSteps = 31

// Path search costs (0x0043F031, 0x0043F48D): a diagonal step (row ±1)
// costs 10, a straight one (same row, or row ±2) 14.
const (
	pathCostDiagonal = 10
	pathCostStraight = 14
	pathQueueLen     = 4096 // 0x0051ECD0: 16-byte entries, 16-bit indices
)

type pathNode struct {
	cell Cell
	cost int
}

// pathScratch is the search's node table (0x0052ECD0): the best cost and
// the direction each cell was reached by, valid when stamp is the current
// generation (0x00592CD0).
type pathScratch struct {
	stamp [WalkRows][WalkCols]uint32
	cost  [WalkRows][WalkCols]int32
	dir   [WalkRows][WalkCols]int8
	gen   uint32
	queue [pathQueueLen]pathNode
}

var pathPool = sync.Pool{New: func() any { return new(pathScratch) }}

func insideGrid(c Cell) bool { return c.Row >= 0 && c.Row < WalkRows && c.Col >= 0 && c.Col < WalkCols }

// pathOrder is the order the search tries a cell's neighbours, as
// Neighbors indices (= direction codes): the diagonals by row parity, then
// W, E, N, S.
var pathOrder = [2][8]int{
	{1, 3, 7, 5, 0, 4, 2, 6}, // even row
	{3, 1, 5, 7, 0, 4, 2, 6}, // odd row
}

// FindPath is the game's path search 0x0043EDB7: the cells from start
// (exclusive) toward goal, at most PathSteps of them, or nil. It fails when
// start is goal, goal is blocked, or no neighbour of goal is free (or
// start). The search floods from start through a FIFO queue (a ring of
// 4096 entries) in pathOrder, each step costing 10 (diagonal) or 14
// (straight), recording a cell when its cost improves; the goal is
// recorded but never queued, and once it is found no new cells are
// queued. There is no search limit. start itself counts as free (the
// walker frees its cell for the search, 0x00414326). A walker whose path
// ends short of its goal searches again from there.
func FindPath(start, goal Cell, blocked func(Cell) bool) []Cell {
	if start == goal || !insideGrid(start) || !insideGrid(goal) || blocked(goal) {
		return nil
	}
	open := false
	for _, n := range Neighbors(goal) {
		if insideGrid(n.Cell) && (n.Cell == start || !blocked(n.Cell)) {
			open = true
			break
		}
	}
	if !open {
		return nil
	}
	sc := pathPool.Get().(*pathScratch)
	defer pathPool.Put(sc)
	sc.gen++
	if sc.gen == 0 { // wrapped: clear the table (0x0043EFFC)
		sc.stamp = [WalkRows][WalkCols]uint32{}
		sc.gen = 1
	}
	gen := sc.gen
	costOf := func(c Cell) int {
		if sc.stamp[c.Row][c.Col] != gen {
			return math.MaxInt
		}
		return int(sc.cost[c.Row][c.Col])
	}
	set := func(c Cell, cost, d int) {
		sc.stamp[c.Row][c.Col], sc.cost[c.Row][c.Col], sc.dir[c.Row][c.Col] = gen, int32(cost), int8(d)
	}
	found := false
	var head, tail uint16 // indices into the ring; empty when equal
	cur := pathNode{start, 0}
	for first := true; ; first = false {
		if !first {
			if head == tail {
				break
			}
			cur = sc.queue[head%pathQueueLen]
			head = (head + 1) % pathQueueLen
		}
		nb := Neighbors(cur.cell)
		for _, k := range pathOrder[cur.cell.Row&1] {
			n := nb[k].Cell
			if !insideGrid(n) {
				continue
			}
			cost := cur.cost + pathCostDiagonal
			if k%2 == 0 {
				cost = cur.cost + pathCostStraight
			}
			if n == goal {
				if costOf(n) > cost {
					found = true
					set(n, cost, k)
				}
				continue
			}
			if found || (n != start && blocked(n)) || costOf(n) <= cost {
				continue
			}
			set(n, cost, k)
			sc.queue[tail%pathQueueLen] = pathNode{n, cost}
			tail = (tail + 1) % pathQueueLen
		}
	}
	if !found {
		return nil
	}
	// Back from the goal by the recorded directions (step back = d + 4).
	var rev []Cell
	for c := goal; c != start; {
		rev = append(rev, c)
		d := int(sc.dir[c.Row][c.Col])
		c = Neighbors(c)[(d+4)%8].Cell
	}
	n := min(len(rev), PathSteps)
	path := make([]Cell, n)
	for i := range path {
		path[i] = rev[len(rev)-1-i]
	}
	return path
}

// NearestFree returns the unblocked cell closest (in world pixels) to c,
// searching up to radius cells away, or c itself when none is found.
func NearestFree(c Cell, blocked func(Cell) bool, radius int) Cell {
	if !blocked(c) {
		return c
	}
	cx, cy := CellCenter(c)
	best, bestD := c, math.MaxInt
	for r := c.Row - 2*radius; r <= c.Row+2*radius; r++ {
		for col := c.Col - radius; col <= c.Col+radius; col++ {
			n := Cell{r, col}
			if r < 0 || col < 0 || blocked(n) {
				continue
			}
			x, y := CellCenter(n)
			if d := (x-cx)*(x-cx) + (y-cy)*(y-cy); d < bestD {
				best, bestD = n, d
			}
		}
	}
	return best
}
