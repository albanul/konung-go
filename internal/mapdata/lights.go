package mapdata

import "fmt"

// LIGHTS.N (N = map number) is read by the map loader 0x0043C0FC before
// KONUNG.N: 0x3200 bytes to 0x00671440 (80 × 160, per terrain cell, read by
// the lit terrain path 0x004257A9) and 0xC800 bytes to 0x00664C40 (320 × 160,
// per fine cell). Both buffers are zero-filled when the file is missing.
const (
	LightsFileSize = 0x3200 + litCellsSize
	litCellsOffset = 0x3200
	litCellsSize   = WalkRows * WalkCols
)

// LitCells is the fine-grid part of LIGHTS.N (0x00664C40, indexed
// row*0xA0 + col). A character standing on a non-zero cell is drawn with
// its raw palette instead of the day/night tinted one (0x00427285).
type LitCells [WalkRows][WalkCols]byte

// ParseLitCells reads the fine-grid part of LIGHTS.N. nil data (no file)
// gives an all-zero grid, as the loader does.
func ParseLitCells(data []byte) (*LitCells, error) {
	l := &LitCells{}
	if data == nil {
		return l, nil
	}
	if len(data) != LightsFileSize {
		return nil, fmt.Errorf("LIGHTS.N has size %d, want %d", len(data), LightsFileSize)
	}
	for r := range WalkRows {
		copy(l[r][:], data[litCellsOffset+r*WalkCols:][:WalkCols])
	}
	return l, nil
}

// Lit reports whether a fine cell is lit (false off the grid).
func (l *LitCells) Lit(row, col int) bool {
	return l != nil && row >= 0 && row < WalkRows && col >= 0 && col < WalkCols && l[row][col] != 0
}
