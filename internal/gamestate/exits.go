package gamestate

import "encoding/binary"

// Map exits: GAME.N section 4 (file 0x29600, 250 × 0x11 bytes, loaded to
// VA 0x007AA2DC). The loader (0x0043BCEF) counts the records before the first
// with target byte +4 = 0 into 0x00A3E81C. On the map, each exit sits under a
// teleport decoration (GRAPH.RES tile 285 or 286, KONUNG.N section 2).
const (
	exitOffset = 0x29600
	exitStride = 0x11
	exitCount  = 250
)

// MapExit is one exit record.
type MapExit struct {
	Index  int
	Facing int // +0x02 int8: the hero's facing on arrival (party +0x0C → entity +0x01, 0x00411A1D)
	From   int // +0x03: map (KONUNG.N) the exit is on
	To     int // +0x04: map it leads to
	Row    int // +0x05 word: arrival fine-grid row (party group +0x08)
	Col    int // +0x07 word: arrival fine-grid column (party group +0x10)
	// Trigger rectangle on map From, inclusive: rows +0x09..+0x0D,
	// columns +0x0B..+0x0F (words). The map loader flags these cells
	// (fine-grid bit 0x1000, 0x0043D11B).
	MinRow, MinCol, MaxRow, MaxCol int
}

// Contains reports whether fine-grid cell (row, col) is inside the trigger
// rectangle.
func (x MapExit) Contains(row, col int) bool {
	return row >= x.MinRow && row <= x.MaxRow && col >= x.MinCol && col <= x.MaxCol
}

func parseExits(data []byte) []MapExit {
	var out []MapExit
	for i := 0; i < exitCount; i++ {
		r := data[exitOffset+i*exitStride:][:exitStride]
		if r[4] == 0 {
			break
		}
		w := func(o int) int { return int(binary.LittleEndian.Uint16(r[o:])) }
		out = append(out, MapExit{
			Index: i, Facing: int(int8(r[2])), From: int(r[3]), To: int(r[4]),
			Row: w(5), Col: w(7), MinRow: w(9), MinCol: w(0xB), MaxRow: w(0xD), MaxCol: w(0xF),
		})
	}
	return out
}

// ExitAt returns the first exit of map m whose rectangle holds (row, col),
// in record order like 0x00421BB8. The game calls that routine when a party
// member's walk ends without the pick-up intent on a flagged cell
// (0x00422368). It also skips exit i while the party is in settlement i
// unless settlement byte +0x40 < 0 or byte +0x41 = 3; the port does not track
// the current settlement (0x00A3E6A8), so that gate is not applied.
func (g *Game) ExitAt(m, row, col int) (MapExit, bool) {
	for _, x := range g.Exits {
		if x.From == m && x.Contains(row, col) {
			return x, true
		}
	}
	return MapExit{}, false
}

// TakeExit moves the party (group 0) to the exit's target map, like
// 0x00421CCD: the map byte, the group row/column and the arrival facing. The
// party's positions are cleared so OnMap places it at the group cell, or the
// nearest free cell when that one is taken (0x004119E9 places the hero there
// only when the cell is free and spreads the party around it); the other
// party members are placed around it too.
func (g *Game) TakeExit(x MapExit) {
	g.SetArrival(x)
	gr := &g.Groups[0]
	for k := 0; k < gr.Count; k++ {
		if i := gr.First + k; i >= 0 && i < len(g.Entities) {
			e := &g.Entities[i]
			e.Row, e.Col = 0, 0
			if k == 0 {
				e.Variant = x.Facing
			}
		}
	}
}

// SetArrival writes exit x's target to the party group: map +0x1B, the
// arrival cell +0x08/+0x10 and facing +0x0C (0x00421CCD, 0x004106E0). The
// members keep their cells; for a jump to the map the party is on, that is
// all before the placement (0x004377A6 → 0x004118DC).
func (g *Game) SetArrival(x MapExit) {
	gr := &g.Groups[0]
	gr.Map, gr.Row, gr.Col = x.To, x.Row, x.Col
	gr.Raw[0x0C] = byte(x.Facing)
}

// ExitOpen is the gate check of the exit trigger (0x00421C82): exit i is
// closed while the party is in settlement i (the settlement of the exit's
// map, 0x00A3E6A8) and that village has a bridge (building record 5, type
// ≥ 0) that does not stand (state ≠ 3). Only exits 0–2 pair with their
// village this way (maps 14, 19, 22). On a closed exit the game does
// nothing; it does not try a later exit.
func (g *Game) ExitOpen(x MapExit) bool {
	s := g.settlementOn(x.From)
	if s == nil || s.Index != x.Index {
		return true
	}
	b := s.building(BuildingBridge)
	return int8(b[0]) < 0 || b[1] == BuildingStanding
}
