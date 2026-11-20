package main

import (
	"log"
	"time"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
)

// sessionPause is the pause between two sessions: the original's 105
// ticks (0x00434515: 0x69 into 0xF5EC5C, counted down by 0x00448BB8),
// 6.3 s.
const sessionPause = 105 * netplay.TickDuration

// sessionServer is what the session cycle needs of the network server.
type sessionServer interface {
	EndChat(text string)
	StartChat()
	SetPaused(on bool)
	SetSessionEnd(t time.Time)
}

// sessionCycle ends a session and starts the next one (docs/konung_lan.md
// #session-end).
type sessionCycle struct {
	srv    sessionServer
	length time.Duration // a session
	pause  time.Duration // between two sessions
	ticker *time.Ticker  // the session's end
	reset  func()        // the world anew, the ratings
	// pauseOver fires when the pause is over (nil outside one).
	pauseOver <-chan time.Time
}

// end is a session's end (0x00434438; winner −1: the time ran out): the
// players are told («ИГРА ОКОНЧЕНА! » and the winner's line, 0x004345CB),
// the pause begins (the clock stands, orders are ignored, the time left
// shows 0), then the world anew and the ratings (reset). The next session
// starts when the pause is over (start).
func (c *sessionCycle) end(winner int) {
	text := gamestate.SessionEndText
	if winner >= 0 && winner < len(gamestate.NationWins) {
		text += gamestate.NationWins[winner]
	}
	c.srv.EndChat(text)
	c.srv.SetPaused(true)
	c.srv.SetSessionEnd(time.Now())
	c.ticker.Stop()
	c.pauseOver = time.After(c.pause)
	c.reset()
}

// start ends the pause (0x00448BB8 when its count reaches 0): the
// session's full length, the players' chat cleared (0x00448CD3), the
// clock on.
func (c *sessionCycle) start() {
	c.pauseOver = nil
	c.srv.SetSessionEnd(time.Now().Add(c.length))
	c.ticker.Reset(c.length)
	c.srv.StartChat()
	c.srv.SetPaused(false)
	log.Print("a new session starts")
}
