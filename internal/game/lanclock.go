package game

import (
	"time"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
)

// In a LAN world the server owns the clock (netplay/clock.go): the scene's
// tick (0x00A3E770, the time of day) is the server's world tick (each view
// carries it, lanview.go), the world speed is the server's (numpad +/− do
// nothing), the moon is the same for everyone, and the session's time left
// is shown in the status line at the top (lanamulets.go).

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
