package resource

import (
	"encoding/binary"
	"fmt"
	"image"
)

// EntitySprite is one frame of a character/creature as drawn by the game,
// with offsets relative to the entity's world position (fine-cell centre).
type EntitySprite struct {
	Image            *image.RGBA
	ImageDX, ImageDY int
	Shadow           *image.Alpha // nil when the frame has no shadow
	ShadowDX         int
	ShadowDY         int
}

// EntityModel selects the art for an entity, mirroring the model initializer
// at VA 0x00414078 (see docs/konung_game_format.md).
type EntityModel struct {
	Animated      bool   // entity +4 bit 0x40: OBJECTS.RES animated slot; else GRAPH.RES Type 0 block
	Model         int    // entity +0xD6: OBJECTS slot 0..29 or Type 0 block index
	Type, Variant int    // entity +0, +1
	Direction     int    // entity +0x10
	PaletteOffset uint32 // entity +0xD8, GRAPH.RES palette byte offset
}

const (
	type2MetaBytes = 0x3228 // animated OBJECTS block header incl. tag/size
	type0MetaBytes = 8 + 2960*2 + 1536*4 + 3840*2
)

// LoadEntitySprite decodes the frame an entity shows for its type, variant
// and direction. The frame index, image/shadow pair and anchor record are
// located exactly as the model initializer does; the anchor record holds
// int16 shadow dx, dy, image dx, dy.
func LoadEntitySprite(objects, graph []byte, m EntityModel) (*EntitySprite, error) {
	var (
		block                    []byte
		idxOff, pairsOff, recOff int
		pairsEnd                 int // end of the frame-pair table
		dataStart                int
		src                      = "GRAPH.RES Type 0"
	)
	if m.Animated {
		src = "OBJECTS.RES animated"
		if m.Model < 0 || m.Model >= 30 || len(objects) < 8+30*8 {
			return nil, fmt.Errorf("invalid animated object slot %d", m.Model)
		}
		off := binary.LittleEndian.Uint32(objects[8+m.Model*8:])
		size := binary.LittleEndian.Uint32(objects[12+m.Model*8:])
		base := 0x1008 + int(off)
		if size < type2MetaBytes || uint64(base)+uint64(size) > uint64(len(objects)) {
			return nil, fmt.Errorf("OBJECTS.RES slot %d has invalid range", m.Model)
		}
		block = objects[base : base+int(size)]
		t := m.Type
		if t >= 6 { // 0x0041409A
			t -= 6
		}
		idxOff = 0xA + t*0x130 + m.Variant*0x26 + m.Direction*2
		pairsOff, pairsEnd, recOff = 0x728, 0x1728, 0x1728+t*0x480+m.Variant*0x90+m.Direction*8
		dataStart = type2MetaBytes
	} else {
		b, err := type0Block(graph, m.Model)
		if err != nil {
			return nil, err
		}
		block = b
		idxOff = 0xA + m.Type*0xD0 + m.Variant*0x1A + m.Direction*2
		pairsOff, pairsEnd, recOff = 0x1728, 0x2F28, 0x2F28+m.Type*0x300+m.Variant*0x60+m.Direction*8
		dataStart = type0MetaBytes
	}
	if m.Type < 0 || m.Variant < 0 || m.Direction < 0 || idxOff+2 > pairsOff || recOff+8 > dataStart {
		return nil, fmt.Errorf("%s model %d: type %d variant %d dir %d out of range", src, m.Model, m.Type, m.Variant, m.Direction)
	}
	frame := int(int16(binary.LittleEndian.Uint16(block[idxOff:])))
	if frame < 0 || pairsOff+frame*8+8 > pairsEnd {
		return nil, fmt.Errorf("%s model %d: frame index %d out of range", src, m.Model, frame)
	}
	shadowOff := binary.LittleEndian.Uint32(block[pairsOff+frame*8:])
	imageOff := binary.LittleEndian.Uint32(block[pairsOff+frame*8+4:])
	data := block[dataStart:]
	if imageOff == 0xffffffff || int(imageOff) >= len(data) {
		return nil, fmt.Errorf("%s model %d: frame %d has no image", src, m.Model, frame)
	}
	p := int(m.PaletteOffset / 512)
	if p >= 256 || len(graph) < (p+1)*512 {
		return nil, fmt.Errorf("invalid palette offset %#x", m.PaletteOffset)
	}
	palette, err := RGB555Palette(graph[p*512 : (p+1)*512])
	if err != nil {
		return nil, err
	}
	img, err := DecodeIndexedRLEFrame(data[imageOff:], &palette)
	if err != nil {
		img, err = DecodeIndexedRLELegacyFrame(data[imageOff:], &palette)
	}
	if err != nil {
		return nil, fmt.Errorf("%s model %d frame %d: %w", src, m.Model, frame, err)
	}
	word := func(i int) int { return int(int16(binary.LittleEndian.Uint16(block[recOff+i*2:]))) }
	s := &EntitySprite{Image: img, ShadowDX: word(0), ShadowDY: word(1), ImageDX: word(2), ImageDY: word(3)}
	if shadowOff != 0xffffffff && int(shadowOff) < len(data) {
		if s.Shadow, err = DecodeShadowMask(data[shadowOff:]); err != nil {
			s.Shadow = nil // tolerate: the image is still usable
		}
	}
	return s, nil
}

// type0Block returns GRAPH.RES Type 0 block n (0-based), header included.
func type0Block(graph []byte, n int) ([]byte, error) {
	if n < 0 {
		return nil, fmt.Errorf("invalid Type 0 model %d", n)
	}
	off := Type0BlockStart
	for i := 0; ; i++ {
		if off+8 > len(graph) || binary.BigEndian.Uint32(graph[off:]) != 0 {
			return nil, fmt.Errorf("GRAPH.RES Type 0 model %d not found", n)
		}
		size := int(binary.LittleEndian.Uint32(graph[off+4:]))
		if size < type0MetaBytes || off+size > len(graph) {
			return nil, fmt.Errorf("GRAPH.RES Type 0 block %d has invalid size", i)
		}
		if i == n {
			return graph[off : off+size], nil
		}
		off += size
	}
}
