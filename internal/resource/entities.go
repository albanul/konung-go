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
	Type          int    // entity +0: action / animation set
	Variant       int    // entity +1: facing direction (8 variants)
	Step          int    // entity +0x10: animation step
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
		idxOff = 0xA + t*0x130 + m.Variant*0x26 + m.Step*2
		pairsOff, pairsEnd, recOff = 0x728, 0x1728, 0x1728+t*0x480+m.Variant*0x90+m.Step*8
		dataStart = type2MetaBytes
	} else {
		b, err := type0Block(graph, m.Model)
		if err != nil {
			return nil, err
		}
		block = b
		idxOff = 0xA + m.Type*0xD0 + m.Variant*0x1A + m.Step*2
		pairsOff, pairsEnd, recOff = 0x1728, 0x2F28, 0x2F28+m.Type*0x300+m.Variant*0x60+m.Step*8
		dataStart = type0MetaBytes
	}
	if m.Type < 0 || m.Variant < 0 || m.Step < 0 || idxOff+2 > pairsOff || recOff+8 > dataStart {
		return nil, fmt.Errorf("%s model %d: type %d variant %d step %d out of range", src, m.Model, m.Type, m.Variant, m.Step)
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

// AnimationLength returns the number of steps in the entity's current
// type/variant sequence: the first word of its record, which precedes the
// per-step frame indices (12 steps for the idle sets seen so far). The game
// advances entity +0x10 once per animation tick until it reaches the count
// held in entity +0xD7 (VA 0x00413DE4).
func AnimationLength(objects, graph []byte, m EntityModel) (int, error) {
	m.Step = 0
	var block []byte
	var off, maxSteps int
	if m.Animated {
		if m.Model < 0 || m.Model >= 30 || len(objects) < 8+30*8 {
			return 0, fmt.Errorf("invalid animated object slot %d", m.Model)
		}
		o := 0x1008 + int(binary.LittleEndian.Uint32(objects[8+m.Model*8:]))
		size := int(binary.LittleEndian.Uint32(objects[12+m.Model*8:]))
		if size < type2MetaBytes || o+size > len(objects) {
			return 0, fmt.Errorf("OBJECTS.RES slot %d has invalid range", m.Model)
		}
		block = objects[o : o+size]
		t := m.Type
		if t >= 6 {
			t -= 6
		}
		off, maxSteps = 8+t*0x130+m.Variant*0x26, 18
	} else {
		b, err := type0Block(graph, m.Model)
		if err != nil {
			return 0, err
		}
		block, off, maxSteps = b, 8+m.Type*0xD0+m.Variant*0x1A, 12
	}
	if off < 0 || off+2 > len(block) {
		return 0, fmt.Errorf("animation record out of range")
	}
	n := int(int16(binary.LittleEndian.Uint16(block[off:])))
	return max(1, min(n, maxSteps)), nil
}

// LoadEntityAnimation decodes every step of the entity's current sequence.
// Steps that repeat a frame share the same decoded images.
func LoadEntityAnimation(objects, graph []byte, m EntityModel) ([]*EntitySprite, error) {
	n, err := AnimationLength(objects, graph, m)
	if err != nil {
		return nil, err
	}
	out := make([]*EntitySprite, 0, n)
	for step := 0; step < n; step++ {
		m.Step = step
		s, err := LoadEntitySprite(objects, graph, m)
		if err != nil {
			return nil, err
		}
		if k := len(out); k > 0 && sameFrame(out[k-1], s) {
			s.Image, s.Shadow = out[k-1].Image, out[k-1].Shadow
		}
		out = append(out, s)
	}
	return out, nil
}

func sameFrame(a, b *EntitySprite) bool {
	return a.ImageDX == b.ImageDX && a.ImageDY == b.ImageDY && a.Image.Bounds() == b.Image.Bounds() &&
		string(a.Image.Pix) == string(b.Image.Pix)
}
