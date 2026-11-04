package resource

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
	const tableOffset, dataOffset, headerBytes = 8, 0x1008, 0x100
	if objectIndex < 1 || objectIndex > 512 || len(objects) < tableOffset+512*8 {
		return nil, fmt.Errorf("OBJECTS.RES index table truncated")
	}
	i := tableOffset + (objectIndex-1)*8
	blockOffset := binary.LittleEndian.Uint32(objects[i:])
	blockSize := binary.LittleEndian.Uint32(objects[i+4:])
	base := dataOffset + int(blockOffset)
	if base < dataOffset || blockSize < headerBytes || uint64(base)+uint64(blockSize) > uint64(len(objects)) {
		return nil, fmt.Errorf("OBJECTS.RES object %d has invalid range", objectIndex)
	}
	if binary.BigEndian.Uint32(objects[base:]) == 2 {
		return nil, fmt.Errorf("OBJECTS.RES object %d is animated, want a static image", objectIndex)
	}
	paletteOffset := binary.LittleEndian.Uint32(objects[base+20:])
	paletteIndex := paletteOffset / 512
	if paletteIndex >= 256 || len(graph) < int(paletteIndex+1)*512 {
		return nil, fmt.Errorf("OBJECTS.RES object %d has invalid palette offset %#x", objectIndex, paletteOffset)
	}
	palette, err := RGB555Palette(graph[int(paletteIndex)*512 : int(paletteIndex+1)*512])
	if err != nil {
		return nil, err
	}
	img, err := DecodeIndexedRLEFrame(objects[base+headerBytes:base+int(blockSize)], &palette)
	if err != nil {
		return nil, fmt.Errorf("decode OBJECTS.RES object %d: %w", objectIndex, err)
	}
	return img, nil
}

// Type0BlockStart is the file offset of the first of 27 GRAPH.RES Type 0
// character-model blocks.
const Type0BlockStart = 0x1ee8b6

// DecodeType0Frames decodes the first count image frames of GRAPH.RES Type 0
// model block modelID (0-based) using palette paletteID. The runtime palette
// (dye) selection is not traced; palette 2 is the project's preview choice.
func DecodeType0Frames(graph []byte, modelID, count, paletteID int) ([]*image.RGBA, error) {
	const (
		blockStart          = Type0BlockStart
		metadataBytes       = 2960*2 + 1536*4 + 3840*2
		metadataOffsetTable = 2960 * 2
	)
	if modelID < 0 || count < 1 || paletteID < 0 || paletteID > 255 || blockStart+8 > len(graph) {
		return nil, fmt.Errorf("invalid Type 0 model request")
	}
	off := blockStart
	for id := 0; id < modelID; id++ {
		if off+8 > len(graph) || binary.BigEndian.Uint32(graph[off:]) != 0 {
			return nil, fmt.Errorf("GRAPH.RES Type 0 block %d has invalid header", id)
		}
		size := int(binary.LittleEndian.Uint32(graph[off+4:]))
		if size < 8 || off+size > len(graph) {
			return nil, fmt.Errorf("GRAPH.RES Type 0 block %d has invalid size", id)
		}
		off += size
	}
	if off+8 > len(graph) || binary.BigEndian.Uint32(graph[off:]) != 0 {
		return nil, fmt.Errorf("GRAPH.RES Type 0 model %d not found", modelID)
	}
	blockSize := int(binary.LittleEndian.Uint32(graph[off+4:]))
	if blockSize < 8+metadataBytes || off+blockSize > len(graph) {
		return nil, fmt.Errorf("GRAPH.RES Type 0 model %d has invalid size", modelID)
	}
	if len(graph) < (paletteID+1)*512 {
		return nil, fmt.Errorf("GRAPH.RES palettes are truncated")
	}
	palette, err := RGB555Palette(graph[paletteID*512 : (paletteID+1)*512])
	if err != nil {
		return nil, err
	}
	dataStart := off + 8 + metadataBytes
	dataBytes := blockSize - 8 - metadataBytes
	offsetsAt := off + 8 + metadataOffsetTable
	offsets := make([]uint32, count*2+2)
	for i := range offsets {
		at := offsetsAt + i*4
		if at+4 > off+8+metadataBytes {
			return nil, fmt.Errorf("GRAPH.RES Type 0 offset table truncated")
		}
		offsets[i] = binary.LittleEndian.Uint32(graph[at:])
	}
	for i := 0; i+1 < len(offsets); i += 2 {
		offsets[i], offsets[i+1] = offsets[i+1], offsets[i]
	}
	frames := make([]*image.RGBA, 0, count)
	for i := 0; i < count; i++ {
		start := offsets[i*2]
		end := uint32(dataBytes)
		if i+1 < count {
			end = offsets[(i+1)*2]
		}
		if end <= start || uint64(end) > uint64(dataBytes) {
			return nil, fmt.Errorf("GRAPH.RES Type 0 model %d frame %d has invalid range", modelID, i)
		}
		frameData := graph[dataStart+int(start) : dataStart+int(end)]
		img, decodeErr := DecodeIndexedRLEFrame(frameData, &palette)
		if decodeErr != nil {
			img, decodeErr = DecodeIndexedRLELegacyFrame(frameData, &palette)
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("decode GRAPH.RES Type 0 model %d frame %d: %w", modelID, i, decodeErr)
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
	const tableOffset, dataOffset, headerBytes = 8, 0x1008, 0x100
	if slot < 30 || slot >= 512 || variant < 0 || variant > 7 || len(objects) < tableOffset+512*8 {
		return nil, fmt.Errorf("invalid static object slot %d variant %d", slot, variant)
	}
	blockOffset := binary.LittleEndian.Uint32(objects[tableOffset+slot*8:])
	blockSize := binary.LittleEndian.Uint32(objects[tableOffset+slot*8+4:])
	base := dataOffset + int(blockOffset)
	if blockOffset == 0xffffffff || blockSize < headerBytes || uint64(base)+uint64(blockSize) > uint64(len(objects)) {
		return nil, fmt.Errorf("OBJECTS.RES slot %d has invalid range", slot)
	}
	hdr := objects[base : base+headerBytes]
	data := objects[base+headerBytes : base+int(blockSize)]
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
