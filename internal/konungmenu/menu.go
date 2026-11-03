package konungmenu

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"konung1.5/internal/resource"
)

const (
	ScreenWidth            = 800
	ScreenHeight           = 600
	menuItems              = 6
	swordCursorSpriteIndex = 4
)

type Menu struct {
	background  *ebiten.Image
	sword       *ebiten.Image
	buttons     [menuItems][2]*ebiten.Image
	selected    int
	buttonRects [menuItems]rect
}

type rect struct{ x, y, w, h int }

func New(resourcePath, graphPath string) (*Menu, error) {
	frames, err := resource.LoadType5File(resourcePath)
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

	m := &Menu{
		background: ebiten.NewImageFromImage(frames[0]),
		sword:      ebiten.NewImageFromImage(swordFrame),
	}
	for i := 0; i < menuItems; i++ {
		m.buttons[i][0] = ebiten.NewImageFromImage(frames[3+i*2])
		m.buttons[i][1] = ebiten.NewImageFromImage(frames[4+i*2])
		w, h := frames[3+i*2].Bounds().Dx(), frames[3+i*2].Bounds().Dy()
		m.buttonRects[i] = rect{x: 408 - w/2, y: 185 + i*46, w: w, h: h}
	}
	return m, nil
}

func (m *Menu) Update() error {
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		return ebiten.Termination
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

	mx, my := ebiten.CursorPosition()
	for i, r := range m.buttonRects {
		if mx >= r.x && mx < r.x+r.w && my >= r.y && my < r.y+r.h {
			m.selected = i
			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && i == menuItems-1 {
				return ebiten.Termination
			}
		}
	}
	return nil
}

func (m *Menu) Draw(screen *ebiten.Image) {
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
