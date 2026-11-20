package game

import (
	"strings"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

// hireSmith starts Volk's game and hires the smith of «Родники» (358).
func hireSmith(t *testing.T) (*startScene, *member) {
	t.Helper()
	s := newVolkScene(t)
	tk := s.newTalk()
	tk.NPC = 358
	if d := tk.OpenDialogue(358, nil); d == nil {
		t.Skip("smith has no dialogue")
	}
	if gamestate.ScriptFunc(tk, 291) != 1 {
		t.Fatal("hire failed")
	}
	s.afterTalk(tk, nil)
	if len(s.Party) != 2 {
		t.Fatalf("party %d after the hire", len(s.Party))
	}
	m, _ := s.PartyMember(s.Party[1].Index) // the hire copies him into group 0's next slot
	if m == nil {
		t.Fatal("the smith has no member walker")
	}
	return s, m
}

// run advances the scene by n game ticks.
func run(s *startScene, n int) {
	for range n {
		s.Step()
	}
}

// freeCellAway returns a free cell about dr rows from c.
func freeCellAway(s *startScene, c mapdata.Cell, dr int) mapdata.Cell {
	for d := 0; d < 40; d++ {
		for _, r := range []int{c.Row + dr + d, c.Row - dr - d} {
			if x := (mapdata.Cell{Row: r, Col: c.Col}); !s.Blocked(x) {
				if p := mapdata.FindPath(c, x, s.Blocked); len(p) > 0 {
					return x
				}
			}
		}
	}
	return c
}

// TestHiredMemberFollows: the hired smith follows the hero (0x0041FA5D)
// once the hero is 10 cells away; a walk order makes him stay; «Все ко мне»
// makes him follow again.
func TestHiredMemberFollows(t *testing.T) {
	s, smith := hireSmith(t)
	hero := s.Lead()
	goal := freeCellAway(s, hero.Cell, 30)
	s.WalkOrder(hero, goal, false)
	run(s, 600)
	if hero.Cell == goal && gamestate.Dist(hero.Cell.Row, hero.Cell.Col, smith.Cell.Row, smith.Cell.Col) >= gamestate.FollowDistance+4 {
		t.Fatalf("the smith stayed behind: hero %v, smith %v", hero.Cell, smith.Cell)
	}
	if hero.Cell != goal {
		t.Skipf("the hero did not reach %v (at %v)", goal, hero.Cell)
	}

	// The smith alone is ordered somewhere: he stays there afterwards.
	s.Sel = []int{s.Party[1].Index}
	stay := freeCellAway(s, smith.Cell, 6)
	for _, m := range s.Selected() {
		s.WalkOrder(m, stay, false)
	}
	run(s, 200)
	if smith.Intent&gamestate.IntentFollow != 0 {
		t.Fatalf("after a walk order the smith follows (+2 %#x)", smith.Intent)
	}
	away := freeCellAway(s, hero.Cell, 30)
	s.WalkOrder(hero, away, false)
	run(s, 600)
	if d := gamestate.Dist(away.Row, away.Col, smith.Cell.Row, smith.Cell.Col); d < gamestate.FollowDistance {
		t.Fatalf("the smith followed after a walk order (%d cells from the hero)", d)
	}
	s.GatherOrder()
	run(s, 600)
	if smith.Intent&gamestate.IntentFollow == 0 ||
		gamestate.Dist(hero.Cell.Row, hero.Cell.Col, smith.Cell.Row, smith.Cell.Col) >= gamestate.FollowDistance+4 {
		t.Fatalf("after «Все ко мне»: smith %v (+2 %#x), hero %v", smith.Cell, smith.Intent, hero.Cell)
	}
}

// TestSelectionOrders: a portrait click selects one member, Shift adds;
// the stance toggle works on the selection.
func TestSelectionOrders(t *testing.T) {
	s, smith := hireSmith(t)
	if len(s.Sel) != 1 || s.Sel[0] != s.Party[0].Index {
		t.Fatalf("selection after the hire %v", s.Sel)
	}
	s.portraitClick(1)
	if len(s.Sel) != 1 || s.Sel[0] != s.Party[1].Index {
		t.Fatalf("selection after the portrait click %v", s.Sel)
	}
	s.ToggleSelectionStance()
	if !s.Party[1].InStance() || s.Party[0].InStance() {
		t.Fatalf("stance: smith %v, hero %v", s.Party[1].InStance(), s.Party[0].InStance())
	}
	s.portraitRightClick(1)
	if len(s.Sel) != 0 {
		t.Fatalf("selection after the right click %v", s.Sel)
	}
	// Dead: revived by «Живая Вода» through Game.Reviver.
	e := &s.Party[1]
	e.Flags4 |= 0x80
	delete(s.Occ, smith.Cell)
	smith.Act = gamestate.ActDeath
	if !s.Game.Reviver(e) || e.Dead() || smith.Act != 0 || !s.Occ[smith.Cell] || e.HP != gamestate.HPFull {
		t.Fatalf("revival: dead %v, act %d, cell held %v, HP %d", e.Dead(), smith.Act, s.Occ[smith.Cell], e.HP)
	}
}

// TestFlinchWhileWalking: a heavy hit stops a walk with the hit reaction
// (0x00422131 has no walking check); the walk order goes on afterwards.
func TestFlinchWhileWalking(t *testing.T) {
	s := newVolkScene(t)
	hero := s.Lead()
	goal := freeCellAway(s, hero.Cell, 20)
	s.WalkOrder(hero, goal, false)
	run(s, 3)
	if !hero.Walking {
		t.Skip("the hero does not walk")
	}
	s.Flinch(&hero.Walker)
	if hero.Act != gamestate.ActFlinch || hero.Walking || len(hero.Path) > 0 {
		t.Fatalf("after the hit: act %d, walking %v", hero.Act, hero.Walking)
	}
	flash := false
	for i := 0; i < 40 && hero.Act == gamestate.ActFlinch; i++ {
		flash = flash || s.Members[0].Act == gamestate.ActFlinch
		s.CombatTick()
	}
	if hero.Act != 0 || !flash {
		t.Fatalf("hit reaction did not end: act %d", hero.Act)
	}
	s.CombatTick()
	if !hero.Walking && len(hero.Path) == 0 {
		t.Fatal("the walk did not go on after the hit reaction")
	}
}

// TestPortraitStatus: hovering a portrait shows the member's status line
// (0x00424131), not while it is dead.
func TestPortraitStatus(t *testing.T) {
	s, _ := hireSmith(t)
	line := resfmt.CP866ToUTF8(s.portraitStatus(0))
	if !strings.Contains(line, "Хар:") || !strings.Contains(line, "воин дружины") {
		t.Fatalf("hero portrait status %q", line)
	}
	if !strings.Contains(resfmt.CP866ToUTF8(s.portraitStatus(1)), "воин дружины") {
		t.Fatalf("smith portrait status %q", resfmt.CP866ToUTF8(s.portraitStatus(1)))
	}
	s.Party[1].Flags4 |= 0x80
	if s.portraitStatus(1) != nil {
		t.Error("a dead member's portrait shows a status line")
	}
}

// TestDeadMemberLeftBehind: the leave cleanup 0x00436AD0 starts at group 0,
// so a dead party member is removed when the party takes a map exit.
func TestDeadMemberLeftBehind(t *testing.T) {
	s, smith := hireSmith(t)
	var exit *gamestate.MapExit
	for i := range s.Game.Exits {
		if x := s.Game.Exits[i]; x.From == s.MapNumber && s.Game.ExitOpen(x) {
			exit = &s.Game.Exits[i]
			break
		}
	}
	if exit == nil {
		t.Skip("no exit from the start map")
	}
	s.Party[1].Flags4 |= 0x80
	smith.Act = gamestate.ActDeath
	s.changeMap(*exit)
	if s.MapNumber != exit.To {
		t.Fatalf("still on map %d", s.MapNumber)
	}
	if len(s.Party) != 1 || s.Game.Groups[0].Count != 1 || len(s.Sel) != 1 || s.Sel[0] != s.Party[0].Index {
		t.Fatalf("party %d, group count %d, selection %v", len(s.Party), s.Game.Groups[0].Count, s.Sel)
	}
}

// TestWordsOfResurrection: «Слова Воскрешения» (misc item 6, 0x00414FFB)
// revives every dead character on the map, party and others, and is used up.
func TestWordsOfResurrection(t *testing.T) {
	s, smith := hireSmith(t)
	words := 0
	for i := 1; i < 8192 && words == 0; i++ {
		if it, ok := s.Game.Item(i); ok && it.Category == 11 && it.Type == 6 {
			words = i
		}
	}
	if words == 0 {
		t.Skip("no «Слова Воскрешения» in GAME.0")
	}
	var victim *npc
	for _, n := range s.NPCs {
		if n.Mode == npcVillager {
			victim = n
			break
		}
	}
	if victim == nil {
		t.Skip("no villager on the map")
	}
	kill := func(w *walker, e *gamestate.Entity) {
		e.Flags4 |= 0x80
		e.HP = 0
		w.Act = gamestate.ActDeath
		delete(s.Occ, w.Cell)
	}
	kill(&smith.Walker, &s.Party[1])
	kill(&victim.Walker, &s.Game.Entities[victim.Entity])
	victim.Mode = npcStill
	hero := &s.Party[0]
	hero.Bag[0] = words
	s.Game.UseBagItem(&s.hand, hero, s.Tables, 0)
	if !s.Game.ReviveAll {
		t.Fatal("using the words asked for nothing")
	}
	s.Game.ReviveAll = false
	s.ReviveAll()
	if s.Party[1].Dead() || smith.Act != 0 || s.Party[1].HP != gamestate.HPFull {
		t.Errorf("smith: dead %v, act %d, HP %d", s.Party[1].Dead(), smith.Act, s.Party[1].HP)
	}
	if e := s.Game.Entities[victim.Entity]; e.Dead() || victim.Mode != npcVillager || !s.Occ[victim.Cell] {
		t.Errorf("villager: dead %v, mode %d, cell held %v", e.Dead(), victim.Mode, s.Occ[victim.Cell])
	}
	if hero.Bag[0] == words {
		t.Error("the words stay in the bag")
	}
}

// TestJumpToSameMap: an exit or jump to the map the party is on
// (0x004377A6) keeps the scene and only places the party again
// (0x004118DC): the hero on the arrival cell with its facing, idle, the
// smith next to it; the old cells are free.
func TestJumpToSameMap(t *testing.T) {
	s, smith := hireSmith(t)
	hero := s.Lead()
	objects, npcs := &s.objects[0], s.NPCs[0]
	oldHero, oldSmith := hero.Cell, smith.Cell
	to := freeCellAway(s, hero.Cell, 30)
	if to == hero.Cell {
		t.Skip("no free cell")
	}
	hero.WalkTo(freeCellAway(s, hero.Cell, -20)) // a walk in progress is dropped
	s.changeMap(gamestate.MapExit{To: s.MapNumber, Row: to.Row, Col: to.Col, Facing: 5})
	if &s.objects[0] != objects || s.NPCs[0] != npcs {
		t.Fatal("the scene was rebuilt")
	}
	if hero.Cell != to || hero.Dir != 5 || hero.Walking || len(hero.Path) > 0 {
		t.Fatalf("hero at %v dir %d walking %v", hero.Cell, hero.Dir, hero.Walking)
	}
	if e := s.Game.Entities[s.Party[0].Index]; e.Row != to.Row || e.Col != to.Col || s.Game.Groups[0].Map != s.MapNumber {
		t.Errorf("game hero at %d,%d", e.Row, e.Col)
	}
	if smith.Cell == oldSmith || smith.Dir != 5 || max(absInt(smith.Cell.Row-to.Row), absInt(smith.Cell.Col-to.Col)) > 12 {
		t.Errorf("smith at %v dir %d, hero at %v", smith.Cell, smith.Dir, to)
	}
	if s.Occ[oldHero] || s.Occ[oldSmith] || !s.Occ[to] || !s.Occ[smith.Cell] {
		t.Error("occupancy not moved")
	}
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// TestStepHoldsNewCell: a walker takes the next cell at the step's start
// and frees the one it leaves (0x0041492E); a hit reaction mid-step leaves
// it in the new cell.
func TestStepHoldsNewCell(t *testing.T) {
	s := newVolkScene(t)
	hero := s.Lead()
	start := hero.Cell
	hero.WalkTo(freeCellAway(s, start, 10))
	run(s, 1)
	if !hero.Walking || hero.From != start || hero.Cell == start {
		t.Fatalf("walking %v from %v into %v", hero.Walking, hero.From, hero.Cell)
	}
	if s.Occ[start] || !s.Occ[hero.Cell] {
		t.Errorf("occupancy: left %v %v, entered %v %v", start, s.Occ[start], hero.Cell, s.Occ[hero.Cell])
	}
	entered := hero.Cell
	s.Flinch(&hero.Walker)
	if hero.Walking || hero.Cell != entered || !s.Occ[entered] {
		t.Errorf("after the hit: walking %v at %v", hero.Walking, hero.Cell)
	}
	if x, y := hero.Position(); true {
		if cx, cy := mapdata.CellCenter(entered); x != float64(cx) || y != float64(cy) {
			t.Errorf("drawn at %.0f,%.0f, want the entered cell %d,%d", x, y, cx, cy)
		}
	}
}
