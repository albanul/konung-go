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
	InHand    image.Point    // centre of the in-hand box item 0x00A3E7A0 (0x00465864)
	Stats     [25]image.Point
	// Stats are the top-left corners of the 25 numbers (0x00465874), in the
	// order of gamestate.CharSheet.
	// Raise[row] is the raise column (0x0042F2B6, table 0x00465754 rows
	// 0–4: level, Хар, Сил, Лов, Вын): a stat row prints its cost at the
	// point and draws the button (RaiseButtonSlot) RaiseButtonDX to the
	// right; the level row draws the button at the point. Buttons sit 2 px
	// lower than the point. Raise[0].X is also the left edge of the
	// «Свободный опыт» number, centred in the button's width at Stats[24].Y.
	Raise [5]image.Point
	// AutoLevel is the top-left of the auto-level toggle (row 5), INTR slot
	// AutoLevelOnSlot when entity +3 bit 0x02 is set, else AutoLevelOffSlot.
	AutoLevel image.Point
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

// Raise column images and offsets (0x0042F2B6).
const (
	RaiseButtonSlot  = 171 // INTR slot 171 (image pointer 0x00630A10)
	RaiseButtonDX    = 0x18
	RaiseButtonDY    = 2
	AutoLevelOffSlot = 0xA9
	AutoLevelOnSlot  = 0xAA
)

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
	w.InHand = pt(0x00465864)
	for i := range w.Raise {
		w.Raise[i] = pt(0x00465754 + i*16)
	}
	w.AutoLevel = pt(0x00465754 + 5*16)
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
