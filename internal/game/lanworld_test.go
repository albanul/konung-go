package game

import (
	"os"
	"strings"
	"testing"
	"time"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/lanfight"
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
		if e := s.Party[0]; e.Model != m || e.PaletteOffset != gamestate.HeroPalette[m] {
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
	if !s.lan || s.MapNumber != 7 || s.Hero == nil || s.Hero.Cell != (mapdata.Cell{Row: 65, Col: 8}) || s.Hero.Dir != 4 {
		t.Fatalf("scene: lan %v, map %d, hero %+v", s.lan, s.MapNumber, s.Hero)
	}
	e := s.Party[0]
	if e.Model != 2 || e.PaletteOffset != 35*512 || e.Gold != 250 || e.Level != 3 || e.BaseStats != [4]int{4, 16, 5, 11} || e.Fencing != 150 {
		t.Errorf("hero entity = %+v", e)
	}
	// The hero carries the account's name, in the status line too.
	if got := resfmt.CP866ToUTF8(s.Game.EntityName(e, s.Tables)); got != "Снежок" {
		t.Errorf("hero name %q", got)
	}
	if got := resfmt.CP866ToUTF8(s.Game.StatusLine(e, s.Tables)); !strings.HasPrefix(got, "Снежок, ") {
		t.Errorf("status line %q", got)
	}
	if len(s.NPCs) != 0 || len(s.Party) != 1 {
		t.Errorf("%d characters besides a party of %d", len(s.NPCs), len(s.Party))
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
	x, ok := s.Game.ExitAt(1, 65, 7) // GAME.RES exit 3: map 1 → map 15
	if !ok || x.To != 15 {
		t.Fatalf("exit at (65,7) on map 1 = %+v, %v", x, ok)
	}
	s.changeMap(x)
	if s.MapNumber != 15 || !s.lan {
		t.Fatalf("after the exit: map %d, lan %v", s.MapNumber, s.lan)
	}
	if s.Hero == nil || s.Hero.Cell != (mapdata.Cell{Row: x.Row, Col: x.Col}) {
		t.Errorf("hero at %+v, want the arrival (%d, %d)", s.Hero, x.Row, x.Col)
	}
	maps, _ := os.ReadFile(dir + "MAPS.RES")
	e, _ := mapdata.SplitMapsRes(maps)
	m15, _ := mapdata.Parse(e[14].Map)
	if s.m.LastRow != m15.LastRow || s.m.Cells[20][20] != m15.Cells[20][20] {
		t.Error("the new map is not MAPS.RES #15")
	}
	// And back: map 15 has exits too.
	back := 0
	for _, x := range s.Game.Exits {
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
	n := s.ByEntity[r.Entity]
	if n == nil || n.Mode != npcRemote || n.Cell != (mapdata.Cell{Row: 100, Col: 32}) {
		t.Fatalf("remote walker = %+v", n)
	}
	e := s.Game.Entities[r.Entity]
	if got := resfmt.CP866ToUTF8(s.Game.EntityName(e, s.Tables)); got != "Ragnar" {
		t.Errorf("remote name %q", got)
	}
	if e.Model != 2 || e.PaletteOffset != 36*512 || e.Level != 4 || e.Group == 0 || s.PartyIndex(r.Entity) >= 0 {
		t.Errorf("remote Entity: model %d palette %#x level %d group %d", e.Model, e.PaletteOffset, e.Level, e.Group)
	}
	if got := resfmt.CP866ToUTF8(remoteStatus(r)); got != "Ragnar, клан Викинги" {
		t.Errorf("status %q", got)
	}

	// A nearby Cell: a walk. A far one: a jump.
	s.remoteMove("r1", netplay.Position{Map: 1, Row: 104, Col: 33, Facing: 1})
	if n := s.ByEntity[r.Entity]; n == nil || len(n.Path) == 0 && !n.Walking {
		t.Error("no walk to a nearby cell")
	}
	s.remoteMove("r1", netplay.Position{Map: 1, Row: 200, Col: 40})
	if n := s.ByEntity[r.Entity]; n == nil || chebyshev(n.Cell, mapdata.Cell{Row: 200, Col: 40}) > 3 {
		t.Errorf("no jump: %+v", n)
	}

	// Off this map: taken off; still known.
	s.remoteMove("r1", netplay.Position{Map: 15, Row: 66, Col: 57})
	if s.ByEntity[r.Entity] != nil || s.remotes["r1"] == nil {
		t.Error("remote on another map is still drawn here")
	}
	// The own hero takes exit 3 to map 15: the remote is there.
	x, _ := s.Game.ExitAt(1, 65, 7)
	s.changeMap(x)
	if s.MapNumber != 15 || s.ByEntity[s.remotes["r1"].Entity] == nil {
		t.Errorf("after the map change: map %d, remote placed %v", s.MapNumber, s.ByEntity[s.remotes["r1"].Entity] != nil)
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
	for _, n := range s.NPCs {
		if n.Mode == npcRemote {
			t.Error("a remote walker is left on the map")
		}
	}
}

// A remote hero wears what its look says (the server's MsgLook): its own
// items, and in the stance the body's combat set together with the stance
// bit, so the drawn weapon shows (the bug: type 0 without the bit drew the
// stance with no weapon).
func TestLANRemoteLook(t *testing.T) {
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
	own := lanfight.LookOf(s.Game, s.Party[0]) // the same type's gear
	s.remoteAppear(netplay.AppearPayload{Player: netplay.PlayerInfo{ID: "x", Name: "Twin", Model: 0}, Look: own, HP: 1600,
		Pos: netplay.Position{Map: 1, Row: 100, Col: 32}})
	r := s.remotes["x"]
	e := s.Game.Entities[r.Entity]
	if e.InStance() || e.Type < 6 {
		t.Errorf("out of the stance: stance %v type %d", e.InStance(), e.Type)
	}
	if got := lanfight.LookOf(s.Game, e); got.Equip != own.Equip {
		t.Errorf("remote equipment %+v, want %+v", got.Equip, own.Equip)
	}
	if e.Equipment == s.Party[0].Equipment {
		t.Error("the remote hero shares the own hero's item records")
	}
	l := own
	l.Stance = true
	for k, it := range l.Equip {
		if it.Item && k <= gamestate.WeaponBlade && k != gamestate.WeaponBow {
			l.Weapon = k
		}
	}
	s.remoteLook(netplay.LookPayload{ID: "x", Look: l})
	e = s.Game.Entities[r.Entity]
	if !e.InStance() || e.Type >= 6 || e.WeaponMode != l.Weapon {
		t.Fatalf("in the stance: stance %v type %d weapon %d", e.InStance(), e.Type, e.WeaponMode)
	}
	if n := s.ByEntity[r.Entity]; n == nil || !n.Look.Stance {
		t.Error("the walker's look did not change")
	}
	if l.Equip[l.Weapon].Item && len(s.Game.Overlays(e, s.Tables)) == 0 {
		t.Error("no overlays in the stance: the weapon is not drawn")
	}
	// Hits: the server's verdict sets HP, a kill plays the death.
	s.remoteHit(netplay.HitPayload{Attacker: "me", Target: "x", Result: netplay.HitLight, Damage: 40, HP: 1560}, "me")
	if hp := s.Game.Entities[r.Entity].HP; hp != 1560 {
		t.Errorf("HP %d after a hit", hp)
	}
	s.remoteHit(netplay.HitPayload{Attacker: "me", Target: "x", Result: netplay.HitKill, Killed: true}, "me")
	if n := s.ByEntity[r.Entity]; n == nil || n.Act != gamestate.ActDeath || !r.dead {
		t.Error("the kill did not play the death")
	}
	// The own hero dies by the server's verdict too.
	s.remoteHit(netplay.HitPayload{Attacker: "x", Target: "me", Result: netplay.HitKill, Killed: true}, "me")
	if s.Party[0].HP != 0 || s.Hero.Act != gamestate.ActDeath {
		t.Errorf("own hero: HP %d, act %d", s.Party[0].HP, s.Hero.Act)
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
	if s.Tick != 5000 {
		t.Fatalf("tick %d after a far clock, want 5000", s.Tick)
	}
	s.clock.c.Tick = 5003
	s.stepToServer()
	if s.Tick != 5003 {
		t.Errorf("tick %d after a near clock, want 5003 (three steps)", s.Tick)
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

// On a server that protects teams, a teammate (same hero type) is no
// Target: A picks the foe even when the teammate is closer.
func TestLANTeamSafe(t *testing.T) {
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
	s.remoteAppear(netplay.AppearPayload{Player: netplay.PlayerInfo{ID: "mate", Name: "Mate", Model: 0}, HP: 1600, Pos: netplay.Position{Map: 1, Row: 106, Col: 32}})
	s.remoteAppear(netplay.AppearPayload{Player: netplay.PlayerInfo{ID: "foe", Name: "Foe", Model: 1}, HP: 1600, Pos: netplay.Position{Map: 1, Row: 100, Col: 32}})
	mate, foe := s.remotes["mate"].Entity, s.remotes["foe"].Entity
	if got := s.NearestEnemy(s.Hero.Cell); got != mate {
		t.Fatalf("with team damage the nearest is %d, want the teammate %d", got, mate)
	}
	s.teamSafe = true
	if got := s.NearestEnemy(s.Hero.Cell); got != foe {
		t.Errorf("team-safe: nearest %d, want the foe %d", got, foe)
	}
}

// A remote hero's swing plays out and ends in idle, and the next blow
// swings again; a hero entering where another stands yields the cell.
func TestLANRemoteSwingAndStart(t *testing.T) {
	const dir = "../../assets/Konung/"
	if _, err := os.Stat(dir + "MAPS.RES"); err != nil {
		t.Skip("MAPS.RES not available:", err)
	}
	a, err := loadSceneAssets(dir+"GRAPH.RES", dir+"OBJECTS.RES", dir, dir+"konung.exe", dir+"Intr800r.res")
	if err != nil {
		t.Skip("assets missing:", err)
	}
	start := netplay.EnterPayload{Map: 1, Row: 110, Col: 32, Facing: 2}
	s, err := a.lanWorld(dir, nil, start, nil, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	// Ivar already stands on the start: the own hero yields, Ivar keeps it.
	there := mapdata.Cell{Row: start.Row, Col: start.Col}
	s.yieldStart([]mapdata.Cell{there})
	if s.Hero.Cell == there {
		t.Fatal("the own hero kept the taken start cell")
	}
	look := lanfight.LookOf(s.Game, s.Party[0]) // the same type's gear, in the stance
	look.Stance = true
	s.remoteAppear(netplay.AppearPayload{Player: netplay.PlayerInfo{ID: "ivar", Name: "Ivar"}, HP: 1600, Look: look,
		Pos: netplay.Position{Map: 1, Row: start.Row, Col: start.Col, Facing: 2}})
	n := s.ByEntity[s.remotes["ivar"].Entity]
	if n == nil || n.Cell != there {
		t.Fatalf("Ivar placed at %+v, want his cell %+v", n.Cell, there)
	}
	// A report of the cell Ivar holds (a turn) does not step him off it.
	s.remoteMove("ivar", netplay.Position{Map: 1, Row: start.Row, Col: start.Col, Facing: 5})
	for range 40 {
		s.Step()
	}
	if n.Cell != there {
		t.Fatalf("Ivar stepped off his reported cell to %+v", n.Cell)
	}
	// Ivar's blow at the own hero: the swing plays and ends.
	hit := netplay.HitPayload{Attacker: "ivar", Target: "me", Result: netplay.HitLight, Damage: 10, HP: 1590}
	s.remoteHit(hit, "me")
	if n.Act != gamestate.ActAttack {
		t.Fatalf("no swing: act %d", n.Act)
	}
	steps := 0
	for n.Act != 0 && steps < 200 {
		s.faceRemotes()
		steps++
	}
	if n.Act != 0 || steps < 2 {
		t.Errorf("the swing did not play out: act %d after %d ticks", n.Act, steps)
	}
	s.remoteHit(hit, "me")
	if n.Act != gamestate.ActAttack || n.ActStep != 0 {
		t.Errorf("the next blow did not swing: act %d step %d", n.Act, n.ActStep)
	}
}
