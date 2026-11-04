package game

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

const (
	scrollSpeed    = 12
	edgeScrollZone = 8
	// ticksPerStep is how many 60 Hz updates each animation step lasts. The
	// game's animation tick rate is not traced; 6 gives 10 steps/s, i.e. the
	// usual 12-step idle loop (6 frames, each held 2 steps) in 1.2 s.
	ticksPerStep = 6
)

// sceneFrame is one drawable image with its shadow, in world pixels.
type sceneFrame struct {
	img, shadow *ebiten.Image
	x, y        int // image top-left
	sx, sy      int // shadow top-left
}

// sceneObject is a static map object (one frame) or an entity (its idle
// animation steps), ready to draw.
type sceneObject struct {
	frames []sceneFrame
	phase  int // step offset so neighbours don't animate in lockstep
	depth  int // bottom-edge sort key (from the first step)
}

func (o *sceneObject) frame(tick int) *sceneFrame {
	if len(o.frames) == 1 {
		return &o.frames[0]
	}
	return &o.frames[(tick/ticksPerStep+o.phase)%len(o.frames)]
}

type startScene struct {
	m              *mapdata.Map
	tiles          []*ebiten.Image
	tileMarginX    int
	tileMarginY    int
	worldW, worldH int
	decorations    []mapdata.Decoration
	objects        []sceneObject // objects and entities, in depth order
	shadowLayer    *ebiten.Image
	camX, camY     int
	tick           int
}

// loadStartScene builds the scene for map mapNumber: terrain, decorations
// and static objects from the KONUNG.N file, characters and creatures from
// the GAME.N start state.
func loadStartScene(graphPath, objectsPath, mapPath, gamePath, exePath string, gameNumber, mapNumber int) (*startScene, error) {
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
	m, err := mapdata.Parse(mapData)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", mapPath, err)
	}
	decorations, err := mapdata.ParseDecorations(mapData)
	if err != nil {
		return nil, err
	}
	placements, err := mapdata.ParsePlacements(mapData)
	if err != nil {
		return nil, err
	}
	walk, err := mapdata.ParseWalkGrid(mapData)
	if err != nil {
		return nil, err
	}
	exeData, err := os.ReadFile(exePath)
	if err != nil {
		return nil, err
	}
	tables, err := resfmt.NewExeTables(exeData)
	if err != nil {
		return nil, err
	}
	gameData, err := os.ReadFile(gamePath)
	if err != nil {
		return nil, err
	}
	game, err := gamestate.Parse(gameData)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", gamePath, err)
	}
	rawTiles, err := mapdata.LoadTerrainTiles(graphData)
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

	objs, errs := mapdata.LoadPlacedObjects(placements, objectsData, graphData)
	for _, e := range errs {
		log.Printf("%s: %v", mapPath, e)
	}
	images := map[*image.RGBA]*ebiten.Image{}
	shadows := map[*image.Alpha]*ebiten.Image{}
	upload := func(img *image.RGBA, sh *image.Alpha) (*ebiten.Image, *ebiten.Image) {
		ei, ok := images[img]
		if !ok {
			ei = toEbiten(img)
			images[img] = ei
		}
		if sh == nil {
			return ei, nil
		}
		es, ok := shadows[sh]
		if !ok {
			es = ebiten.NewImageFromImage(sh) // white, alpha = coverage
			shadows[sh] = es
		}
		return ei, es
	}
	for _, sp := range mapdata.ObjectSprites(mapdata.DrawOrder(placements, objs)) {
		img, sh := upload(sp.Image, sp.Shadow)
		s.objects = append(s.objects, sceneObject{
			frames: []sceneFrame{{img: img, shadow: sh, x: sp.X, y: sp.Y, sx: sp.SX, sy: sp.SY}},
			depth:  sp.Depth,
		})
	}
	// Characters and creatures, each with its idle loop. Center the camera on
	// the player (group 0's first entity) when it starts here.
	camX, camY := s.worldW/2, s.worldH/2
	game.HeroModelFromGame(gameNumber, gamestate.PlaceholderHeroPalette)
	for _, p := range game.OnMap(mapNumber, walk.Blocked) {
		anim, err := game.LoadAnimation(p, objectsData, graphData, tables)
		if err != nil {
			log.Printf("%s: %v", gamePath, err)
			continue
		}
		o := sceneObject{phase: p.Index * 5, depth: anim[0].Y + anim[0].Sprite.Image.Bounds().Dy()}
		for _, f := range anim {
			img, sh := upload(f.Sprite.Image, f.Sprite.Shadow)
			o.frames = append(o.frames, sceneFrame{img: img, shadow: sh, x: f.X, y: f.Y, sx: f.SX, sy: f.SY})
		}
		s.objects = append(s.objects, o)
		if p.Group == 0 && p.Index == game.Groups[0].First {
			camX, camY = gamestate.WorldPos(p.Row, p.Col)
		}
	}
	sort.SliceStable(s.objects, func(i, j int) bool { return s.objects[i].depth < s.objects[j].depth })
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
	rowMin := max(0, (s.camY-s.tileMarginY)/mapdata.CellHeight)
	rowMax := min(mapdata.Rows-1, (s.camY+ScreenHeight)/mapdata.CellHeight)
	colMin := max(0, (s.camX-s.tileMarginX)/mapdata.CellWidth)
	colMax := min(mapdata.Cols-1, (s.camX+ScreenWidth)/mapdata.CellWidth)
	for r := rowMin; r <= rowMax; r++ {
		for c := colMin; c <= colMax; c++ {
			x, y := mapdata.CellOrigin(r, c)
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
	for i := range s.objects {
		f := s.objects[i].frame(s.tick)
		if f.shadow != nil && s.visible(f.sx, f.sy, f.shadow.Bounds().Dx(), f.shadow.Bounds().Dy()) {
			drawAt(s.shadowLayer, f.shadow, f.sx-s.camX, f.sy-s.camY)
		}
	}
	op := &ebiten.DrawImageOptions{}
	op.ColorScale.Scale(0, 0, 0, 0.5)
	screen.DrawImage(s.shadowLayer, op)

	// Objects and entities are depth-sorted by their bottom edge.
	for i := range s.objects {
		f := s.objects[i].frame(s.tick)
		if s.visible(f.x, f.y, f.img.Bounds().Dx(), f.img.Bounds().Dy()) {
			drawAt(screen, f.img, f.x-s.camX, f.y-s.camY)
		}
	}
}

func toEbiten(img *image.RGBA) *ebiten.Image { return ebiten.NewImageFromImage(img) }
