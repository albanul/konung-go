package game

import (
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"konung1.5/internal/gamestate"
)

// «Установки» from the LAN menu: the boxes flip the shared settings,
// «Отмена» and Escape restore them, «Ok» writes them with the video mode;
// each returns to the LAN menu.
func TestLANSettings(t *testing.T) {
	const dir = "../../assets/Konung/"
	a, err := loadSceneAssets(dir+"GRAPH.RES", dir+"OBJECTS.RES", dir, dir+"konung.exe", dir+"Intr800r.res")
	if err != nil {
		t.Skip("assets missing:", err)
	}
	u, err := newLANUI(dir, a, dir+"ME800R.RES", dir+"PLAY800R.RES", "")
	if err != nil {
		t.Skip("LAN files missing:", err)
	}
	u.cfgPath = filepath.Join(t.TempDir(), "LAN.CFG") // never the player's LAN.CFG
	cfg := gamestate.DefaultConfig()
	u.config = &cfg
	writes, video := 0, int32(-1)
	u.writeSettings = func(v int32) { writes, video = writes+1, v }
	center := func(i int) (int, int) {
		r := settingsRects800[i]
		return (r[0] + r[2]) / 2, (r[1] + r[3]) / 2
	}
	open := func() {
		t.Helper()
		u.state = lanMenu
		u.menuItem(lanItemSettings)
		if u.state != lanSettings {
			t.Fatalf("state %d, want «Установки»", u.state)
		}
	}
	open()
	u.draw(ebiten.NewImage(ScreenWidth, ScreenHeight))
	u.settingsClick(center(3)) // Тени
	if cfg.Shadows != 0 {
		t.Fatal("Тени did not flip")
	}
	u.settingsClick(center(settingsCancel))
	if cfg.Shadows == 0 || u.state != lanMenu || writes != 0 {
		t.Fatalf("«Отмена»: shadows %d, state %d, writes %d", cfg.Shadows, u.state, writes)
	}
	open()
	u.settingsClick(center(2)) // Прозрачность
	u.settingsClick(center(1)) // 800×600
	u.settingsClick(center(settingsOK))
	if cfg.Transparency != 0 || u.state != lanMenu || writes != 1 || video != 1 {
		t.Fatalf("«Ok»: transparency %d, state %d, writes %d, video %d", cfg.Transparency, u.state, writes, video)
	}
	open()
	u.settingsClick(center(4)) // Смена дня и ночи
	if u.escape() || cfg.DayNight == 0 || u.state != lanMenu {
		t.Fatalf("Escape: day/night %d, state %d", cfg.DayNight, u.state)
	}
}
