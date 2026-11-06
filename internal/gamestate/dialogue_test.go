package gamestate

import (
	"testing"

	"konung1.5/internal/resfmt"
)

// talkSetup loads GAME.0 and QUESTR.RES as a new game does and returns a
// Talk for the hero on KONUNG.1.
func talkSetup(t *testing.T) *Talk {
	t.Helper()
	g, err := Parse(readAsset(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	questr := readAsset(t, "questr.RES")
	if err := g.SetQuestFlags(questr); err != nil {
		t.Fatal(err)
	}
	g.InitSettlementFlags(0)
	q, err := resfmt.ParseQuests(questr)
	if err != nil {
		t.Fatal(err)
	}
	tb, err := resfmt.NewExeTables(readAsset(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	return &Talk{G: g, Q: q, T: tb, Hero: g.Groups[0].First, Map: 1, Rand: func(n int) int { return 0 }}
}

func utf(q *resfmt.Quests, id uint32) string { return resfmt.CP866ToUTF8(q.Text(id)) }

// TestDialogueElder opens the elder of «Родники» (entity 355, dialogue 77)
// and follows the first reply to the end of the Amulet chain.
func TestDialogueElder(t *testing.T) {
	tk := talkSetup(t)
	last := 0
	d := tk.OpenDialogue(355, &last)
	if d == nil || d.Node != 1169 || len(d.Lines) != 1 || len(d.Replies) != 1 {
		t.Fatalf("open: %+v", d)
	}
	if got := utf(tk.Q, d.Lines[0]); got != "Здравствуй, Волк! Зачем пожаловал? Новую партию беличьих шкурок принес?" {
		t.Errorf("greeting line %q", got)
	}
	if tk.Events.Greeting != SoundGreetingMan || last != SoundGreetingMan {
		t.Errorf("greeting sound %d, last %d", tk.Events.Greeting, last)
	}
	steps := 0
	for ; steps < 20 && tk.Choose(d, 0); steps++ {
	}
	if steps != 5 || len(tk.Events.Unported) != 0 {
		t.Errorf("chain closed after %d replies, unported %v", steps, tk.Events.Unported)
	}
	if got := utf(tk.Q, d.Replies[0].Text); got != "Тогда Путята точно мне пригодится. Пойду поговорю с ним." {
		t.Errorf("last reply %q", got)
	}
}

// TestDialogueRoles: the posted villagers of «Родники» open the role
// dialogues 101 (healer), 103 (merchant), 102 (smith); the girl 371 gets
// the women's greeting.
func TestDialogueRoles(t *testing.T) {
	tk := talkSetup(t)
	for npc, want := range map[int]string{
		356: "Здоров будешь, господин! Я местный знахарь.",
		357: "Я - честный торговец.",
		358: "Здоров будешь, путник. Я - местный кузнец.",
		371: "Привет, князь!",
	} {
		d := tk.OpenDialogue(npc, nil)
		if d == nil || len(d.Lines) == 0 {
			t.Fatalf("%d: no dialogue", npc)
		}
		if got := utf(tk.Q, d.Lines[0]); len(got) < len(want) || got[:len(want)] != want {
			t.Errorf("%d: %q", npc, got)
		}
	}
	if tk.OpenDialogue(371, nil); tk.Events.Greeting != 17 {
		t.Errorf("girl greeting %d", tk.Events.Greeting)
	}
}

func TestQuestLog(t *testing.T) {
	tk := talkSetup(t)
	if len(tk.G.QuestLog()) != 0 {
		t.Fatal("log not empty at start")
	}
	tk.G.SetQuestFlag(1, true)
	log := tk.G.QuestLog()
	if len(log) != 1 || utf(tk.Q, log[0])[:10] != "Волхв у Бе"[:10] {
		t.Errorf("log %v", log)
	}
	tk.G.SetQuestFlag(1, false)
	if len(tk.G.QuestLog()) != 0 {
		t.Error("flag not cleared")
	}
}

// TestScriptMoney checks the loan pair 284/282 and a hire-price test.
func TestScriptMoney(t *testing.T) {
	tk := talkSetup(t)
	tk.NPC = 357
	h := &tk.G.Entities[tk.Hero]
	h.Gold = 0
	tk.script(284)
	if h.Gold != 100 || tk.settlement().Flags&SettlementLoan == 0 {
		t.Fatalf("loan: gold %d flags %x", h.Gold, tk.settlement().Flags)
	}
	if tk.script(282) != 0 || h.Gold != 100 {
		t.Fatal("repaid without 120 gold")
	}
	h.Gold = 130
	if tk.script(282) != 1 || h.Gold != 10 || tk.settlement().Flags&SettlementLoan != 0 {
		t.Fatalf("repay: gold %d", h.Gold)
	}
	h.Gold = 299 // level-1 NPC in an independent village: 300
	if tk.script(310) != 0 {
		t.Error("hire price below 300")
	}
	tk.script(365) // vassal: half price
	if tk.script(310) != 1 || tk.independent() {
		t.Error("vassal hire price")
	}
}

// TestRecruitAndDismiss hires the smith (358) into the party (291) and
// leaves him in the village again (332).
func TestRecruitAndDismiss(t *testing.T) {
	tk := talkSetup(t)
	g := tk.G
	s := g.SettlementOnMap(1)
	vg := s.Group
	before, party := g.Groups[vg].Count, g.Groups[0].Count
	smith := g.Entities[358]
	tk.NPC = 358
	if tk.script(291) != 1 || !tk.Events.PartyChanged {
		t.Fatal("recruit failed")
	}
	at := g.Groups[0].First + party
	if g.Groups[0].Count != party+1 || g.Groups[vg].Count != before-1 {
		t.Fatalf("counts party %d village %d", g.Groups[0].Count, g.Groups[vg].Count)
	}
	if e := g.Entities[at]; e.Group != 0 || e.Index != at || e.NameIndex != smith.NameIndex {
		t.Fatalf("party member %+v", e.Index)
	}
	if s.Posts[3].Entity != 0 || g.Entities[357].NameIndex == smith.NameIndex {
		t.Errorf("posts after recruit %+v", s.Posts)
	}
	tk.NPC = at
	if tk.script(332) != 1 {
		t.Fatal("dismiss failed")
	}
	if g.Groups[0].Count != party || g.Groups[vg].Count != before {
		t.Fatalf("counts after dismiss party %d village %d", g.Groups[0].Count, g.Groups[vg].Count)
	}
	last := g.Entities[g.Groups[vg].First+before-1]
	if last.NameIndex != smith.NameIndex || last.Group != vg {
		t.Errorf("dismissed member %d group %d", last.NameIndex, last.Group)
	}
}
