package gamestate

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func readAsset(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../assets/Konung/" + name)
	if err != nil {
		t.Skipf("asset missing: %v", err)
	}
	return data
}

// An unmodified GAME.N encodes back to the same bytes.
func TestMarshalRoundTrip(t *testing.T) {
	for _, name := range []string{"GAME.0", "GAME.1", "GAME.2"} {
		t.Run(name, func(t *testing.T) { roundTrip(t, readAsset(t, name)) })
	}
}

func roundTrip(t *testing.T, data []byte) {
	g, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	out, err := g.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, data) {
		for i := range out {
			if out[i] != data[i] {
				t.Fatalf("first difference at %#x: %#x, want %#x", i, out[i], data[i])
			}
		}
	}
}

func TestSaveRoundTrip(t *testing.T) {
	data := readAsset(t, "GAME.0")
	questr := readAsset(t, "questr.RES")
	g, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.SetQuestFlags(questr); err != nil {
		t.Fatal(err)
	}
	hero := &g.Entities[g.Groups[0].First]
	hero.HP, hero.Gold, hero.Row, hero.Col = 900, 1234, 70, 30
	g.Piles[3].Gold = 0
	g.MiscUsed[5] = 1
	st := SaveState{
		Time:   4321,
		Arrows: []Arrow{{X: 100, Y: 200, VX: 4, VY: -1.5, Shooter: 7, Bow: 3, Poison: 2, Facing: 5, Life: 30, Special: -1, Type: 2}},
		Active: []ActivePile{
			{Pile: g.Piles[3], X: 10, Y: 20, Image: 0xA8},
			{Pile: Pile{Index: -1, Map: 1, Row: 40, Col: 20, Items: [pileMaxItems]int{55}}, X: 1, Y: 2, Image: 0xA0},
		},
	}
	save, err := g.MarshalSave(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(save) != SaveSize {
		t.Fatalf("size %d", len(save))
	}
	if m, err := SaveHeroModel(save); err != nil || m != hero.Model {
		t.Fatalf("hero model %d, %v; want %d", m, err, hero.Model)
	}
	g2, st2, err := ParseSave(save, data)
	if err != nil {
		t.Fatal(err)
	}
	h2 := g2.Entities[g2.Groups[0].First]
	if h2.HP != 900 || h2.Gold != 1234 || h2.Row != 70 || h2.Col != 30 {
		t.Errorf("hero %+v", h2)
	}
	if g2.Quests != g.Quests || g2.MiscUsed != g.MiscUsed || g2.Options != g.Options {
		t.Error("quests, misc flags or options differ")
	}
	if st2.Time != st.Time || len(st2.Arrows) != 1 || st2.Arrows[0] != st.Arrows[0] {
		t.Errorf("state %+v", st2)
	}
	if len(st2.Active) != 2 || st2.Active[0].Index != 3 || st2.Active[1].Index != -1 || st2.Active[1].Items[0] != 55 || st2.Active[0].Image != 0xA8 {
		t.Errorf("active %+v", st2.Active)
	}
	if again, err := g2.MarshalSave(st2); err != nil || !bytes.Equal(again, save) {
		t.Errorf("second save differs (%v)", err)
	}
}

func TestSlotNames(t *testing.T) {
	empty := []byte("empty")
	n := SlotNames(nil, empty)
	if string(n[9]) != "empty" {
		t.Fatalf("%q", n[9])
	}
	n[2] = []byte("abc")
	back := SlotNames(MarshalSlotNames(n), empty)
	if string(back[2]) != "abc" || string(back[0]) != "empty" {
		t.Fatalf("%q", back)
	}
	got := SaveName([]byte("X"), time.Date(1999, 6, 5, 21, 49, 8, 0, time.UTC))
	if string(got) != "X Jun  5 21:49" {
		t.Fatalf("%q", got)
	}
}
