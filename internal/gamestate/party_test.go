package gamestate

import (
	"math/rand/v2"
	"testing"

	"konung1.5/internal/resfmt"
)

func TestSelection(t *testing.T) {
	sel := Select(nil, 5, false)
	sel = Select(sel, 7, true)
	sel = Select(sel, 9, true)
	if !Selected(sel, 7) || len(sel) != 3 {
		t.Fatalf("selection %v", sel)
	}
	if sel = Select(sel, 3, false); len(sel) != 1 || sel[0] != 3 {
		t.Fatalf("Select without Shift: %v", sel)
	}
	sel = []int{1, 2, 3}
	if sel = Deselect(sel, 2); len(sel) != 2 || sel[0] != 1 || sel[1] != 3 {
		t.Fatalf("Deselect: %v", sel)
	}
	full := make([]int, MaxSelection)
	if len(Select(full, 99, true)) != MaxSelection {
		t.Error("the list holds at most 10")
	}
}

// TestFormationDir follows 0x00439AA4's branches.
func TestFormationDir(t *testing.T) {
	for _, c := range []struct{ r1, c1, r2, c2, want int }{
		{10, 10, 10, 20, 0}, // same row, east
		{10, 10, 10, 2, 4},
		{10, 10, 12, 20, 0}, // |dr| ≤ |dc|/2
		{20, 10, 10, 10, 6}, // straight up
		{20, 10, 10, 5, 5},
		{20, 10, 10, 15, 7},
		{10, 10, 20, 10, 2}, // straight down
		{10, 10, 20, 5, 3},
		{10, 10, 20, 15, 1},
	} {
		if got := FormationDir(c.r1, c.c1, c.r2, c.c2); got != c.want {
			t.Errorf("FormationDir(%d,%d → %d,%d) = %d, want %d", c.r1, c.c1, c.r2, c.c2, got, c.want)
		}
	}
}

func exeTables(t *testing.T) *resfmt.ExeTables {
	t.Helper()
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

// TestFollowTarget: closer than 10 nothing happens; from afar the target is
// a free formation cell near the leader, from table 0x0045FF79.
func TestFollowTarget(t *testing.T) {
	tb := exeTables(t)
	if dr, dc := tb.FormationOffset(0, 1, 3); dr != 0 || dc != -5 {
		t.Fatalf("formation slot (0, 1, 3) = (%d, %d), want (0, -5)", dr, dc)
	}
	rng := rand.New(rand.NewPCG(1, 1)).IntN
	free := func(r, c int) bool { return true }
	if _, _, ok := FollowTarget(tb, 100, 40, 105, 42, rng, free); ok {
		t.Fatal("a member 5 cells away walks")
	}
	r, c, ok := FollowTarget(tb, 100, 40, 160, 40, rng, free)
	if !ok || Dist(100, 40, r, c) >= FollowDistance {
		t.Fatalf("follow target (%d, %d) %v", r, c, ok)
	}
	// Every slot taken: no target.
	if _, _, ok := FollowTarget(tb, 100, 40, 160, 40, rng, func(int, int) bool { return false }); ok {
		t.Error("a target with no free cell")
	}
}

// TestPlaceParty: after an exit the hero stands on the group cell with the
// arrival facing and a hired member next to it, behind the hero.
func TestPlaceParty(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	tb := exeTables(t)
	gr := &g.Groups[0]
	// A second member: the next entity, alive.
	gr.Count = 2
	member := &g.Entities[gr.First+1]
	member.Flags4 &^= 0x80
	member.Type = 6
	g.TakeExit(MapExit{To: gr.Map, Row: 100, Col: 40, Facing: 2})
	taken := map[[2]int]bool{}
	free := func(r, c int) bool { return !taken[[2]int{r, c}] }
	take := func(r, c int) { taken[[2]int{r, c}] = true }
	g.PlaceParty(tb, rand.New(rand.NewPCG(2, 2)).IntN, free, take)
	hero := g.Entities[gr.First]
	if hero.Row != 100 || hero.Col != 40 || hero.Variant != 2 {
		t.Fatalf("hero at (%d, %d) facing %d", hero.Row, hero.Col, hero.Variant)
	}
	m := g.Entities[gr.First+1]
	if m.Row == 0 && m.Col == 0 || Dist(100, 40, m.Row, m.Col) > 8 || (m.Row == 100 && m.Col == 40) {
		t.Fatalf("member at (%d, %d)", m.Row, m.Col)
	}
	// The group cell taken: the hero is placed around it too.
	g.TakeExit(MapExit{To: gr.Map, Row: 100, Col: 40, Facing: 2})
	taken = map[[2]int]bool{{100, 40}: true}
	g.PlaceParty(tb, rand.New(rand.NewPCG(3, 3)).IntN, free, take)
	if h := g.Entities[gr.First]; h.Row == 0 || (h.Row == 100 && h.Col == 40) {
		t.Fatalf("hero at (%d, %d) with the group cell taken", h.Row, h.Col)
	}
}

// TestRevive: «Живая Вода» on a dead member revives it on its cell, or on
// a free neighbour when the cell is taken.
func TestRevive(t *testing.T) {
	g := &Game{Groups: make([]Group, 1)}
	e := Entity{Flags4: 0x80 | 0x01, Row: 50, Col: 20, Poison: 7, Flags3: 0x04, Type: 3}
	taken := map[[2]int]bool{{50, 20}: true}
	free := func(r, c int) bool { return !taken[[2]int{r, c}] }
	if !g.Revive(&e, 50, 20, rand.New(rand.NewPCG(1, 1)).IntN, free) {
		t.Fatal("no revival")
	}
	if e.Dead() || e.HP != HPFull || e.Poison != 0 || e.Type != ActIdle || (e.Row == 50 && e.Col == 20) {
		t.Fatalf("revived: %+v", e)
	}
	if Dist(50, 20, e.Row, e.Col) > 2 {
		t.Errorf("revived at (%d, %d), not next to its cell", e.Row, e.Col)
	}
	e2 := Entity{Flags4: 0x80, Row: 50, Col: 20}
	if g.Revive(&e2, 50, 20, rand.New(rand.NewPCG(1, 1)).IntN, func(int, int) bool { return false }) || !e2.Dead() {
		t.Error("revived without a free cell")
	}
}
