// Package mapdata decodes KONUNG.N terrain layers and the GRAPH.RES terrain
// tiles they reference. It has no rendering-engine dependency so it can be used
// by the Ebitengine scene and offline tools alike.
//
// The layout follows the map loader at VA 0x0043C0FC and the terrain renderer
// at VA 0x0042562F in konung.exe; see docs/konung_map_format.md.
package mapdata

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"

	"konung1.5/internal/resfmt"
)

const (
	// FileSize is the size of every installed KONUNG.N file. The loader reads
	// four consecutive sections of 0x6400, 0xC800, 0x3E80 and 0x8CA0 bytes.
	FileSize = 0x1f720

	Rows        = 160 // terrain rows (section 0 is 160 records of 0xA0 bytes)
	Cols        = 80  // two-byte cells per terrain row
	Layers      = 2   // each cell holds a base and an overlay tile byte
	rowBytes    = Cols * Layers
	terrainSize = Rows * rowBytes // 0x6400

	// CellWidth is the horizontal distance between cells in one row; odd rows
	// are shifted right by CellWidth/2. CellHeight is the row-to-row step.
	CellWidth  = 0x74
	CellHeight = 0x20

	graphTileBlock = 0x227e8 // embedded Type 1 block header in GRAPH.RES
	graphTileSlots = 512
)

// Map holds the terrain section of a KONUNG.N file.
type Map struct {
	// Cells[row][col][layer] is a 1-based GRAPH.T1 tile ID; 0 means empty.
	Cells [Rows][Cols][Layers]byte
	// LastRow and LastCol are the extents the game's loader derives from the
	// first layer: the highest row/column containing a non-zero base tile.
	LastRow, LastCol int
}

// Parse decodes the terrain section (file offset 0, 0x6400 bytes).
func Parse(data []byte) (*Map, error) {
	if len(data) != FileSize {
		return nil, fmt.Errorf("KONUNG map has size %d, want %d", len(data), FileSize)
	}
	m := &Map{}
	for r := 0; r < Rows; r++ {
		for c := 0; c < Cols; c++ {
			for l := 0; l < Layers; l++ {
				b := data[r*rowBytes+c*Layers+l]
				m.Cells[r][c][l] = b
				if l == 0 && b != 0 {
					m.LastRow = max(m.LastRow, r)
					m.LastCol = max(m.LastCol, c)
				}
			}
		}
	}
	return m, nil
}

// CellOrigin returns the top-left world pixel where the tiles of a cell are
// blitted (the renderer passes this position straight to its blit routine).
func CellOrigin(row, col int) (x, y int) {
	x = col * CellWidth
	if row&1 != 0 {
		x += CellWidth / 2
	}
	return x, row * CellHeight
}

// WorldSize returns the scrollable pixel size implied by the map extents,
// using the width/height formulas computed by the game's loader
// ((lastCol)*0x74 and lastRow*0x20) plus one cell of tile overhang.
func (m *Map) WorldSize() (w, h int) {
	return (m.LastCol+1)*CellWidth + CellWidth/2, (m.LastRow + 2) * CellHeight
}

// CameraLimits are the game's right and bottom limits for the view, in world
// pixels (map loader 0x0043C3BE/0x0043C430): (LastCol − 1)·116 and
// LastRow·32. The view never reaches the ragged tile edges beyond them.
func (m *Map) CameraLimits() (w, h int) {
	return (m.LastCol - 1) * CellWidth, m.LastRow * CellHeight
}

// LoadTerrainTiles decodes the GRAPH.RES Type 1 terrain images indexed by the
// table the executable loads to VA 0x006314B8. The returned slice is indexed
// by 0-based table slot; a map byte b refers to slot b-1. Unused or
// undecodable slots are nil.
func LoadTerrainTiles(graph []byte) ([]*image.RGBA, error) {
	tableStart := graphTileBlock + 8
	dataStart := tableStart + graphTileSlots*8
	if len(graph) < dataStart {
		return nil, fmt.Errorf("GRAPH.RES is truncated before terrain index")
	}
	end := tableStart + int(binary.LittleEndian.Uint32(graph[graphTileBlock+4:]))
	if end > len(graph) || end < dataStart {
		return nil, fmt.Errorf("GRAPH.RES terrain block has invalid size")
	}
	tiles := make([]*image.RGBA, graphTileSlots)
	for i := range tiles {
		off := binary.LittleEndian.Uint32(graph[tableStart+i*8:])
		palOff := binary.LittleEndian.Uint32(graph[tableStart+i*8+4:])
		if off == 0xffffffff || palOff == 0xffffffff || palOff/512 >= 256 {
			continue
		}
		stop := uint32(end - dataStart)
		for j := i + 1; j < graphTileSlots; j++ {
			next := binary.LittleEndian.Uint32(graph[tableStart+j*8:])
			if next != 0xffffffff && next > off {
				stop = min(stop, next)
				break
			}
		}
		if dataStart+int(stop) > len(graph) || stop <= off {
			continue
		}
		p := int(palOff / 512)
		palette, err := resfmt.RGB555Palette(graph[p*512 : p*512+512])
		if err != nil {
			return nil, err
		}
		img, err := resfmt.DecodeIndexedRLEFrame(graph[dataStart+int(off):dataStart+int(stop)], &palette)
		if err != nil {
			continue
		}
		tiles[i] = img
	}
	return tiles, nil
}

// Tile returns the decoded image for a map byte, or nil for empty/unknown.
func Tile(tiles []*image.RGBA, b byte) *image.RGBA {
	if b == 0 || int(b) > len(tiles) {
		return nil
	}
	return tiles[b-1]
}

// Render composes the whole terrain into one image in the game's draw order:
// rows top to bottom, columns left to right, base layer then overlay.
func Render(m *Map, tiles []*image.RGBA) *image.RGBA {
	w, h := m.WorldSize()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	drawTerrain(dst, m, tiles)
	return dst
}

// drawTerrain draws the terrain into dst at world coordinates.
func drawTerrain(dst *image.RGBA, m *Map, tiles []*image.RGBA) {
	for r := 0; r < Rows; r++ {
		for c := 0; c < Cols; c++ {
			x, y := CellOrigin(r, c)
			for _, b := range m.Cells[r][c] {
				if t := Tile(tiles, b); t != nil {
					rect := t.Bounds().Add(image.Pt(x, y))
					draw.Draw(dst, rect, t, t.Bounds().Min, draw.Over)
				}
			}
		}
	}
}

const (
	walkOffset = 0x6400 // section 1: 320 rows x 160 bytes
	WalkRows   = 320
	WalkCols   = 160
)

// WalkGrid is KONUNG.N section 1 on the fine grid (same grid as objects and
// entities). The loader at VA 0x0043C498 expands each byte b to a word:
// 0xFF -> 0x0FFF, (b&0x0F)==0x0F -> (b<<8)|0x0FFF, else b<<8; game code then
// treats (word & 0x0FFF) != 0 as occupied (e.g. VA 0x004119F7).
type WalkGrid [WalkRows][WalkCols]byte

// ParseWalkGrid reads section 1.
func ParseWalkGrid(data []byte) (*WalkGrid, error) {
	if len(data) != FileSize {
		return nil, fmt.Errorf("KONUNG map has size %d, want %d", len(data), FileSize)
	}
	g := &WalkGrid{}
	for r := 0; r < WalkRows; r++ {
		copy(g[r][:], data[walkOffset+r*WalkCols:][:WalkCols])
	}
	return g, nil
}

// Blocked reports whether a fine cell is an obstacle (or off the grid).
func (g *WalkGrid) Blocked(row, col int) bool {
	if row < 0 || row >= WalkRows || col < 0 || col >= WalkCols {
		return true
	}
	b := g[row][col]
	return b == 0xFF || b&0x0F == 0x0F
}

// ArrowBlocked reports a cell that stops arrows: the game's fine-grid word
// is this byte << 8 (plus 0xFFF for walls), and arrows stop at bits 0xC000
// (0x0041130C, 0x00421170), i.e. byte bits 0xC0. Plain walls (0xFFF only,
// e.g. water) let arrows pass.
func (g *WalkGrid) ArrowBlocked(row, col int) bool {
	if row < 0 || row >= WalkRows || col < 0 || col >= WalkCols {
		return true
	}
	return g[row][col]&0xC0 != 0
}
