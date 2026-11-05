package game

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
)

// The settings screen «Настройки» (docs/konung_settings.md): screen state 6,
// opened by menu item 4 (0x00437DDD), drawn by 0x004315F8 and run by the
// click switch at 0x00438A94 (jump table 0x00438CE8).

// settingsRects800 is the 800x600 half of the hotspot table 0x0046219C
// (32 bytes per entry: 640x480 x1,y1,x2,y2, then 800x600), hit-tested with
// 0x004398C4. The index is the click switch case.
var settingsRects800 = [settingsHotspots][4]int{
	{435, 144, 475, 184}, // 0 Видеорежим 640x480
	{588, 144, 628, 184}, // 1 Видеорежим 800x600
	{513, 206, 553, 246}, // 2 Прозрачность
	{513, 250, 553, 290}, // 3 Тени
	{513, 294, 553, 334}, // 4 Смена дня и ночи
	{513, 338, 553, 378}, // 5 Световые эффекты
	{513, 382, 553, 422}, // 6 Музыка
	{513, 426, 553, 466}, // 7 Звук
	{300, 510, 399, 600}, // 8 Ok
	{400, 510, 499, 600}, // 9 Отмена
}

const (
	settingsHotspots = 10
	settingsOK       = 8
	settingsCancel   = 9
)

// settingsScreen holds the background (MENU800R.RES frame 1), the marker
// (frame 2) and the values saved when the screen opened (0x00A5399C …
// 0x00A539B4), which «Отмена» and Escape restore.
type settingsScreen struct {
	bg, marker *ebiten.Image
	backup     gamestate.Config
	// video is the chosen mode 0x00A539A0. The game switches modes on «Ok»
	// (0x00427B1C); the port always runs at 800x600 and only stores it.
	video int32
}

func newSettingsScreen(bg, marker image.Image) *settingsScreen {
	return &settingsScreen{bg: ebiten.NewImageFromImage(bg), marker: ebiten.NewImageFromImage(marker)}
}

// open saves the current values (0x00437DE7).
func (s *settingsScreen) open(c gamestate.Config) {
	s.backup, s.video = c, c.Video
}

// hit returns the hotspot under (x, y), or -1.
func (s *settingsScreen) hit(x, y int) int {
	for i, r := range settingsRects800 {
		if x >= r[0] && x < r[2] && y >= r[1] && y < r[3] {
			return i
		}
	}
	return -1
}

// toggles maps hotspots 2–7 to their settings.
func toggles(c *gamestate.Config) [6]*int32 {
	return [6]*int32{&c.Transparency, &c.Shadows, &c.DayNight, &c.Lighting, &c.Music, &c.Sound}
}

// click runs hotspot i on c and reports whether the screen closes and, if
// so, whether the result is accepted («Ok») or c was restored.
func (s *settingsScreen) click(i int, c *gamestate.Config) (closed, ok bool) {
	switch {
	case i == 0 || i == 1: // 0x00438AC6, 0x00438AD5
		s.video = int32(i)
	case i >= 2 && i <= 7: // 0x00438AE4 … 0x00438BDD: flip 0/1
		f := toggles(c)[i-2]
		if *f == 0 {
			*f = 1
		} else {
			*f = 0
		}
	case i == settingsOK: // 0x00438C0C
		return true, true
	case i == settingsCancel: // 0x00438C85
		s.restore(c)
		return true, false
	}
	return false, false
}

// restore puts back the values saved on open (0x00438C99, Escape
// 0x00437517). The video mode is not touched: only the chosen mode changes.
func (s *settingsScreen) restore(c *gamestate.Config) {
	b := s.backup
	c.Transparency, c.DayNight, c.Lighting = b.Transparency, b.DayNight, b.Lighting
	c.Music, c.Shadows, c.Sound = b.Music, b.Shadows, b.Sound
}

// draw is 0x004315F8: the background, then the marker centred in the
// chosen mode's box and in the box of every setting that is on.
func (s *settingsScreen) draw(dst *ebiten.Image, c gamestate.Config) {
	dst.DrawImage(s.bg, nil)
	if s.video == 0 || s.video == 1 {
		s.drawMarker(dst, int(s.video))
	}
	for i, f := range toggles(&c) {
		if *f != 0 {
			s.drawMarker(dst, i+2)
		}
	}
}

func (s *settingsScreen) drawMarker(dst *ebiten.Image, i int) {
	r := settingsRects800[i]
	w, h := s.marker.Bounds().Dx(), s.marker.Bounds().Dy()
	drawAt(dst, s.marker, r[0]+(r[2]-r[0]-w)/2, r[1]+(r[3]-r[1]-h)/2)
}
