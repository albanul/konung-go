package resource

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
)

// ExeTables reads the small lookup tables in konung.exe's DGROUP section that
// the equipment overlay routine (VA 0x004281A4) uses.
type ExeTables struct{ exe []byte }

const (
	dgroupVA   = 0x00450000
	dgroupFile = 0x00038200
	dgroupSize = 0x0015F000

	itemTypeTableVA = 0x00459BEC // 64 bytes per item type; int16 at +2 = overlay index
	drawOrderVA     = 0x00462E1C // 8 facings x 6 int32 slot numbers (peaceful path)
	overlayPalVA    = 0x00465DCC // 24 int32 palette byte offsets, one per overlay block
	overlayCount    = 24
	bodyBlocks      = 3 // GRAPH.RES Type 0 blocks 0..2 are bodies, 3..26 overlays
)

// NewExeTables validates konung.exe enough to read DGROUP tables.
func NewExeTables(exe []byte) (*ExeTables, error) {
	if len(exe) < dgroupFile+dgroupSize || string(exe[:2]) != "MZ" {
		return nil, fmt.Errorf("konung.exe: unexpected size %d or header", len(exe))
	}
	return &ExeTables{exe: exe}, nil
}

func (t *ExeTables) at(va int) []byte { return t.exe[va-dgroupVA+dgroupFile:] }

// ItemOverlay returns the overlay block index (0..23) drawn for an item type,
// or -1 when the item has no visible overlay.
func (t *ExeTables) ItemOverlay(itemType int) int {
	if itemType < 0 || itemType > 255 {
		return -1
	}
	k := int(int16(binary.LittleEndian.Uint16(t.at(itemTypeTableVA + itemType*64 + 2))))
	if k >= overlayCount {
		return -1
	}
	return k
}

// DrawOrder returns the equipment slot draw order for a facing (variant).
// This is the table used when entity +3 bit 0x04 is clear (VA 0x00428484).
func (t *ExeTables) DrawOrder(variant int) [6]int {
	var out [6]int
	b := t.at(drawOrderVA + (variant&7)*0x18)
	for i := range out {
		out[i] = int(int32(binary.LittleEndian.Uint32(b[i*4:])))
	}
	return out
}

// OverlayPalette returns overlay block k's fixed palette byte offset (set by
// the GRAPH.RES loader at VA 0x0043B797).
func (t *ExeTables) OverlayPalette(k int) uint32 {
	return binary.LittleEndian.Uint32(t.at(overlayPalVA + k*4))
}

// ComposeEquipment draws equipment overlays over a Type 0 body frame, as the
// routine at VA 0x004281A4 does after the body blit. overlays lists overlay
// block indices in draw order. Each overlay frame is positioned at the body's
// per-step attachment point (int16 x, y at body+0x828+type*0x180+variant*0x30
// +step*4) plus the overlay's own anchor record (image dx, dy). The result is
// a new sprite whose image covers body and overlays; offsets stay relative to
// the entity's world position.
func ComposeEquipment(graph []byte, body *EntitySprite, m EntityModel, overlays []int, t *ExeTables) (*EntitySprite, error) {
	if m.Animated || len(overlays) == 0 {
		return body, nil
	}
	bb, err := type0Block(graph, m.Model)
	if err != nil {
		return nil, err
	}
	att := 0x828 + m.Type*0x180 + m.Variant*0x30 + m.Step*4
	if att+4 > 0x1728 {
		return nil, fmt.Errorf("attachment record out of range")
	}
	ax := int(int16(binary.LittleEndian.Uint16(bb[att:])))
	ay := int(int16(binary.LittleEndian.Uint16(bb[att+2:])))

	type layer struct {
		img  *image.RGBA
		x, y int
	}
	layers := []layer{{body.Image, body.ImageDX, body.ImageDY}}
	for _, k := range overlays {
		if k < 0 || k >= overlayCount {
			continue
		}
		ob, err := type0Block(graph, bodyBlocks+k)
		if err != nil {
			return nil, err
		}
		idx := int(int16(binary.LittleEndian.Uint16(ob[0xA+m.Type*0xD0+m.Variant*0x1A+m.Step*2:])))
		if idx < 0 || 0x1728+idx*8+8 > 0x2F28 {
			continue
		}
		imgOff := binary.LittleEndian.Uint32(ob[0x1728+idx*8+4:])
		data := ob[type0MetaBytes:]
		if imgOff == 0xffffffff || int(imgOff) >= len(data) {
			continue
		}
		p := int(t.OverlayPalette(k) / 512)
		if p >= 256 {
			continue
		}
		pal, err := RGB555Palette(graph[p*512 : (p+1)*512])
		if err != nil {
			return nil, err
		}
		img, err := DecodeIndexedRLEFrame(data[imgOff:], &pal)
		if err != nil {
			if img, err = DecodeIndexedRLELegacyFrame(data[imgOff:], &pal); err != nil {
				return nil, fmt.Errorf("overlay %d frame %d: %w", k, idx, err)
			}
		}
		rec := 0x2F28 + m.Type*0x300 + m.Variant*0x60 + m.Step*8
		dx := int(int16(binary.LittleEndian.Uint16(ob[rec+4:])))
		dy := int(int16(binary.LittleEndian.Uint16(ob[rec+6:])))
		layers = append(layers, layer{img, ax + dx, ay + dy})
	}
	bounds := image.Rectangle{}
	for i, l := range layers {
		r := l.img.Bounds().Add(image.Pt(l.x, l.y))
		if i == 0 {
			bounds = r
		} else {
			bounds = bounds.Union(r)
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for _, l := range layers {
		r := l.img.Bounds().Add(image.Pt(l.x-bounds.Min.X, l.y-bounds.Min.Y))
		draw.Draw(dst, r, l.img, l.img.Bounds().Min, draw.Over)
	}
	out := *body
	out.Image, out.ImageDX, out.ImageDY = dst, bounds.Min.X, bounds.Min.Y
	return &out, nil
}
