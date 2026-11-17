package gamestate

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"

	"konung1.5/internal/resfmt"
)

// LAN version screens, read from LAN_KONG.EXE (docs/konung_lan.md#screens).
// Hotspot tables hold 32-byte entries: the 640×480 rectangle, then the
// 800×600 one (x1, y1, x2, y2 dwords, inclusive), ended by x1 = −1; the hit
// test 0x00417934 returns the first match.
const (
	lanDGroupVA   = 0x00470000
	lanDGroupFile = 0x54600

	lanMenuRectsVA  = 0x00479884 // LAN menu (ME800R)
	lanLoginRectsVA = 0x0047E7A4 // login «Новая игра» (PLAY800R frame 0)
	lanMsgRectsVA   = 0x00478F90 // message box buttons
	lanMessagesVA   = 0x0047E264 // message texts: 0x80-byte records, line 2 at +0x40
	lanMessageCount = 9          // ids 0 (none) … 8

	// The map window (draw 0x0041E8EC, hit test 0x004176AB): per video
	// mode 16 bytes (int32 area x, y, width, height); row offsets and
	// (region, length) runs as the single-player window, region = map number
	// (1–20, 0 = none); per map an int32 x and y (from the area's top-left)
	// where the number of players there is drawn, +3.
	lanMapAreaVA    = 0x0047EFA4
	lanMapRowsVA    = 0x00477750
	lanMapRunsVA    = 0x004714D0
	lanMapMarkerXVA = 0x0047F004
	lanMapMarkerYVA = 0x0047F058
	lanMapMode      = 1 // 800×600
)

// Rect is an inclusive 800×600 hotspot.
type Rect struct{ X1, Y1, X2, Y2 int }

// Contains reports whether (x, y) is inside r (bounds inclusive).
func (r Rect) Contains(x, y int) bool { return x >= r.X1 && x <= r.X2 && y >= r.Y1 && y <= r.Y2 }

// HitRects returns the first rectangle containing (x, y), or -1.
func HitRects(rects []Rect, x, y int) int {
	for i, r := range rects {
		if r.Contains(x, y) {
			return i
		}
	}
	return -1
}

// LANExe holds the LAN screens' layout and texts.
type LANExe struct {
	MenuRects, LoginRects, MessageRects []Rect
	// Messages[id] are the message box's two CP866 lines (id 1–8; 0 none).
	Messages [lanMessageCount][2][]byte
	// Map is the map window's area (800×600) and row runs; a run's region is
	// the map number. MapMarkers[m] is where map m's player count is drawn,
	// relative to the area's top-left (index 0 unused).
	Map        resfmt.WorldMap
	MapMarkers [LANMaps + 1]image.Point
}

// LANMaps is the number of LAN maps (MAPS.RES).
const LANMaps = 20

// LANMapTint is the current map's red tint in the map window
// (0x0044FF51(0x30, 0, 0); the single-player window uses 24).
const LANMapTint = 0x30

// Login screen hotspots (index into LANExe.LoginRects, switch 0x0042F269).
const (
	LoginPortrait0  = 0 // three portraits: models 1, 2, 0
	LoginOk         = 3
	LoginCancel     = 4
	LoginName       = 5
	LoginPassword   = 6
	LoginScrollUp   = 7
	LoginScrollDown = 8
	LoginAddress    = 9
	LoginRow0       = 10 // nine server list rows
	LoginRows       = 9
	LoginParams     = 19 // 7 text anchors x=537 («Параметры»), then 7 at x=700 («Навыки»)
	LoginSavePass   = 33
)

// LoginModels maps the three portraits to hero models (0x0042F269).
var LoginModels = [3]int{1, 2, 0}

// Message box ids (texts at 0x0047E264).
const (
	MsgOldHero      = 1 // the only one with Ok and Отмена
	MsgVersion      = 2
	MsgTooMany      = 3
	MsgNoName       = 4
	MsgUnavailable  = 5
	MsgNoServer     = 6
	MsgWrongPass    = 7
	MsgLeaving      = 8
	MessageBoxX     = 144 // PLAY frame 12 (513×214) top-left
	MessageBoxY     = 193
	MessageTextY    = 233 // first line, centred on x = 400
	MessageButtonsY = 347
)

// ParseLANExe reads the LAN screens' data from LAN_KONG.EXE.
func ParseLANExe(exe []byte) (*LANExe, error) {
	at := func(va int) ([]byte, error) {
		o := va - lanDGroupVA + lanDGroupFile
		if o < 0 || o >= len(exe) {
			return nil, fmt.Errorf("LAN_KONG.EXE: %#x outside the file", va)
		}
		return exe[o:], nil
	}
	rects := func(va int) ([]Rect, error) {
		b, err := at(va)
		if err != nil {
			return nil, err
		}
		var out []Rect
		for i := 0; i+32 <= len(b); i += 32 {
			if int32(binary.LittleEndian.Uint32(b[i:])) == -1 {
				return out, nil
			}
			v := func(k int) int { return int(int32(binary.LittleEndian.Uint32(b[i+16+k*4:]))) }
			out = append(out, Rect{v(0), v(1), v(2), v(3)})
		}
		return nil, fmt.Errorf("LAN_KONG.EXE: hotspot table %#x not terminated", va)
	}
	x := &LANExe{}
	var err error
	if x.MenuRects, err = rects(lanMenuRectsVA); err != nil {
		return nil, err
	}
	if x.LoginRects, err = rects(lanLoginRectsVA); err != nil {
		return nil, err
	}
	if x.MessageRects, err = rects(lanMsgRectsVA); err != nil {
		return nil, err
	}
	if len(x.MenuRects) != 5 || len(x.LoginRects) != LoginSavePass+1 {
		return nil, fmt.Errorf("LAN_KONG.EXE: %d menu and %d login hotspots, want 5 and %d", len(x.MenuRects), len(x.LoginRects), LoginSavePass+1)
	}
	// The 800×600 scroll arrows were copied from the NEW screen's class
	// boxes in the original, so the drawn arrows could not be clicked. Put
	// them on the arrows drawn in PLAY800R frame 0 (port fix).
	x.LoginRects[LoginScrollUp] = Rect{344, 193, 362, 209}
	x.LoginRects[LoginScrollDown] = Rect{344, 479, 362, 494}

	b, err := at(lanMessagesVA)
	if err != nil || len(b) < lanMessageCount*0x80 {
		return nil, fmt.Errorf("LAN_KONG.EXE: message table")
	}
	for id := range lanMessageCount {
		r := b[id*0x80:]
		x.Messages[id] = [2][]byte{cString(r[:0x40]), cString(r[0x40:0x80])}
	}

	var bad error
	at32 := func(va int) int {
		b, err := at(va)
		if err != nil || len(b) < 4 {
			bad = fmt.Errorf("LAN_KONG.EXE: map tables at %#x", va)
			return 0
		}
		return int(int32(binary.LittleEndian.Uint32(b)))
	}
	area := lanMapAreaVA + 16*lanMapMode
	ax, ay := at32(area), at32(area+4)
	rect := image.Rect(ax, ay, ax+at32(area+8), ay+at32(area+12))
	for m := 1; m <= LANMaps; m++ {
		x.MapMarkers[m] = image.Pt(at32(lanMapMarkerXVA+4*m), at32(lanMapMarkerYVA+4*m))
	}
	if bad != nil {
		return nil, bad
	}
	x.Map = resfmt.ReadWorldMap(func(va int) []byte { b, _ := at(va); return b }, rect, lanMapRowsVA, lanMapRunsVA)
	return x, nil
}

// TextField is a login-screen text field being edited (focus values and
// limits from 0x0042CB94: the limit counts the blinking '_' cursor, so a
// field of n bytes holds n−1 characters; a character is also refused when
// the text would no longer fit the field's width).
type TextField struct {
	Text []byte // CP866
	Max  int    // bytes including the cursor
}

// Type adds c if the field has room; width reports the text's pixel width
// and fieldWidth is the hotspot's width.
func (f *TextField) Type(c byte, width func([]byte) int, fieldWidth int) bool {
	if len(f.Text)+1 >= f.Max {
		return false
	}
	next := append(bytes.Clone(f.Text), c)
	if width != nil && width(next)+15 >= fieldWidth {
		return false
	}
	f.Text = next
	return true
}

// Backspace deletes the last character.
func (f *TextField) Backspace() {
	if len(f.Text) > 0 {
		f.Text = f.Text[:len(f.Text)-1]
	}
}

// Field limits in bytes including the cursor (0x0042CB94).
const (
	FieldClan     = 31
	FieldName     = 32
	FieldPassword = 8
	FieldAddress  = 63
)
