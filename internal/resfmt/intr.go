package resfmt

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
)

// Interface resources INTR640R.RES / Intr800r.res (loader near VA 0x0042D21A:
// "INTR" + screen width + "R.RES"). Layout: big-endian tag 1, little-endian
// data size, 512 x (uint32 image offset, uint32 palette byte offset) read to
// VA 0x006304B8, then the image data read to [0x00A3E6B8]. Offsets are
// relative to the data start (file offset 0x1008); 0xFFFFFFFF marks an
// unused slot. Every image is indexed row-RLE drawn with the GRAPH.RES
// palette at the given byte offset (palette index = offset / 0x200), as the
// HUD draws push [0x0051ECB4] + palette offset.
const (
	interfaceSlots = 512
	interfaceData  = 8 + interfaceSlots*8
	paletteBytes   = 0x200
)

// InterfaceRes is a parsed INTR*R.RES file.
type InterfaceRes struct {
	data  []byte
	table []byte
}

// ParseInterface checks the header and slot table of an INTR*R.RES file.
func ParseInterface(file []byte) (*InterfaceRes, error) {
	if len(file) < interfaceData || binary.BigEndian.Uint32(file) != 1 {
		return nil, fmt.Errorf("not an interface resource")
	}
	if size := binary.LittleEndian.Uint32(file[4:]); int(size) != len(file)-interfaceData {
		return nil, fmt.Errorf("interface data size %d, file has %d", size, len(file)-interfaceData)
	}
	return &InterfaceRes{data: file[interfaceData:], table: file[8:interfaceData]}, nil
}

// Slot returns the raw image and palette byte offset of slot (0-based).
func (r *InterfaceRes) Slot(slot int) (img []byte, paletteOffset uint32, ok bool) {
	if slot < 0 || slot >= interfaceSlots {
		return nil, 0, false
	}
	off := binary.LittleEndian.Uint32(r.table[slot*8:])
	pal := binary.LittleEndian.Uint32(r.table[slot*8+4:])
	if off == 0xFFFFFFFF || int(off)+4 > len(r.data) {
		return nil, 0, false
	}
	return r.data[off:], pal, true
}

// Image decodes slot with its palette from palettes (resfmt.Palettes of
// GRAPH.RES).
func (r *InterfaceRes) Image(slot int, palettes [][256]color.RGBA) (*image.RGBA, error) {
	src, pal, ok := r.Slot(slot)
	if !ok {
		return nil, fmt.Errorf("interface slot %d is empty", slot)
	}
	p := int(pal / paletteBytes)
	if pal%paletteBytes != 0 || p >= len(palettes) {
		return nil, fmt.Errorf("interface slot %d: palette offset %#x", slot, pal)
	}
	return DecodeIndexedRLEFrame(src, &palettes[p])
}
