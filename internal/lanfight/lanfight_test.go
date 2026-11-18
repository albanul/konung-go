package lanfight

import (
	"os"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
)

func world(t *testing.T) *World {
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
	w, err := New(exe, games)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// Two heroes fight with the game's rules until one dies; the killer gets
// the kill experience.
func TestFight(t *testing.T) {
	w := world(t)
	volk := &netplay.Hero{Level: 3, Model: 0, Fencing: 100, Strength: 15, Dexterity: 3, Endurance: 10, Charisma: 3}
	look, hp := w.Enter("a", 0, volk)
	if hp != gamestate.HPFull || look.Stance {
		t.Fatalf("entered: look %+v hp %d", look, hp)
	}
	w.Enter("b", 2, nil)
	req := netplay.StrikeRequest{Attacker: "a", Target: "b", Distance: 1}
	if r := w.Strike(req); !r.Refused {
		t.Error("a strike out of the stance was taken")
	}
	look = w.Stance("a", true, gamestate.WeaponBlade)
	if !look.Stance {
		t.Fatalf("stance refused: %+v", look)
	}
	if look.Equip[look.Weapon].Category < 0 {
		t.Logf("look %+v: no item in the weapon slot, the mode stays %d", look, look.Weapon)
	}
	hits, last := 0, gamestate.HPFull
	for range 2000 {
		r := w.Strike(req)
		if r.Refused {
			t.Fatalf("strike refused: %+v", r)
		}
		if r.Result == netplay.HitLight || r.Result == netplay.HitHeavy {
			hits++
			if r.HP >= last || r.HP != last-r.Damage {
				t.Fatalf("HP %d after %d damage from %d", r.HP, r.Damage, last)
			}
			last = r.HP
		}
		if r.Result == netplay.HitKill {
			if r.HP != 0 || r.Killer == nil || r.Killer.FreePoints <= volk.FreePoints {
				t.Errorf("kill: %+v", r)
			}
			if r := w.Strike(req); !r.Refused {
				t.Error("the dead were struck again")
			}
			t.Logf("killed after %d hits", hits)
			return
		}
	}
	t.Fatal("no kill in 2000 strikes")
}

// The client builds its hero with Hero too: the type's gear, the account's
// values, the type's model and look.
func TestHero(t *testing.T) {
	w := world(t)
	h := &netplay.Hero{Level: 5, Class: gamestate.ClassHunter, Model: 2, Palette: 37, Strength: 20, Endurance: 12}
	e := Hero(w.tmpl[2], 2, h)
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
