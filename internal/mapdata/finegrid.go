package mapdata

import (
	"container/heap"
	"math"
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

// CellAt returns the fine cell whose centre is nearest to world pixel (x, y).
func CellAt(x, y int) Cell {
	best, bestD := Cell{}, math.MaxInt
	r0 := (y - FineCellHeight + FineCellHeight/2) / FineCellHeight
	for r := r0 - 1; r <= r0+1; r++ {
		if r < 0 {
			continue
		}
		off := FineCellWidth
		if r&1 != 0 {
			off = FineCellWidth / 2
		}
		c := int(math.Round(float64(x-off) / FineCellWidth))
		for cc := c - 1; cc <= c+1; cc++ {
			if cc < 0 {
				continue
			}
			cx, cy := CellCenter(Cell{r, cc})
			if d := (cx-x)*(cx-x) + (cy-y)*(cy-y); d < bestD {
				best, bestD = Cell{r, cc}, d
			}
		}
	}
	return best
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

// FindPath returns the cells from start (exclusive) to goal (inclusive) along
// a shortest path in world pixels, avoiding blocked cells, or nil when goal is
// unreachable. It explores at most limit cells (0 = whole grid).
func FindPath(start, goal Cell, blocked func(Cell) bool, limit int) []Cell {
	if start == goal || blocked(goal) {
		return nil
	}
	dist := func(a, b Cell) float64 {
		ax, ay := CellCenter(a)
		bx, by := CellCenter(b)
		return math.Hypot(float64(ax-bx), float64(ay-by))
	}
	g := map[Cell]float64{start: 0}
	from := map[Cell]Cell{}
	open := &cellHeap{{start, dist(start, goal)}}
	closed := map[Cell]bool{}
	for open.Len() > 0 {
		cur := heap.Pop(open).(scored).c
		if cur == goal {
			var path []Cell
			for c := goal; c != start; c = from[c] {
				path = append(path, c)
			}
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path
		}
		if closed[cur] {
			continue
		}
		closed[cur] = true
		if limit > 0 && len(closed) > limit {
			return nil
		}
		for _, n := range Neighbors(cur) {
			if closed[n.Cell] || blocked(n.Cell) {
				continue
			}
			ng := g[cur] + dist(cur, n.Cell)
			if old, ok := g[n.Cell]; ok && old <= ng {
				continue
			}
			g[n.Cell], from[n.Cell] = ng, cur
			heap.Push(open, scored{n.Cell, ng + dist(n.Cell, goal)})
		}
	}
	return nil
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

type scored struct {
	c Cell
	f float64
}

type cellHeap []scored

func (h cellHeap) Len() int           { return len(h) }
func (h cellHeap) Less(i, j int) bool { return h[i].f < h[j].f }
func (h cellHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *cellHeap) Push(x any)        { *h = append(*h, x.(scored)) }
func (h *cellHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}
