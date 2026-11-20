package lanserver

import (
	"testing"

	"konung1.5/internal/world"
)

// The villagers of the three villages stay inside their village area
// (villageBounds) however long they wander; GAME.RES's own rectangle is the
// whole map.
func TestVillagersStayHome(t *testing.T) {
	w := load(t)
	for _, m := range []int{1, 4, 7} {
		a, ok := w.VillageArea(m)
		if !ok {
			t.Fatalf("map %d has no village", m)
		}
		if a.RowMax-a.RowMin >= 120 || a.ColMax-a.ColMin >= 60 {
			t.Errorf("map %d: village area %+v is the whole map", m, a)
		}
	}
	moved, out := 0, 0
	start := map[int]world.Walker{}
	for _, m := range []int{1, 4, 7} {
		for _, n := range w.Maps[m].NPCs {
			start[n.Entity] = n.Walker
		}
	}
	for tick := 1; tick <= 6000; tick++ {
		w.Step()
		if tick%200 != 0 {
			continue
		}
		for _, m := range []int{1, 4, 7} {
			for _, n := range w.Maps[m].NPCs {
				if n.Mode != world.NPCVillager || n.Walking {
					continue
				}
				if !n.Area.Contains(n.Cell.Row, n.Cell.Col) {
					out++
					t.Errorf("tick %d: villager %d on map %d at %+v, outside %+v", tick, n.Entity, m, n.Cell, n.Area)
				}
			}
		}
		if out > 5 {
			t.FailNow()
		}
	}
	for _, m := range []int{1, 4, 7} {
		for _, n := range w.Maps[m].NPCs {
			if n.Mode == world.NPCVillager && n.Cell != start[n.Entity].Cell {
				moved++
			}
		}
	}
	if moved < 10 {
		t.Errorf("only %d villagers moved in 6000 ticks", moved)
	}
	t.Logf("%d villagers moved; none left their village", moved)
}
