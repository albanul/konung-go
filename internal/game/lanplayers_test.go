package game

import (
	"bytes"
	"testing"

	"konung1.5/internal/netplay"
)

// Window 2's LAN part follows MsgPlayers and MsgAmulets: a row per player
// by the list's order, « клан » and the clan when set, « Владелец
// амулета» for an amulet item's holder, the own nation's rows in the
// highlight font; the own quest log replaces the client's; the nations'
// counts come from the amulet state.
func TestLANQWindow(t *testing.T) {
	s := viewScene(t)
	me := s.view.slot
	other := (me + 1) % 16
	s.applyAmulets(netplay.AmuletsPayload{
		Parts:   [3]netplay.AmuletPart{{Kind: 1, Holder: other}, {Kind: 2, Holder: -1}, {Kind: 3, Holder: -1}},
		Nations: [3]int{4, 0, 2},
	})
	s.applyPlayers(netplay.PlayersPayload{
		Players: []netplay.ListedPlayer{
			{Slot: me, Name: "Ivar", Nation: 1},
			{Slot: other, Name: "Olaf", Clan: "Волки", Nation: 0},
			{Slot: 15, Name: "Rurik", Nation: 1},
		},
		Log: []uint32{7, 9},
	})
	q := s.talk.lan
	if q == nil {
		t.Fatal("no LAN list in window 2")
	}
	want := []lanQRow{
		{cp866("Ivar - Викинги"), true},
		{cp866("Olaf клан Волки - Русские, Владелец амулета"), false},
		{cp866("Rurik - Викинги"), true},
	}
	if len(q.rows) != len(want) {
		t.Fatalf("rows %d, want %d", len(q.rows), len(want))
	}
	for k, r := range q.rows {
		if !bytes.Equal(r.text, want[k].text) || r.own != want[k].own {
			t.Errorf("row %d: %q own %v, want %q own %v", k, r.text, r.own, want[k].text, want[k].own)
		}
	}
	if q.nations != [3]int{4, 0, 2} || len(q.log) != 2 || q.log[0] != 7 {
		t.Errorf("nations %v log %v", q.nations, q.log)
	}
	// The amulet passes to the ground: the holder text goes with the next
	// amulet state.
	s.applyAmulets(netplay.AmuletsPayload{Parts: [3]netplay.AmuletPart{{Kind: 1, Holder: -1}}, Nations: [3]int{4, 0, 2}})
	if r := s.talk.lan.rows[1]; !bytes.Equal(r.text, cp866("Olaf клан Волки - Русские")) {
		t.Errorf("row after the drop: %q", r.text)
	}
	// Drawing the open window does not fail.
	s.talk.toggleLog()
	if !s.talk.log {
		t.Fatal("Q did not open window 2")
	}
}
