package game

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math/rand/v2"
	"os"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
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

	hero   *walker // nil when the player does not start on this map
	npcs   []*npc  // every other character; those in a valid group area wander
	walk   *mapdata.WalkGrid
	occ    map[mapdata.Cell]bool // cells held or being entered by a character
	follow bool                  // camera follows the hero (toggle with F)
	rng    *rand.Rand
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
		walk:        walk,
		occ:         map[mapdata.Cell]bool{},
		follow:      true,
		rng:         rand.New(rand.NewPCG(1, uint64(mapNumber))),
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
	// Characters and creatures are walkers with lazily loaded idle/walk
	// loops. The player (group 0's first entity) is controllable; NPCs whose
	// group has a wander rectangle roam inside it.
	camX, camY := s.worldW/2, s.worldH/2
	game.HeroModelFromGame(gameNumber, gamestate.PlaceholderHeroPalette)
	uploadSprite := func(sp *resfmt.EntitySprite) (*ebiten.Image, *ebiten.Image) { return upload(sp.Image, sp.Shadow) }
	logErr := func(err error) { log.Printf("%s: %v", gamePath, err) }
	reserve := func(c mapdata.Cell) bool {
		if s.occ[c] {
			return false
		}
		s.occ[c] = true
		return true
	}
	release := func(c mapdata.Cell) { delete(s.occ, c) }
	for _, p := range game.OnMap(mapNumber, walk.Blocked) {
		cell := mapdata.Cell{Row: p.Row, Col: p.Col}
		s.occ[cell] = true
		w := walker{
			cell: cell, dir: mapdata.Direction(p.Variant & 7), phase: p.Index * 5,
			load:    entityLoader(game, p, objectsData, graphData, tables, uploadSprite, logErr),
			reserve: reserve, release: release,
		}
		if p.Group == 0 && p.Index == game.Groups[0].First {
			w.blocked, w.searchCap = s.blocked, pathSearchLimit
			s.hero = &w
			camX, camY = gamestate.WorldPos(p.Row, p.Col)
			continue
		}
		n := &npc{walker: w, rng: s.rng.IntN, wait: s.rng.IntN(npcPauseMin + npcPauseRange)}
		area, wanders := game.Groups[p.Group].WanderArea()
		n.area = area
		n.searchCap = npcSearchCap
		n.blocked = func(c mapdata.Cell) bool { return !wanders || !area.Contains(c.Row, c.Col) || s.blocked(c) }
		s.npcs = append(s.npcs, n)
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

// blocked reports cells a character cannot path through: walk-grid
// obstacles, cells outside the map, and cells other characters hold.
func (s *startScene) blocked(c mapdata.Cell) bool {
	if s.walk.Blocked(c.Row, c.Col) || s.occ[c] {
		return true
	}
	x, y := mapdata.CellCenter(c)
	return x < 0 || y < 0 || x >= s.worldW || y >= s.worldH
}

func (s *startScene) Update() {
	s.tick++
	mx, my := ebiten.CursorPosition()
	if s.hero != nil {
		if inpututil.IsKeyJustPressed(ebiten.KeyF) {
			s.follow = !s.follow
		}
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			s.hero.walkTo(mapdata.CellAt(mx+s.camX, my+s.camY))
		}
		s.hero.update()
	}
	for _, n := range s.npcs {
		n.think()
		n.update()
	}
	if s.hero != nil {
		if s.follow {
			x, y := s.hero.position()
			s.centerOn(int(x), int(y))
			return
		}
	}
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
	drawShadow := func(f *sceneFrame) {
		if f.shadow != nil && s.visible(f.sx, f.sy, f.shadow.Bounds().Dx(), f.shadow.Bounds().Dy()) {
			drawAt(s.shadowLayer, f.shadow, f.sx-s.camX, f.sy-s.camY)
		}
	}
	for i := range s.objects {
		drawShadow(s.objects[i].frame(s.tick))
	}
	// Characters move, so they are depth-sorted each frame and merged into
	// the static objects' order.
	var chars []sceneFrame
	addChar := func(w *walker) {
		if f, ok := w.frame(s.tick); ok && s.visible(f.x-64, f.y-64, f.img.Bounds().Dx()+128, f.img.Bounds().Dy()+128) {
			chars = append(chars, f)
			drawShadow(&chars[len(chars)-1])
		}
	}
	if s.hero != nil {
		addChar(s.hero)
	}
	for _, n := range s.npcs {
		addChar(&n.walker)
	}
	depth := func(f *sceneFrame) int { return f.y + f.img.Bounds().Dy() }
	sort.SliceStable(chars, func(i, j int) bool { return depth(&chars[i]) < depth(&chars[j]) })
	op := &ebiten.DrawImageOptions{}
	op.ColorScale.Scale(0, 0, 0, 0.5)
	screen.DrawImage(s.shadowLayer, op)

	// Static objects are depth-sorted by their bottom edge; characters are
	// merged in where their own bottom edge falls.
	drawFrame := func(f *sceneFrame) {
		if s.visible(f.x, f.y, f.img.Bounds().Dx(), f.img.Bounds().Dy()) {
			drawAt(screen, f.img, f.x-s.camX, f.y-s.camY)
		}
	}
	ci := 0
	for i := range s.objects {
		for ci < len(chars) && depth(&chars[ci]) < s.objects[i].depth {
			drawFrame(&chars[ci])
			ci++
		}
		drawFrame(s.objects[i].frame(s.tick))
	}
	for ; ci < len(chars); ci++ {
		drawFrame(&chars[ci])
	}
	if s.hero != nil {
		state := "off"
		if s.follow {
			state = "on"
		}
		ebitenutil.DebugPrintAt(screen, "Click: move   F: camera follow "+state+"   Esc: menu", 8, ScreenHeight-20)
	}
}

func toEbiten(img *image.RGBA) *ebiten.Image { return ebiten.NewImageFromImage(img) }
