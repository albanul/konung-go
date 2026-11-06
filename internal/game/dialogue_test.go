package game

import (
	"testing"

	"konung1.5/internal/gamestate"
)

func newVolkScene(t *testing.T) *startScene {
	t.Helper()
	const dir = "../../assets/Konung/"
	a, err := loadSceneAssets(dir+"GRAPH.RES", dir+"OBJECTS.RES", dir, dir+"konung.exe", dir+"Intr800r.res")
	if err != nil || a.quests == nil {
		t.Skip("assets missing:", err)
	}
	hero := gamestate.NewHeroTemplate()
	gamestate.SelectHero(&hero, gamestate.HeroVolk)
	gamestate.SelectClass(&hero, gamestate.ClassHunter)
	s, err := a.newGame(hero)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestRightClickTalks: a right click on the elder opens his dialogue at
// once, the hero does not walk, and the replies lead to the end.
func TestRightClickTalks(t *testing.T) {
	s := newVolkScene(t)
	from := s.hero.cell
	s.hovered = 355 // the elder of «Родники»
	s.click(&s.party[0], viewLeft+100, viewTop+100, 0, true)
	if s.talk.d == nil || len(s.talk.d.Replies) != 1 {
		t.Fatalf("no dialogue: %+v", s.talk.d)
	}
	if s.hero.walking || len(s.hero.path) > 0 || s.pickIntent || s.hero.cell != from {
		t.Fatal("the hero walks to the NPC")
	}
	r := s.talk.replies[0]
	if k := s.talk.replyAt(r.Min.X+5, r.Min.Y+5); k != 0 {
		t.Fatalf("reply hit test %d (box %v)", k, r)
	}
	if s.escape() || s.talk.d == nil {
		t.Fatal("Escape closed the dialogue")
	}
	for n := 0; s.talk.d != nil; n++ {
		if n > 10 {
			t.Fatal("dialogue does not end")
		}
		s.chooseReply(0)
	}
	// The quest log toggles and lists nothing yet.
	s.talk.toggleLog()
	if !s.talk.open() || len(s.game.QuestLog()) != 0 || s.escape() || s.talk.log {
		t.Fatal("quest log")
	}
}

// TestHireRebuildsScene: hiring the smith moves him into the party and the
// rebuilt scene shows him as a party member.
func TestHireRebuildsScene(t *testing.T) {
	s := newVolkScene(t)
	tk := s.newTalk()
	tk.NPC = 358
	if d := tk.OpenDialogue(358, nil); d == nil {
		t.Fatal("smith has no dialogue")
	}
	ok := gamestate.ScriptFunc(tk, 291)
	s.afterTalk(tk, nil)
	if ok != 1 || len(s.party) != 2 || s.party[1].Group != 0 {
		t.Fatalf("party %d after hire", len(s.party))
	}
	if _, ok := s.byEntity[s.party[1].Index]; !ok {
		t.Error("the hired smith has no walker")
	}
	if s.hero == nil || s.party[0].Index != s.hero.entity {
		t.Error("hero lost in rebuild")
	}
}
