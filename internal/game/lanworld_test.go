package game

import (
	"os"
	"strings"
	"testing"
	"time"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
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
		s, err := a.lanWorld(dir, nil, at, nil, m, "")
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
	s, err := a.lanWorld(dir, nil, at, hero, 0, "Снежок")
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
	// The hero carries the account's name, in the status line too.
	if got := resfmt.CP866ToUTF8(s.game.EntityName(e, s.tables)); got != "Снежок" {
		t.Errorf("hero name %q", got)
	}
	if got := resfmt.CP866ToUTF8(s.game.StatusLine(e, s.tables)); !strings.HasPrefix(got, "Снежок, ") {
		t.Errorf("status line %q", got)
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
	s, err := a.lanWorld(dir, nil, netplay.EnterPayload{Map: 1, Row: 110, Col: 32, Facing: 2}, nil, 0, "")
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

// Another player's hero on the map: it appears as a walker outside the
// party, walks to reported cells, jumps when far, leaves the map, comes
// back after a map change, and is removed when the player leaves.
func TestLANRemotePlayers(t *testing.T) {
	const dir = "../../assets/Konung/"
	if _, err := os.Stat(dir + "MAPS.RES"); err != nil {
		t.Skip("MAPS.RES not available:", err)
	}
	a, err := loadSceneAssets(dir+"GRAPH.RES", dir+"OBJECTS.RES", dir, dir+"konung.exe", dir+"Intr800r.res")
	if err != nil {
		t.Skip("assets missing:", err)
	}
	s, err := a.lanWorld(dir, nil, netplay.EnterPayload{Map: 1, Row: 110, Col: 32, Facing: 2}, nil, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	var sent []netplay.Position
	s.onMove = func(p netplay.Position) { sent = append(sent, p) }

	ragnar := netplay.AppearPayload{
		Player: netplay.PlayerInfo{ID: "r1", Name: "Ragnar", Clan: "Викинги", Model: 2},
		Hero:   &netplay.Hero{Level: 4, Model: 2, Palette: 36},
		Pos:    netplay.Position{Map: 1, Row: 100, Col: 32, Facing: 3},
	}
	s.remoteAppear(ragnar)
	r := s.remotes["r1"]
	n := s.byEntity[r.entity]
	if n == nil || n.mode != npcRemote || n.cell != (mapdata.Cell{Row: 100, Col: 32}) {
		t.Fatalf("remote walker = %+v", n)
	}
	e := s.game.Entities[r.entity]
	if got := resfmt.CP866ToUTF8(s.game.EntityName(e, s.tables)); got != "Ragnar" {
		t.Errorf("remote name %q", got)
	}
	if e.Model != 2 || e.PaletteOffset != 36*512 || e.Level != 4 || e.Group == 0 || s.partyIndex(r.entity) >= 0 {
		t.Errorf("remote entity: model %d palette %#x level %d group %d", e.Model, e.PaletteOffset, e.Level, e.Group)
	}
	if got := resfmt.CP866ToUTF8(remoteStatus(r)); got != "Ragnar, клан Викинги" {
		t.Errorf("status %q", got)
	}

	// A nearby cell: a walk. A far one: a jump.
	s.remoteMove("r1", netplay.Position{Map: 1, Row: 104, Col: 33, Facing: 1})
	if n := s.byEntity[r.entity]; n == nil || len(n.path) == 0 && !n.walking {
		t.Error("no walk to a nearby cell")
	}
	s.remoteMove("r1", netplay.Position{Map: 1, Row: 200, Col: 40})
	if n := s.byEntity[r.entity]; n == nil || chebyshev(n.cell, mapdata.Cell{Row: 200, Col: 40}) > 3 {
		t.Errorf("no jump: %+v", n)
	}

	// Off this map: taken off; still known.
	s.remoteMove("r1", netplay.Position{Map: 15, Row: 66, Col: 57})
	if s.byEntity[r.entity] != nil || s.remotes["r1"] == nil {
		t.Error("remote on another map is still drawn here")
	}
	// The own hero takes exit 3 to map 15: the remote is there.
	x, _ := s.game.ExitAt(1, 65, 7)
	s.changeMap(x)
	if s.mapNumber != 15 || s.byEntity[s.remotes["r1"].entity] == nil {
		t.Errorf("after the map change: map %d, remote placed %v", s.mapNumber, s.byEntity[s.remotes["r1"].entity] != nil)
	}

	// The own position is reported once per change.
	sent = nil
	s.reportMove()
	s.reportMove()
	if len(sent) != 1 || sent[0].Map != 15 {
		t.Errorf("sent %+v", sent)
	}

	// The map window counts the own hero and Ragnar, both on map 15.
	if c := s.playerCounts(); c[15] != 2 || len(c) != 1 {
		t.Errorf("player counts %v", c)
	}

	s.remoteLeave("r1")
	if len(s.remotes) != 0 {
		t.Error("the player who left is still known")
	}
	for _, n := range s.npcs {
		if n.mode == npcRemote {
			t.Error("a remote walker is left on the map")
		}
	}
}

// A remote hero wears what the own hero wears at the start (the GAME.N
// template's equipment), not the creation screen's preview gear.
func TestLANRemoteEquipment(t *testing.T) {
	const dir = "../../assets/Konung/"
	if _, err := os.Stat(dir + "MAPS.RES"); err != nil {
		t.Skip("MAPS.RES not available:", err)
	}
	a, err := loadSceneAssets(dir+"GRAPH.RES", dir+"OBJECTS.RES", dir, dir+"konung.exe", dir+"Intr800r.res")
	if err != nil {
		t.Skip("assets missing:", err)
	}
	s, err := a.lanWorld(dir, nil, netplay.EnterPayload{Map: 1, Row: 110, Col: 32, Facing: 2}, nil, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	s.remoteAppear(netplay.AppearPayload{Player: netplay.PlayerInfo{ID: "x", Name: "Twin", Model: 0}, Pos: netplay.Position{Map: 1, Row: 100, Col: 32}})
	if own, other := s.party[0].Equipment, s.game.Entities[s.remotes["x"].entity].Equipment; own != other {
		t.Errorf("equipment: own %v, remote %v", own, other)
	}
}

// In a LAN world the scene follows the server's clock: a far clock is
// jumped to, a near one is reached by running ticks; the moon is the same
// for everyone.
func TestLANClock(t *testing.T) {
	const dir = "../../assets/Konung/"
	if _, err := os.Stat(dir + "MAPS.RES"); err != nil {
		t.Skip("MAPS.RES not available:", err)
	}
	a, err := loadSceneAssets(dir+"GRAPH.RES", dir+"OBJECTS.RES", dir, dir+"konung.exe", dir+"Intr800r.res")
	if err != nil {
		t.Skip("assets missing:", err)
	}
	s, err := a.lanWorld(dir, nil, netplay.EnterPayload{Map: 1, Row: 110, Col: 32, Facing: 2}, nil, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	s.clock = &serverClock{c: netplay.ClockPayload{Tick: 5000, Speed: 1, SessionLeft: -1}, at: time.Now()}
	s.stepToServer()
	if s.tick != 5000 {
		t.Fatalf("tick %d after a far clock, want 5000", s.tick)
	}
	s.clock.c.Tick = 5003
	s.stepToServer()
	if s.tick != 5003 {
		t.Errorf("tick %d after a near clock, want 5003 (three steps)", s.tick)
	}
	if left, ok := s.clock.sessionLeft(); ok || left != 0 {
		t.Error("unknown session time shown")
	}
	moonlit := 0
	for d := range 1000 {
		if lanMoon(d) {
			moonlit++
		}
	}
	if moonlit < 850 || moonlit > 950 {
		t.Errorf("%d of 1000 nights moonlit, want about %d%%", moonlit, gamestate.MoonChance)
	}
}
