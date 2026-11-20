package main

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// fakeSessionServer records the calls in order.
type fakeSessionServer struct{ calls []string }

func (f *fakeSessionServer) EndChat(text string)       { f.calls = append(f.calls, "end "+text) }
func (f *fakeSessionServer) StartChat()                { f.calls = append(f.calls, "start") }
func (f *fakeSessionServer) SetPaused(on bool)         { f.calls = append(f.calls, fmt.Sprint("paused ", on)) }
func (f *fakeSessionServer) SetSessionEnd(t time.Time) { f.calls = append(f.calls, "session-end") }

// A session's end: the end line (with the winner), the pause, the reset;
// after the pause the full session, the clear, the clock on. A time-out
// ends without a winner's line.
func TestSessionCycle(t *testing.T) {
	f := &fakeSessionServer{}
	resets := 0
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	c := &sessionCycle{srv: f, length: time.Hour, pause: 20 * time.Millisecond, ticker: ticker,
		reset: func() {
			resets++
			if !slices.Contains(f.calls, "paused true") {
				t.Error("the world reset before the pause began")
			}
		}}
	c.end(1)
	want := []string{"end ИГРА ОКОНЧЕНА! Победа викингов!", "paused true", "session-end"}
	if !slices.Equal(f.calls, want) || resets != 1 || c.pauseOver == nil {
		t.Fatalf("end: calls %q, resets %d", f.calls, resets)
	}
	select {
	case <-c.pauseOver:
	case <-time.After(time.Second):
		t.Fatal("the pause did not end")
	}
	f.calls = nil
	c.start()
	if want := []string{"session-end", "start", "paused false"}; !slices.Equal(f.calls, want) || c.pauseOver != nil {
		t.Fatalf("start: calls %q", f.calls)
	}
	f.calls = nil
	c.end(-1)
	if f.calls[0] != "end ИГРА ОКОНЧЕНА! " {
		t.Errorf("time-out line %q", f.calls[0])
	}
	if sessionPause != 6300*time.Millisecond {
		t.Errorf("pause %v, want the original's 105 ticks, 6.3 s", sessionPause)
	}
}
