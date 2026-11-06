package resfmt

import (
	"encoding/binary"
	"image"
	"image/draw"
)

// World map window (window 5, HUD button 2 «Карта (M)»). The draw
// 0x00428698 blits INTR slot WorldMapSlot at the view's top-left, then, for
// every location the party knows (flag byte 0x00A3E0A8[map − 1]), blits INTR
// slot WorldMapRevealSlot at the area's top-left clipped to that location's
// row runs. The current map's runs use the reveal palette tinted (24, 0, 0)
// (0x0043FD18, 0x0043F763).
//
// Exe tables (DGROUP):
//
//	0x0046280C  per video mode, 4 × int16: area x, y (from the view's
//	            top-left), width, height
//	0x00456A7C  int32 per area row: byte offset of the row in the run data,
//	            −1 = no row (one −1 after the last row)
//	0x00450F1C  run data: per row (region, length) byte pairs; length 0 ends
//	            the row, and that pair's region covers the rest of it
//
// A region is map − 1. The hit test 0x00439A28 returns the region whose run
// contains x (relative to the area); a row of −1 gives region 0. Over a
// known region the hotspot id is region << 8 + 0x103 (0x00439526), so
// id >> 8 is the map number; over an unknown one it is −1.
const (
	WorldMapSlot       = 4
	WorldMapRevealSlot = 179
	// WorldMapCurrentTint is the current map's reveal tint (0x004286DC).
	WorldMapCurrentTint = 24
	// WorldMapRegions is the size of the known-map flags 0x00A3E0A8.
	WorldMapRegions = 0x38

	worldMapAreaVA = 0x0046280C
	worldMapRowsVA = 0x00456A7C
	worldMapRunsVA = 0x00450F1C
)

// WorldMapRun is one run of a row: region covers [X0, X1) relative to the
// area. Last marks the row's end pair, which the draw extends to the view's
// right edge.
type WorldMapRun struct {
	Region, X0, X1 int
	Last           bool
}

// WorldMap is the map window's area and its per-row region runs.
type WorldMap struct {
	Area image.Rectangle // relative to the view's top-left
	Rows [][]WorldMapRun // nil = no row
}

// WorldMap reads the map window tables for a video mode (0 = 640×480,
// 1 = 800×600).
func (t *ExeTables) WorldMap(mode int) WorldMap {
	a := t.at(worldMapAreaVA + 8*mode)
	i16 := func(k int) int { return int(int16(binary.LittleEndian.Uint16(a[2*k:]))) }
	x, y, w, h := i16(0), i16(1), i16(2), i16(3)
	m := WorldMap{Area: image.Rect(x, y, x+w, y+h), Rows: make([][]WorldMapRun, h)}
	for r := range h {
		off := t.i32(worldMapRowsVA + 4*r)
		if off < 0 {
			continue
		}
		p := t.at(worldMapRunsVA + off)
		run := []WorldMapRun{}
		for x := 0; ; p = p[2:] {
			region, n := int(p[0]), int(p[1])
			if n == 0 {
				run = append(run, WorldMapRun{Region: region, X0: x, X1: w, Last: true})
				break
			}
			run = append(run, WorldMapRun{Region: region, X0: x, X1: x + n})
			x += n
		}
		m.Rows[r] = run
	}
	return m
}

// RegionAt is the hit test 0x00439A28 at (x, y) relative to the area: the
// region of the run containing x, the row's end pair past the last run, 0
// for a row without runs. ok is false outside the area (the caller
// 0x0043946E requires 0 < x < width and 0 < y < height).
func (m *WorldMap) RegionAt(x, y int) (region int, ok bool) {
	if x <= 0 || y <= 0 || x >= m.Area.Dx() || y >= m.Area.Dy() {
		return 0, false
	}
	for _, r := range m.Rows[y] {
		if x < r.X1 || r.Last {
			return r.Region, true
		}
	}
	return 0, true
}

// Compose draws the known regions of the reveal image into an image the
// size of the area, transparent elsewhere: the clipped blits of 0x00428698.
// The runs of region here (the current map − 1) come from current (the
// reveal image with the tinted palette); a row's end pair extends to the
// view's right edge in the game, which the area-sized reveal image never
// reaches.
func (m *WorldMap) Compose(reveal, current image.Image, known func(region int) bool, here int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, m.Area.Dx(), m.Area.Dy()))
	for y, row := range m.Rows {
		for _, r := range row {
			if r.X0 >= r.X1 || !known(r.Region) {
				continue
			}
			src := reveal
			if r.Region == here {
				src = current
			}
			rect := image.Rect(r.X0, y, r.X1, y+1)
			draw.Draw(out, rect, src, rect.Min.Add(src.Bounds().Min), draw.Over)
		}
	}
	return out
}
