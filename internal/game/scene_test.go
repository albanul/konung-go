package game

import (
	"os"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// TestStoreWorldKeepsCharacters checks that a character that moved on map 1
// is placed on its new cell, with its facing, when map 1 is entered again.
func TestStoreWorldKeepsCharacters(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/GAME.0")
	if err != nil {
		t.Skip("GAME.0 not available:", err)
	}
	g, err := gamestate.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var villager gamestate.Placed
	for _, p := range g.OnMap(1, nil) {
		if p.Group != 0 {
			villager = p
			break
		}
	}
	moved := mapdata.Cell{Row: villager.Row + 2, Col: villager.Col}
	s := &startScene{game: g, npcs: []*npc{{walker: walker{entity: villager.Index, cell: moved, dir: 3}}}}
	s.storeWorld()
	for _, p := range g.OnMap(1, nil) {
		if p.Index == villager.Index {
			if p.Row != moved.Row || p.Col != moved.Col || p.Variant != 3 {
				t.Fatalf("entity %d back at (%d,%d) facing %d, want %v facing 3", p.Index, p.Row, p.Col, p.Variant, moved)
			}
			return
		}
	}
	t.Fatalf("entity %d not on map 1 any more", villager.Index)
}

// TestNewGameEachHero starts a game with each hero from the creation
// screen: GAME.<model> and KONUNG.<model·3+1> load and the hero carries the
// chosen class and stats at level 1.
func TestNewGameEachHero(t *testing.T) {
	const dir = "../../assets/Konung/"
	a, err := loadSceneAssets(dir+"GRAPH.RES", dir+"OBJECTS.RES", dir, dir+"konung.exe", dir+"Intr800r.res")
	if err != nil {
		t.Skip("assets missing:", err)
	}
	for _, model := range []int{gamestate.HeroVolk, gamestate.HeroDragomir, gamestate.HeroMichail} {
		hero := gamestate.NewHeroTemplate()
		gamestate.SelectHero(&hero, model)
		gamestate.SelectClass(&hero, gamestate.ClassHunter)
		w, err := a.newGame(hero)
		if err != nil {
			t.Fatalf("model %d: %v", model, err)
		}
		if w.mapNumber != gamestate.HeroStartMap(model) || w.hero == nil {
			t.Fatalf("model %d: map %d, hero %v", model, w.mapNumber, w.hero != nil)
		}
		e := w.game.Entities[w.hero.entity]
		if e.Model != model || e.Level != 1 || e.Flags4 != gamestate.ClassHunter || e.BaseStats != [4]int{3, 3, 15, 10} {
			t.Errorf("model %d: hero %+v", model, e)
		}
	}
}

// TestCamera: the game's camera (docs/konung_map_format.md, camera): follow
// off at a new game, the hero's sprite at the screen centre after the load
// unless a limit holds, the limits, and the push follow.
func TestCamera(t *testing.T) {
	s := newVolkScene(t)
	if s.follow {
		t.Error("follow is on at a new game")
	}
	inside := func() bool {
		c, top := s.camX+viewLeft, s.camY+viewTop
		return c >= cameraMinX && top >= cameraMinY && c+viewW <= s.camW && top+viewH <= s.camH
	}
	if !inside() {
		t.Fatalf("camera %d,%d outside the limits (%d×%d)", s.camX, s.camY, s.camW, s.camH)
	}
	if f, ok := s.hero.frame(s.tick); ok {
		x, y := f.x-s.camX, f.y-s.camY
		atLimit := s.camX+viewLeft == cameraMinX || s.camY+viewTop == cameraMinY ||
			s.camX+viewLeft+viewW == s.camW || s.camY+viewTop+viewH == s.camH
		if !atLimit && (x != ScreenWidth/2 || y != ScreenHeight/2) {
			t.Errorf("hero sprite at screen %d,%d, want the centre", x, y)
		}
	}
	for _, c := range [][2]int{{-5000, -5000}, {50000, 50000}} {
		s.camX, s.camY = c[0], c[1]
		s.clampCamera()
		if !inside() {
			t.Errorf("clamp from %v gives %d,%d", c, s.camX, s.camY)
		}
	}
	// The push follow: a point well inside the view does not move the
	// camera; one left of the margin pulls the view's left edge to it − 60.
	s.camX, s.camY = 1000-viewLeft, 600-viewTop
	s.clampCamera()
	c0, t0 := s.camX, s.camY
	s.pushCamera(s.camX+viewLeft+viewW/2, s.camY+viewTop+viewH/2)
	if s.camX != c0 || s.camY != t0 {
		t.Errorf("a centred point moved the camera to %d,%d", s.camX, s.camY)
	}
	x := s.camX + viewLeft + 10
	s.pushCamera(x, s.camY+viewTop+viewH/2)
	if s.camX+viewLeft != x-followMarginX {
		t.Errorf("view left %d, want %d", s.camX+viewLeft, x-followMarginX)
	}
}

// TestFollowStopsScroll: with follow on, a scroll away from the hero stops
// at the follow margin and the camera then stays put (no shaking).
func TestFollowStopsScroll(t *testing.T) {
	s := newVolkScene(t)
	s.follow = true
	scroll := func() { // one update with the mouse at the right edge
		s.camX += scrollSpeed
		s.clampCamera()
		s.followHero()
	}
	scroll()
	var last int
	for i := range 200 {
		scroll()
		if i > 100 && s.camX != last {
			t.Fatalf("the camera still moves: %d after %d", s.camX, last)
		}
		last = s.camX
	}
	x, _, _ := s.followPoint(s.hero)
	if left := s.camX + viewLeft; x-left != followMarginX && left != cameraMinX {
		t.Errorf("hero %d px inside the view, want %d", x-left, followMarginX)
	}
}
