package netplay_test

import (
	"testing"

	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

// report is the walker's state as the server reports it (lanserver's
// snapshot): the walk fields while walking or about to, the action's
// frame count while acting.
func report(w *world.Walker, throttle int) netplay.CharState {
	c := netplay.CharState{Row: w.Cell.Row, Col: w.Cell.Col, Walking: w.Walking, Facing: int(w.Dir), Act: w.Act, ActStep: w.ActStep}
	if w.Walking {
		c.FromRow, c.FromCol, c.Step = w.From.Row, w.From.Col, w.StepT
	}
	if w.Walking || len(w.Path) > 0 {
		c.Steps, c.Throttle = w.WalkSteps(), throttle
		for _, p := range w.Path {
			c.Path = append(c.Path, p.Row, p.Col)
		}
	}
	if w.Act != 0 {
		c.ActSteps = w.ActSteps()
	}
	return c
}

// Predict follows world.Walker.Update exactly while nothing else changes:
// a walk that starts, turns toward each step and crosses several cells, on
// every tick or on a throttle's ticks; and an action playing out.
func TestPredictFollowsWalker(t *testing.T) {
	frames := func(_ world.Look, kind, dir int) int {
		if kind == 1 {
			return 6
		}
		return 9
	}
	start := mapdata.Cell{Row: 20, Col: 10}
	path := []mapdata.Cell{{Row: 21, Col: 10}, {Row: 23, Col: 10}, {Row: 23, Col: 9}, {Row: 22, Col: 9}, {Row: 20, Col: 9}}
	for _, throttle := range []int{0, 1, 3} {
		w := &world.Walker{Cell: start, Dir: 2, Frames: frames, Path: append([]mapdata.Cell(nil), path...)}
		rep, from := report(w, throttle).Report(), 0
		reports := 0
		for tick := 1; tick <= 400; tick++ {
			if throttle == 0 || tick&throttle == throttle {
				w.Update()
			}
			got := report(w, throttle)
			if !rep.Predict(from, tick).Same(got) {
				// Only when the report's cells ran out is a new one due.
				if p := rep.Predict(from, tick); len(p.Path) > 0 || p.Walking {
					t.Fatalf("throttle %d tick %d: predicted %+v, walker %+v", throttle, tick, rep.Predict(from, tick), got)
				}
				rep, from = got.Report(), tick
				reports++
			}
		}
		if w.Walking || len(w.Path) > 0 || w.Cell != path[len(path)-1] {
			t.Fatalf("throttle %d: the walk did not end at its goal: %+v", throttle, w.Cell)
		}
		// 5 cells, PredictCells per report: two reports at most.
		if reports > 2 {
			t.Errorf("throttle %d: %d reports for one walk", throttle, reports)
		}
	}
	// An action: one step a tick, held on its last frame.
	w := &world.Walker{Cell: start, Frames: frames}
	w.StartAct(3)
	rep := report(w, 0)
	for tick := 1; tick <= 15; tick++ {
		w.AdvanceAct()
		if got := report(w, 0); !rep.Predict(0, tick).Same(got) {
			t.Fatalf("action tick %d: predicted %+v, walker %+v", tick, rep.Predict(0, tick), got)
		}
	}
}
