package konungmenu

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/konunggame"
	"konung1.5/internal/konungmap"
)

const (
	scrollSpeed    = 12
	edgeScrollZone = 8
)

// sceneObject is a static map object or an entity, ready to draw.
type sceneObject struct {
	img, shadow *ebiten.Image
	x, y        int // image top-left in world pixels
	sx, sy      int // shadow top-left in world pixels
	depth       int // bottom-edge sort key
}

type startScene struct {
	m              *konungmap.Map
	tiles          []*ebiten.Image
	tileMarginX    int
	tileMarginY    int
	worldW, worldH int
	decorations    []konungmap.Decoration
	objects        []sceneObject // objects and entities, in depth order
	shadowLayer    *ebiten.Image
	camX, camY     int
	tick           int
}

// loadStartScene builds the scene for map mapNumber: terrain, decorations
// and static objects from the KONUNG.N file, characters and creatures from
// the GAME.N start state.
func loadStartScene(graphPath, objectsPath, mapPath, gamePath string, mapNumber int) (*startScene, error) {
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
	decorations, err := konungmap.ParseDecorations(mapData)
	if err != nil {
		return nil, err
	}
	placements, err := konungmap.ParsePlacements(mapData)
	if err != nil {
		return nil, err
	}
	walk, err := konungmap.ParseWalkGrid(mapData)
	if err != nil {
		return nil, err
	}
	gameData, err := os.ReadFile(gamePath)
	if err != nil {
		return nil, err
	}
	game, err := konunggame.Parse(gameData)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", gamePath, err)
	}
	rawTiles, err := konungmap.LoadTerrainTiles(graphData)
	if err != nil {
		return nil, err
	}
	s := &startScene{
		m:           m,
		tiles:       make([]*ebiten.Image, len(rawTiles)),
		shadowLayer: ebiten.NewImage(ScreenWidth, ScreenHeight),
	}
	// Only upload tiles the map references; slot 304 is a large non-tile image.
	useTile := func(slot int) {
		if slot < 0 || slot >= len(rawTiles) || rawTiles[slot] == nil || s.tiles[slot] != nil {
			return
		}
		t := rawTiles[slot]
		s.tiles[slot] = ebiten.NewImageFromImage(t)
		s.tileMarginX = max(s.tileMarginX, t.Bounds().Dx())
		s.tileMarginY = max(s.tileMarginY, t.Bounds().Dy())
	}
	for r := range m.Cells {
		for c := range m.Cells[r] {
			for _, b := range m.Cells[r][c] {
				if b != 0 {
					useTile(int(b) - 1)
				}
			}
		}
	}
	for _, d := range decorations {
		if d.Tile < len(rawTiles) && rawTiles[d.Tile] != nil {
			useTile(d.Tile)
			s.decorations = append(s.decorations, d)
		}
	}
	s.worldW, s.worldH = m.WorldSize()

	objs, errs := konungmap.LoadPlacedObjects(placements, objectsData, graphData)
	for _, e := range errs {
		log.Printf("%s: %v", mapPath, e)
	}
	sprites := konungmap.ObjectSprites(konungmap.DrawOrder(placements, objs))
	// Center on the player (group 0's first entity) when it starts here.
	camX, camY := s.worldW/2, s.worldH/2
	for _, p := range game.OnMap(mapNumber, walk.Blocked) {
		es, x, y, sx, sy, depth, err := p.LoadSprite(objectsData, graphData)
		if err != nil {
			log.Printf("%s: %v", gamePath, err)
			continue
		}
		sprites = append(sprites, konungmap.Sprite{Image: es.Image, X: x, Y: y, Shadow: es.Shadow, SX: sx, SY: sy, Depth: depth})
		if p.Group == 0 && p.Index == game.Groups[0].First {
			camX, camY = konunggame.WorldPos(p.Row, p.Col)
		}
	}
	konungmap.SortSprites(sprites)
	images := map[*image.RGBA]*ebiten.Image{}
	shadows := map[*image.Alpha]*ebiten.Image{}
	for _, sp := range sprites {
		img, ok := images[sp.Image]
		if !ok {
			img = toEbiten(sp.Image)
			images[sp.Image] = img
		}
		var sh *ebiten.Image
		if sp.Shadow != nil {
			if sh, ok = shadows[sp.Shadow]; !ok {
				sh = ebiten.NewImageFromImage(sp.Shadow) // white, alpha = coverage
				shadows[sp.Shadow] = sh
			}
		}
		s.objects = append(s.objects, sceneObject{img: img, shadow: sh, x: sp.X, y: sp.Y, sx: sp.SX, sy: sp.SY, depth: sp.Depth})
	}
	s.centerOn(camX, camY)
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

// visible reports whether a w*h image at world (x, y) intersects the viewport.
func (s *startScene) visible(x, y, w, h int) bool {
	return x+w > s.camX && x < s.camX+ScreenWidth && y+h > s.camY && y < s.camY+ScreenHeight
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
	// Flat decorations follow the terrain pass (VA 0x00425A95).
	for _, d := range s.decorations {
		t := s.tiles[d.Tile]
		if s.visible(d.X, d.Y, t.Bounds().Dx(), t.Bounds().Dy()) {
			drawAt(screen, t, d.X-s.camX, d.Y-s.camY)
		}
	}
	// Shadows are unioned, then every covered pixel is halved once (0x0043E5DF).
	s.shadowLayer.Clear()
	for _, o := range s.objects {
		if o.shadow != nil && s.visible(o.sx, o.sy, o.shadow.Bounds().Dx(), o.shadow.Bounds().Dy()) {
			drawAt(s.shadowLayer, o.shadow, o.sx-s.camX, o.sy-s.camY)
		}
	}
	op := &ebiten.DrawImageOptions{}
	op.ColorScale.Scale(0, 0, 0, 0.5)
	screen.DrawImage(s.shadowLayer, op)

	// Objects and entities are depth-sorted by their bottom edge.
	for _, o := range s.objects {
		if s.visible(o.x, o.y, o.img.Bounds().Dx(), o.img.Bounds().Dy()) {
			drawAt(screen, o.img, o.x-s.camX, o.y-s.camY)
		}
	}
}

func toEbiten(img *image.RGBA) *ebiten.Image { return ebiten.NewImageFromImage(img) }
