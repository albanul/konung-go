package game

import (
	"os"
	"testing"

	"konung1.5/internal/gamestate"
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
	return s.party[0].Bag[0]
}

// TestMagicOrbJumps: «Магический шар волхва Родников» (misc 18, 0x00415383)
// moves the party to map 3 once the click is handled, and is kept.
func TestMagicOrbJumps(t *testing.T) {
	if _, err := os.Stat("../../assets/Konung/KONUNG.3"); err != nil {
		t.Skip("KONUNG.3 not available")
	}
	s := newVolkScene(t)
	orb := giveMisc(t, s, 18)
	s.game.World = sceneWorld{s}
	s.game.UseBagItem(&s.hand, &s.party[0], s.tables, 0)
	if !s.miscRequests() {
		t.Fatal("the orb asked for no map change")
	}
	if s.mapNumber != 3 || s.game.Groups[0].Row != 141 || s.game.Groups[0].Col != 54 {
		t.Fatalf("map %d, party at (%d,%d)", s.mapNumber, s.game.Groups[0].Row, s.game.Groups[0].Col)
	}
	if s.party[0].Bag[0] != orb {
		t.Error("the orb was used up")
	}
}

// TestShovelClosesWindow: the shovel from the bag (0x00415504) closes the
// character window, keeps the strip and enters dig mode.
func TestShovelClosesWindow(t *testing.T) {
	s := newVolkScene(t)
	giveMisc(t, s, gamestate.MiscShovel)
	s.game.World = sceneWorld{s}
	s.inv.open = true
	s.game.UseBagItem(&s.hand, &s.party[0], s.tables, 0)
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
	for _, c := range s.npcs {
		if c.mode == npcCreature {
			n = c
			break
		}
	}
	if n == nil {
		t.Skip("no creature on the start map")
	}
	e := &s.game.Entities[n.entity]
	e.Flags4 = 0x41
	if s.retreat(e) {
		t.Fatal("kind 0x41 retreated")
	}
	e.Flags4 = kindRetreat
	from := n.cell
	if !s.retreat(e) {
		t.Fatal("kind 0x49 did not retreat")
	}
	goal := n.path[len(n.path)-1]
	dr, dc := goal.Row-from.Row, goal.Col-from.Col
	far := max(dr, -dr) >= gamestate.VillageRowRange || max(dc, -dc) >= gamestate.VillageColRange
	if n.intent != intentWalk || !far {
		t.Errorf("intent %#x, walk from %v to %v", n.intent, from, goal)
	}
}
