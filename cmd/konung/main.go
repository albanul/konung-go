package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"konung1.5/internal/resource"
)

const (
	screenWidth  = 800
	screenHeight = 600
	menuItems    = 6
)

type menu struct {
	background  *ebiten.Image
	sword       *ebiten.Image
	buttons     [menuItems][2]*ebiten.Image
	selected    int
	buttonRects [menuItems]rect
}

type rect struct{ x, y, w, h int }

func loadMenu(resourcePath string) (*menu, error) {
	frames, err := resource.LoadType5File(resourcePath)
	if err != nil {
		return nil, fmt.Errorf("load menu resource %q: %w", resourcePath, err)
	}
	if len(frames) < 3+menuItems*2 {
		return nil, fmt.Errorf("%s has %d frames; need at least %d", resourcePath, len(frames), 3+menuItems*2)
	}

	m := &menu{
		background: ebiten.NewImageFromImage(frames[0]),
		sword:      ebiten.NewImageFromImage(frames[2]),
	}
	for i := 0; i < menuItems; i++ {
		m.buttons[i][0] = ebiten.NewImageFromImage(frames[3+i*2])
		m.buttons[i][1] = ebiten.NewImageFromImage(frames[4+i*2])
		w, h := frames[3+i*2].Bounds().Dx(), frames[3+i*2].Bounds().Dy()
		m.buttonRects[i] = rect{x: 408 - w/2, y: 185 + i*46, w: w, h: h}
	}
	return m, nil
}

func (m *menu) Update() error {
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

func (m *menu) Draw(screen *ebiten.Image) {
	screen.DrawImage(m.background, nil)
	for i, r := range m.buttonRects {
		button := m.buttons[i][0]
		if i == m.selected {
			button = m.buttons[i][1]
		}
		drawAt(screen, button, r.x, r.y)
		if i == m.selected {
			w, h := m.sword.Size()
			// The 13x13 sword comes from MENU640R.RES frame 2.
			drawAt(screen, m.sword, r.x-w-10, r.y+(r.h-h)/2)
		}
	}
}

func drawAt(dst, src *ebiten.Image, x, y int) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(src, op)
}

func (m *menu) Layout(_, _ int) (int, int) { return screenWidth, screenHeight }

func defaultMenuResource() (string, error) {
	const resourceRelativePath = "assets/Konung/MENU800R.RES"

	var roots []string
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
	}
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		roots = append(roots, filepath.Dir(sourceFile))
	}

	seen := make(map[string]bool)
	for _, root := range roots {
		for dir := root; ; dir = filepath.Dir(dir) {
			if !seen[dir] {
				seen[dir] = true
				candidate := filepath.Join(dir, resourceRelativePath)
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					return candidate, nil
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}

	return "", fmt.Errorf("could not find %s from working directory or source location; run from the repository or pass the resource path as an argument", resourceRelativePath)
}

func main() {
	resourcePath, err := defaultMenuResource()
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 {
		resourcePath = os.Args[1]
	}
	g, err := loadMenu(resourcePath)
	if err != nil {
		log.Fatal(err)
	}

	ebiten.SetFullscreen(true)
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Князь: Легенды лесной страны")
	if err := ebiten.RunGame(g); err != nil && err != ebiten.Termination {
		log.Fatal(err)
	}
}
