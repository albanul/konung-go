package gamestate

import (
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
	if got := resfmt.CP866ToUTF8(x.NewServer); got != "Новый сервер" {
		t.Errorf("NewServer = %q", got)
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
