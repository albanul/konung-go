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
	if w.MapNumber != 1 {
		t.Fatalf("Volk starts on KONUNG.%d, want KONUNG.1", w.MapNumber)
	}
	if w.Hero == nil {
		t.Fatal("no hero")
	}
	cell := mapdata.Cell{Row: w.Hero.Cell.Row + 2, Col: w.Hero.Cell.Col}
	w.Hero.Cell = cell
	w.Party[0].HP = 777
	drop := mapdata.Cell{Row: cell.Row + 4, Col: cell.Col}
	w.Game.Piles = append(w.Game.Piles, gamestate.Pile{Index: -1, Map: 1, Row: drop.Row, Col: drop.Col, Gold: 5})
	w.PileAt[drop] = len(w.Game.Piles) - 1
	w.Tick = 40

	tmp := t.TempDir()
	data, err := w.Game.MarshalSave(w.saveState())
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
	if l.Hero == nil || l.Hero.Cell != cell {
		t.Fatalf("hero at %v, want %v", l.Hero.Cell, cell)
	}
	if l.Party[0].HP != 777 {
		t.Errorf("hero HP %d", l.Party[0].HP)
	}
	if l.GameTime() != 40 {
		t.Errorf("time %d", l.GameTime())
	}
	i, ok := l.PileAt[drop]
	if !ok || l.Game.Piles[i].Gold != 5 || l.Game.Piles[i].Index != -1 {
		t.Errorf("dropped pile: %v %+v", ok, l.Game.Piles[i])
	}
	if len(l.NPCs) != len(w.NPCs) {
		t.Errorf("%d characters, want %d", len(l.NPCs), len(w.NPCs))
	}
	if l.saveName(time.Time{})[0] == ' ' {
		t.Error("no location name")
	}
}
