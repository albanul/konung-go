package resfmt

import (
	"os"
	"strings"
	"testing"
)

func TestParseQuests(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/questr.RES")
	if err != nil {
		t.Skip("questr.RES not available:", err)
	}
	q, err := ParseQuests(data)
	if err != nil {
		t.Fatal(err)
	}
	// Dialogue 0: one NPC line, one reply that ends the dialogue.
	h := q.Node(q.Starts[0])
	if q.Starts[0] != 0 || h.Lines != 1 || h.Options != 1 {
		t.Fatalf("dialogue 0 header %+v", h)
	}
	line := CP866ToUTF8(q.Text(q.Node(1).Text))
	if !strings.HasPrefix(line, "Извини, князь, но я теперь подчиняюсь только старосте Черного Бора.") {
		t.Errorf("line = %q", line)
	}
	if n := q.Node(2); n.Next != QuestEnd || !strings.HasPrefix(CP866ToUTF8(q.Text(n.Text)), "Спасибо, скоро я снова буду вашим князем") {
		t.Errorf("reply %+v %q", n, CP866ToUTF8(q.Text(n.Text)))
	}
	// Dialogue 100 starts with a redirect over three branches.
	r := q.Node(q.Starts[100])
	if q.Starts[100] != 1994 || r.Action != QuestRedirect || r.Lines != 3 {
		t.Errorf("dialogue 100 %d %+v", q.Starts[100], r)
	}
	if c := q.Condition(r.Cond); len(c) != 1 || c[0] != QuestTermAnd|QuestTermFunc|350 {
		t.Errorf("redirect condition %x", c)
	}
	if len(q.Flags()) != 0x4B0 {
		t.Errorf("flags %d bytes", len(q.Flags()))
	}
}

func TestDialogueLayout(t *testing.T) {
	exe, err := os.ReadFile("../../assets/Konung/konung.exe")
	if err != nil {
		t.Skip("konung.exe not available:", err)
	}
	tb, err := NewExeTables(exe)
	if err != nil {
		t.Fatal(err)
	}
	l := tb.DialogueLayout(1)
	if l.Frame.X != 144 || l.Frame.Y != 0 || l.Divider.X != 161 || l.Divider.Y != 187 ||
		l.Lines.X != 166 || l.Lines.Y != 25 || l.Replies.Y != 232 || l.Width != 606 {
		t.Errorf("layout %+v", l)
	}
	if s := tb.QuestLogScroll(1); s[0].Min.X != 175 || s[1].Max.X != 801 || s[1].Max.Y != 31 {
		t.Errorf("scroll %v", s)
	}
	if tb.BuildingSkill(14) != 5 || CP866ToUTF8(tb.BuildingName(3)) != "Изба знахаря" {
		t.Errorf("building table %d %q", tb.BuildingSkill(14), CP866ToUTF8(tb.BuildingName(3)))
	}
	if _, err := tb.Font(FontHighlightVA); err != nil {
		t.Errorf("highlight font: %v", err)
	}
}
