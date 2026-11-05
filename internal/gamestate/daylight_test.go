package gamestate

import (
	"testing"

	"konung1.5/internal/resfmt"
)

func TestDayTint(t *testing.T) {
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	th := tb.DayThresholds()
	if th != [12]int{1800, 1830, 3570, 3600, 5400, 8100, 11700, 11880, 11910, 16155, 16200, 18900} {
		t.Fatalf("thresholds %v", th)
	}
	type tc struct {
		d     int
		moon  bool
		want  resfmt.Tint
		night bool
	}
	for _, c := range []tc{
		{0, true, resfmt.Tint{}, false},
		{1810, true, resfmt.Tint{R: 5, G: 2}, false},
		{2000, true, resfmt.Tint{R: 12, G: 6}, false},
		{5450, true, resfmt.Tint{R: 5, G: -10, B: -25}, false},
		{6000, true, resfmt.Tint{R: 10, G: -20, B: -50}, false},
		{9000, true, resfmt.Tint{G: -50, B: -70}, true},
		{13000, true, resfmt.Tint{R: -60, G: -50, B: -70}, true},
		{13000, false, resfmt.Tint{R: -60, G: -70, B: -70}, true},
		{17000, true, resfmt.Tint{R: -30, G: -60, B: -20}, true},
		{18950, true, resfmt.Tint{R: -18, G: -35, B: -12}, false},
		{20000, true, resfmt.Tint{}, false},
		{DayLength + 2000, true, resfmt.Tint{R: 12, G: 6}, false},
	} {
		got, night := DayTint(c.d, th, c.moon)
		if got != c.want || night != c.night {
			t.Errorf("DayTint(%d, moon %v) = %+v %v, want %+v %v", c.d, c.moon, got, night, c.want, c.night)
		}
	}
	// The transitions are smooth: no channel jumps by more than 1 per tick,
	// except at the end of moonrise: the segment is 30 ticks but its ramp is
	// written for 40 (0x00429692), so green jumps from −56 to −50.
	for _, moon := range []bool{false, true} {
		prev, _ := DayTint(DayLength-1, th, moon)
		for d := range DayLength {
			cur, _ := DayTint(d, th, moon)
			if moon && d == th[8] {
				if prev.G != -56 || cur.G != -50 {
					t.Fatalf("moonrise end %+v -> %+v", prev, cur)
				}
			} else if abs(cur.R-prev.R) > 1 || abs(cur.G-prev.G) > 1 || abs(cur.B-prev.B) > 1 {
				t.Fatalf("moon %v: jump at %d: %+v -> %+v", moon, d, prev, cur)
			}
			prev = cur
		}
	}
	if l, ok := tb.MapLight(1); ok || !l.Zero() {
		t.Fatal("KONUNG.1 follows the time of day")
	}
	if l, ok := tb.MapLight(42); !ok || l != (resfmt.Tint{R: -70}) {
		t.Fatalf("KONUNG.42 fixed light %+v %v", l, ok)
	}
}
