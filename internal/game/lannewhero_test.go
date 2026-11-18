package game

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/lanstore"
	"konung1.5/internal/netplay"
)

// «Новый герой» end to end without input: a build, «Ok», a server row, the
// warning, its «Ok» — the server rebuilds the account's hero from it.
func TestLANNewHero(t *testing.T) {
	const dir = "../../assets/Konung/"
	a, err := loadSceneAssets(dir+"GRAPH.RES", dir+"OBJECTS.RES", dir, dir+"konung.exe", dir+"Intr800r.res")
	if err != nil {
		t.Skip("assets missing:", err)
	}
	u, err := newLANUI(dir, a, dir+"ME800R.RES", dir+"PLAY800R.RES", dir+"NEW800R.RES")
	if err != nil {
		t.Skip("LAN files missing:", err)
	}
	if u.newFrames == nil {
		t.Fatal("NEW800R.RES not loaded")
	}
	u.cfgPath = filepath.Join(t.TempDir(), "LAN.CFG") // never the player's LAN.CFG
	u.name.Text, u.pass.Text, u.clan.Text = []byte("Snow"), []byte("pw"), cp866("Викинги")
	if len(u.clans) != 3 {
		t.Errorf("CLANS.CFG: %d clans", len(u.clans))
	}

	store, err := lanstore.Open(filepath.Join(t.TempDir(), "konung.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv, err := netplay.Listen("127.0.0.1:0", netplay.ServerConfig{Accounts: store})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	store.Login(netplay.JoinInfo{Name: "Snow", Password: "pw", Clan: "Русские", Model: 1})

	u.model = 1
	u.openNewHero()
	if u.state != lanNewHero || u.nh.Level != gamestate.LANHeroEditLevel {
		t.Fatalf("state %d, level %d", u.state, u.nh.Level)
	}
	// Hunter, Меткость, Торговля; Хар 3 → 0 (3 × 5) pays for Лов 15 → 16
	// (15: the favourite column).
	gamestate.SelectClass(&u.nh, gamestate.ClassHunter)
	gamestate.SelectCombatSkill(&u.nh, true)
	gamestate.SelectSkill(&u.nh, gamestate.SkillTrade)
	for range 3 {
		gamestate.LANTradeStat(&u.nh, 0, false)
	}
	if !gamestate.LANTradeStat(&u.nh, 2, true) {
		t.Fatalf("raise Лов refused with %d points", u.nh.FreeExp)
	}
	u.look = 5
	u.setNewLook()
	if u.nh.Model != gamestate.HeroMichail || u.nh.PaletteOffset != 39*512 {
		t.Errorf("preview: model %d palette %d", u.nh.Model, u.nh.PaletteOffset/512)
	}
	// The right click's other type is a demonstration only: the build and
	// the login's type stay.
	before := gamestate.ChoiceOf(u.nh)
	u.previewModel = gamestate.HeroVolk
	u.setNewLook()
	if u.nh.Model != gamestate.HeroVolk || u.nh.PaletteOffset != uint32(gamestate.LANLookPalette(gamestate.HeroVolk, 5))*512 {
		t.Errorf("Волк preview: model %d palette %d", u.nh.Model, u.nh.PaletteOffset/512)
	}
	if gamestate.ChoiceOf(u.nh) != before || u.model != 1 {
		t.Errorf("the preview changed the build or the login type (model %d)", u.model)
	}
	// The screen draws (the portrait is the hero sprite).
	img := ebiten.NewImage(ScreenWidth, ScreenHeight)
	u.drawNewHero(img)

	u.newHeroOK()
	if u.state != lanLogin || u.pending == nil || u.pending.Class != int(gamestate.ClassHunter) {
		t.Fatalf("after Ok: state %d, pending %+v", u.state, u.pending)
	}
	u.found = []netplay.Found{{Addr: srv.Addr().String()}}
	u.pickRow(0)
	if u.msg != gamestate.MsgOldHero {
		t.Fatalf("no warning before the rebuild: msg %d", u.msg)
	}
	u.msg = 0
	u.join(u.pendingRow, u.pending)
	select {
	case f := <-u.results:
		f()
	case <-time.After(5 * time.Second):
		t.Fatal("join timed out")
	}
	defer u.disconnect()
	h := u.hero
	if u.msg != 0 || h == nil || u.pending != nil {
		t.Fatalf("join: msg %d, hero %+v, pending %v", u.msg, h, u.pending)
	}
	if h.Class != int(gamestate.ClassHunter) || h.Charisma != 0 || h.Dexterity != 16 || h.Archery != 100 || h.Trade != 20 ||
		h.Look != 5 || h.Palette != gamestate.LANLookPalette(1, 5) || h.Level != 1 {
		t.Errorf("rebuilt hero = %+v", h)
	}
	if v := heroValues(h); v[5] != "13.3" || v[6] != "1" {
		t.Errorf("Макс. груз %q, Макс. отряд %q", v[5], v[6])
	}
	found := false
	for _, c := range u.clans {
		found = found || string(c) == string(cp866("Викинги"))
	}
	if !found {
		t.Error("clan list lost Викинги")
	}
}
