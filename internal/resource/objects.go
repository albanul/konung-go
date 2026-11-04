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
