package game

import (
	"testing"

	"konung1.5/internal/gamestate"
)

// TestNewHeroIdleVoice: the screen's idle action (0x004143DC(hero, 0))
// plays the chosen hero's idle voice, model·8 + 0x23, only on
// rand() % 100 > 95.
func TestNewHeroIdleVoice(t *testing.T) {
	e := gamestate.NewHeroTemplate()
	for _, h := range []int{gamestate.HeroVolk, gamestate.HeroDragomir, gamestate.HeroMichail} {
		gamestate.SelectHero(&e, h)
		if id := idleVoice(e, func(int) int { return 96 }); id != e.Model*8+0x23 {
			t.Errorf("hero %d (model %d): voice %d, want %d", h, e.Model, id, e.Model*8+0x23)
		}
		if id := idleVoice(e, func(int) int { return 95 }); id != 0 {
			t.Errorf("hero %d: voice %d on roll 95, want none", h, id)
		}
	}
}
