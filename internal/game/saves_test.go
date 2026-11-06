package game

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// TestSaveAndLoadScene saves a New Game scene with the hero moved and hurt
// and a pile dropped, then loads the slot into a new scene.
func TestSaveAndLoadScene(t *testing.T) {
	const dir = "../../assets/Konung/"
	for _, f := range []string{"GRAPH.RES", "OBJECTS.RES", "KONUNG.1", "GAME.0", "konung.exe", "Intr800r.res"} {
		if _, err := os.Stat(dir + f); err != nil {
			t.Skip("asset missing:", err)
		}
	}
	a, err := loadSceneAssets(dir+"GRAPH.RES", dir+"OBJECTS.RES", dir, dir+"konung.exe", dir+"Intr800r.res")
	if err != nil {
		t.Fatal(err)
	}
	hero := gamestate.NewHeroTemplate()
	gamestate.SelectHero(&hero, gamestate.HeroVolk)
	w, err := a.newGame(hero)
	if err != nil {
		t.Fatal(err)
	}
	if w.mapNumber != 1 {
		t.Fatalf("Volk starts on KONUNG.%d, want KONUNG.1", w.mapNumber)
	}
	if w.hero == nil {
		t.Fatal("no hero")
	}
	cell := mapdata.Cell{Row: w.hero.cell.Row + 2, Col: w.hero.cell.Col}
	w.hero.cell = cell
	w.party[0].HP = 777
	drop := mapdata.Cell{Row: cell.Row + 4, Col: cell.Col}
	w.game.Piles = append(w.game.Piles, gamestate.Pile{Index: -1, Map: 1, Row: drop.Row, Col: drop.Col, Gold: 5})
	w.pileAt[drop] = len(w.game.Piles) - 1
	w.tick = 40 * walkTicksPerStep

	tmp := t.TempDir()
	data, err := w.game.MarshalSave(w.saveState())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, gamestate.SlotFile(3)), data, 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := w.assets.loadSaved(tmp, 3)
	if err != nil {
		t.Fatal(err)
	}
	if l.hero == nil || l.hero.cell != cell {
		t.Fatalf("hero at %v, want %v", l.hero.cell, cell)
	}
	if l.party[0].HP != 777 {
		t.Errorf("hero HP %d", l.party[0].HP)
	}
	if l.gameTime() != 40 {
		t.Errorf("time %d", l.gameTime())
	}
	i, ok := l.pileAt[drop]
	if !ok || l.game.Piles[i].Gold != 5 || l.game.Piles[i].Index != -1 {
		t.Errorf("dropped pile: %v %+v", ok, l.game.Piles[i])
	}
	if len(l.npcs) != len(w.npcs) {
		t.Errorf("%d characters, want %d", len(l.npcs), len(w.npcs))
	}
	if l.saveName(time.Time{})[0] == ' ' {
		t.Error("no location name")
	}
}
