package game

import (
	"konung1.5/internal/world"
	"os"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// giveMisc puts a new misc item of type typ at the front of the hero's bag
// (dialogue script 166 + type, 0x004359D4).
func giveMisc(t *testing.T, s *startScene, typ int) int {
	t.Helper()
	tk := s.newTalk()
	if gamestate.ScriptFunc(tk, 166+typ) != 1 {
		t.Fatalf("script %d failed", 166+typ)
	}
	s.afterTalk(tk, nil)
	return s.Party[0].Bag[0]
}

// TestMagicOrbJumps: «Магический шар волхва Родников» (misc 18, 0x00415383)
// moves the party to map 3 once the click is handled, and is kept.
func TestMagicOrbJumps(t *testing.T) {
	if _, err := os.Stat("../../assets/Konung/KONUNG.3"); err != nil {
		t.Skip("KONUNG.3 not available")
	}
	s := newVolkScene(t)
	orb := giveMisc(t, s, 18)
	s.Game.World = s.Access()
	s.Game.UseBagItem(&s.hand, &s.Party[0], s.Tables, 0)
	if !s.miscRequests() {
		t.Fatal("the orb asked for no map change")
	}
	if s.MapNumber != 3 || s.Game.Groups[0].Row != 141 || s.Game.Groups[0].Col != 54 {
		t.Fatalf("map %d, party at (%d,%d)", s.MapNumber, s.Game.Groups[0].Row, s.Game.Groups[0].Col)
	}
	if s.Party[0].Bag[0] != orb {
		t.Error("the orb was used up")
	}
}

// TestShovelClosesWindow: the shovel from the bag (0x00415504) closes the
// character window, keeps the strip and enters dig mode.
func TestShovelClosesWindow(t *testing.T) {
	s := newVolkScene(t)
	giveMisc(t, s, gamestate.MiscShovel)
	s.Game.World = s.Access()
	s.inv.open = true
	s.Game.UseBagItem(&s.hand, &s.Party[0], s.Tables, 0)
	s.miscRequests()
	if s.inv.open || !s.inv.quick || !s.hand.Digging() {
		t.Errorf("window %v, strip %v, digging %v", s.inv.open, s.inv.quick, s.hand.Digging())
	}
}

// TestKind49Retreat: after a blow that did not kill, a kind 0x49 creature
// walks off (0x004126A1 → 0x004101E4) with the walk intent; other kinds
// swing on.
func TestKind49Retreat(t *testing.T) {
	s := newVolkScene(t)
	var n *npc
	for _, c := range s.NPCs {
		if c.Mode == npcCreature {
			n = c
			break
		}
	}
	if n == nil {
		t.Skip("no creature on the start map")
	}
	e := &s.Game.Entities[n.Entity]
	e.Flags4 = 0x41
	if s.Retreat(e) {
		t.Fatal("kind 0x41 retreated")
	}
	e.Flags4 = world.KindRetreat
	from := n.Cell
	if !s.Retreat(e) {
		t.Fatal("kind 0x49 did not retreat")
	}
	goal := n.Path[len(n.Path)-1]
	dr, dc := goal.Row-from.Row, goal.Col-from.Col
	far := max(dr, -dr) >= gamestate.VillageRowRange || max(dc, -dc) >= gamestate.VillageColRange
	if n.Intent != world.IntentWalk || !far {
		t.Errorf("intent %#x, walk from %v to %v", n.Intent, from, goal)
	}
}

// TestApprenticeWalksToPost: an apprentice (+4 bits 0x38, script 354) walks
// to a free cell next to its master's post and, once there, turns to the
// post's facing (0x00416A94).
func TestApprenticeWalksToPost(t *testing.T) {
	s := newVolkScene(t)
	n := s.ByEntity[359]
	if n == nil || n.Mode != npcVillager {
		t.Skip("villager 359 is not on the start map")
	}
	s.Game.Entities[359].Flags4 |= 1 << 3
	p, ok := s.Game.ApprenticePost(359)
	if !ok {
		t.Fatal("no apprentice post")
	}
	apprentice := func() (gamestate.Post, bool) { return s.Game.ApprenticePost(359) }
	next := func(c mapdata.Cell) bool {
		for _, nb := range mapdata.Neighbors(mapdata.Cell{Row: p.Row, Col: p.Col}) {
			if nb.Cell == c {
				return true
			}
		}
		return false
	}
	if n.VillageTick(1, apprentice) != 1 || len(n.Path) == 0 || !next(n.Path[len(n.Path)-1]) {
		t.Fatalf("no walk next to the post (%d,%d): path %v", p.Row, p.Col, n.Path)
	}
	goal := n.Path[len(n.Path)-1]
	n.Halt()
	n.Cell, n.Path = goal, nil
	n.Dir = mapdata.Direction(p.Facing+1) & 7
	if n.VillageTick(1, apprentice) != 0 || n.Dir != mapdata.Direction(p.Facing&7) {
		t.Errorf("next to the Post: facing %d, want %d", n.Dir, p.Facing&7)
	}
}

// TestSurrenderFilm: a village that surrenders asks for DEFEAT<map>.FLM,
// after which the game goes on (0x0041D777).
func TestSurrenderFilm(t *testing.T) {
	s := newVolkScene(t)
	st := s.Game.SettlementOnMap(s.MapNumber)
	if st == nil {
		t.Skip("no settlement on the start map")
	}
	s.Game.Groups[st.Group].Alarm = gamestate.AlarmAttack
	// One man left alive and a strong hero: more than twice his strength.
	s.Party[0].BaseStats = [4]int{250, 250, 250, 250}
	s.Party[0].Stats = s.Party[0].BaseStats
	gr, men := s.Game.Groups[st.Group], 0
	for k := range gr.Count {
		if e := &s.Game.Entities[gr.First+k]; e.Model < 3 && !e.Dead() {
			if men++; men > 1 {
				e.Flags4 |= 0x80
			}
		}
	}
	s.SettlementTick()
	if s.Film != "defeat1" || s.FilmToMenu {
		t.Fatalf("film %q, to menu %v", s.Film, s.FilmToMenu)
	}
}

// TestWinPile: arriving on pile record 0 (map 46) asks for WIN.FLM and the
// menu, and takes nothing (0x00411154).
func TestWinPile(t *testing.T) {
	s := newVolkScene(t)
	p := s.Game.Piles[0]
	if p.Index != 0 || p.Map != 46 {
		t.Skip("pile 0 is not the map-46 pile")
	}
	m := s.Lead()
	s.PileAt[m.Cell] = 0 // as if the hero stood on it
	bag := s.Party[0].Bag
	s.Arrive(m)
	if s.Film != "win" || !s.FilmToMenu {
		t.Fatalf("film %q, to menu %v", s.Film, s.FilmToMenu)
	}
	if s.Party[0].Bag != bag || s.Game.Piles[0].Items != p.Items {
		t.Error("the win pile was picked up")
	}
}

// TestGameOverAtOnce: the main loop ends the game as soon as the hero's
// dead bit is set (0x004376A3), with no delay.
func TestGameOverAtOnce(t *testing.T) {
	s := newVolkScene(t)
	if s.GameOver() {
		t.Fatal("game over with a living hero")
	}
	s.Party[0].Flags4 |= 0x80
	if !s.GameOver() {
		t.Fatal("no game over on the frame the hero died")
	}
}
