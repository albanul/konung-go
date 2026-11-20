package lanserver

import (
	"slices"
	"testing"
	"time"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
)

// playerList is the viewer's last MsgPlayers, false before one came.
func (v *viewer) playerList() (netplay.PlayersPayload, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.list == nil {
		return netplay.PlayersPayload{}, false
	}
	return *v.list, true
}

// The Q window's player list (MsgPlayers) reaches every player in the
// world, wherever its hero is: every hero by slot with its name and
// nation, a hero left behind by its player still listed until it goes,
// and each player's own quest log from the flags the server keeps.
func TestPlayerList(t *testing.T) {
	w := load(t)
	ss := startSession(t, w)
	a := enter(t, ss, "Ivar", 2)
	b := enter(t, ss, "Olaf", 0) // another nation, another map
	want := []netplay.ListedPlayer{
		{Slot: a.at.Slot, Name: "Ivar", Nation: 2},
		{Slot: b.at.Slot, Name: "Olaf", Nation: 0},
	}
	if a.at.Slot > b.at.Slot {
		want[0], want[1] = want[1], want[0]
	}
	for _, v := range []*viewer{a, b} {
		waitUntil(t, "both players listed", func() bool {
			l, ok := v.playerList()
			return ok && slices.Equal(l.Players, want)
		})
	}

	// Sent on a change only: nothing new while the world runs unchanged.
	count := func() int { a.mu.Lock(); defer a.mu.Unlock(); return a.lists }
	n := count()
	time.Sleep(300 * time.Millisecond) // the session advances the world meanwhile
	if c := count(); c != n {
		t.Fatalf("%d more player lists with no change", c-n)
	}

	// A quest flag with a log text set on Ivar's flags shows in his log
	// only.
	q := w.Quests
	if q == nil {
		t.Skip("QUESTR.RES not loaded")
	}
	flag, text := -1, uint32(0)
	init := q.Flags()
	for i := range gamestate.QuestFlagCount {
		if id := int(init[4*i+2]) | int(init[4*i+3])<<8; id != 0xFFFF {
			flag, text = i, uint32(id)
			break
		}
	}
	if flag < 0 {
		t.Fatal("no quest flag with a log text")
	}
	ss.host.mu.Lock()
	w.state(a.at.Slot).quests[4*flag] |= 0x80
	ss.host.mu.Unlock()
	waitUntil(t, "Ivar's log", func() bool {
		l, _ := a.playerList()
		return slices.Contains(l.Log, text)
	})
	if l, _ := b.playerList(); slices.Contains(l.Log, text) {
		t.Fatal("Olaf got Ivar's quest log")
	}

	// Olaf leaves away from his home village: his hero stays listed while
	// it lingers, and goes with it.
	ss.host.mu.Lock()
	nextTo(t, w, b.at.Slot, merchant7) // Ivar's village, map 7
	away := w.PlayerMap(b.at.Slot) != gamestate.LANStartMap(0)
	ss.host.mu.Unlock()
	if !away {
		t.Fatal("Olaf still at home")
	}
	b.c.Close()
	waitUntil(t, "Olaf lingering", func() bool { return ss.host.Lingering("Olaf") })
	if l, _ := a.playerList(); !slices.Equal(l.Players, want) {
		t.Fatalf("list while Olaf lingers: %+v", l.Players)
	}
	ss.host.mu.Lock()
	ss.host.remove(ss.host.bySlot[b.at.Slot], nil)
	ss.host.mu.Unlock()
	waitUntil(t, "Olaf gone from the list", func() bool {
		l, _ := a.playerList()
		return len(l.Players) == 1 && l.Players[0].Name == "Ivar"
	})
}
