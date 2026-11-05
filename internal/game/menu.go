package game

import (
	"fmt"

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

	m := &Menu{
		background: ebiten.NewImageFromImage(frames[0]),
		sword:      ebiten.NewImageFromImage(swordFrame),
		world:      world,
		newWorld:   newWorld,
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
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if m.inWorld {
			m.inWorld = false
			return nil
		}
		return ebiten.Termination
	}
	if m.inWorld {
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
			m.inWorld = false
		}
		m.world.Update()
		if m.world.gameOver() {
			// 0x004376A8: the hero died; the game plays GAMEOVER.FLM (not in
			// the installed files) and leaves the game loop for the menu.
			m.inWorld = false
			w, err := m.newWorld()
			if err != nil {
				return err
			}
			m.world = w
		}
		return nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		m.selected = (m.selected + menuItems - 1) % menuItems
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		m.selected = (m.selected + 1) % menuItems
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) && m.selected == menuItems-1 {
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) && m.selected == 3 {
		m.inWorld = true
	}

	mx, my := ebiten.CursorPosition()
	for i, r := range m.buttonRects {
		if mx >= r.x && mx < r.x+r.w && my >= r.y && my < r.y+r.h {
			m.selected = i
			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && i == menuItems-1 {
				return ebiten.Termination
			}
			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && i == 3 {
				m.inWorld = true
			}
		}
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
	screen.DrawImage(m.background, nil)
	for i, r := range m.buttonRects {
		button := m.buttons[i][0]
		if i == m.selected {
			button = m.buttons[i][1]
		}
		drawAt(screen, button, r.x, r.y)
	}
	// Keep the blade tip at the native cursor position, matching the in-game pointer.
	mx, my := ebiten.CursorPosition()
	drawAt(screen, m.sword, mx-1, my-1)
}

func drawAt(dst, src *ebiten.Image, x, y int) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(src, op)
}

func (m *Menu) Layout(_, _ int) (int, int) { return ScreenWidth, ScreenHeight }
