package gamestate

import (
	"image"
	"os"
	"testing"

	"konung1.5/internal/resfmt"
)

func TestParseLANExe(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/LAN_KONG.EXE")
	if err != nil {
		t.Skip("LAN_KONG.EXE not available:", err)
	}
	x, err := ParseLANExe(data)
	if err != nil {
		t.Fatal(err)
	}
	if r := x.LoginRects[LoginOk]; r != (Rect{291, 532, 391, 565}) {
		t.Errorf("Ok = %+v", r)
	}
	if r := x.LoginRects[LoginRow0+8]; r != (Rect{76, 454, 354, 478}) {
		t.Errorf("last row = %+v", r)
	}
	// The «Рейтинг» screen at 800×600 (0x004797E8).
	if r := x.Rating; r != (LANRatingLayout{PlayerY: 53, PlayerDY: 19, Rank: 92, Name: 129, Clan: 282, Amulets: 476,
		Heroes: 517, Hirelings: 557, Monsters: 597, Rating: 658, ClanY: 300, ClanDY: 19, ClanRankX: 92, ClanNameX: 129, ClanRatingX: 389}) {
		t.Errorf("rating layout = %+v", r)
	}
	// Fixed light: only map 18 («B1»), tint (−1, −1, −1).
	for m := 1; m <= LANMaps; m++ {
		tint, fixed := x.MapLight(m)
		if fixed != (m == 18) || fixed && tint != (resfmt.Tint{R: -1, G: -1, B: -1}) {
			t.Errorf("map %d: fixed %v tint %+v", m, fixed, tint)
		}
	}
	// Nested menu hotspots: the first match wins.
	if HitRects(x.MenuRects, 300, 140) != 0 || HitRects(x.MenuRects, 300, 320) != 2 || HitRects(x.MenuRects, 300, 430) != 4 || HitRects(x.MenuRects, 100, 100) != -1 {
		t.Error("menu hit test")
	}
	if got := resfmt.CP866ToUTF8(x.Messages[MsgNoName][0]); got != "Ошибка: Не указано имя игрока" {
		t.Errorf("message 4 = %q", got)
	}
	if got := resfmt.CP866ToUTF8(x.Messages[MsgVersion][1]); got != "Выполните обновление Вашей программы" {
		t.Errorf("message 2 line 2 = %q", got)
	}
	if len(x.MessageRects) != 5 || x.MessageRects[4] != (Rect{347, 347, 452, 393}) {
		t.Errorf("message rects = %+v", x.MessageRects)
	}
}

func TestTextField(t *testing.T) {
	f := TextField{Max: FieldPassword}
	for _, c := range []byte("12345678") {
		f.Type(c, nil, 1000)
	}
	if string(f.Text) != "1234567" { // 8 bytes with the cursor
		t.Errorf("password = %q", f.Text)
	}
	f.Backspace()
	if string(f.Text) != "123456" {
		t.Errorf("after backspace %q", f.Text)
	}
	// Width limit: 10 px per character, 100 px field: text + 15 must stay < 100.
	g := TextField{Max: FieldName}
	for range 20 {
		g.Type('x', func(b []byte) int { return 10 * len(b) }, 100)
	}
	if len(g.Text) != 8 {
		t.Errorf("width-limited field holds %d characters, want 8", len(g.Text))
	}
}

// The map window: the 800×600 area, and each map's player-count spot lies
// inside that map's region of the run data.
func TestLANMapWindow(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/LAN_KONG.EXE")
	if err != nil {
		t.Skip("LAN_KONG.EXE not available:", err)
	}
	x, err := ParseLANExe(data)
	if err != nil {
		t.Fatal(err)
	}
	if x.Map.Area != image.Rect(80, 60, 80+502, 60+361) {
		t.Errorf("area = %v", x.Map.Area)
	}
	for m := 1; m <= LANMaps; m++ {
		p := x.MapMarkers[m].Add(image.Pt(12, 12))
		if r, ok := x.Map.RegionAt(p.X, p.Y); !ok || r != m {
			t.Errorf("map %d marker %v lies in region %d", m, x.MapMarkers[m], r)
		}
	}
	// Map 1 (model 0's village) is the top middle tile, map 4 the bottom left.
	if x.MapMarkers[1] != image.Pt(202, 13) || x.MapMarkers[4] != image.Pt(14, 178) {
		t.Errorf("markers 1, 4 = %v, %v", x.MapMarkers[1], x.MapMarkers[4])
	}
	// The location names (0x00479944) are the grid cells: row letter A–D
	// from the top, column 1–5 from the left, where each map's region lies
	// in the picture (5 × 4 tiles).
	for m := 1; m <= LANMaps; m++ {
		p := x.MapMarkers[m].Add(image.Pt(12, 12))
		want := string([]byte{byte('A' + p.Y*4/x.Map.Area.Dy()), byte('1' + p.X*5/x.Map.Area.Dx())})
		if got := string(x.MapNames[m]); got != want {
			t.Errorf("map %d name %q, want %q", m, got, want)
		}
	}
	if string(x.MapNames[1]) != "A3" || string(x.MapNames[3]) != "B3" || string(x.MapNames[20]) != "B5" {
		t.Errorf("names 1, 3, 20 = %q %q %q", x.MapNames[1], x.MapNames[3], x.MapNames[20])
	}
}
