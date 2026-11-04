package konungmenu

import (
	"fmt"
	"image"
	"image/color"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/konungmap"
	"konung1.5/internal/resource"
)

const (
	houseObjectID  = 88
	shadowObjectID = 89
	warriorModelID = 0
	warriorFrames  = 6
	warriorPalette = 2 // preview only; runtime dye selection not traced
	warriorScale   = 1.65
	scrollSpeed    = 12
	edgeScrollZone = 8
)

// Preview placements in world pixels. The house and warrior are not decoded
// from KONUNG.1's object/entity sections yet; they are hand-placed near the
// road so the terrain render has a focal point.
var (
	housePos   = image.Pt(2900, 2500)
	warriorPos = image.Pt(3330, 2800)
)

type startScene struct {
	m              *konungmap.Map
	tiles          []*ebiten.Image
	tileMarginX    int
	tileMarginY    int
	worldW, worldH int
	house, shadow  *ebiten.Image
	warrior        []*ebiten.Image
	camX, camY     int
	tick           int
}

func loadStartScene(graphPath, objectsPath, mapPath string) (*startScene, error) {
	graphData, err := os.ReadFile(graphPath)
	if err != nil {
		return nil, err
	}
	objectsData, err := os.ReadFile(objectsPath)
	if err != nil {
		return nil, err
	}
	mapData, err := os.ReadFile(mapPath)
	if err != nil {
		return nil, err
	}
	m, err := konungmap.Parse(mapData)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", mapPath, err)
	}
	rawTiles, err := konungmap.LoadTerrainTiles(graphData)
	if err != nil {
		return nil, err
	}
	s := &startScene{m: m, tiles: make([]*ebiten.Image, len(rawTiles))}
	// Only upload tiles the map references; slot 304 is a large non-tile image.
	for r := range m.Cells {
		for c := range m.Cells[r] {
			for _, b := range m.Cells[r][c] {
				t := konungmap.Tile(rawTiles, b)
				if t == nil || s.tiles[b-1] != nil {
					continue
				}
				s.tiles[b-1] = ebiten.NewImageFromImage(t)
				s.tileMarginX = max(s.tileMarginX, t.Bounds().Dx())
				s.tileMarginY = max(s.tileMarginY, t.Bounds().Dy())
			}
		}
	}
	s.worldW, s.worldH = m.WorldSize()
	house, err := resource.DecodeStaticObject(objectsData, graphData, houseObjectID)
	if err != nil {
		return nil, err
	}
	shadow, err := resource.DecodeStaticObject(objectsData, graphData, shadowObjectID)
	if err != nil {
		return nil, err
	}
	warrior, err := resource.DecodeType0Frames(graphData, warriorModelID, warriorFrames, warriorPalette)
	if err != nil {
		return nil, err
	}
	s.house, s.shadow = toEbiten(house), toEbiten(shadow)
	for _, f := range warrior {
		s.warrior = append(s.warrior, toEbiten(f))
	}
	s.centerOn(warriorPos.X, warriorPos.Y)
	return s, nil
}

func (s *startScene) centerOn(x, y int) {
	s.camX, s.camY = x-ScreenWidth/2, y-ScreenHeight/2
	s.clampCamera()
}

func (s *startScene) clampCamera() {
	s.camX = max(0, min(s.camX, s.worldW-ScreenWidth))
	s.camY = max(0, min(s.camY, s.worldH-ScreenHeight))
}

func (s *startScene) Update() {
	s.tick++
	mx, my := ebiten.CursorPosition()
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || ebiten.IsKeyPressed(ebiten.KeyA) || mx < edgeScrollZone {
		s.camX -= scrollSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) || ebiten.IsKeyPressed(ebiten.KeyD) || mx >= ScreenWidth-edgeScrollZone {
		s.camX += scrollSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW) || my < edgeScrollZone {
		s.camY -= scrollSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyS) || my >= ScreenHeight-edgeScrollZone {
		s.camY += scrollSpeed
	}
	s.clampCamera()
}

func (s *startScene) Draw(screen *ebiten.Image) {
	screen.Fill(color.Black)
	// Same order as the game's terrain pass (VA 0x0042562F): rows top to
	// bottom, columns left to right, base layer then overlay layer.
	rowMin := max(0, (s.camY-s.tileMarginY)/konungmap.CellHeight)
	rowMax := min(konungmap.Rows-1, (s.camY+ScreenHeight)/konungmap.CellHeight)
	colMin := max(0, (s.camX-s.tileMarginX)/konungmap.CellWidth)
	colMax := min(konungmap.Cols-1, (s.camX+ScreenWidth)/konungmap.CellWidth)
	for r := rowMin; r <= rowMax; r++ {
		for c := colMin; c <= colMax; c++ {
			x, y := konungmap.CellOrigin(r, c)
			for _, b := range s.m.Cells[r][c] {
				if b != 0 && int(b) <= len(s.tiles) && s.tiles[b-1] != nil {
					drawAt(screen, s.tiles[b-1], x-s.camX, y-s.camY)
				}
			}
		}
	}
	drawAt(screen, s.shadow, housePos.X-s.camX, housePos.Y-s.camY)
	drawAt(screen, s.house, housePos.X-s.camX, housePos.Y-s.camY)
	if len(s.warrior) > 0 {
		frame := s.warrior[(s.tick/8)%len(s.warrior)]
		drawScaledAt(screen, frame, warriorPos.X-s.camX, warriorPos.Y-s.camY, warriorScale, warriorScale)
	}
}

func drawScaledAt(dst, src *ebiten.Image, x, y int, scaleX, scaleY float64) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scaleX, scaleY)
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(src, op)
}

func toEbiten(img *image.RGBA) *ebiten.Image { return ebiten.NewImageFromImage(img) }
