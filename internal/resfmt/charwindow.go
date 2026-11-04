package resfmt

import (
	"encoding/binary"
	"image"
)

// CharWindow is the screen layout of the character window (window 3, drawn
// by 0x0042E04C) for 800x600, read from konung.exe. Each table entry holds
// (x, y) for 640x480 then for 800x600; the port reads the 800 column.
type CharWindow struct {
	// Equipment[slot] is the icon cell of equipment slot 0–5: the icon is
	// centred horizontally in a bag-cell width from X; Y is its top
	// (slot → position index 0x0046573C, positions 0x004657B4).
	Equipment [6]image.Point
	ItemCA    image.Point    // top-left of the item in entity word +0xCA (0x00465804)
	Extras    [5]image.Point // centres of the five items at +0xB2 (0x00465814)
	Stats     [25]image.Point
	// Stats are the top-left corners of the 25 numbers (0x00465874), in the
	// order of gamestate.CharSheet.
	Label image.Point // left edge and top of the centred label (0x00465754)
	// NameY and TitleY are the text tops of the name and the title
	// (0x0042E131: 10·H/640; 0x0042E1F3: (0x00465778 + 10 − 0x00465768 + 4)·H/640,
	// H = 0x00A3E838 = 600).
	NameY, TitleY int
}

// Background is window 3's INTR slot, drawn at the game view's top-left.
const CharWindowBackground = 2

// CharWindowTitlesVA holds 16-byte titles indexed by portrait face − 1
// (entity +4 & 7): «Храбрец», «Купец», «Охотник», «Вождь», «Воин».
const CharWindowTitlesVA = 0x0045AE20

const screenHeight800 = 600 // 0x00A3E838 in 800x600 mode (0x00427C24)

// CharWindow800 reads the 800x600 layout tables.
func (t *ExeTables) CharWindow800() CharWindow {
	i32 := func(va int) int { return int(int32(binary.LittleEndian.Uint32(t.at(va)))) }
	pt := func(va int) image.Point { return image.Pt(i32(va+8), i32(va+12)) } // 800 column
	var w CharWindow
	for slot := range w.Equipment {
		w.Equipment[slot] = pt(0x004657B4 + i32(0x0046573C+slot*4)*16)
	}
	w.ItemCA = pt(0x00465804)
	for i := range w.Extras {
		w.Extras[i] = pt(0x00465814 + i*16)
	}
	for i := range w.Stats {
		w.Stats[i] = pt(0x00465874 + i*16)
	}
	w.Label = pt(0x00465754)
	w.NameY = 10 * screenHeight800 / 640
	w.TitleY = (i32(0x00465778+8) + 10 - i32(0x00465768+8) + 4) * screenHeight800 / 640
	return w
}

// CharWindowTitle returns the title shown under the name for portrait face
// f (entity +4 & 7, 1–5), or nil.
func (t *ExeTables) CharWindowTitle(face int) []byte {
	if face < 1 || face > 5 {
		return nil
	}
	return t.CString(CharWindowTitlesVA + (face-1)*16)
}
