package gamestate

import "testing"

func TestHeroKillPoints(t *testing.T) {
	for _, c := range []struct {
		victim, killer int
		amulet         bool
		want           int
	}{
		{1, 1, false, 2},  // 2·1 − 1 + 1
		{5, 2, false, 9},  // 2·5 − 2 + 1
		{1, 10, false, 1}, // at least 1
		{5, 2, true, 18},  // doubled with an amulet part
	} {
		if got := HeroKillPoints(c.victim, c.killer, c.amulet); got != c.want {
			t.Errorf("HeroKillPoints(%d, %d, %v) = %d, want %d", c.victim, c.killer, c.amulet, got, c.want)
		}
	}
}

func TestSessionRatings(t *testing.T) {
	// Two players at 1000: R̄ = 1000, P = 30, p̄ = 15. The winner gains
	// (25 − 15) · 1000 / 30 = 333, the other loses as much.
	got := SessionRatings([]int{1000, 1000}, []int{25, 5})
	if got[0] != 1333 || got[1] != 666 {
		t.Errorf("SessionRatings = %v, want [1333 666]", got)
	}
	// −1 (joined, no points) counts as 0 points.
	got = SessionRatings([]int{1000, 1000}, []int{10, -1})
	if got[0] != 1500 || got[1] != 500 {
		t.Errorf("with a pointless player = %v, want [1500 500]", got)
	}
	// No points: unchanged; clamped at 0.
	if got := SessionRatings([]int{10, 20}, []int{0, -1}); got[0] != 10 || got[1] != 20 {
		t.Errorf("no points = %v", got)
	}
	if got := SessionRatings([]int{10, 5000}, []int{0, 100}); got[0] != 0 {
		t.Errorf("not clamped at 0: %v", got)
	}
}
