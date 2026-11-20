package game

import (
	"os"
	"testing"
	"time"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
)

// viewScene builds a LAN viewer for a Волк hero in slot 3 on map 1's start.
func viewScene(t *testing.T) *startScene {
	t.Helper()
	const dir = "../../assets/Konung/"
	if _, err := os.Stat(dir + "MAPS.RES"); err != nil {
		t.Skip("MAPS.RES not available:", err)
	}
	a, err := loadSceneAssets(dir+"GRAPH.RES", dir+"OBJECTS.RES", dir, dir+"konung.exe", dir+"Intr800r.res")
	if err != nil {
		t.Skip("assets missing:", err)
	}
	hero := &netplay.Hero{Level: 3, Class: gamestate.ClassHunter, Model: 0, Palette: 71, Gold: 250, Strength: 16, Endurance: 11, Fencing: 150}
	at := netplay.EnterPayload{Position: netplay.Position{Map: 1, Row: 110, Col: 32, Facing: 2}, Slot: 3}
	s, err := a.lanView(dir, nil, at, hero, 0, "Snow")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The viewer: GAME.RES's world, the MAPS.RES map, the own hero alone as
// party member 0 (its slot's entity, group 0 here) with the account's
// values; nothing else until the server reports it.
func TestLANView(t *testing.T) {
	s := viewScene(t)
	if !s.lan || s.view == nil || s.MapNumber != 1 || s.Hero == nil || s.Hero.Cell != (mapdata.Cell{Row: 110, Col: 32}) {
		t.Fatalf("scene: lan %v view %v map %d hero %+v", s.lan, s.view != nil, s.MapNumber, s.Hero)
	}
	e := s.Party[0]
	if len(s.Party) != 1 || e.Index != 3 || e.Group != 0 || e.Level != 3 || e.Flags4&7 != gamestate.ClassHunter || e.PaletteOffset != 71*512 || e.Equipment == ([6]int{}) {
		t.Errorf("own hero = %+v", e)
	}
	if len(s.NPCs) != 0 {
		t.Errorf("%d characters before any view", len(s.NPCs))
	}
	if got := string(s.Game.EntityName(e, s.Tables)); got != string(cp866("Snow")) {
		t.Errorf("name %q", got)
	}
}

// Views make, move and remove walkers; a player's hero is drawn from
// MsgPlayer; the own hero's HP and death come from the server.
func TestLANViewStates(t *testing.T) {
	s := viewScene(t)
	// A map-1 villager of GAME.RES.
	villager := -1
	for i, e := range s.Game.Entities {
		if i >= 16 && e.Group >= 16 && s.Game.Groups[e.Group].Map == 1 && !e.Dead() && e.Row > 0 {
			villager = i
			break
		}
	}
	if villager < 0 {
		t.Fatal("no villager on map 1 in GAME.RES")
	}
	ve := s.Game.Entities[villager]
	s.applyPlayer(netplay.PlayerPayload{Slot: 5, Player: netplay.PlayerInfo{ID: "o", Name: "Olaf", Clan: "Викинги", Model: 1},
		Hero: &netplay.Hero{Level: 2, Model: 1, Palette: 29}})
	s.applyView(netplay.ViewPayload{Map: 1, Tick: 900, Full: true, Chars: []netplay.CharState{
		{Entity: 3, Player: true, Row: 110, Col: 32, Facing: 2, HP: 1600},
		{Entity: villager, Row: ve.Row, Col: ve.Col, Facing: 1, HP: ve.HP},
		{Entity: 5, Player: true, Row: 100, Col: 30, Facing: 4, HP: 1500, Stance: true, Weapon: gamestate.WeaponBlade},
	}})
	if s.Tick != 900 || len(s.NPCs) != 2 {
		t.Fatalf("tick %d, %d walkers", s.Tick, len(s.NPCs))
	}
	olaf := s.ByEntity[5]
	oe := s.Game.Entities[5]
	if olaf == nil || olaf.Cell != (mapdata.Cell{Row: 100, Col: 30}) || oe.Model != 1 || oe.PaletteOffset != 29*512 || !oe.InStance() || oe.HP != 1500 {
		t.Errorf("Olaf: walker %+v, record model %d palette %d stance %v HP %d", olaf, oe.Model, oe.PaletteOffset/512, oe.InStance(), oe.HP)
	}
	if st, ok := s.viewStatus(5); !ok || string(st) != string(cp866("Olaf, клан Викинги")) {
		t.Errorf("status %q", st)
	}
	// A walk step, then the villager is gone.
	s.applyView(netplay.ViewPayload{Map: 1, Tick: 901, Chars: []netplay.CharState{
		{Entity: 5, Player: true, Row: 102, Col: 30, FromRow: 100, FromCol: 30, Walking: true, Step: 3, Facing: 4, HP: 1500, Stance: true},
	}, Gone: []int{villager}})
	if !olaf.Walking || olaf.Cell != (mapdata.Cell{Row: 102, Col: 30}) || olaf.From != (mapdata.Cell{Row: 100, Col: 30}) || olaf.StepT != 3 {
		t.Errorf("Olaf walking: %+v", olaf.Walker)
	}
	if s.ByEntity[villager] != nil || len(s.NPCs) != 1 {
		t.Error("the villager did not leave")
	}
	// The own hero is hit, then dies: the game is over.
	s.applyView(netplay.ViewPayload{Map: 1, Tick: 902, Chars: []netplay.CharState{{Entity: 3, Player: true, Row: 110, Col: 32, HP: 700, Act: gamestate.ActFlinch, ActStep: 1}}})
	if s.Party[0].HP != 700 || s.Hero.Act != gamestate.ActFlinch {
		t.Errorf("own hero after a hit: HP %d act %d", s.Party[0].HP, s.Hero.Act)
	}
	s.applyView(netplay.ViewPayload{Map: 1, Tick: 903, Chars: []netplay.CharState{{Entity: 3, Player: true, Row: 110, Col: 32, HP: 0, Dead: true, Act: gamestate.ActDeath, ActStep: 5}}})
	if !s.GameOver() {
		t.Error("the own hero's death did not end the game")
	}
	// A view of another map (before its map change) is not applied.
	s.applyView(netplay.ViewPayload{Map: 2, Tick: 904, Full: true})
	if len(s.NPCs) != 1 {
		t.Error("a view of another map was applied")
	}
}

// The own hero's map change (MsgMapChange) builds the new map's scene with
// the hero on its arrival cell, the viewer kept.
func TestLANViewMapChange(t *testing.T) {
	s := viewScene(t)
	s.applyView(netplay.ViewPayload{Map: 1, Tick: 10, Full: true, Chars: []netplay.CharState{{Entity: 3, Player: true, Row: 110, Col: 32, HP: 1234}}})
	v := s.view
	s.hideHelp = true // F1 pressed before the map change
	s.viewChangeMap(netplay.EnterPayload{Position: netplay.Position{Map: 3, Row: 70, Col: 40, Facing: 1}, Slot: 3})
	if !s.hideHelp {
		t.Error("the key help came back after the map change")
	}
	if s.MapNumber != 3 || s.view != v || s.Hero == nil || s.Hero.Cell != (mapdata.Cell{Row: 70, Col: 40}) || s.Party[0].HP != 1234 || len(s.NPCs) != 0 {
		t.Errorf("after the change: map %d view kept %v hero %+v HP %d npcs %d", s.MapNumber, s.view == v, s.Hero, s.Party[0].HP, len(s.NPCs))
	}
}

// The session timer and the shared moon.
func TestLANClock(t *testing.T) {
	k := &serverClock{c: netplay.ClockPayload{Tick: 5000, SessionLeft: -1}, at: time.Now()}
	if _, ok := k.sessionLeft(); ok {
		t.Error("unknown session time shown")
	}
	k.c.SessionLeft = 90
	if left, ok := k.sessionLeft(); !ok || left > 90*time.Second || left < 89*time.Second {
		t.Errorf("session left %v", left)
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

// LAN fixed light: «B1» (map 18) keeps its tint at night, the other maps
// follow the time of day; single player's cave flags are not read with
// the LAN map numbers.
func TestLANFixedLight(t *testing.T) {
	exeData, err := os.ReadFile("../../assets/Konung/LAN_KONG.EXE")
	if err != nil {
		t.Skip("LAN_KONG.EXE not available:", err)
	}
	exe, err := gamestate.ParseLANExe(exeData)
	if err != nil {
		t.Fatal(err)
	}
	s := viewScene(t)
	s.assets.lanExe = exe
	s.dayNight = true
	s.Tick = 12000 // night (8100 ≤ time of day < 18900)
	night := func(m int) resfmt.Tint {
		s.MapNumber = m
		s.updateLight()
		return s.tint
	}
	if got := night(18); got != (resfmt.Tint{R: -1, G: -1, B: -1}) {
		t.Errorf("B1 at night: tint %+v, want its fixed (−1, −1, −1)", got)
	}
	if got := night(1); got == (resfmt.Tint{R: -1, G: -1, B: -1}) || got.Zero() {
		t.Errorf("A3 at night: tint %+v, want the night's", got)
	}
}
