package game

import (
	"bytes"
	"image"
	"os"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
)

// The LAN status line is drawn always: the nations' amulet counts (0
// before the first MsgAmulets), then the session's time left as h:mm:ss
// when known; the three amulet items' icons follow the amulet state.
func TestLANStatusLine(t *testing.T) {
	s := viewScene(t)
	if got := string(s.lanStatusLine()); got != "0 : 0 : 0" {
		t.Errorf("before any state: %q", got)
	}
	if icons := s.lanStatusIcons(); icons != [3]int{-1, -1, -1} {
		t.Errorf("icons before any state: %v", icons)
	}
	s.applyAmulets(netplay.AmuletsPayload{
		Parts:   [3]netplay.AmuletPart{{Kind: 1, Holder: -1}, {Kind: 0, Holder: -1}, {Kind: 3, Holder: 2}},
		Nations: [3]int{1, 0, 12},
	})
	s.clock = &serverClock{c: netplay.ClockPayload{SessionLeft: 3725}, at: time.Now()}
	if got := string(s.lanStatusLine()); got != "1 : 0 : 12   1:02:05" && got != "1 : 0 : 12   1:02:04" {
		t.Errorf("line %q, want 1 : 0 : 12   1:02:05", got)
	}
	s.clock = &serverClock{c: netplay.ClockPayload{SessionLeft: -1}, at: time.Now()}
	if got := string(s.lanStatusLine()); got != "1 : 0 : 12" {
		t.Errorf("unknown time: %q", got)
	}
	icons := s.lanStatusIcons()
	if icons[0] < 0 || icons[1] != -1 || icons[2] < 0 || icons[0] == icons[2] {
		t.Errorf("icons %v: want the right part's and the bottom part's, none in the middle", icons)
	}
	// Drawing the scene draws the line with the map window shut and open.
	img := newScreen()
	s.Draw(img)
	s.wmap.open = true
	s.Draw(img)
}

// The map window's hover: over map m its location name (row letter A–D,
// column 1–5: "A3" for map 1, "B3" for map 3), outside the maps but in the
// area nothing; the players on m listed by slot from the server's list,
// the own nation's marked; the per-map counts from the same list.
func TestLANMapHover(t *testing.T) {
	s := viewScene(t)
	const dir = "../../assets/Konung/"
	exeData, err := os.ReadFile(dir + "LAN_KONG.EXE")
	into, err1 := os.ReadFile(dir + "INTO640R.RES")
	if err != nil || err1 != nil {
		t.Skip("LAN_KONG.EXE or INTO640R.RES not available")
	}
	exe, err := gamestate.ParseLANExe(exeData)
	if err != nil {
		t.Fatal(err)
	}
	l, err := newLANWorldMap(exe, into, s.assets.graph)
	if err != nil {
		t.Fatal(err)
	}
	s.wmap.lan = l
	for m := 1; m <= gamestate.LANMaps; m++ {
		p := l.markers[m].Add(image.Pt(viewLeft+l.m.Area.Min.X+12, viewTop+l.m.Area.Min.Y+12))
		if got := s.wmap.lanRegionAt(p.X, p.Y); got != m {
			t.Errorf("map %d: region %d at %v", m, got, p)
			continue
		}
		text, ok := s.lanMapStatus(p.X, p.Y)
		want := string([]byte{byte('A' + (l.markers[m].Y+12)*4/l.m.Area.Dy()), byte('1' + (l.markers[m].X+12)*5/l.m.Area.Dx())})
		if !ok || string(text) != want {
			t.Errorf("map %d: status %q, want %q", m, text, want)
		}
	}
	p := l.markers[3].Add(image.Pt(viewLeft+l.m.Area.Min.X+12, viewTop+l.m.Area.Min.Y+12))
	if text, _ := s.lanMapStatus(p.X, p.Y); string(text) != "B3" {
		t.Errorf("map 3 is %q, want B3", text)
	}
	if _, ok := s.lanMapStatus(viewLeft+1, viewBottom-1); ok {
		t.Error("outside the map area claimed by the map")
	}
	me := s.view.slot
	s.applyPlayers(netplay.PlayersPayload{Players: []netplay.ListedPlayer{
		{Slot: 0, Name: "Olaf", Nation: 1, Map: 3},
		{Slot: me, Name: "Ivar", Nation: 0, Map: 1},
		{Slot: 9, Name: "Rurik", Nation: 0, Map: 3},
	}})
	rows := s.lanMapPlayers(3)
	if len(rows) != 2 || !bytes.Equal(rows[0].text, cp866("Olaf")) || rows[0].own || !bytes.Equal(rows[1].text, cp866("Rurik")) || !rows[1].own {
		t.Errorf("map 3 rows %+v", rows)
	}
	if rows := s.lanMapPlayers(2); len(rows) != 0 {
		t.Errorf("map 2 rows %+v", rows)
	}
	if c := s.playerCounts(); c[3] != 2 || c[1] != 1 || len(c) != 2 {
		t.Errorf("counts %v", c)
	}
	s.wmap.open = true
	s.wmap.hoverNames = rows
	s.Draw(newScreen())
}

// With the map window open the world under it does not set the pointer:
// over the own hero's cell (an occupied cell: the barred circle with the
// window shut) the hand's cursor shows.
func TestNoWorldCursorUnderWindows(t *testing.T) {
	s := viewScene(t)
	s.Draw(newScreen())
	x, y := mapdata.CellCenter(s.Hero.Cell)
	mx, my := x-s.camX, y-s.camY
	if !inView(mx, my) {
		t.Skip("hero not in view")
	}
	s.hovered = -1
	if got := s.cursorSpriteAt(mx, my); got != gamestate.CursorBlocked {
		t.Fatalf("window shut: cursor %d, want the occupied cell's %d", got, gamestate.CursorBlocked)
	}
	s.wmap.open = true
	if got := s.cursorSpriteAt(mx, my); got != s.hand.Cursor {
		t.Errorf("map open: cursor %d, want the hand's %d", got, s.hand.Cursor)
	}
	s.wmap.open = false
	s.talk.toggleLog()
	if got := s.cursorSpriteAt(mx, my); got != s.hand.Cursor {
		t.Errorf("Q open: cursor %d, want the hand's %d", got, s.hand.Cursor)
	}
}

func newScreen() *ebiten.Image { return ebiten.NewImage(ScreenWidth, ScreenHeight) }
