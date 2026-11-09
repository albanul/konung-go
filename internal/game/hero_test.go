package game

import (
	"testing"

	"konung1.5/internal/mapdata"
)

func TestHeroWalksToGoal(t *testing.T) {
	start := mapdata.Cell{Row: 86, Col: 33}
	h := &walker{cell: start}
	wall := func(c mapdata.Cell) bool { return c.Col == 36 && c.Row > 80 && c.Row < 96 } // force a detour
	goal := mapdata.Cell{Row: 88, Col: 40}
	h.blocked = wall
	h.walkTo(goal)
	if len(h.path) == 0 {
		t.Fatal("no path planned")
	}
	steps := len(h.path)
	// One tick per walk frame, plus one per octant turned before a step
	// (0x004391BC).
	turns, dir, at := 0, h.dir, start
	for _, c := range h.path {
		for _, n := range mapdata.Neighbors(at) {
			if n.Cell == c {
				d := (int(n.Dir) - int(dir) + 8) % 8
				turns += min(d, 8-d)
				dir = n.Dir
			}
		}
		at = c
	}
	ticks := 0
	for ; ticks < 100000 && (h.walking || len(h.path) > 0); ticks++ {
		prev := h.cell
		h.update()
		if wall(h.cell) {
			t.Fatalf("hero entered blocked cell %v", h.cell)
		}
		if h.cell != prev {
			adjacent := false
			for _, n := range mapdata.Neighbors(prev) {
				adjacent = adjacent || n.Cell == h.cell
			}
			if !adjacent {
				t.Fatalf("hero jumped from %v to %v", prev, h.cell)
			}
		}
	}
	if want := steps*defaultWalkSteps + turns; ticks != want {
		t.Errorf("walk took %d ticks, want %d frames per cell + %d turns = %d", ticks, defaultWalkSteps, turns, want)
	}
	if h.cell != goal || h.walking {
		t.Fatalf("hero stopped at %v (walking %v), want %v", h.cell, h.walking, goal)
	}
	if x, y := h.position(); true {
		if gx, gy := mapdata.CellCenter(goal); x != float64(gx) || y != float64(gy) {
			t.Fatalf("hero at %.1f,%.1f, want cell centre %d,%d", x, y, gx, gy)
		}
	}
	// Re-targeting mid-step keeps the step in progress.
	// The walker holds the cell it is entering from the step's start
	// (0x0041492E) and frees the one it left.
	h.walkTo(mapdata.Cell{Row: 88, Col: 30})
	h.update()
	entering, left := h.cell, h.from
	if !h.walking || entering == goal || left != goal {
		t.Fatalf("step: walking %v from %v into %v", h.walking, left, entering)
	}
	h.walkTo(mapdata.Cell{Row: 70, Col: 40})
	if !h.walking || h.cell != entering || h.from != left {
		t.Fatalf("re-target dropped the current step into %v (now %v)", entering, h.cell)
	}
}

// TestIdleLoop: the idle action starts at frame 0 when a walker becomes
// idle, advances one frame per tick and starts over after its last frame
// (0x004223AD, 0x0041291C).
func TestIdleLoop(t *testing.T) {
	s := &startScene{}
	w := &walker{idle: -1, load: func(kind, dir int) []sceneFrame { return make([]sceneFrame, 12) }}
	w.act = 2 // busy: a hit reaction
	s.idleLoop(w, false)
	if w.idle != -1 {
		t.Fatalf("busy walker idles at %d", w.idle)
	}
	w.act = 0
	var seen []int
	for range 26 {
		s.idleLoop(w, false)
		seen = append(seen, w.idle)
	}
	want := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 0, 1}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("idle frames %v, want %v", seen, want)
		}
	}
	s.idleLoop(w, true)
	if w.idle != -1 {
		t.Error("a dead walker idles")
	}
}
