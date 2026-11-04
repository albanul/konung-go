package resfmt

import (
	"encoding/binary"
	"fmt"
	"image"
)

// DecodeStaticObject decodes a static OBJECTS.RES entry. objectIndex is
// 1-based; the loader treats 0-based table slots >= 30 as static. The block has a
// 0x100-byte header whose dword at +20 is a GRAPH.RES palette byte offset;
// indexed-RLE image data follows. graph supplies the palettes.
func DecodeStaticObject(objects, graph []byte, objectIndex int) (*image.RGBA, error) {
	const headerBytes = 0x100
	if objectIndex < 1 {
		return nil, fmt.Errorf("invalid OBJECTS.RES object %d", objectIndex)
	}
	block, _, err := ObjectBlock(objects, objectIndex-1)
	if err != nil {
		return nil, err
	}
	if len(block) < headerBytes {
		return nil, fmt.Errorf("OBJECTS.RES object %d is too small", objectIndex)
	}
	if binary.BigEndian.Uint32(block) == 2 {
		return nil, fmt.Errorf("OBJECTS.RES object %d is animated, want a static image", objectIndex)
	}
	paletteOffset := binary.LittleEndian.Uint32(block[0x14:])
	paletteIndex := paletteOffset / 512
	if paletteIndex >= 256 || len(graph) < int(paletteIndex+1)*512 {
		return nil, fmt.Errorf("OBJECTS.RES object %d has invalid palette offset %#x", objectIndex, paletteOffset)
	}
	palette, err := RGB555Palette(graph[int(paletteIndex)*512 : int(paletteIndex+1)*512])
	if err != nil {
		return nil, err
	}
	img, err := DecodeIndexedRLEFrame(block[headerBytes:], &palette)
	if err != nil {
		return nil, fmt.Errorf("decode OBJECTS.RES object %d: %w", objectIndex, err)
	}
	return img, nil
}

// DecodeType0Frames decodes the first count image frames of GRAPH.RES Type 0
// block n (0-based) in stored order, using palette paletteID. For drawing
// characters use LoadEntitySprite, which selects frames like the game.
func DecodeType0Frames(graph []byte, n, count, paletteID int) ([]*image.RGBA, error) {
	if count < 1 || paletteID < 0 || paletteID > 255 || len(graph) < (paletteID+1)*512 {
		return nil, fmt.Errorf("invalid Type 0 frame request")
	}
	block, err := type0Block(graph, n)
	if err != nil {
		return nil, err
	}
	palette, err := RGB555Palette(graph[paletteID*512 : (paletteID+1)*512])
	if err != nil {
		return nil, err
	}
	data := block[Type0DataOffset:]
	ranges := FrameRanges(block[Type0PairsOffset:Type0PairsOffset+768*8], len(data))
	if len(ranges) < count {
		return nil, fmt.Errorf("GRAPH.RES Type 0 block %d has %d frames, want %d", n, len(ranges), count)
	}
	frames := make([]*image.RGBA, 0, count)
	for i, r := range ranges[:count] {
		if r.End <= r.Start || int(r.End) > len(data) {
			return nil, fmt.Errorf("GRAPH.RES Type 0 block %d frame %d has invalid range", n, i)
		}
		img, err := DecodeIndexedRLEFrame(data[r.Start:r.End], &palette)
		if err != nil {
			img, err = DecodeIndexedRLELegacyFrame(data[r.Start:r.End], &palette)
		}
		if err != nil {
			return nil, fmt.Errorf("decode GRAPH.RES Type 0 block %d frame %d: %w", n, i, err)
		}
		frames = append(frames, img)
	}
	return frames, nil
}

// MapObject is a static OBJECTS.RES image as placed by KONUNG.N section 3,
// with its shadow mask and the per-variant draw offsets from the 0x100-byte
// header (see docs/konung_map_format.md, "Static object placements").
type MapObject struct {
	Image            *image.RGBA
	ImageDX, ImageDY int          // added to the placement position (header +0xC4/+0xC6 + 8*variant)
	Shadow           *image.Alpha // nil when the variant has no shadow or the header disables it
	ShadowDX         int          // header +0xC0 + 8*variant
	ShadowDY         int          // header +0xC2 + 8*variant
}

// MapObjectSlot converts a KONUNG.N section-3 object ID to the 0-based
// OBJECTS.RES table slot (the loader at VA 0x0043CBD6 adds 0x1E).
func MapObjectSlot(id int) int { return id + 30 }

// LoadMapObject decodes static OBJECTS.RES slot (0-based) for the given
// variant. paletteOffset overrides the header palette when non-zero, matching
// the map loader (0x0043CBF6). Image offsets in the header are relative to the
// data that follows the 0x100-byte header.
func LoadMapObject(objects, graph []byte, slot, variant int, paletteOffset uint32) (*MapObject, error) {
	const headerBytes = 0x100
	if slot < AnimatedObjects || variant < 0 || variant > 7 {
		return nil, fmt.Errorf("invalid static object slot %d variant %d", slot, variant)
	}
	block, _, err := ObjectBlock(objects, slot)
	if err != nil {
		return nil, err
	}
	if len(block) < headerBytes {
		return nil, fmt.Errorf("OBJECTS.RES slot %d is too small", slot)
	}
	hdr := block[:headerBytes]
	data := block[headerBytes:]
	if paletteOffset == 0 {
		paletteOffset = binary.LittleEndian.Uint32(hdr[0x14:])
	}
	p := int(paletteOffset / 512)
	if p >= 256 || len(graph) < (p+1)*512 {
		return nil, fmt.Errorf("OBJECTS.RES slot %d has invalid palette offset %#x", slot, paletteOffset)
	}
	palette, err := RGB555Palette(graph[p*512 : (p+1)*512])
	if err != nil {
		return nil, err
	}
	shadowOff := binary.LittleEndian.Uint32(hdr[0x80+variant*8:])
	imageOff := binary.LittleEndian.Uint32(hdr[0x84+variant*8:])
	if imageOff == 0xffffffff || int(imageOff) >= len(data) {
		return nil, fmt.Errorf("OBJECTS.RES slot %d variant %d has no image", slot, variant)
	}
	img, err := DecodeIndexedRLEFrame(data[imageOff:], &palette)
	if err != nil {
		return nil, fmt.Errorf("decode OBJECTS.RES slot %d image: %w", slot, err)
	}
	word := func(off int) int { return int(int16(binary.LittleEndian.Uint16(hdr[off+variant*8:]))) }
	obj := &MapObject{Image: img, ImageDX: word(0xC4), ImageDY: word(0xC6), ShadowDX: word(0xC0), ShadowDY: word(0xC2)}
	// The draw pass at 0x004268C3 skips the shadow when its offset is -1 or the
	// header dword at +8 is >= 2.
	if shadowOff != 0xffffffff && int32(binary.LittleEndian.Uint32(hdr[8:])) < 2 && int(shadowOff) < len(data) {
		if obj.Shadow, err = DecodeShadowMask(data[shadowOff:]); err != nil {
			return nil, fmt.Errorf("decode OBJECTS.RES slot %d shadow: %w", slot, err)
		}
	}
	return obj, nil
}

// DecodeShadowMask decodes a shadow image: the indexed row-RLE header and
// row sizes, but a positive command is a run of shadow pixels with no pixel
// payload (span collector at VA 0x0043D3D8). The game halves the RGB of every
// covered screen pixel once (0x0043E5DF), so overlapping shadows don't stack.
func DecodeShadowMask(src []byte) (*image.Alpha, error) {
	if len(src) < 4 {
		return nil, fmt.Errorf("shadow header truncated")
	}
	w := int(binary.LittleEndian.Uint16(src))
	h := int(binary.LittleEndian.Uint16(src[2:]))
	if w < 1 || h < 1 || len(src) < 4+h*2 {
		return nil, fmt.Errorf("invalid shadow dimensions %dx%d", w, h)
	}
	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	pos := 4 + h*2
	for y := 0; y < h; y++ {
		end := pos + int(binary.LittleEndian.Uint16(src[4+y*2:]))
		if end > len(src) {
			return nil, fmt.Errorf("shadow row %d truncated", y)
		}
		for x := 0; pos < end; {
			n := int(src[pos])
			pos++
			if n == 0 {
				break
			}
			if n&0x80 != 0 {
				x += n & 0x7f
				continue
			}
			for i := 0; i < n && x < w; i++ {
				mask.Pix[y*mask.Stride+x] = 0xff
				x++
			}
		}
		pos = end
	}
	return mask, nil
}
