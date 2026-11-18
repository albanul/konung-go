package game

import (
	"fmt"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
)

// In a LAN world the server owns the clock (netplay/clock.go): the scene's
// tick (0x00A3E770, the time of day) is the server's world tick (each view
// carries it, lanview.go), the world speed is the server's (numpad +/− do
// nothing), the moon is the same for everyone, and the session's time left
// is shown on screen.

// serverClock is the last clock the server sent and when it arrived.
type serverClock struct {
	c  netplay.ClockPayload
	at time.Time
}

// tick is the server's tick now.
func (k serverClock) tick() int {
	return int(k.c.Tick + k.c.TicksSince(time.Since(k.at)))
}

// sessionLeft is the session's time left now, false when unknown.
func (k serverClock) sessionLeft() (time.Duration, bool) {
	if k.c.SessionLeft < 0 {
		return 0, false
	}
	return max(0, time.Duration(k.c.SessionLeft)*time.Second-time.Since(k.at)), true
}

// lanMoon is whether day's night is moonlit: the single-player roll
// (MoonChance) made the same on every client from the day number.
func lanMoon(day int) bool {
	h := uint32(day)*2654435761 + 0x9E37
	h ^= h >> 15
	return int(h%100) < gamestate.MoonChance
}

// drawSessionLeft shows the rating session's time left in the view's
// top-left corner (port addition), in the key help's font.
func (s *startScene) drawSessionLeft(dst *ebiten.Image) {
	if s.clock == nil || helpFace == nil {
		return
	}
	left, ok := s.clock.sessionLeft()
	if !ok {
		return
	}
	sec := int(left.Seconds())
	line := fmt.Sprintf("До конца сессии: %d:%02d:%02d", sec/3600, sec/60%60, sec%60)
	const pad = 6
	w := float32(text.Advance(line, helpFace)) + 2*pad
	h := float32(helpFace.Metrics().HAscent+helpFace.Metrics().HDescent) + 2*pad
	x0, y0 := float32(viewLeft)+4, float32(viewTop)+4
	vector.FillRect(dst, x0, y0, w, h, color.RGBA{0, 0, 0, 0xA0}, false)
	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(x0)+pad, float64(y0)+pad)
	op.ColorScale.ScaleWithColor(color.RGBA{0xE8, 0xE0, 0xC8, 0xFF})
	text.Draw(dst, line, helpFace, op)
}
