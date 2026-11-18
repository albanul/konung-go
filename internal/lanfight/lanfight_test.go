package lanfight

import (
	"os"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
)

func templates(t *testing.T) [netplay.Models]*gamestate.Game {
	t.Helper()
	const dir = "../../assets/Konung/"
	exe, err := os.ReadFile(dir + "konung.exe")
	if err != nil {
		t.Skip("konung.exe not available:", err)
	}
	var games [netplay.Models][]byte
	for m := range games {
		if games[m], err = os.ReadFile(dir + "GAME." + string(rune('0'+m))); err != nil {
			t.Skip("GAME.N not available:", err)
		}
	}
	_ = exe
	var out [netplay.Models]*gamestate.Game
	for m, data := range games {
		if out[m], err = gamestate.Parse(data); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// The client builds its hero with Hero too: the type's gear, the account's
// values, the type's model and look.
func TestHero(t *testing.T) {
	tmpl := templates(t)
	h := &netplay.Hero{Level: 5, Class: gamestate.ClassHunter, Model: 2, Palette: 37, Strength: 20, Endurance: 12}
	e := Hero(tmpl[2], 2, h)
	if e.Flags4&7 != gamestate.ClassHunter {
		t.Errorf("class %d, want Охотник (%d)", e.Flags4&7, gamestate.ClassHunter)
	}
	if e.Model != 2 || e.PaletteOffset != 37*512 || e.Level != 5 || e.BaseStats[1] != 20 || e.HP != gamestate.HPFull {
		t.Errorf("hero = %+v", e)
	}
	if e.Equipment == ([6]int{}) {
		t.Error("the template hero has no gear")
	}
}
