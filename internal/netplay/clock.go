package netplay

import "time"

// The world clock belongs to the server (a port rule: the original's
// clients each ran their own clock): every client's time of day, AI timers
// and world speed follow it. It counts game ticks (gamestate's 0x00A3E770,
// 60 ms each at normal speed, 1 + Speed ticks per 60 ms) and stands still
// while nobody is in the world.

// TickDuration is one game tick at normal speed.
const TickDuration = 60 * time.Millisecond

// MaxSpeed is the fastest world speed (single-player 0x00A3E764: 0 normal,
// 1 double, 2 triple).
const MaxSpeed = 2

// ClockPayload is the server's clock (MsgClock): the tick now, the world
// speed, whether the clock runs, and the seconds left in the rating
// session (-1 unknown).
type ClockPayload struct {
	Tick        int64 `json:"tick"`
	Speed       int   `json:"speed"`
	Running     bool  `json:"running"`
	SessionLeft int   `json:"session_left"`
}

// TicksSince is how many ticks the clock advanced in d at this speed.
func (c ClockPayload) TicksSince(d time.Duration) int64 {
	if !c.Running || d <= 0 {
		return 0
	}
	return int64(d) * int64(1+c.Speed) / int64(TickDuration)
}

// clockLocked is the clock now; s.mu held.
func (s *Server) clockLocked() int64 {
	if !s.clockOn {
		return s.clockBase
	}
	return s.clockBase + ClockPayload{Running: true, Speed: s.cfg.Speed}.TicksSince(time.Since(s.clockFrom))
}

// runClockLocked starts or stops the clock as someone is or nobody is in
// the world, and tells everyone in it; s.mu held.
func (s *Server) runClockLocked() {
	on := false
	for _, p := range s.players {
		on = on || p.entered
	}
	if on != s.clockOn {
		s.clockBase, s.clockFrom, s.clockOn = s.clockLocked(), time.Now(), on
	}
	s.sendClockLocked()
}

// clockMessageLocked is the clock as a message; s.mu held.
func (s *Server) clockMessageLocked() *Message {
	left := -1
	if !s.sessionEnd.IsZero() {
		left = max(0, int(time.Until(s.sessionEnd).Seconds()))
	}
	return mustMessage(MsgClock, ClockPayload{Tick: s.clockLocked(), Speed: s.cfg.Speed, Running: s.clockOn, SessionLeft: left})
}

// sendClockLocked sends the clock to everyone in the world; s.mu held.
func (s *Server) sendClockLocked() {
	m := s.clockMessageLocked()
	for _, p := range s.players {
		if p.entered {
			p.conn.Send(m)
		}
	}
}

// SyncClock sends the clock to everyone in the world, so clients that
// drift come back in step; the dedicated server calls it now and then.
func (s *Server) SyncClock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sendClockLocked()
}

// SetSessionEnd sets when the rating session ends (the server's session
// clock) and tells everyone in the world.
func (s *Server) SetSessionEnd(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionEnd = t
	s.sendClockLocked()
}

// Clock is the clock now.
func (s *Server) Clock() ClockPayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c ClockPayload
	s.clockMessageLocked().Decode(&c)
	return c
}
