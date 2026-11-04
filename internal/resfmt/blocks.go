package resfmt

import (
	"encoding/binary"
	"fmt"
	"image/color"
)

// Shared block lookup for OBJECTS.RES and GRAPH.RES Type 0 data, plus the
// paired image/shadow offset tables both formats use.

const (
	objectsTable    = 8      // 512 x (offset, size) entries follow the 8-byte header
	objectsData     = 0x1008 // object offsets are relative to this file offset
	objectsSlots    = 512
	AnimatedObjects = 30 // 0-based slots 0..29 are animated (Type 2); the rest are static

	// Type0BlockStart is the file offset of the first of 27 GRAPH.RES Type 0
	// blocks (3 bodies, then 24 equipment overlays).
	Type0BlockStart = 0x1ee8b6
)

// ObjectBlock returns OBJECTS.RES table slot (0-based) as stored in the file,
// header included, and its file offset.
func ObjectBlock(objects []byte, slot int) (block []byte, offset int, err error) {
	if slot < 0 || slot >= objectsSlots || len(objects) < objectsTable+objectsSlots*8 {
		return nil, 0, fmt.Errorf("invalid OBJECTS.RES slot %d", slot)
	}
	rel := binary.LittleEndian.Uint32(objects[objectsTable+slot*8:])
	size := binary.LittleEndian.Uint32(objects[objectsTable+slot*8+4:])
	base := objectsData + uint64(rel)
	if rel == 0xffffffff || uint64(base)+uint64(size) > uint64(len(objects)) {
		return nil, 0, fmt.Errorf("OBJECTS.RES slot %d has invalid range", slot)
	}
	return objects[base : base+uint64(size)], int(base), nil
}

// Type0Block is one GRAPH.RES Type 0 block (header included) and its offset.
type Type0Block struct {
	Offset int
	Data   []byte
}

// Type0Blocks walks every GRAPH.RES Type 0 block (big-endian tag 0, size
// including the header) from Type0BlockStart to EOF.
func Type0Blocks(graph []byte) ([]Type0Block, error) {
	var out []Type0Block
	for off := Type0BlockStart; off < len(graph); {
		if off+8 > len(graph) || binary.BigEndian.Uint32(graph[off:]) != 0 {
			return nil, fmt.Errorf("GRAPH.RES Type 0 block %d has invalid header at %#x", len(out), off)
		}
		size := int(binary.LittleEndian.Uint32(graph[off+4:]))
		if size < type0MetaBytes || off+size > len(graph) {
			return nil, fmt.Errorf("GRAPH.RES Type 0 block %d has invalid size %#x at %#x", len(out), size, off)
		}
		out = append(out, Type0Block{Offset: off, Data: graph[off : off+size]})
		off += size
	}
	return out, nil
}

// type0Block returns GRAPH.RES Type 0 block n (0-based), header included.
func type0Block(graph []byte, n int) ([]byte, error) {
	if n < 0 {
		return nil, fmt.Errorf("invalid Type 0 block %d", n)
	}
	off := Type0BlockStart
	for i := 0; ; i++ {
		if off+8 > len(graph) || binary.BigEndian.Uint32(graph[off:]) != 0 {
			return nil, fmt.Errorf("GRAPH.RES Type 0 block %d not found", n)
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

// FrameRange is one image frame's byte range within a block's image data.
type FrameRange struct{ Start, End uint32 }

// FrameRanges lists the image frames of a paired offset table: each 8-byte
// entry is (shadow offset, image offset); the list stops at the first image
// offset of 0xFFFFFFFF. A frame ends where the next one starts, or at
// dataLen. Callers validate Start < End <= dataLen.
func FrameRanges(table []byte, dataLen int) []FrameRange {
	var out []FrameRange
	for i := 0; i+8 <= len(table); i += 8 {
		start := binary.LittleEndian.Uint32(table[i+4:])
		if start == 0xffffffff {
			break
		}
		end := uint32(dataLen)
		if i+16 <= len(table) {
			if next := binary.LittleEndian.Uint32(table[i+12:]); next != 0xffffffff {
				end = next
			}
		}
		out = append(out, FrameRange{start, end})
	}
	return out
}

// Type 2 (animated OBJECTS.RES) and Type 0 table locations within a block.
const (
	Type2PairsOffset = 0x728 // 512 pairs
	Type2DataOffset  = type2MetaBytes
	Type0PairsOffset = 0x1728 // 768 pairs
	Type0DataOffset  = type0MetaBytes
)

// Palettes decodes all 256 GRAPH.RES palettes.
func Palettes(graph []byte) ([][256]color.RGBA, error) {
	if len(graph) < 256*512 {
		return nil, fmt.Errorf("GRAPH.RES palette table truncated")
	}
	out := make([][256]color.RGBA, 256)
	for i := range out {
		p, err := RGB555Palette(graph[i*512 : (i+1)*512])
		if err != nil {
			return nil, err
		}
		out[i] = p
	}
	return out, nil
}
