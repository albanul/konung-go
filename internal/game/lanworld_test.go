package game

import (
	"os"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
)

// Without an account (a server without accounts) the hero is the
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
		s, err := a.lanWorld(dir, nil, at, nil, m)
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
	s, err := a.lanWorld(dir, nil, at, hero, 0)
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

// A LAN teleport changes the map: the next MAPS.RES map, the hero on the
// exit's arrival cell, still a LAN world.
func TestLANWorldChangesMap(t *testing.T) {
	const dir = "../../assets/Konung/"
	if _, err := os.Stat(dir + "MAPS.RES"); err != nil {
		t.Skip("MAPS.RES not available:", err)
	}
	a, err := loadSceneAssets(dir+"GRAPH.RES", dir+"OBJECTS.RES", dir, dir+"konung.exe", dir+"Intr800r.res")
	if err != nil {
		t.Skip("assets missing:", err)
	}
	s, err := a.lanWorld(dir, nil, netplay.EnterPayload{Map: 1, Row: 110, Col: 32, Facing: 2}, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	x, ok := s.game.ExitAt(1, 65, 7) // GAME.RES exit 3: map 1 → map 15
	if !ok || x.To != 15 {
		t.Fatalf("exit at (65,7) on map 1 = %+v, %v", x, ok)
	}
	s.changeMap(x)
	if s.mapNumber != 15 || !s.lan {
		t.Fatalf("after the exit: map %d, lan %v", s.mapNumber, s.lan)
	}
	if s.hero == nil || s.hero.cell != (mapdata.Cell{Row: x.Row, Col: x.Col}) {
		t.Errorf("hero at %+v, want the arrival (%d, %d)", s.hero, x.Row, x.Col)
	}
	maps, _ := os.ReadFile(dir + "MAPS.RES")
	e, _ := mapdata.SplitMapsRes(maps)
	m15, _ := mapdata.Parse(e[14].Map)
	if s.m.LastRow != m15.LastRow || s.m.Cells[20][20] != m15.Cells[20][20] {
		t.Error("the new map is not MAPS.RES #15")
	}
	// And back: map 15 has exits too.
	back := 0
	for _, x := range s.game.Exits {
		if x.From == 15 {
			back++
		}
	}
	if back != 8 {
		t.Errorf("map 15 has %d exits", back)
	}
}

// The LAN map window's picture: every location is drawn, the current one
// with the red-tinted palette.
func TestLANWorldMap(t *testing.T) {
	const dir = "../../assets/Konung/"
	exeData, err := os.ReadFile(dir + "LAN_KONG.EXE")
	if err != nil {
		t.Skip("LAN_KONG.EXE not available:", err)
	}
	into, err1 := os.ReadFile(dir + "INTO640R.RES")
	graph, err2 := os.ReadFile(dir + "GRAPH.RES")
	if err1 != nil || err2 != nil {
		t.Skip("INTO640R.RES or GRAPH.RES not available")
	}
	exe, err := gamestate.ParseLANExe(exeData)
	if err != nil {
		t.Fatal(err)
	}
	l, err := newLANWorldMap(exe, into, graph)
	if err != nil {
		t.Fatal(err)
	}
	img := l.m.Compose(l.reveal, l.current, func(int) bool { return true }, 1)
	// Count opaque pixels per tile: every map is drawn; only map 1's pixels
	// come from the tinted picture.
	for m := 1; m <= gamestate.LANMaps; m++ {
		p := l.markers[m]
		drawn, tinted := 0, 0
		for y := p.Y; y < p.Y+70; y++ {
			for x := p.X; x < p.X+80; x++ {
				if r, ok := l.m.RegionAt(x, y); !ok || r != m {
					continue
				}
				c := img.RGBAAt(x, y)
				if c.A == 0 {
					continue
				}
				drawn++
				if c != l.reveal.At(x, y) {
					tinted++
				}
			}
		}
		if drawn == 0 {
			t.Errorf("map %d is not drawn", m)
		}
		if (m == 1) != (tinted > drawn/2) {
			t.Errorf("map %d: %d of %d pixels tinted", m, tinted, drawn)
		}
	}
}
