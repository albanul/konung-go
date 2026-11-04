package game

import (
	"testing"

	"konung1.5/internal/mapdata"
)

func TestHeroWalksToGoal(t *testing.T) {
	start := mapdata.Cell{Row: 86, Col: 33}
	x, y := mapdata.CellCenter(start)
	h := &hero{cell: start, x: float64(x), y: float64(y)}
	wall := func(c mapdata.Cell) bool { return c.Col == 36 && c.Row > 80 && c.Row < 96 } // force a detour
	goal := mapdata.Cell{Row: 88, Col: 40}
	h.walkTo(goal, wall)
	if len(h.path) == 0 {
		t.Fatal("no path planned")
	}
	for i := 0; i < 10000 && len(h.path) > 0; i++ {
		h.update()
		if wall(h.cell) {
			t.Fatalf("hero entered blocked cell %v", h.cell)
		}
	}
	if h.cell != goal || h.walking {
		t.Fatalf("hero stopped at %v (walking %v), want %v", h.cell, h.walking, goal)
	}
	if gx, gy := mapdata.CellCenter(goal); h.x != float64(gx) || h.y != float64(gy) {
		t.Fatalf("hero at %.1f,%.1f, want cell centre %d,%d", h.x, h.y, gx, gy)
	}
	// Re-targeting mid-step keeps the step in progress.
	h.walkTo(mapdata.Cell{Row: 88, Col: 30}, wall)
	h.update()
	next := h.path[0]
	h.walkTo(mapdata.Cell{Row: 70, Col: 40}, wall)
	if h.path[0] != next {
		t.Fatalf("re-target dropped the current step %v (now %v)", next, h.path[0])
	}
}
