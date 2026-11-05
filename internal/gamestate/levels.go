package gamestate

import "konung1.5/internal/resfmt"

// The character window's raise column (drawn by 0x0042F2B6, clicked through
// hotspots 40–45 in 0x00422864). The rules behind it are RaiseCost
// (0x00410B08) and GainExperience (0x0041079C) in loot.go.

// RaiseRows is the number of raise buttons: level, Хар, Сил, Лов, Вын.
const RaiseRows = 5

// RaiseIDs are the RaiseCost ids of the raise rows, in drawing order (jump
// table 0x0042F328) and click order (hotspots 40–44): 0 = level, 3 = Хар,
// 5 = Сил, 2 = Лов, 6 = Вын. Rows 1–4 raise BaseStats[row−1].
var RaiseIDs = [RaiseRows]int{0, 3, 5, 2, 6}

// Hotspots of the raise column.
const (
	HotspotRaise     = 40 // 40–44: raise row 0–4
	HotspotAutoLevel = 45 // the auto-level toggle (+3 bit 0x02)
)

// AutoLevel reports the auto-level toggle (+3 bit 0x02): GainExperience
// then spends free experience by itself.
func (e Entity) AutoLevel() bool { return e.Flags3&0x02 != 0 }

// RaiseCosts returns, per raise row, what 0x0042F2B6 tests: RaiseCost of the
// row's id. Row 0 is 1 when the level can go up; rows 1–4 are the
// free-experience cost of the stat. 0 hides the row's button.
func (g *Game) RaiseCosts(e Entity, t *resfmt.ExeTables) (c [RaiseRows]int) {
	for row, id := range RaiseIDs {
		c[row] = g.RaiseCost(e, t, id)
	}
	return c
}

// Raise is a click on raise button row (hotspot 40 + row, 0x004230D3 for the
// level, 0x0042310E–0x0042326D for the stats). The level goes up by one for
// free; a stat's base value goes up by one, its cost moves from free
// experience (+0x38) to experience (+0x3C). Nothing happens when RaiseCost
// is 0. The redraw that follows (0x0042DBBC(2)) recomputes the effective
// stats, as the port does on e. It reports whether anything changed.
func (g *Game) Raise(e *Entity, t *resfmt.ExeTables, row int) bool {
	if row < 0 || row >= RaiseRows {
		return false
	}
	c := g.RaiseCost(*e, t, RaiseIDs[row])
	if c == 0 {
		return false
	}
	if row == 0 {
		e.Level++
	} else {
		e.FreeExp -= c
		e.Experience += c
		e.BaseStats[row-1]++
	}
	*e = g.Recompute(*e, t)
	return true
}

// ToggleAutoLevel is hotspot 45 (0x00423272): a set toggle is cleared;
// otherwise it is set and GainExperience(e, 0) spends the free experience at
// once. The window is redrawn (and e recomputed) afterwards.
func (g *Game) ToggleAutoLevel(e *Entity, t *resfmt.ExeTables) {
	if e.AutoLevel() {
		e.Flags3 &^= 0x02
	} else {
		e.Flags3 |= 0x02
		g.GainExperience(e, t, 0)
	}
	*e = g.Recompute(*e, t)
}
