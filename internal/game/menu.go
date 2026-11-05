package game

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
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
	world       *startScene
	newWorld    func() (*startScene, error) // a fresh New Game scene (after a game over)
	inWorld     bool
	// running mirrors [0x00A3E728]: a game is in progress, so the menu also
	// offers «Вернуться в игру» and «Сохранить игру» (items 0 and 1).
	running bool
	fresh   bool // world has not been played yet
	saves   *saveScreen
	inSaves bool // the load or save screen is shown
}

// Main-menu items in the order of the exe layout table (0x0045B314) and the
// click switch at 0x00437E50.
const (
	itemReturn   = 0 // 0x004379F8
	itemSave     = 1 // 0x00437A25
	itemLoad     = 2 // 0x00437B54, opens the load screen
	itemNewGame  = 3 // 0x00437B91
	itemSettings = 4 // 0x00437DDD (not implemented)
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
	mapPath, err := FindGameResource("assets/Konung/KONUNG.1")
	if err != nil {
		return nil, err
	}
	// The game opens GAME.<hero model>; model 0 is Volk, whose start map is
	// model*3+1 = KONUNG.1. Hero choice is not implemented yet.
	gamePath, err := FindGameResource("assets/Konung/GAME.0")
	if err != nil {
		return nil, err
	}
	exePath, err := FindGameResource("assets/Konung/konung.exe")
	if err != nil {
		return nil, err
	}
	intrPath, err := FindGameResource("assets/Konung/Intr800r.res")
	if err != nil {
		return nil, err
	}
	newWorld := func() (*startScene, error) {
		return loadStartScene(graphPath, objectsPath, mapPath, gamePath, exePath, intrPath, 0, 1)
	}
	world, err := newWorld()
	if err != nil {
		return nil, fmt.Errorf("load KONUNG.1 scene: %w", err)
	}

	saveRes, err := FindGameResource("assets/Konung/SAVE800R.RES")
	if err != nil {
		return nil, err
	}
	// 0x0042D3DC: SAVE<width>R.RES; slots and names live next to the game files.
	saves, err := newSaveScreen(saveRes, filepath.Dir(mapPath), world.tables, world.assets.largeFont)
	if err != nil {
		return nil, err
	}

	m := &Menu{
		saves:      saves,
		background: ebiten.NewImageFromImage(frames[0]),
		sword:      ebiten.NewImageFromImage(swordFrame),
		world:      world,
		newWorld:   newWorld,
		fresh:      true,
		selected:   itemNewGame,
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
	return m, nil
}

func (m *Menu) Update() error {
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
		// Menu (0x004374A5): resume a running game, else nothing.
		if m.running {
			m.inWorld = true
		}
		return nil
	}
	if m.inWorld {
		m.world.Update()
		if m.world.gameOver() {
			// 0x004376A8: the hero died; the game plays GAMEOVER.FLM (not in
			// the installed files) and leaves the game loop for the menu.
			m.inWorld = false
			m.running = false
			if m.selected < m.firstItem() {
				m.selected = itemNewGame
			}
			w, err := m.newWorld()
			if err != nil {
				return err
			}
			m.world = w
			m.fresh = true
		}
		return nil
	}
	if m.inSaves {
		return m.updateSaves()
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
				activate = i
			}
		}
	}
	return m.activate(activate)
}

// activate runs a menu item. Settings is drawn but has no action yet.
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
	case itemNewGame:
		if !m.fresh {
			w, err := m.newWorld()
			if err != nil {
				return err
			}
			m.world = w
		}
		m.fresh = false
		m.running = true
		m.inWorld = true
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
		if err := s.save(s.selected, m.world); err != nil {
			log.Printf("save slot %d: %v", s.selected, err)
			return nil
		}
		m.inWorld = true
	case screenLoad:
		w, err := m.world.assets.loadSaved(s.dir, s.selected)
		if err != nil {
			log.Printf("load slot %d: %v", s.selected, err)
			return nil
		}
		m.world, m.fresh, m.running, m.inWorld = w, false, true, true
	}
	return nil
}

func (m *Menu) Draw(screen *ebiten.Image) {
	if m.inWorld && m.world != nil {
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
