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
	npcs   []*npc  // every other character (see npcMode)
	aiNext int     // round-robin start of the village AI's path budget
	walk   *mapdata.WalkGrid
	occ    map[mapdata.Cell]bool // cells held or being entered by a character
	follow bool                  // camera follows the hero (toggle with F)
	rng    *rand.Rand

	hud   *hud
	party []gamestate.Entity // group 0's entities (copies; [0] = hero) shown in the portrait grid

	game    *gamestate.Game
	tables  *resfmt.ExeTables
	font    *gameFont // small status-line font (resfmt.FontSmallVA)
	inv     *charWindow
	hovered int // entity under the cursor in the last frame, -1 = none
}

// loadStartScene builds the scene for map mapNumber: terrain, decorations
// and static objects from the KONUNG.N file, characters and creatures from
// the GAME.N start state.
func loadStartScene(graphPath, objectsPath, mapPath, gamePath, exePath, intrPath string, gameNumber, mapNumber int) (*startScene, error) {
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
	intrData, err := os.ReadFile(intrPath)
	if err != nil {
		return nil, err
	}
	hud, err := newHUD(intrData, graphData)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", intrPath, err)
	}
	smallFont, err := tables.Font(resfmt.FontSmallVA)
	if err != nil {
		return nil, err
	}
	largeFont, err := tables.Font(resfmt.FontLargeVA)
	if err != nil {
		return nil, err
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
		hud:         hud,
		game:        game,
		tables:      tables,
		font:        newGameFont(smallFont),
		hovered:     -1,
		occ:         map[mapdata.Cell]bool{},
		follow:      true,
		rng:         rand.New(rand.NewPCG(1, uint64(mapNumber))),
	}
	s.inv = newCharWindow(hud, tables, newGameFont(largeFont), s.font)
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
	// loops. The player (group 0's first entity) is controllable; villagers
	// run the ported village AI, creatures a rectangle-roaming stand-in (see
	// npc).
	camX, camY := s.worldW/2, s.worldH/2
	game.HeroModelFromGame(gameNumber, gamestate.PlaceholderHeroPalette)
	if g0 := game.Groups[0]; g0.Map == mapNumber {
		for k := 0; k < g0.Count; k++ {
			s.party = append(s.party, game.Entities[g0.First+k])
		}
	}
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
		cur := func() gamestate.Entity { return p.Entity }
		hero := p.Group == 0 && p.Index == game.Groups[0].First && len(s.party) > 0 && s.party[0].Index == p.Index
		if hero {
			cur = func() gamestate.Entity { return s.party[0] } // stance and weapon change at runtime
		}
		w := walker{
			entity: p.Index, cell: cell, dir: mapdata.Direction(p.Variant & 7), phase: p.Index * 5,
			look:    lookOf(p.Entity),
			load:    entityLoader(game, p, cur, objectsData, graphData, tables, uploadSprite, logErr),
			reserve: reserve, release: release,
		}
		if hero {
			w.blocked, w.searchCap = s.blocked, pathSearchLimit
			s.hero = &w
			camX, camY = gamestate.WorldPos(p.Row, p.Col)
			continue
		}
		n := &npc{walker: w, rng: s.rng.IntN}
		n.searchCap = npcSearchCap
		gr := game.Groups[p.Group]
		area, hasArea := gr.WanderArea()
		n.area = area
		switch {
		case gr.VillageGroup() && hasArea:
			n.mode = npcVillager
			n.post, n.posted = game.PostOf(p.Entity)
			n.moveAt = gamestate.VillageMoveThreshold(p.Entity)
			n.blocked = s.blocked
		case hasArea:
			n.mode = npcRoamer
			n.wait = s.rng.IntN(npcPauseMin + npcPauseRange)
			n.blocked = func(c mapdata.Cell) bool { return !area.Contains(c.Row, c.Col) || s.blocked(c) }
		default:
			n.blocked = s.blocked
		}
		s.npcs = append(s.npcs, n)
	}
	sort.SliceStable(s.objects, func(i, j int) bool { return s.objects[i].depth < s.objects[j].depth })
	s.centerOn(camX, camY)
	return s, nil
}

// updateNPCs runs the AI and advances every NPC by one update. The village AI
// runs once per game tick (one animation step, walkTicksPerStep updates),
// like 0x00421DA4 in the game loop; at most VillagePathBudget villagers may
// start a walk per tick, served round-robin (0x004116CC).
func (s *startScene) updateNPCs() {
	if len(s.npcs) > 0 && s.tick%walkTicksPerStep == 0 {
		budget := gamestate.VillagePathBudget
		for i := range s.npcs {
			k := (s.aiNext + i) % len(s.npcs)
			if n := s.npcs[k]; n.mode == npcVillager {
				if used := n.villageTick(budget); used > 0 {
					budget -= used
					s.aiNext = k + 1
				}
			}
		}
	}
	for _, n := range s.npcs {
		if n.mode == npcRoamer {
			n.roamTick()
		}
		n.update()
	}
}

// updateControls handles the hero's HUD buttons and keys. The selection is
// the hero alone (the game's selection list 0x0083B93C holds up to 10 party
// members). Space and the HUD stance button call gamestate.ToggleStance
// (0x0042179C); keys 1/2/3 select the blade, the axe/club and the bow, and
// the HUD weapon button cycles weapons (gamestate.SelectWeapon, 0x0042192C);
// I and the HUD bag button toggle the character window (0x00424610), whose
// bag strip scrolls with hotspots 120/121. Each action presses its button
// like the game (0x004290AC). It reports whether a click was used.
func (s *startScene) updateControls(mx, my int) (clicked bool) {
	if len(s.party) == 0 || s.party[0].Index != s.hero.entity {
		return false
	}
	hero := &s.party[0]
	button, spot := -1, 0
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		spot = s.tables.HUDHotspot800(mx, my)
		button = buttonAt(s.tables, mx, my)
		clicked = !inView(mx, my) || (s.inv.open && (spot == hotspotBagLeft || spot == hotspotBagRight))
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || button == hudStanceButton {
		gamestate.ToggleStance([]*gamestate.Entity{hero})
		s.hud.press(hudStanceButton, gamestate.HUDButtonSlots(*hero)[hudStanceButton])
	}
	weapon := func(mode int) {
		before := gamestate.HUDButtonSlots(*hero)[hudWeaponButton]
		gamestate.SelectWeapon(hero, mode)
		s.hud.press(hudWeaponButton, before) // 0x00421B9D presses the image shown so far
	}
	for key, mode := range map[ebiten.Key]int{
		ebiten.Key1: gamestate.WeaponBlade, ebiten.Key2: gamestate.WeaponAxe, ebiten.Key3: gamestate.WeaponBow,
	} {
		if inpututil.IsKeyJustPressed(key) {
			weapon(mode)
		}
	}
	if button == hudWeaponButton {
		weapon(gamestate.WeaponCycle)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyI) || button == hudBagButton {
		s.inv.toggle()
		s.hud.press(hudBagButton, hudBagImage)
	}
	if s.inv.open {
		switch spot {
		case hotspotBagLeft:
			s.inv.scrollBy(*hero, -1, s.tick)
		case hotspotBagRight:
			s.inv.scrollBy(*hero, +1, s.tick)
		}
	}
	s.hero.look = lookOf(*hero)
	return clicked
}

// centerOn puts world point (x, y) at the centre of the game view (the part
// of the screen the HUD frame leaves open).
func (s *startScene) centerOn(x, y int) {
	s.camX = x - (viewLeft+viewRight+1)/2
	s.camY = y - (viewTop+viewBottom+1)/2
	s.clampCamera()
}

// clampCamera keeps the map filling the game view: its left edge may reach
// the HUD's inner edge and its bottom edge the bottom bar.
func (s *startScene) clampCamera() {
	s.camX = max(-viewLeft, min(s.camX, s.worldW-ScreenWidth))
	s.camY = max(-viewTop, min(s.camY, s.worldH-viewBottom-1))
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
	s.hud.now = s.tick
	mx, my := ebiten.CursorPosition()
	if s.hero != nil {
		if inpututil.IsKeyJustPressed(ebiten.KeyF) {
			s.follow = !s.follow
		}
		clicked := s.updateControls(mx, my)
		// While the character window covers the view, clicks don't move the hero.
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && inView(mx, my) && !clicked && !s.inv.open {
			s.hero.walkTo(mapdata.CellAt(mx+s.camX, my+s.camY))
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyH) && len(s.party) > 0 {
			s.party[0] = debugDamage(s.party[0])
		}
		s.hero.update()
	}
	s.updateNPCs()
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
	var owners []int // entity index per chars entry
	addChar := func(w *walker) {
		if f, ok := w.frame(s.tick); ok && s.visible(f.x-64, f.y-64, f.img.Bounds().Dx()+128, f.img.Bounds().Dy()+128) {
			chars = append(chars, f)
			owners = append(owners, w.entity)
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
	order := make([]int, len(chars))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return depth(&chars[order[i]]) < depth(&chars[order[j]]) })
	sorted := make([]sceneFrame, len(chars))
	sortedOwners := make([]int, len(chars))
	for i, k := range order {
		sorted[i], sortedOwners[i] = chars[k], owners[k]
	}
	chars, owners = sorted, sortedOwners
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
	s.hovered = s.hitTest(chars, owners)
	if s.inv.open {
		s.hovered = -1 // the window covers the characters
	}
	if s.hero != nil && !s.inv.open {
		state := "off"
		if s.follow {
			state = "on"
		}
		ebitenutil.DebugPrintAt(screen, "Click: move   Space: stance   1/2/3: blade/axe/bow   I: inventory   F: follow "+state+"   H: test damage   Esc: menu", viewLeft+8, viewBottom-20)
	}
	s.hud.Draw(screen, s.party, func(k int) bool { return k == 0 })
	if s.inv.open && len(s.party) > 0 {
		s.inv.draw(screen, s.hud, s.game, s.tables, s.party[0])
	}
	if s.hovered >= 0 {
		// The status line in the bottom bar (0x0043195C), small font.
		s.font.drawWrapped(screen, s.game.StatusLine(s.entity(s.hovered), s.tables),
			viewLeft+10, viewBottom+5, viewRight-viewLeft-15)
	}
}

// hitTest returns the entity whose drawn sprite has an opaque pixel under
// the cursor, like the entity draw pass (0x0043F89C after each body blit,
// result in 0x00A3E6A0): characters are tested in draw order, so the last
// one drawn (front-most) wins; static objects don't block. -1 = none.
func (s *startScene) hitTest(chars []sceneFrame, owners []int) int {
	mx, my := ebiten.CursorPosition()
	if !inView(mx, my) {
		return -1
	}
	for i := len(chars) - 1; i >= 0; i-- {
		f := &chars[i]
		b := f.img.Bounds()
		x, y := mx+s.camX-f.x+b.Min.X, my+s.camY-f.y+b.Min.Y
		if image.Pt(x, y).In(b) {
			if _, _, _, a := f.img.At(x, y).RGBA(); a != 0 {
				return owners[i]
			}
		}
	}
	return -1
}

// entity returns the current state of an entity: the HUD copy for party
// members (it carries HP changes), otherwise the GAME.N record.
func (s *startScene) entity(i int) gamestate.Entity {
	for _, e := range s.party {
		if e.Index == i {
			return e
		}
	}
	return s.game.Entities[i]
}

// debugDamage is a test hook until combat exists: each press takes 400 HP
// (one portrait state); after death the hero is healed to full.
func debugDamage(e gamestate.Entity) gamestate.Entity {
	switch {
	case e.Dead():
		e.Flags4 &^= 0x80
		e.HP = gamestate.HPFull
	case e.HP <= 400:
		e.HP = 0
		e.Flags4 |= 0x80
	default:
		e.HP -= 400
	}
	return e
}

func toEbiten(img *image.RGBA) *ebiten.Image { return ebiten.NewImageFromImage(img) }
