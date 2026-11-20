package game

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
)

// The «Рейтинг» screen: the installed HISCORES.DAT is read at start; a
// table from the server replaces it and is written to the cache; the rows
// draw at the exe's places (something in the first player and clan row,
// nothing below the last rows).
func TestLANRatingScreen(t *testing.T) {
	const dir = "../../assets/Konung/"
	a, err := loadSceneAssets(dir+"GRAPH.RES", dir+"OBJECTS.RES", dir, dir+"konung.exe", dir+"Intr800r.res")
	if err != nil {
		t.Skip("assets missing:", err)
	}
	u, err := newLANUI(dir, a, dir+"ME800R.RES", dir+"PLAY800R.RES", "")
	if err != nil {
		t.Skip("LAN files missing:", err)
	}
	if _, err := os.Stat(dir + gamestate.LANHiscoresFile); err == nil && len(u.rating) != gamestate.LANRatingRows {
		t.Errorf("HISCORES.DAT: %d rows read", len(u.rating))
	}
	u.cfgPath = filepath.Join(t.TempDir(), "LAN.CFG")                      // never the player's LAN.CFG
	u.hiscoresPath = filepath.Join(t.TempDir(), gamestate.LANHiscoresFile) // nor HISCORES.DAT
	u.name.Text, u.clan.Text = cp866("Ivar"), cp866("Викинги")
	var p netplay.RatingPayload
	for k := range p.Rows {
		p.Rows[k].Rank = k + 1
	}
	p.Rows[0] = gamestate.LANRatingRow{Rank: 1, Name: "Olaf", Clan: "Русские", Amulets: 2, Rating: 150, ClanRank: 1, ClanName: "Викинги", ClanRating: 120}
	p.Rows[1] = gamestate.LANRatingRow{Rank: 2, Name: "Ivar", Clan: "Викинги", KillsHeroes: 3, Rating: 90, ClanRank: 2, ClanName: "Русские", ClanRating: 100}
	u.setRating(p.Rows[:])
	b, err := os.ReadFile(u.hiscoresPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := gamestate.ParseLANHiscores(b); len(got) != gamestate.LANRatingRows || got[1] != p.Rows[1] {
		t.Fatalf("cache: %+v", got)
	}
	texts := ratingTexts(u.rating, u.exe.Rating, u.name.Text, u.clan.Text)
	l := u.exe.Rating
	type at struct{ x, y int }
	got := map[at]ratingText{}
	for _, tx := range texts {
		got[at{tx.x, tx.y}] = tx
	}
	check := func(x, y int, text string, own bool) {
		t.Helper()
		tx, ok := got[at{x, y}]
		if !ok || string(tx.text) != string(cp866(text)) || tx.own != own {
			t.Errorf("at (%d, %d): %q own %v, want %q own %v", x, y, tx.text, tx.own, text, own)
		}
	}
	check(l.Name, l.PlayerY, "Olaf", false)
	check(l.Amulets, l.PlayerY, "2", false)
	check(l.Rating, l.PlayerY, "15", false) // 150 sent, ÷ 10 again on the screen
	check(l.Name, l.PlayerY+l.PlayerDY, "Ivar", true)
	check(l.Heroes, l.PlayerY+l.PlayerDY, "3", true)
	check(l.ClanNameX, l.ClanY, "Викинги", true)
	check(l.ClanRatingX, l.ClanY, "12", true)
	check(l.ClanNameX, l.ClanY+l.ClanDY, "Русские", false)
	if len(texts) != 2*8+2*3 {
		t.Errorf("%d texts, want the two players' and two clans' only", len(texts))
	}
	// The screen draws.
	u.state = lanRating
	u.draw(ebiten.NewImage(ScreenWidth, ScreenHeight))
}

// The table over the network: the server's MsgRating reaches the LAN
// screens in any state and is kept and cached.
func TestLANRatingFromServer(t *testing.T) {
	srv, err := netplay.Listen("127.0.0.1:0", netplay.ServerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	c, err := netplay.Dial(srv.Addr().String(), netplay.JoinInfo{Name: "Ivar", Model: 1}, netplay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	u := &lanUI{client: c, state: lanWaiting, hiscoresPath: filepath.Join(t.TempDir(), gamestate.LANHiscoresFile)}
	var p netplay.RatingPayload
	p.Rows[0] = gamestate.LANRatingRow{Rank: 1, Name: "Ivar", Rating: 100}
	if err := srv.SendRating(c.PlayerID, p); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(u.rating) == 0 && time.Now().Before(deadline) {
		u.readSession()
		time.Sleep(5 * time.Millisecond)
	}
	if len(u.rating) != gamestate.LANRatingRows || u.rating[0].Name != "Ivar" {
		t.Fatalf("rating = %+v", u.rating)
	}
	if b, err := os.ReadFile(u.hiscoresPath); err != nil || gamestate.ParseLANHiscores(b)[0].Rating != 100 {
		t.Errorf("cache: %v", err)
	}
}
