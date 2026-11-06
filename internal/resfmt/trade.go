package resfmt

import "image"

// TradeLayout is the geometry of window 7 (0x00431D70), the trade and pile
// window, for one video mode (tables of 32-byte records, 16 bytes per
// mode: x1, y1, x2, y2):
//
//   - Lists (0x00465AB4): 0 the character's bag, 1 what it offers, 2 the
//     other side's goods (stock, bag or the ground), 3 what it takes. Each
//     row is drawn like the bag strip from (x1, y1); the rectangle
//     (inclusive) is its hit area (0x004392F4).
//   - Buttons (0x00465A54): 0 «Закрыть» (INTR slot 180, hotspot 0xC8),
//     1 «Ok» (slot 182, hotspot 0xC9), drawn at (x1, y1).
//   - Numbers (0x00465B54, x, y): the other side's money, what it pays,
//     its total; what the character pays, its money, its total.
type TradeLayout struct {
	Lists   [4]image.Rectangle
	Buttons [2]image.Rectangle
	Numbers [6]image.Point
	NameY   int // 0x00A3E838 (screen height) / 24: the trader's name, large font, centred
}

// Window 7 INTR slots.
const (
	TradeBackgroundSlot = 6
	TradeCloseSlot      = 180
	TradeOkSlot         = 182
)

// TradeLayout returns window 7's layout for mode 0 (640×480) or 1 (800×600).
func (t *ExeTables) TradeLayout(mode int) TradeLayout {
	rect := func(va int) image.Rectangle {
		va += 16 * mode
		return image.Rect(t.i32(va), t.i32(va+4), t.i32(va+8)+1, t.i32(va+12)+1)
	}
	var l TradeLayout
	for k := range l.Lists {
		l.Lists[k] = rect(0x00465AB4 + 32*k)
	}
	for k := range l.Buttons {
		l.Buttons[k] = rect(0x00465A54 + 32*k)
	}
	for k := range l.Numbers {
		va := 0x00465B54 + 32*k + 16*mode
		l.Numbers[k] = image.Pt(t.i32(va), t.i32(va+4))
	}
	l.NameY = []int{480, 600}[mode] / 24
	return l
}
