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
	"konung1.5/internal/resfmt"
)

const (
	ScreenWidth            = 800
	ScreenHeight           = 600
	menuItems              = 6
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
)

// firstItem is the first visible menu item: the draw routine 0x0042F8D8
// starts at 0 while a game is running and at 2 otherwise, and the menu loops
// (0x0043799F, 0x00438D81) ignore hits on items below 2 without a game.
func (m *Menu) firstItem() int {
	if m.running {
		return itemReturn
	}
	return itemLoad
}

type rect struct{ x, y, w, h int }

// menuButtons800 is the 800x600 half of the main-menu layout table in
// konung.exe (DGROUP VA 0x0045B314, 32 bytes per button: 640x480 x1,y1,x2,y2
// then 800x600 x1,y1,x2,y2). The draw routine at VA 0x0042F8D8 blits each
// button frame with its top-left at (x1, y1).
var menuButtons800 = [menuItems][4]int{
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
	if len(frames) < 3+menuItems*2 {
		return nil, fmt.Errorf("%s has %d frames; need at least %d", resourcePath, len(frames), 3+menuItems*2)
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
	for i := range menuItems {
		m.buttons[i][0] = ebiten.NewImageFromImage(frames[3+i*2])
		m.buttons[i][1] = ebiten.NewImageFromImage(frames[4+i*2])
		b := menuButtons800[i]
		m.buttonRects[i] = rect{x: b[0], y: b[1], w: b[2] - b[0], h: b[3] - b[1]}
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
		if m.film.update() {
			m.endFilm()
		}
		return nil
	}
	// Escape is switched on the screen state 0x00A3E6D8 (0x00437430, jump
	// table 0x0043756C).
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if m.inWorld {
			if m.world.escape() {
				m.inWorld = false
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
	if m.inWorld {
		// T (VK 0x54, 0x00436F58) toggles background tiling and writes
		// KONUNG.CFG at once.
		if inpututil.IsKeyJustPressed(ebiten.KeyT) {
			m.config.Tiling = 1 - min(m.config.Tiling, 1)
			m.writeConfig()
		}
		m.world.Update()
		if m.world.gameOver() {
			// 0x004376A8: the hero died; no game is running any more
			// ([0x00A3E728] = 0), GAMEOVER.FLM plays, then the menu (state 7).
			m.running = false
			m.playFilm(resfmt.FilmGameOver, false, func() {
				m.inWorld = false
				if m.selected < m.firstItem() {
					m.selected = itemNewGame
				}
				m.world = nil
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
	first := m.firstItem()
	if m.selected < first {
		m.selected = first
	}
	n := menuItems - first
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		m.selected = first + (m.selected-first+n-1)%n
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		m.selected = first + (m.selected-first+1)%n
	}
	activate := -1
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		activate = m.selected
	}
	mx, my := ebiten.CursorPosition()
	for i := first; i < menuItems; i++ {
		r := m.buttonRects[i]
		if mx >= r.x && mx < r.x+r.w && my >= r.y && my < r.y+r.h {
			m.selected = i
			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
				if m.running {
					sfx.play(gamestate.SoundClick, 0, 0) // in-game menu only (0x004379E2)
				}
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
	switch s.mode {
	case screenSave:
		// A save stores the settings 0x00A3E71C and 0x00A3E734.
		m.world.game.Options = [2]int32{m.config.DayNight, m.config.Lighting}
		if err := s.save(s.selected, m.world); err != nil {
			log.Printf("save slot %d: %v", s.selected, err)
			return nil
		}
		m.inWorld = true
	case screenLoad:
		w, err := m.assets.loadSaved(s.dir, s.selected)
		if err != nil {
			log.Printf("load slot %d: %v", s.selected, err)
			return nil
		}
		m.world, m.running, m.inWorld = w, true, true
		// Loading restores them (0x004249AC); KONUNG.CFG is not rewritten.
		m.config.DayNight, m.config.Lighting = w.game.Options[0], w.game.Options[1]
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
	screen.DrawImage(m.background, nil)
	for i := m.firstItem(); i < menuItems; i++ {
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
