package mapdata

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"sort"

	"konung1.5/internal/resfmt"
)

const (
	decorOffset   = 0x12c00 // section 2: 0x3E80 bytes of 16-byte records
	decorStride   = 0x10
	objectOffset  = 0x16a80 // section 3: 0x8CA0 bytes of 36-byte records
	objectStride  = 0x24
	maxPlacements = 999 // both loaders stop at 0x3E7 records or a -1 first dword

	// Static objects use a grid twice as fine as terrain cells (the same
	// 160x320 grid as section 1): x = col*0x3A - (row odd ? 0x1D : 0), y = row*0x10.
	FineCellWidth  = CellWidth / 2
	FineCellHeight = CellHeight / 2
)

// Decoration is a KONUNG.N section-2 record: a GRAPH.T1 image blitted flat on
// the terrain, top-left at (X, Y) world pixels, right after the terrain pass
// (VA 0x00425A95). Tile is a 0-based GRAPH.T1 slot, unlike terrain bytes.
type Decoration struct {
	Tile, X, Y int
}

// Placement is a KONUNG.N section-3 record: a static OBJECTS.RES image.
type Placement struct {
	Index         int    // record number in section 3 (village building records refer to it)
	ID            int    // OBJECTS.RES slot = ID + 30 (resfmt.MapObjectSlot)
	PaletteOffset uint32 // 0 = use the object header's palette
	Col, Row      int    // fine-grid position
	Flags         byte   // +0x22; bit 0x08 raises the depth-sort baseline by h/4
}

// Pos returns the placement's world anchor (computed by the loader at
// VA 0x0043CC0B); the object header's offsets are added to it.
func (p Placement) Pos() (x, y int) {
	x = p.Col * FineCellWidth
	if p.Row&1 != 0 {
		x -= FineCellWidth / 2
	}
	return x, p.Row * FineCellHeight
}

// ParseDecorations reads section 2 of a KONUNG.N file.
func ParseDecorations(data []byte) ([]Decoration, error) {
	if len(data) != FileSize {
		return nil, fmt.Errorf("KONUNG map has size %d, want %d", len(data), FileSize)
	}
	var out []Decoration
	for i := 0; i < maxPlacements; i++ {
		r := data[decorOffset+i*decorStride:]
		tile := int32(binary.LittleEndian.Uint32(r))
		if tile <= -1 {
			break
		}
		out = append(out, Decoration{
			Tile: int(tile),
			X:    int(int16(binary.LittleEndian.Uint16(r[0xC:]))),
			Y:    int(int16(binary.LittleEndian.Uint16(r[0xE:]))),
		})
	}
	return out, nil
}

// ParsePlacements reads section 3 of a KONUNG.N file.
func ParsePlacements(data []byte) ([]Placement, error) {
	if len(data) != FileSize {
		return nil, fmt.Errorf("KONUNG map has size %d, want %d", len(data), FileSize)
	}
	var out []Placement
	for i := 0; i < maxPlacements; i++ {
		r := data[objectOffset+i*objectStride:]
		id := int32(binary.LittleEndian.Uint32(r))
		if id <= -1 {
			break
		}
		out = append(out, Placement{
			Index:         i,
			ID:            int(id),
			PaletteOffset: binary.LittleEndian.Uint32(r[4:]),
			Col:           int(binary.LittleEndian.Uint16(r[0x1E:])),
			Row:           int(binary.LittleEndian.Uint16(r[0x20:])),
			Flags:         r[0x22],
		})
	}
	return out, nil
}

// ObjectKey identifies a decoded object image (palette overrides differ).
type ObjectKey struct {
	Slot          int
	PaletteOffset uint32
}

// LoadPlacedObjects decodes every distinct object referenced by placements,
// using variant 0 (the loader picks a random variant only for headers with a
// positive +0x18 field; frame animation is not ported). Objects that fail to
// decode are reported in errs and left out of the map.
func LoadPlacedObjects(placements []Placement, objects, graph []byte) (map[ObjectKey]*resfmt.MapObject, []error) {
	out := map[ObjectKey]*resfmt.MapObject{}
	var errs []error
	for _, p := range placements {
		k := ObjectKey{resfmt.MapObjectSlot(p.ID), p.PaletteOffset}
		if _, done := out[k]; done {
			continue
		}
		obj, err := resfmt.LoadMapObject(objects, graph, k.Slot, 0, p.PaletteOffset)
		if err != nil {
			errs = append(errs, fmt.Errorf("object ID %d: %w", p.ID, err))
		}
		out[k] = obj // nil on failure so it is only reported once
	}
	return out, errs
}

// SortedPlacement is a placement resolved to its image, in draw order.
type SortedPlacement struct {
	Placement
	Object *resfmt.MapObject
	X, Y   int // world position of the image's top-left corner
}

// DrawOrder resolves placements and sorts them by the image's bottom edge
// (minus h/4 when flag 0x08 is set), the depth key used by the object pass at
// VA 0x00426A6B. Ties keep file order.
func DrawOrder(placements []Placement, objs map[ObjectKey]*resfmt.MapObject) []SortedPlacement {
	var out []SortedPlacement
	for _, p := range placements {
		o := objs[ObjectKey{resfmt.MapObjectSlot(p.ID), p.PaletteOffset}]
		if o == nil {
			continue
		}
		x, y := p.Pos()
		out = append(out, SortedPlacement{Placement: p, Object: o, X: x + o.ImageDX, Y: y + o.ImageDY})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DepthKey() < out[j].DepthKey() })
	return out
}

// DepthKey is the world-space baseline used for depth sorting.
func (s SortedPlacement) DepthKey() int {
	h := s.Object.Image.Bounds().Dy()
	key := s.Y + h
	if s.Flags&0x08 != 0 {
		key -= h / 4
	}
	return key
}

// Sprite is anything drawn in the depth-sorted pass (static objects and
// entities), with optional shadow. Positions are world pixels (top-left).
type Sprite struct {
	Image  *image.RGBA
	X, Y   int
	Shadow *image.Alpha
	SX, SY int
	Depth  int  // bottom-edge sort key
	Raw    bool // drawn with the raw palette, not the day/night tint (resfmt.MapObject.SelfLit)
	// Flames drawn over the sprite, at world positions (placement position
	// plus the flame offset).
	Flames []PlacedFlame
}

// PlacedFlame is a flame at a world position (top-left of its frame).
type PlacedFlame struct {
	Type, X, Y int
}

// ObjectSprites converts resolved static objects to sprites.
func ObjectSprites(sorted []SortedPlacement) []Sprite {
	out := make([]Sprite, 0, len(sorted))
	for _, s := range sorted {
		px, py := s.Placement.Pos()
		var flames []PlacedFlame
		for _, f := range s.Object.Flames {
			flames = append(flames, PlacedFlame{Type: f.Type, X: px + f.DX, Y: py + f.DY})
		}
		out = append(out, Sprite{
			Flames: flames,
			Image:  s.Object.Image, X: s.X, Y: s.Y,
			Shadow: s.Object.Shadow, SX: px + s.Object.ShadowDX, SY: py + s.Object.ShadowDY,
			Depth: s.DepthKey(), Raw: s.Object.SelfLit,
		})
	}
	return out
}

// SortSprites orders sprites by depth; ties keep their input order.
func SortSprites(s []Sprite) {
	sort.SliceStable(s, func(i, j int) bool { return s[i].Depth < s[j].Depth })
}

// RenderScene composes terrain, decorations, shadows and sprites into one
// image in the game's order. sprites must already be depth-sorted.
func RenderScene(m *Map, tiles []*image.RGBA, decorations []Decoration, sprites []Sprite) *image.RGBA {
	dst := Render(m, tiles)
	for _, d := range decorations {
		if d.Tile < len(tiles) && tiles[d.Tile] != nil {
			t := tiles[d.Tile]
			draw.Draw(dst, t.Bounds().Add(image.Pt(d.X, d.Y)), t, t.Bounds().Min, draw.Over)
		}
	}
	// Union all shadows into one mask, then halve covered pixels once.
	mask := image.NewAlpha(dst.Bounds())
	for _, s := range sprites {
		if s.Shadow != nil {
			r := s.Shadow.Bounds().Add(image.Pt(s.SX, s.SY))
			draw.DrawMask(mask, r, image.Opaque, image.Point{}, s.Shadow, s.Shadow.Bounds().Min, draw.Over)
		}
	}
	draw.DrawMask(dst, dst.Bounds(), image.NewUniform(color.RGBA{0, 0, 0, 0x80}), image.Point{}, mask, image.Point{}, draw.Over)
	for _, s := range sprites {
		draw.Draw(dst, s.Image.Bounds().Add(image.Pt(s.X, s.Y)), s.Image, s.Image.Bounds().Min, draw.Over)
	}
	return dst
}
