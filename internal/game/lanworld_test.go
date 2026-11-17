package game

import (
	"os"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
)

// Without an account (a session hosted from the game) the hero is the
// model's single-player hero: GAME.N's hero model is a placeholder (1 in all
// three files) that the start replaces.
func TestLANWorldWithoutAccount(t *testing.T) {
	const dir = "../../assets/Konung/"
	if _, err := os.Stat(dir + "MAPS.RES"); err != nil {
		t.Skip("MAPS.RES not available:", err)
	}
	a, err := loadSceneAssets(dir+"GRAPH.RES", dir+"OBJECTS.RES", dir, dir+"konung.exe", dir+"Intr800r.res")
	if err != nil {
		t.Skip("assets missing:", err)
	}
	for m, at := range []netplay.EnterPayload{{Map: 1, Row: 110, Col: 32, Facing: 2}, {Map: 4, Row: 66, Col: 57}, {Map: 7, Row: 65, Col: 8, Facing: 4}} {
		s, err := a.lanWorld(dir, at, nil, m)
		if err != nil {
			t.Fatal(err)
		}
		if e := s.party[0]; e.Model != m || e.PaletteOffset != gamestate.HeroPalette[m] {
			t.Errorf("model %d: hero model %d, palette %#x", m, e.Model, e.PaletteOffset)
		}
	}
}

func TestLANWorld(t *testing.T) {
	const dir = "../../assets/Konung/"
	if _, err := os.Stat(dir + "MAPS.RES"); err != nil {
		t.Skip("MAPS.RES not available:", err)
	}
	a, err := loadSceneAssets(dir+"GRAPH.RES", dir+"OBJECTS.RES", dir, dir+"konung.exe", dir+"Intr800r.res")
	if err != nil {
		t.Skip("assets missing:", err)
	}
	hero := &netplay.Hero{Level: 3, Model: 2, Palette: 35, Gold: 250, Charisma: 4, Strength: 16, Dexterity: 5, Endurance: 11,
		Fencing: 150, Archery: 20, Herbalism: 50, Smithing: 10, Trade: 10, Carpentry: 1, Identification: 20}
	at := netplay.EnterPayload{Map: 7, Row: 65, Col: 8, Facing: 4}
	s, err := a.lanWorld(dir, at, hero, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !s.lan || s.mapNumber != 7 || s.hero == nil || s.hero.cell != (mapdata.Cell{Row: 65, Col: 8}) || s.hero.dir != 4 {
		t.Fatalf("scene: lan %v, map %d, hero %+v", s.lan, s.mapNumber, s.hero)
	}
	e := s.party[0]
	if e.Model != 2 || e.PaletteOffset != 35*512 || e.Gold != 250 || e.Level != 3 || e.BaseStats != [4]int{4, 16, 5, 11} || e.Fencing != 150 {
		t.Errorf("hero entity = %+v", e)
	}
	if len(s.npcs) != 0 || len(s.party) != 1 {
		t.Errorf("%d characters besides a party of %d", len(s.npcs), len(s.party))
	}
	// The LAN map is MAPS.RES #7, not KONUNG.7.
	maps, _ := os.ReadFile(dir + "MAPS.RES")
	e7, _ := mapdata.SplitMapsRes(maps)
	m7, _ := mapdata.Parse(e7[6].Map)
	if s.m.LastRow != m7.LastRow || s.m.Cells[10][10] != m7.Cells[10][10] {
		t.Error("the scene's map is not MAPS.RES #7")
	}
	// The single-player assets still read KONUNG.N.
	if a.lanMaps != nil {
		t.Error("lanWorld changed the shared assets")
	}
}
