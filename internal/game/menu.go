package game

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
)

const (
	ScreenWidth            = 800
	ScreenHeight           = 600
	menuItems              = 7 // the six MENU800R.RES buttons + «Сетевая игра»
	paintedItems           = 6 // items with button frames in MENU800R.RES
	swordCursorSpriteIndex = 4
	cursorSprites          = 11
)

type Menu struct {
	background  *ebiten.Image
	sword       *ebiten.Image
	cursors     [cursorSprites]*ebiten.Image // GRAPH.RES Type 5 sprites 0–10
	buttons     [menuItems][2]*ebiten.Image
	selected    int
	buttonRects [menuItems]rect
	world       *startScene // nil until a game starts
	assets      *sceneAssets
	inWorld     bool
	// running mirrors [0x00A3E728]: a game is in progress, so the menu also
	// offers «Вернуться в игру» and «Сохранить игру» (items 0 and 1).
	running bool
	saves   *saveScreen
	inSaves bool // the load or save screen is shown
	// newHero is the character-creation screen (state 2); starting is set
	// for the frame that shows «Ok» pressed before the game loads.
	newHero   *newHeroScreen
	inNewHero bool
	starting  bool
	// config holds the settings (KONUNG.CFG at cfgPath, next to the game
	// files as in the original 0x0083DE58); settings is their screen.
	config     gamestate.Config
	cfgPath    string
	settings   *settingsScreen
	inSettings bool
	// film is the film playing (0x0042B690, [0x00A3E724] set), afterFilm
	// the state it leads to ([0x00A3E7F0]); gameDir holds Video\.
	film      *filmPlayer
	afterFilm func()
	gameDir   string
	// lan runs the LAN version's screens behind «Сетевая игра» (nil when
	// the LAN files are missing from the game folder; the item is hidden then).
	lan   *lanUI
	inLAN bool
	// lanWorld: the running game is a LAN world (no saving; leaving it for
	// a single-player game drops the connection).
	lanWorld bool
}

// Main-menu items in the order of the exe layout table (0x0045B314) and the
// click switch at 0x00437E50.
const (
	itemReturn   = 0 // 0x004379F8
	itemSave     = 1 // 0x00437A25
	itemLoad     = 2 // 0x00437B54, opens the load screen
	itemNewGame  = 3 // 0x00437B91
	itemSettings = 4 // 0x00437DDD, opens the settings screen
	itemExit     = 5 // 0x00437E3D
	itemNetwork  = 6 // port addition: «Сетевая игра», the LAN login screen
)

// menuOrder is the on-screen order of the items, top to bottom, for the
// arrow keys; «Сетевая игра» sits where «Сохранить игру» appears in a game.
var menuOrder = [menuItems]int{itemReturn, itemSave, itemNetwork, itemLoad, itemNewGame, itemSettings, itemExit}

// firstItem is the first visible menu item: the draw routine 0x0042F8D8
// starts at 0 while a game is running and at 2 otherwise, and the menu loops
// (0x0043799F, 0x00438D81) ignore hits on items below 2 without a game.
func (m *Menu) firstItem() int {
	if m.running {
		return itemReturn
	}
	return itemLoad
}

// visible reports whether item i is shown: «Вернуться в игру» and
// «Сохранить игру» only in a game, «Сетевая игра» only without one.
func (m *Menu) visible(i int) bool {
	if i == itemNetwork {
		return !m.running && m.lan != nil
	}
	if i == itemSave && m.lanWorld {
		return false // a LAN world is not saved as a single-player game
	}
	return i >= m.firstItem()
}

// checkMenuFrames checks that a MENU*R.RES file has the two screens, the
// marker and a normal/selected pair for each painted button (frames 3..14).
func checkMenuFrames(path string, n int) error {
	if need := 3 + paintedItems*2; n < need {
		return fmt.Errorf("%s has %d frames; need at least %d", path, n, need)
	}
	return nil
}

type rect struct{ x, y, w, h int }

// menuButtons800 is the 800x600 half of the main-menu layout table in
// konung.exe (DGROUP VA 0x0045B314, 32 bytes per button: 640x480 x1,y1,x2,y2
// then 800x600 x1,y1,x2,y2). The draw routine at VA 0x0042F8D8 blits each
// button frame with its top-left at (x1, y1).
var menuButtons800 = [paintedItems][4]int{
	{290, 161, 590, 210}, // Вернуться в игру
	{288, 205, 504, 250}, // Сохранить игру
	{265, 250, 488, 303}, // Загрузить игру
	{280, 303, 440, 354}, // Новая игра
	{268, 353, 416, 402}, // Установки
	{276, 409, 366, 464}, // Выход
}

func New(resourcePath, graphPath string) (*Menu, error) {
	ebiten.SetCursorMode(ebiten.CursorModeHidden)
	frames, err := resfmt.LoadType5File(resourcePath)
	if err != nil {
		return nil, fmt.Errorf("load menu resource %q: %w", resourcePath, err)
	}
	if err := checkMenuFrames(resourcePath, len(frames)); err != nil {
		return nil, err
	}
	swordFrame, err := loadGraphSprite(graphPath, swordCursorSpriteIndex)
	if err != nil {
		return nil, err
	}
	objectsPath, err := FindGameResource("assets/Konung/OBJECTS.RES")
	if err != nil {
		return nil, err
	}
	// KONUNG.N, GAME.N, the saves and KONUNG.CFG live in the game directory.
	mapPath, err := FindGameResource("assets/Konung/KONUNG.1")
	if err != nil {
		return nil, err
	}
	gameDir := filepath.Dir(mapPath)
	exePath, err := FindGameResource("assets/Konung/konung.exe")
	if err != nil {
		return nil, err
	}
	intrPath, err := FindGameResource("assets/Konung/Intr800r.res")
	if err != nil {
		return nil, err
	}
	assets, err := loadSceneAssets(graphPath, objectsPath, gameDir, exePath, intrPath)
	if err != nil {
		return nil, err
	}
	newhPath, err := FindGameResource("assets/Konung/NEWH800R.RES")
	if err != nil {
		return nil, err
	}
	newHero, err := newNewHeroScreen(newhPath, assets)
	if err != nil {
		return nil, err
	}

	saveRes, err := FindGameResource("assets/Konung/SAVE800R.RES")
	if err != nil {
		return nil, err
	}
	// 0x0042D3DC: SAVE<width>R.RES; slots and names live next to the game files.
	saves, err := newSaveScreen(saveRes, gameDir, assets.tables, assets.largeFont)
	if err != nil {
		return nil, err
	}

	// 0x0042BA60: read the settings at startup; defaults without the file.
	cfgPath := filepath.Join(gameDir, gamestate.ConfigFile)
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		cfgData = nil
	} else if cfgData == nil {
		cfgData = []byte{}
	}

	config := gamestate.ParseConfig(cfgData)
	// 0x00438E04: SOUNDS.RES and MUSICS.RES load at startup and a random
	// track starts, before the menu.
	startAudio(gameDir, config.Music != 0, config.Sound != 0)

	m := &Menu{
		config:     config,
		cfgPath:    cfgPath,
		settings:   newSettingsScreen(frames[1], frames[2]),
		saves:      saves,
		background: ebiten.NewImageFromImage(frames[0]),
		sword:      ebiten.NewImageFromImage(swordFrame),
		assets:     assets,
		newHero:    newHero,
		selected:   itemNewGame,
		gameDir:    gameDir,
	}
	for i := range m.cursors {
		img, err := loadGraphSprite(graphPath, i)
		if err != nil {
			return nil, err
		}
		m.cursors[i] = ebiten.NewImageFromImage(img)
	}
	for i := range paintedItems {
		m.buttons[i][0] = ebiten.NewImageFromImage(frames[3+i*2])
		m.buttons[i][1] = ebiten.NewImageFromImage(frames[4+i*2])
		b := menuButtons800[i]
		m.buttonRects[i] = rect{x: b[0], y: b[1], w: b[2] - b[0], h: b[3] - b[1]}
	}
	// «Сетевая игра» only when the LAN version's files are installed (in the
	// game folder): LAN_KONG.EXE (layout, texts), ME800R.RES, PLAY800R.RES
	// (and NEW800R.RES for «Новый герой»).
	mePath, err1 := FindGameResource("assets/Konung/ME800R.RES")
	playPath, err2 := FindGameResource("assets/Konung/PLAY800R.RES")
	if err1 == nil && err2 == nil {
		newPath, _ := FindGameResource("assets/Konung/NEW800R.RES") // «Новый герой»; optional
		lan, err := newLANUI(gameDir, assets, mePath, playPath, newPath)
		if err != nil {
			log.Printf("LAN screens unavailable: %v", err)
		} else {
			labels, err := networkButton(assets.tables)
			if err != nil {
				return nil, err
			}
			m.lan = lan
			m.buttons[itemNetwork] = [2]*ebiten.Image{toEbiten(labels[0]), toEbiten(labels[1])}
			b := labels[0].Bounds()
			m.buttonRects[itemNetwork] = rect{x: networkLabelX, y: networkLabelY, w: b.Dx(), h: b.Dy()}
		}
	}
	// State 9 (0x0043914B), after loading: POSTER.FLM, then the menu
	// (state 7) with no game running.
	m.playFilm(resfmt.FilmPoster, true, func() {})
	return m, nil
}

// playFilm is 0x0042B690: mute the music and play the film; next is the
// state queued in 0x00A3E7F0, entered when the film is over (0x0042B774),
// or at once when the film cannot play. blank starts on a black screen
// (the start-up poster) instead of the current frame.
func (m *Menu) playFilm(name string, blank bool, next func()) {
	f := openFilm(m.gameDir, name)
	if f == nil {
		next()
		return
	}
	if blank {
		f.frozen = ebiten.NewImage(ScreenWidth, ScreenHeight)
	}
	sfx.setMusic(false)
	m.film, m.afterFilm = f, next
}

// endFilm is 0x0042B774: close the film, restore the music volume
// (0x0042A420([0x00A3E73C])) and enter the queued state.
func (m *Menu) endFilm() {
	m.film.close()
	sfx.setMusic(m.config.Music != 0)
	next := m.afterFilm
	m.film, m.afterFilm = nil, nil
	next()
}

func (m *Menu) Update() error {
	if sfx != nil {
		sfx.soundOn = m.config.Sound != 0 // checked at each play (0x0042A679)
		sfx.update()
	}
	// While a film plays the game loop waits (0x0042BD26).
	if m.film != nil {
		if !m.film.update() {
			return nil
		}
		// 0x0042BE7D: the key that stops a film is queued for the game
		// first, so the state after the film handles it (Escape after
		// GAMEOVER.FLM reaches the menu's Escape). Go on with this
		// frame's input.
		m.endFilm()
	}
	// Escape is switched on the screen state 0x00A3E6D8 (0x00437430, jump
	// table 0x0043756C).
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if m.inWorld {
			if m.world.escape() {
				m.inWorld = false
				if m.lanWorld {
					// A network game has the LAN menu, not the main menu.
					m.lan.openInGame()
					m.inLAN = true
				}
			}
			return nil
		}
		// Load/save screens (0x004374FE): back to the menu.
		if m.inSaves {
			m.inSaves = false
			return nil
		}
		// New game (0x004374DB): back to the menu; the game that was
		// running is over ([0x00A3E728] = 0).
		if m.inNewHero {
			m.inNewHero = false
			m.running = false
			return nil
		}
		// LAN screens (port addition): their own Escape (0x0042D90D).
		if m.inLAN {
			if m.lan.escape() {
				m.inLAN = false
			}
			return nil
		}
		// Settings (0x00437517): restore the values and go back.
		if m.inSettings {
			m.settings.restore(&m.config)
			m.inSettings = false
			return nil
		}
		// Menu (0x004374A5): resume a running game, else nothing.
		if m.running {
			m.inWorld = true
		}
		return nil
	}
	if m.lanWorld {
		// Keep reading the session in the world (the other heroes, a lost
		// connection); unread messages would stall the connection.
		m.lan.readSession()
		m.applyLAN()
		m.world.clock = m.lan.clock
		if m.lan.client == nil {
			// The connection is gone: the game is over; the LAN login
			// shows «Сервер не доступен».
			m.endLANGame()
			m.lan.enterLogin()
			m.lan.msg = gamestate.MsgUnavailable
			m.inWorld, m.inLAN = false, true
			return nil
		}
	}
	if m.inWorld {
		// T (VK 0x54, 0x00436F58) toggles background tiling and writes
		// KONUNG.CFG at once.
		if inpututil.IsKeyJustPressed(ebiten.KeyT) {
			m.config.Tiling = 1 - min(m.config.Tiling, 1)
			m.writeConfig()
		}
		m.world.Update()
		if f := m.world.Film; f != "" {
			toMenu := m.world.FilmToMenu
			m.world.Film = ""
			m.playFilm(f, false, func() {
				if toMenu {
					m.inWorld = false // state 7; m.running stays set
				}
			})
			return nil
		}
		if m.world.GameOver() && !m.lanWorld {
			// 0x004376A8: the hero died; no game is running any more
			// (in a LAN world the dead hero lies in the world and its
			// player watches: a living player's «Слова Воскрешения» or the
			// session's end bring it back; «Закончить игру» leaves).
			// ([0x00A3E728] = 0), GAMEOVER.FLM plays, then the menu (state 7).
			m.running = false
			lan := m.lanWorld
			m.playFilm(resfmt.FilmGameOver, false, func() {
				m.inWorld = false
				if lan { // a network game goes back to the LAN menu
					m.endLANGame()
					m.lan.open()
					m.inLAN = true
					return
				}
				if m.selected < m.firstItem() {
					m.selected = itemNewGame
				}
				m.world = nil
				m.leaveLANWorld()
			})
		}
		return nil
	}
	if m.inNewHero {
		return m.updateNewHero()
	}
	if m.inSaves {
		return m.updateSaves()
	}
	if m.inSettings {
		m.updateSettings()
		return nil
	}
	if m.inLAN {
		left := m.lan.update()
		if m.lan.endGame {
			m.endLANGame()
		}
		if left {
			m.inLAN = false
		}
		if m.lan.Resume {
			m.lan.Resume = false
			if m.lanWorld && m.lan.client != nil {
				m.inLAN, m.inWorld = false, true
			} else {
				m.endLANGame()
			}
			return nil
		}
		// The server answered «Ok»: enter the LAN world (state 10 → 1).
		if at, hero, name := m.lan.takeStart(); at != nil {
			w, err := m.assets.lanView(m.gameDir, m.lan.exe, *at, hero, m.lan.model, name)
			if err != nil {
				log.Printf("LAN world: %v", err)
				m.lan.disconnect()
				m.lan.state, m.lan.msg = lanLogin, gamestate.MsgUnavailable
				return nil
			}
			m.world, m.running, m.inWorld, m.inLAN, m.lanWorld = w, true, true, false, true
			lan := m.lan
			w.view.order = func(o netplay.OrderPayload) {
				if lan.client != nil {
					lan.client.Order(o)
				}
			}
			m.applyLAN()
		}
		return nil
	}
	var order []int
	for _, i := range menuOrder {
		if m.visible(i) {
			order = append(order, i)
		}
	}
	pos := -1
	for k, i := range order {
		if i == m.selected {
			pos = k
		}
	}
	if pos < 0 {
		pos = 0
		for order[pos] != itemNewGame && order[pos] != itemReturn && pos < len(order)-1 {
			pos++
		}
		m.selected = order[pos]
	}
	n := len(order)
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		m.selected = order[(pos+n-1)%n]
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		m.selected = order[(pos+1)%n]
	}
	activate := -1
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		activate = m.selected
	}
	mx, my := ebiten.CursorPosition()
	for _, i := range order {
		r := m.buttonRects[i]
		if mx >= r.x && mx < r.x+r.w && my >= r.y && my < r.y+r.h {
			m.selected = i
			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
				// 0x004379E2: every click on an active item (without a game,
				// items 0 and 1 are inactive and never reach here).
				sfx.play(gamestate.SoundClick, 0, 0)
				activate = i
			}
		}
	}
	return m.activate(activate)
}

// activate runs a menu item.
func (m *Menu) activate(item int) error {
	switch item {
	case itemReturn:
		m.inWorld = true
	case itemSave:
		m.saves.open(screenSave, m.world.saveName(time.Now()))
		m.inSaves = true
	case itemLoad:
		m.saves.open(screenLoad, nil)
		m.inSaves = true
	case itemNewGame: // 0x00437B91: the character-creation screen
		m.newHero.open()
		m.inNewHero = true
	case itemSettings:
		m.settings.open(m.config)
		m.inSettings = true
	case itemExit:
		return ebiten.Termination
	case itemNetwork:
		m.lan.open()
		m.inLAN = true
	}
	return nil
}

// updateSaves runs the load/save screen: the slot under the mouse is
// selected; a left click on a slot loads or saves it and goes to the game,
// a click elsewhere or a failed load returns to the menu (0x00438966,
// 0x004389F4).
func (m *Menu) updateSaves() error {
	mx, my := ebiten.CursorPosition()
	s := m.saves
	s.selected = s.hit(mx, my)
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return nil
	}
	m.inSaves = false
	if s.selected < 0 {
		return nil
	}
	switch s.Mode {
	case screenSave:
		// A save stores the settings 0x00A3E71C and 0x00A3E734.
		m.world.Game.Options = [2]int32{m.config.DayNight, m.config.Lighting}
		if err := s.save(s.selected, m.world); err != nil {
			log.Printf("save slot %d: %v", s.selected, err)
			return nil
		}
		m.inWorld = true
	case screenLoad:
		w, err := m.assets.loadSaved(s.Dir, s.selected)
		if err != nil {
			log.Printf("load slot %d: %v", s.selected, err)
			return nil
		}
		m.leaveLANWorld()
		m.world, m.running, m.inWorld = w, true, true
		// Loading restores them (0x004249AC); KONUNG.CFG is not rewritten.
		m.config.DayNight, m.config.Lighting = w.Game.Options[0], w.Game.Options[1]
	}
	return nil
}

// updateNewHero runs the character-creation screen (0x00437E86): a left
// click acts on the hotspot under the mouse, otherwise it shows that
// hotspot's description. «Ok» (state 8) is drawn pressed for a frame, then
// the game starts with the chosen hero (0x00438F3F); «Отмена» returns to the
// menu and ends a running game ([0x00A3E728] = 0).
func (m *Menu) updateNewHero() error {
	if m.starting {
		m.starting = false
		w, err := m.assets.newGame(m.newHero.hero)
		if err != nil {
			// 0x004390E8: the map failed to load; the game reports it.
			log.Printf("new game: %v", err)
			m.inNewHero, m.running = false, false
			return nil
		}
		m.leaveLANWorld()
		m.world, m.inNewHero, m.running, m.inWorld = w, false, true, true
		return nil
	}
	mx, my := ebiten.CursorPosition()
	hit := gamestate.NewHeroHit(mx, my)
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		m.newHero.hover(hit)
		return nil
	}
	switch ok, cancel := m.newHero.click(hit); {
	case ok:
		m.starting = true
	case cancel:
		m.inNewHero, m.running = false, false
	}
	return nil
}

// updateSettings runs the settings screen: a left click on a hotspot acts
// on it (0x00438A94). «Ok» writes KONUNG.CFG with the chosen video mode
// (0x0043D2CC); «Отмена» restores the values. Both return to the menu.
func (m *Menu) updateSettings() {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	mx, my := ebiten.CursorPosition()
	hit := m.settings.hit(mx, my)
	closed, ok := m.settings.click(hit, &m.config)
	if hit == 6 || hit == settingsCancel {
		sfx.setMusic(m.config.Music != 0) // 0x00438BD0, 0x00438CDB
	}
	if !closed {
		return
	}
	m.inSettings = false
	if ok {
		// The game switches to the chosen mode first (0x00427B1C); the
		// port stays at 800x600, so the running mode is unchanged.
		m.writeConfig()
	}
}

// writeConfig is 0x0043D2CC: KONUNG.CFG with the video mode last chosen on
// the settings screen (0x00A539A0; 0 if the screen was never opened, as in
// the game).
func (m *Menu) writeConfig() {
	c := m.config
	c.Video = m.settings.video
	if err := os.WriteFile(m.cfgPath, c.Marshal(), 0o644); err != nil {
		log.Printf("write %s: %v", m.cfgPath, err)
	}
}

func (m *Menu) Draw(screen *ebiten.Image) {
	if f := m.film; f != nil {
		if f.frozen == nil { // the frame the film starts on stays around it
			f.frozen = ebiten.NewImage(ScreenWidth, ScreenHeight)
			m.drawScreen(f.frozen)
		}
		f.draw(screen)
		return
	}
	m.drawScreen(screen)
}

func (m *Menu) drawScreen(screen *ebiten.Image) {
	if m.inWorld && m.world != nil {
		m.world.shadows = m.config.Shadows != 0
		m.world.dayNight = m.config.DayNight != 0
		m.world.lighting = m.config.Lighting != 0
		m.world.transparency = m.config.Transparency != 0
		m.world.tiling = m.config.Tiling != 0
		m.world.Draw(screen)
		mx, my := ebiten.CursorPosition()
		cur := m.sword
		if n := m.world.cursorSprite(); n >= 0 && n < cursorSprites {
			cur = m.cursors[n]
		}
		drawAt(screen, cur, mx-1, my-1)
		return
	}
	mx, my := ebiten.CursorPosition()
	if m.inSaves {
		m.saves.draw(screen)
		drawAt(screen, m.sword, mx-1, my-1)
		return
	}
	if m.inSettings {
		m.settings.draw(screen, m.config)
		drawAt(screen, m.sword, mx-1, my-1)
		return
	}
	if m.inNewHero {
		m.newHero.draw(screen)
		drawAt(screen, m.sword, mx-1, my-1)
		return
	}
	if m.inLAN {
		m.lan.draw(screen)
		drawAt(screen, m.sword, mx-1, my-1)
		return
	}
	screen.DrawImage(m.background, nil)
	for _, i := range menuOrder {
		if !m.visible(i) {
			continue
		}
		r := m.buttonRects[i]
		button := m.buttons[i][0]
		if i == m.selected {
			button = m.buttons[i][1]
		}
		drawAt(screen, button, r.x, r.y)
	}
	// Keep the blade tip at the native cursor position, matching the in-game pointer.
	drawAt(screen, m.sword, mx-1, my-1)
}

func drawAt(dst, src *ebiten.Image, x, y int) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(src, op)
}

func (m *Menu) Layout(_, _ int) (int, int) { return ScreenWidth, ScreenHeight }

// endLANGame is «Закончить игру» (or a lost connection): the LAN world
// is dropped and the client logged out; the LAN menu stays.
func (m *Menu) endLANGame() {
	m.lan.endGame, m.lan.inGame = false, false
	if m.lanWorld {
		m.world, m.running, m.inWorld, m.lanWorld = nil, false, false, false
	}
	m.lan.disconnect()
}

// leaveLANWorld ends a LAN game when a single-player game replaces it: the
// connection (and a session hosted from this game) closes.
func (m *Menu) leaveLANWorld() {
	if m.lanWorld {
		m.lanWorld = false
		m.lan.disconnect()
		m.lan.state = lanMenu
	}
}

// applyLAN hands the queued server messages to the LAN viewer
// (lanview.go): views, players' heroes, sounds, the own map change and the
// own hero's values.
func (m *Menu) applyLAN() {
	msgs := m.lan.worldMsgs
	m.lan.worldMsgs = nil
	if m.world == nil || m.world.view == nil {
		return
	}
	w := m.world
	for _, msg := range msgs {
		switch msg.Type {
		case netplay.MsgPlayer:
			var p netplay.PlayerPayload
			if msg.Decode(&p) == nil {
				w.applyPlayer(p)
			}
		case netplay.MsgView:
			var v netplay.ViewPayload
			if msg.Decode(&v) == nil {
				w.applyView(v)
			}
		case netplay.MsgEvents:
			var e netplay.EventsPayload
			if msg.Decode(&e) == nil {
				w.applyEvents(e.Events)
			}
		case netplay.MsgMapChange:
			var at netplay.EnterPayload
			if msg.Decode(&at) == nil {
				w.viewChangeMap(at)
			}
		case netplay.MsgHero:
			var h netplay.Hero
			if msg.Decode(&h) == nil {
				w.ownHero(h)
			}
		case netplay.MsgInventory:
			var inv netplay.InventoryPayload
			if msg.Decode(&inv) == nil {
				w.applyInventory(inv)
			}
		case netplay.MsgPiles:
			var p netplay.PilesPayload
			if msg.Decode(&p) == nil {
				w.applyPiles(p)
			}
		case netplay.MsgPileWindow:
			var pw netplay.PileWindowPayload
			if msg.Decode(&pw) == nil {
				w.applyPileWindow(pw)
			}
		case netplay.MsgDialogue:
			var d netplay.DialoguePayload
			if msg.Decode(&d) == nil {
				w.applyDialogue(d)
			}
		case netplay.MsgParty:
			var p netplay.PartyPayload
			if msg.Decode(&p) == nil {
				w.applyParty(p)
			}
		}
	}
}
