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
	if len(s.party) != 2 {
		t.Fatalf("party %d after the hire", len(s.party))
	}
	m, _ := s.partyMember(s.party[1].Index) // the hire copies him into group 0's next slot
	if m == nil {
		t.Fatal("the smith has no member walker")
	}
	return s, m
}

// run advances the scene by n game ticks.
func run(s *startScene, n int) {
	for range n * walkTicksPerStep {
		s.step()
	}
}

// freeCellAway returns a free cell about dr rows from c.
func freeCellAway(s *startScene, c mapdata.Cell, dr int) mapdata.Cell {
	for d := 0; d < 40; d++ {
		for _, r := range []int{c.Row + dr + d, c.Row - dr - d} {
			if x := (mapdata.Cell{Row: r, Col: c.Col}); !s.blocked(x) {
				if p := mapdata.FindPath(c, x, s.blocked, pathSearchLimit); len(p) > 0 {
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
	hero := s.lead()
	goal := freeCellAway(s, hero.cell, 30)
	s.walkOrder(hero, goal, false)
	run(s, 600)
	if hero.cell == goal && gamestate.Dist(hero.cell.Row, hero.cell.Col, smith.cell.Row, smith.cell.Col) >= gamestate.FollowDistance+4 {
		t.Fatalf("the smith stayed behind: hero %v, smith %v", hero.cell, smith.cell)
	}
	if hero.cell != goal {
		t.Skipf("the hero did not reach %v (at %v)", goal, hero.cell)
	}

	// The smith alone is ordered somewhere: he stays there afterwards.
	s.sel = []int{s.party[1].Index}
	stay := freeCellAway(s, smith.cell, 6)
	for _, m := range s.selected() {
		s.walkOrder(m, stay, false)
	}
	run(s, 200)
	if smith.intent&gamestate.IntentFollow != 0 {
		t.Fatalf("after a walk order the smith follows (+2 %#x)", smith.intent)
	}
	away := freeCellAway(s, hero.cell, 30)
	s.walkOrder(hero, away, false)
	run(s, 600)
	if d := gamestate.Dist(away.Row, away.Col, smith.cell.Row, smith.cell.Col); d < gamestate.FollowDistance {
		t.Fatalf("the smith followed after a walk order (%d cells from the hero)", d)
	}
	s.gatherOrder()
	run(s, 600)
	if smith.intent&gamestate.IntentFollow == 0 ||
		gamestate.Dist(hero.cell.Row, hero.cell.Col, smith.cell.Row, smith.cell.Col) >= gamestate.FollowDistance+4 {
		t.Fatalf("after «Все ко мне»: smith %v (+2 %#x), hero %v", smith.cell, smith.intent, hero.cell)
	}
}

// TestSelectionOrders: a portrait click selects one member, Shift adds;
// the stance toggle works on the selection.
func TestSelectionOrders(t *testing.T) {
	s, smith := hireSmith(t)
	if len(s.sel) != 1 || s.sel[0] != s.party[0].Index {
		t.Fatalf("selection after the hire %v", s.sel)
	}
	s.portraitClick(1)
	if len(s.sel) != 1 || s.sel[0] != s.party[1].Index {
		t.Fatalf("selection after the portrait click %v", s.sel)
	}
	s.toggleSelectionStance()
	if !s.party[1].InStance() || s.party[0].InStance() {
		t.Fatalf("stance: smith %v, hero %v", s.party[1].InStance(), s.party[0].InStance())
	}
	s.portraitRightClick(1)
	if len(s.sel) != 0 {
		t.Fatalf("selection after the right click %v", s.sel)
	}
	// Dead: revived by «Живая Вода» through Game.Reviver.
	e := &s.party[1]
	e.Flags4 |= 0x80
	delete(s.occ, smith.cell)
	smith.act = gamestate.ActDeath
	if !s.game.Reviver(e) || e.Dead() || smith.act != 0 || !s.occ[smith.cell] || e.HP != gamestate.HPFull {
		t.Fatalf("revival: dead %v, act %d, cell held %v, HP %d", e.Dead(), smith.act, s.occ[smith.cell], e.HP)
	}
}

// TestFlinchWhileWalking: a heavy hit stops a walk with the hit reaction
// (0x00422131 has no walking check); the walk order goes on afterwards.
func TestFlinchWhileWalking(t *testing.T) {
	s := newVolkScene(t)
	hero := s.lead()
	goal := freeCellAway(s, hero.cell, 20)
	s.walkOrder(hero, goal, false)
	run(s, 3)
	if !hero.walking {
		t.Skip("the hero does not walk")
	}
	s.flinch(&hero.walker)
	if hero.act != gamestate.ActFlinch || hero.walking || len(hero.path) > 0 {
		t.Fatalf("after the hit: act %d, walking %v", hero.act, hero.walking)
	}
	flash := false
	for i := 0; i < 40 && hero.act == gamestate.ActFlinch; i++ {
		flash = flash || s.members[0].act == gamestate.ActFlinch
		s.combatTick()
	}
	if hero.act != 0 || !flash {
		t.Fatalf("hit reaction did not end: act %d", hero.act)
	}
	s.combatTick()
	if !hero.walking && len(hero.path) == 0 {
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
	s.party[1].Flags4 |= 0x80
	if s.portraitStatus(1) != nil {
		t.Error("a dead member's portrait shows a status line")
	}
}

// TestDeadMemberLeftBehind: the leave cleanup 0x00436AD0 starts at group 0,
// so a dead party member is removed when the party takes a map exit.
func TestDeadMemberLeftBehind(t *testing.T) {
	s, smith := hireSmith(t)
	var exit *gamestate.MapExit
	for i := range s.game.Exits {
		if x := s.game.Exits[i]; x.From == s.mapNumber && s.game.ExitOpen(x) {
			exit = &s.game.Exits[i]
			break
		}
	}
	if exit == nil {
		t.Skip("no exit from the start map")
	}
	s.party[1].Flags4 |= 0x80
	smith.act = gamestate.ActDeath
	s.changeMap(*exit)
	if s.mapNumber != exit.To {
		t.Fatalf("still on map %d", s.mapNumber)
	}
	if len(s.party) != 1 || s.game.Groups[0].Count != 1 || len(s.sel) != 1 || s.sel[0] != s.party[0].Index {
		t.Fatalf("party %d, group count %d, selection %v", len(s.party), s.game.Groups[0].Count, s.sel)
	}
}

// TestWordsOfResurrection: «Слова Воскрешения» (misc item 6, 0x00414FFB)
// revives every dead character on the map, party and others, and is used up.
func TestWordsOfResurrection(t *testing.T) {
	s, smith := hireSmith(t)
	words := 0
	for i := 1; i < 8192 && words == 0; i++ {
		if it, ok := s.game.Item(i); ok && it.Category == 11 && it.Type == 6 {
			words = i
		}
	}
	if words == 0 {
		t.Skip("no «Слова Воскрешения» in GAME.0")
	}
	var victim *npc
	for _, n := range s.npcs {
		if n.mode == npcVillager {
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
		w.act = gamestate.ActDeath
		delete(s.occ, w.cell)
	}
	kill(&smith.walker, &s.party[1])
	kill(&victim.walker, &s.game.Entities[victim.entity])
	victim.mode = npcStill
	hero := &s.party[0]
	hero.Bag[0] = words
	s.game.UseBagItem(&s.hand, hero, s.tables, 0)
	if !s.game.ReviveAll {
		t.Fatal("using the words asked for nothing")
	}
	s.game.ReviveAll = false
	s.reviveAll()
	if s.party[1].Dead() || smith.act != 0 || s.party[1].HP != gamestate.HPFull {
		t.Errorf("smith: dead %v, act %d, HP %d", s.party[1].Dead(), smith.act, s.party[1].HP)
	}
	if e := s.game.Entities[victim.entity]; e.Dead() || victim.mode != npcVillager || !s.occ[victim.cell] {
		t.Errorf("villager: dead %v, mode %d, cell held %v", e.Dead(), victim.mode, s.occ[victim.cell])
	}
	if hero.Bag[0] == words {
		t.Error("the words stay in the bag")
	}
}
