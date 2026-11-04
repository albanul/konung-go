package konungmenu

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/konungmap"
	"konung1.5/internal/resource"
)

const (
	warriorModelID = 0
	warriorFrames  = 6
	warriorPalette = 2 // preview only; runtime dye selection not traced
	scrollSpeed    = 12
	edgeScrollZone = 8
)

// warriorPos is a hand-picked world position inside the KONUNG.1 palisade.
// Characters come from a separate entity path (VA 0x00414078) that is not
// decoded yet, so the warrior is still a preview.
var warriorPos = image.Pt(2645, 1525)

// sceneObject is a static map object ready to draw.
type sceneObject struct {
	img, shadow *ebiten.Image
	x, y        int // image top-left in world pixels
	sx, sy      int // shadow top-left in world pixels
	depth       int // bottom-edge sort key (konungmap.SortedPlacement.DepthKey)
}

type startScene struct {
	m              *konungmap.Map
	tiles          []*ebiten.Image
	tileMarginX    int
	tileMarginY    int
	worldW, worldH int
	decorations    []konungmap.Decoration
	objects        []sceneObject // in depth order
	shadowLayer    *ebiten.Image
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
	decorations, err := konungmap.ParseDecorations(mapData)
	if err != nil {
		return nil, err
	}
	placements, err := konungmap.ParsePlacements(mapData)
	if err != nil {
		return nil, err
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
	uploaded := map[*resource.MapObject][2]*ebiten.Image{}
	for _, sp := range konungmap.DrawOrder(placements, objs) {
		imgs, ok := uploaded[sp.Object]
		if !ok {
			imgs[0] = toEbiten(sp.Object.Image)
			if sp.Object.Shadow != nil {
				imgs[1] = ebiten.NewImageFromImage(sp.Object.Shadow) // white, alpha = coverage
			}
			uploaded[sp.Object] = imgs
		}
		px, py := sp.Placement.Pos()
		s.objects = append(s.objects, sceneObject{
			img: imgs[0], shadow: imgs[1],
			x: sp.X, y: sp.Y,
			sx: px + sp.Object.ShadowDX, sy: py + sp.Object.ShadowDY,
			depth: sp.DepthKey(),
		})
	}

	warrior, err := resource.DecodeType0Frames(graphData, warriorModelID, warriorFrames, warriorPalette)
	if err != nil {
		return nil, err
	}
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

	// Objects and the warrior are depth-sorted by their bottom edge.
	var frame *ebiten.Image
	warriorDepth := 1 << 30
	if len(s.warrior) > 0 {
		frame = s.warrior[(s.tick/8)%len(s.warrior)]
		warriorDepth = warriorPos.Y + frame.Bounds().Dy()
	}
	for _, o := range s.objects {
		if frame != nil && o.depth > warriorDepth {
			drawAt(screen, frame, warriorPos.X-s.camX, warriorPos.Y-s.camY)
			frame = nil
		}
		if s.visible(o.x, o.y, o.img.Bounds().Dx(), o.img.Bounds().Dy()) {
			drawAt(screen, o.img, o.x-s.camX, o.y-s.camY)
		}
	}
	if frame != nil {
		drawAt(screen, frame, warriorPos.X-s.camX, warriorPos.Y-s.camY)
	}
}

func toEbiten(img *image.RGBA) *ebiten.Image { return ebiten.NewImageFromImage(img) }
