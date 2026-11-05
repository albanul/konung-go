package game

import (
	"testing"

	"konung1.5/internal/gamestate"
)

// Hotspots 2–7 flip their settings in the screen's row order; «Отмена»
// restores them, the video choice stays separate from the running mode.
func TestSettingsClicks(t *testing.T) {
	c := gamestate.DefaultConfig()
	s := &settingsScreen{}
	s.open(c)
	r := settingsRects800[3] // Тени
	if i := s.hit(r[0], r[1]); i != 3 {
		t.Fatalf("hit %d, want 3", i)
	}
	if s.hit(0, 0) != -1 {
		t.Fatal("hit outside")
	}
	s.click(3, &c)
	s.click(7, &c)
	s.click(0, &c)
	if c.Shadows != 0 || c.Sound != 0 || c.Transparency != 1 || s.video != 0 || c.Video != 1 {
		t.Fatalf("after clicks: %+v video %d", c, s.video)
	}
	if closed, ok := s.click(settingsCancel, &c); !closed || ok {
		t.Fatal("cancel must close without accepting")
	}
	if c != gamestate.DefaultConfig() {
		t.Fatalf("cancel did not restore: %+v", c)
	}
	if closed, ok := s.click(settingsOK, &c); !closed || !ok {
		t.Fatal("ok must close and accept")
	}
}
