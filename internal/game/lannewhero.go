package game

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
)

// «Новый герой» (LAN state 4; draw 0x00427D14, clicks 0x0042E3CA,
// docs/konung_lan.md#new-hero): the single-player creation screen's class,
// skill and stat-trade handlers on the LAN tables (gamestate/lanhero.go),
// a clan field and list, and the portrait that steps the hero's look. «Ok»
// keeps the build and opens the login; the next join, after «Старый
// персонаж будет удален!», sends it (JoinInfo.NewHero) and the server
// rebuilds the account's hero from it.
//
// Port additions: a look is kept per hero type (the original has one look
// index for whichever type is picked at login), and a right click on the
// portrait shows the next type so each type's look can be chosen.

// NEW800R.RES frames (loader 0x00424944, image slots 0x1F9 + i).
const (
	newBackground = 0 // 800×600
	newTile       = 1 // 100×100, tiled under the background
	newMarker     = 2 // 13×13
	newCancel     = 3 // 3 normal, 4 pressed
	newOK         = 5 // 5 normal, 6 pressed
	newFrames     = 7
)

// msgBadHero is a port message: the server refused the build (it checks it
// with the screen's rules; the original reset such a hero to level 1 and
// answered «Неправильный пароль»).
const msgBadHero = 10

// loadNewHero loads NEW800R.RES and CLANS.CFG; without NEW800R the menu
// item does nothing, as before.
func (u *lanUI) loadNewHero(newPath string, a *sceneAssets) error {
	frames, err := resfmt.LoadType5File(newPath)
	if err != nil {
		return err
	}
	if len(frames) < newFrames {
		return fmt.Errorf("%s has %d frames; need %d", newPath, len(frames), newFrames)
	}
	for _, f := range frames[:newFrames] {
		u.newFrames = append(u.newFrames, toEbiten(f))
	}
	u.graph, u.tables, u.items = a.graph, a.tables, gamestate.NewHeroItems()
	// The clan list starts with CLANS.CFG (CP866 lines); the server's list
	// is merged in on every join.
	if data, err := os.ReadFile(filepath.Join(u.gameDir, "CLANS.CFG")); err == nil {
		for _, line := range bytes.Split(data, []byte("\n")) {
			u.addClan(bytes.TrimSpace(line))
		}
	}
	return nil
}

// addClan appends a clan name (CP866) to the list unless empty or known
// (0x0042E756 appends the typed clan at «Ok»; at most 256).
func (u *lanUI) addClan(name []byte) {
	if len(name) == 0 || len(u.clans) >= 256 {
		return
	}
	for _, c := range u.clans {
		if bytes.Equal(c, name) {
			return
		}
	}
	u.clans = append(u.clans, append([]byte(nil), name...))
}

// openNewHero is LAN menu item 3 (0x0042E0DB): logged out, the address
// cleared, a fresh template (the single-player one at level 13, no
// equipment) on the type picked at login.
func (u *lanUI) openNewHero() {
	if u.newFrames == nil {
		return
	}
	if u.hero != nil {
		u.looks = u.hero.Looks // the account's looks, when known
	}
	u.disconnect()
	u.addr.Text = nil
	u.nh = gamestate.NewLANHeroTemplate()
	u.setNewModel(u.model)
	u.state, u.msg, u.focus, u.clanScroll = lanNewHero, 0, 0, 0
}

// setNewModel shows hero type m with its look.
func (u *lanUI) setNewModel(m int) {
	gamestate.SelectHero(&u.nh, m)
	u.nh.Equipment = [6]int{} // the LAN template has no items (0x0042E0DB)
	u.nh.PaletteOffset = uint32(gamestate.LANLookPalette(m, u.looks[m])) * 512
}

func (u *lanUI) newRect(i int) gamestate.Rect { return u.exe.NewHeroRects[i] }

func (u *lanUI) updateNewHero(mx, my int, click bool) {
	if u.focus != 0 {
		u.typeText()
	}
	hit := gamestate.HitRects(u.exe.NewHeroRects, mx, my)
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) && hit == gamestate.LANNewPortrait {
		// Port addition: the next hero type, in the login portraits' order.
		for i, m := range gamestate.LoginModels {
			if m == u.model {
				u.model = gamestate.LoginModels[(i+1)%len(gamestate.LoginModels)]
				break
			}
		}
		u.setNewModel(u.model)
		return
	}
	if !click {
		return
	}
	u.commitField()
	if hit >= 0 {
		sfx.play(gamestate.SoundClick, 0, 0)
	}
	e := &u.nh
	switch {
	case hit == gamestate.LANNewClan:
		u.focus = 1
	case hit == gamestate.LANNewClanUp:
		u.clanScroll = max(u.clanScroll-1, 0)
	case hit == gamestate.LANNewClanDown:
		if u.clanScroll+u.clanRows() < len(u.clans) {
			u.clanScroll++
		}
	case hit >= gamestate.NewHeroClass && hit < gamestate.NewHeroStat:
		gamestate.SelectClass(e, newHeroClasses[hit-gamestate.NewHeroClass])
	case hit == gamestate.NewHeroFencing || hit == gamestate.NewHeroArchery:
		gamestate.SelectCombatSkill(e, hit == gamestate.NewHeroArchery)
	case hit >= gamestate.NewHeroIdentify && hit <= gamestate.NewHeroHealing:
		gamestate.SelectSkill(e, hit-gamestate.NewHeroIdentify)
	case hit == gamestate.NewHeroOK:
		u.newHeroOK()
	case hit == gamestate.NewHeroCancel: // 0x0042E826: the LAN menu
		u.toMenu()
	case hit == gamestate.LANNewPortrait:
		u.looks[u.model] = (u.looks[u.model] + 1) % 8
		u.setNewModel(u.model)
	case hit == gamestate.LANNewClanList:
		r := u.newRect(gamestate.LANNewClanList)
		if k := u.clanScroll + (my-r.Y1-5)/gamestate.LANNewClanRowH; my >= r.Y1+5 && k < len(u.clans) && k-u.clanScroll < u.clanRows() {
			u.clan.Text = append([]byte(nil), u.clans[k]...)
			u.saveConfig()
		}
	case hit >= gamestate.NewHeroMinus:
		k := hit - gamestate.NewHeroMinus
		gamestate.LANTradeStat(e, k/2, k%2 == 1)
	}
}

// clanRows is the list's visible rows ((height − 5) / 15).
func (u *lanUI) clanRows() int {
	r := u.newRect(gamestate.LANNewClanList)
	return (r.Y2 - r.Y1 - 5) / gamestate.LANNewClanRowH
}

// newHeroOK is «Ok» (0x0042E756): the build is kept for the next join
// (the original's flag [0xA6EAF8]), the clan added to the list, LAN.CFG
// saved, and the login opens.
func (u *lanUI) newHeroOK() {
	c := gamestate.ChoiceOf(u.nh)
	u.pending = &netplay.HeroChoice{Class: int(c.Class), Archery: c.Archery, Skill: c.Skill, Stats: c.Stats, Looks: u.looks}
	u.addClan(u.clan.Text)
	u.saveConfig()
	u.enterLogin()
}

func (u *lanUI) drawNewHero(dst *ebiten.Image) {
	tile := u.newFrames[newTile]
	tw, th := tile.Bounds().Dx(), tile.Bounds().Dy()
	for y := 0; y < ScreenHeight; y += th {
		for x := 0; x < ScreenWidth; x += tw {
			drawAt(dst, tile, x, y)
		}
	}
	dst.DrawImage(u.newFrames[newBackground], nil)
	e := u.nh
	R := u.newRect
	u.fieldText(dst, 1, u.clan.Text, R(gamestate.LANNewClan))
	list := R(gamestate.LANNewClanList)
	for i := range u.clanRows() {
		if k := u.clanScroll + i; k < len(u.clans) {
			u.font.drawLine(dst, clip(u.font, u.clans[k], list.X2-list.X1-10), list.X1+5, list.Y1+5+i*gamestate.LANNewClanRowH)
		}
	}
	u.newMarker(dst, int(e.Flags4&7)+1) // class byte 2–5 → hotspot 3–6
	if e.Fencing > e.Marksmanship {
		u.newMarker(dst, gamestate.NewHeroFencing)
	} else {
		u.newMarker(dst, gamestate.NewHeroArchery)
	}
	if k := gamestate.ChosenSkill(e); k >= 0 {
		u.newMarker(dst, gamestate.NewHeroIdentify+k)
	}
	value := func(hotspot int, v string) {
		r := R(hotspot)
		u.font.drawLine(dst, []byte(v), r.X1, r.Y1)
	}
	for i, v := range e.BaseStats {
		value(gamestate.NewHeroStat+i, strconv.Itoa(v))
	}
	class := e.Flags4 & 7
	value(gamestate.NewHeroWeight, fmt.Sprintf("%4.1f", float64(gamestate.LANMaxWeight(e.BaseStats[3]))*0.001))
	value(gamestate.NewHeroParty, strconv.Itoa(gamestate.LANMaxParty(class, e.BaseStats[0])))
	value(gamestate.NewHeroSmithing, strconv.Itoa(e.Skills[1]))
	value(gamestate.NewHeroBuilding, strconv.Itoa(e.Skills[3]))
	value(gamestate.NewHeroFreeExp, strconv.Itoa(e.FreeExp))
	r := R(gamestate.NewHeroOK)
	drawAt(dst, u.newFrames[newOK], r.X1, r.Y1)
	r = R(gamestate.NewHeroCancel)
	drawAt(dst, u.newFrames[newCancel], r.X1, r.Y1)
	r = R(gamestate.LANNewPortrait)
	u.preview.draw(dst, e, u.graph, u.items, u.tables, r.X1, r.Y1)
}

// newMarker centres the marker in the class box's size at hotspot i, like
// the single-player screen (drawMarker).
func (u *lanUI) newMarker(dst *ebiten.Image, i int) {
	m := u.newFrames[newMarker]
	c, r := u.newRect(gamestate.NewHeroClass), u.newRect(i)
	drawAt(dst, m, r.X1+(c.X2-c.X1-m.Bounds().Dx())/2, r.Y1+(c.Y2-c.Y1-m.Bounds().Dy())/2)
}
