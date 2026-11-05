package game

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
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
	assets         *sceneAssets
	m              *mapdata.Map
	tiles          []*ebiten.Image
	tileMarginX    int
	tileMarginY    int
	worldW, worldH int
	decorations    []mapdata.Decoration
	objects        []sceneObject // objects and entities, in depth order
	shadowLayer    *ebiten.Image
	camX, camY     int
	tick           int // game ticks (one per step)
	frame          int // drawn frames (Update calls)
	speed          int // 0x00A3E764

	hero   *walker // nil when the player does not start on this map
	npcs   []*npc  // every other character (see npcMode)
	aiNext int     // round-robin start of the village AI's path budget
	walk   *mapdata.WalkGrid
	occ    map[mapdata.Cell]bool // cells held or being entered by a character
	follow bool                  // camera follows the hero (toggle with F)
	rng    *rand.Rand

	hud   *hud
	party []gamestate.Entity // group 0's entities (copies; [0] = hero) shown in the portrait grid

	game   *gamestate.Game
	tables *resfmt.ExeTables
	font   *gameFont // small status-line font (resfmt.FontSmallVA)
	inv    *charWindow
	hand   gamestate.Hand // item being moved in the character window
	msg    []byte         // status-line message (0x0042F9B8) and when it ends
	msgEnd int

	mapNumber  int
	pileAt     map[mapdata.Cell]int // active piles by cell (index into game.Piles)
	pickIntent bool                 // entity +3 bit 0x40: pick up where the walk ends
	hovered    int                  // entity under the cursor in the last frame, -1 = none

	byEntity   map[int]*npc // non-party characters on this map by entity index
	target     int          // hero's attack target (entity +6), -1 = none; see combat.go
	targetCell mapdata.Cell // the target's cell when last checked (+0x0C/+0x0E)
	deadAt     int          // frame the hero was first seen dead (gameOver)
	aiBudget   int          // creature path searches left this game tick
	melee      bool         // hero's melee intent (+2 bit 0x01): keeps finding targets after a kill
	hideHelp   bool         // F1 hides the key help (help.go)
}

// sceneAssets is what stays loaded across map changes: resource files, exe
// tables, the HUD, fonts and the terrain tiles (uploaded on first use).
type sceneAssets struct {
	graph, objects []byte
	mapDir         string // directory holding KONUNG.N
	gamePath       string // for log messages
	tables         *resfmt.ExeTables
	hud            *hud
	font           *gameFont // small status-line font (resfmt.FontSmallVA)
	largeFont      *gameFont
	rawTiles       []*image.RGBA
	tiles          []*ebiten.Image
}

// loadStartScene loads the shared resources and the GAME.N start state and
// builds the scene for map mapNumber.
func loadStartScene(graphPath, objectsPath, mapPath, gamePath, exePath, intrPath string, gameNumber, mapNumber int) (*startScene, error) {
	a := &sceneAssets{mapDir: filepath.Dir(mapPath), gamePath: gamePath}
	var err error
	if a.graph, err = os.ReadFile(graphPath); err != nil {
		return nil, err
	}
	if a.objects, err = os.ReadFile(objectsPath); err != nil {
		return nil, err
	}
	exeData, err := os.ReadFile(exePath)
	if err != nil {
		return nil, err
	}
	if a.tables, err = resfmt.NewExeTables(exeData); err != nil {
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
	if a.hud, err = newHUD(intrData, a.graph); err != nil {
		return nil, fmt.Errorf("%s: %w", intrPath, err)
	}
	smallFont, err := a.tables.Font(resfmt.FontSmallVA)
	if err != nil {
		return nil, err
	}
	largeFont, err := a.tables.Font(resfmt.FontLargeVA)
	if err != nil {
		return nil, err
	}
	a.font, a.largeFont = newGameFont(smallFont), newGameFont(largeFont)
	if a.rawTiles, err = mapdata.LoadTerrainTiles(a.graph); err != nil {
		return nil, err
	}
	a.tiles = make([]*ebiten.Image, len(a.rawTiles))
	game.HeroModelFromGame(gameNumber, gamestate.PlaceholderHeroPalette)
	return a.buildScene(game, mapNumber, nil)
}

// changeMap takes map exit x (0x00421CCD, then the main loop's map change at
// 0x004377C6): the scene's characters go back into the game (storeWorld),
// the party moves to the exit's target and the scene is rebuilt for that
// map. If the target fails to load, the game state is restored.
func (s *startScene) changeMap(x gamestate.MapExit) {
	group := s.game.Groups[0]
	entities := append([]gamestate.Entity(nil), s.game.Entities...)
	s.storeWorld()
	s.game.TakeExit(x)
	ns, err := s.assets.buildScene(s.game, x.To, s)
	if err != nil {
		log.Printf("map exit %d to KONUNG.%d: %v", x.Index, x.To, err)
		s.game.Groups[0] = group
		s.game.Entities = entities
		return
	}
	*s = *ns
}

// storeWorld writes the scene's runtime state back into s.game: the party
// (stance, weapon, inventory) and every other character's cell and facing.
// The game keeps all characters in one global array (0x007AB378) and runs
// the AI only for the current map, so a map looks as it was left on the
// next visit; the map loader (0x0043C612) puts them back on their stored
// cells, idle.
func (s *startScene) storeWorld() {
	for _, e := range s.party {
		s.game.Entities[e.Index] = e
	}
	for _, n := range s.npcs {
		e := &s.game.Entities[n.entity]
		e.Row, e.Col, e.Variant = n.cell.Row, n.cell.Col, int(n.dir)
	}
}

// buildScene builds the scene for map mapNumber: terrain, decorations and
// static objects from the KONUNG.N file, characters and creatures from game.
// prev, when set, is the scene being left: the clock, speed, camera mode,
// held item and windows carry over.
func (a *sceneAssets) buildScene(game *gamestate.Game, mapNumber int, prev *startScene) (*startScene, error) {
	mapPath := filepath.Join(a.mapDir, fmt.Sprintf("KONUNG.%d", mapNumber))
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
	graphData, objectsData, tables, gamePath := a.graph, a.objects, a.tables, a.gamePath
	rawTiles := a.rawTiles
	s := &startScene{
		assets:      a,
		m:           m,
		tiles:       a.tiles,
		shadowLayer: ebiten.NewImage(ScreenWidth, ScreenHeight),
		walk:        walk,
		hud:         a.hud,
		game:        game,
		tables:      tables,
		font:        a.font,
		hovered:     -1,
		target:      -1,
		byEntity:    map[int]*npc{},
		occ:         map[mapdata.Cell]bool{},
		hand:        gamestate.NewHand(),
		mapNumber:   mapNumber,
		pileAt:      map[mapdata.Cell]int{},
		follow:      true,
		rng:         rand.New(rand.NewPCG(1, uint64(mapNumber))),
	}
	if prev != nil {
		s.inv, s.hand, s.follow, s.speed = prev.inv, prev.hand, prev.follow, prev.speed
		s.tick, s.frame, s.msg, s.msgEnd = prev.tick, prev.frame, prev.msg, prev.msgEnd
		s.shadowLayer = prev.shadowLayer
	} else {
		s.inv = newCharWindow(a.hud, tables, a.largeFont, a.font)
	}
	// Only upload tiles the map references; slot 304 is a large non-tile image.
	useTile := func(slot int) {
		if slot < 0 || slot >= len(rawTiles) || rawTiles[slot] == nil {
			return
		}
		t := rawTiles[slot]
		if s.tiles[slot] == nil {
			s.tiles[slot] = ebiten.NewImageFromImage(t)
		}
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
	// run the ported village AI, creatures stand still while calm (see npc).
	camX, camY := s.worldW/2, s.worldH/2
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
		if p.Group != 0 && p.Dead() {
			continue // the leave cleanup (0x00436AD0) removes the dead
		}
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
		n.target = -1
		switch {
		case gr.CreatureGroup():
			n.mode, n.group = npcCreature, p.Group
			n.blocked = s.blocked
			game.Groups[p.Group].Alarm = gamestate.AlarmCalm // 0x0043C66D
			if p.Type%6 == gamestate.ActShoot {
				n.act = gamestate.ActShoot // dormant (kinds 0x46, 0x4C, 0x55)
			}
			s.setStopAt(n)
		case gr.VillageGroup() && hasArea:
			n.mode = npcVillager
			n.post, n.posted = game.PostOf(p.Entity)
			n.moveAt = gamestate.VillageMoveThreshold(p.Entity)
			n.blocked = s.blocked
		default:
			n.blocked = s.blocked
		}
		s.npcs = append(s.npcs, n)
		s.byEntity[p.Index] = n
	}
	sort.SliceStable(s.objects, func(i, j int) bool { return s.objects[i].depth < s.objects[j].depth })
	s.activatePiles()
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
		if n.act == 0 {
			n.update()
		}
	}
}

// updateControls handles the hero's keys and clicks. The selection is the
// hero alone (the game's selection list 0x0083B93C holds up to 10 party
// members).
//
//   - Space / HUD stance button: gamestate.ToggleStance (0x0042179C); 1/2/3 and
//     the HUD weapon button: gamestate.SelectWeapon (0x0042192C).
//   - I / HUD bag button: character window (0x00424610); ~: the bag strip
//     alone (0x004371D2). The bag arrows are hotspots 120/121.
//   - Left click (0x00422933): HUD hotspots, window boxes and bag cells move
//     items; on the map it walks there without picking up (0x004229F7,
//     +3 &= 0xBF), or drops a held item on the ground at the hero's cell
//     (0x004229BC → 0x004138C8).
//   - Right click (0x004238EE): returns a held item; on a bag cell uses the
//     item (gamestate.UseBagItem); on the map walks there and picks up on
//     arrival (0x004239E3).
//
// The stance, weapon and window routines first return a held item
// (0x00429194(1) at 0x004217BA, 0x00421948, 0x00424635); any click ends the
// shovel's dig mode first.
func (s *startScene) updateControls(mx, my int) {
	if len(s.party) == 0 || s.party[0].Index != s.hero.entity || s.party[0].Dead() || s.hero.act == gamestate.ActDeath {
		return
	}
	hero := &s.party[0]
	left := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	right := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
	button, spot := -1, 0
	if left || right {
		spot = s.tables.HUDHotspot800(mx, my)
	}
	if left {
		button = buttonAt(s.tables, mx, my)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || button == hudStanceButton {
		s.hand.Return(hero)
		gamestate.ToggleStance([]*gamestate.Entity{hero})
		if !hero.InStance() {
			s.endFight() // the swing needs the stance's animation set
		}
		s.hud.press(hudStanceButton, gamestate.HUDButtonSlots(*hero)[hudStanceButton])
	}
	weapon := func(mode int) {
		s.hand.Return(hero)
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
	if inpututil.IsKeyJustPressed(ebiten.KeyF) || button == hudGatherButton {
		s.gatherOrder(hero)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyA) || button == hudAttackButton {
		s.attackOrder(hero)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyI) || button == hudBagButton {
		s.hand.Return(hero)
		s.inv.toggle()
		s.hud.press(hudBagButton, hudBagImage)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackquote) && !s.inv.open {
		s.inv.toggleQuick()
	}
	if left || right {
		s.click(hero, mx, my, spot, right)
	}
	s.hero.look = lookOf(*hero)
}

// click routes one mouse click (button already handled above for the HUD).
func (s *startScene) click(hero *gamestate.Entity, mx, my, spot int, right bool) {
	if s.hand.Digging() {
		s.hand.Cursor = gamestate.CursorPointer
		if s.inv.strip.hit(mx, my) && s.inv.showStrip() && !right {
			return // 0x00423412: a bag click only ends dig mode
		}
	}
	if right && s.hand.Holding() {
		s.hand.Return(hero) // 0x00423919
		return
	}
	msg := 0
	switch {
	case !inView(mx, my):
		return // HUD: buttons were handled by updateControls
	case s.inv.showStrip() && (spot == hotspotBagLeft || spot == hotspotBagRight):
		if !right {
			dir := -1
			if spot == hotspotBagRight {
				dir = 1
			}
			s.inv.scrollBy(*hero, dir, s.frame)
		}
	case s.inv.showStrip() && s.inv.strip.hit(mx, my):
		i := (mx-s.inv.strip.x0)/s.inv.strip.cellW + s.inv.scroll
		if right {
			msg = s.game.UseBagItem(&s.hand, hero, s.tables, i)
			if s.hand.Digging() && s.inv.open {
				s.inv.open = false // 0x00415504 closes the window; the strip stays
				s.inv.quick = true
			}
		} else {
			msg = s.game.ClickBag(&s.hand, hero, s.tables, i)
		}
	case s.inv.open:
		if !right {
			msg = s.windowClick(hero, spot)
		}
	case !right && s.hand.Holding():
		s.dropHeld(hero)
	case right && hero.InStance() && s.hovered >= 0 && s.hovered != hero.Index && !s.entity(s.hovered).Dead():
		s.startFight(s.hovered) // 0x00423C8B: attack the character under the cursor
	default:
		s.endFight()
		s.hero.walkTo(mapdata.CellAt(mx+s.camX, my+s.camY))
		s.pickIntent = right
	}
	if msg != 0 {
		s.showMessage(s.tables.CString(msg))
	}
}

// dropHeld puts the held item on the ground at the hero's cell (0x004138C8).
// The game ignores a failure (full pile, 200 active piles): the item is lost.
func (s *startScene) dropHeld(hero *gamestate.Entity) {
	item := s.hand.Item
	s.hand.Cursor = gamestate.CursorPointer
	at, on := s.pileAt[s.hero.cell]
	if !on {
		if len(s.pileAt) >= gamestate.MaxActivePiles {
			return
		}
		at = -1
	}
	if i, ok := s.game.DropItem(at, s.mapNumber, s.hero.cell.Row, s.hero.cell.Col, item); ok {
		s.pileAt[s.hero.cell] = i
	}
}

// drawSelection draws the selection ellipse under the hero (the port's
// selection list), like 0x00426146: before the ground items, flat, coloured
// by HP and following the walk animation.
func (s *startScene) drawSelection(dst *ebiten.Image) {
	if s.hero == nil || len(s.party) == 0 {
		return
	}
	img := s.hud.slot(gamestate.SelectionMarker(s.party[0]))
	if img == nil {
		return
	}
	step := min(s.hero.stepT/walkTicksPerStep, s.hero.walkSteps()-1)
	dx, dy := gamestate.SelectionOffset(int(s.hero.dir), step, s.hero.walking)
	x, y := mapdata.CellCenter(s.hero.cell)
	drawAt(dst, img, x+dx-s.camX, y+dy-s.camY)
}

// statusText is what the status line shows: a recent message, then (with no
// window open) a pile or a character under the mouse, then the hotspot under
// the mouse (0x004244B3): a panel button's tooltip, an item in the
// character window or in the bag strip (0x004305E4).
func (s *startScene) statusText() []byte {
	if s.frame < s.msgEnd {
		return s.msg
	}
	if !s.inv.open {
		if _, ok := s.pileUnderCursor(); ok {
			return s.tables.CString(gamestate.MsgSomethingOnGround)
		}
		if s.hovered >= 0 {
			return s.game.StatusLine(s.entity(s.hovered), s.tables)
		}
	}
	if len(s.party) == 0 {
		return nil
	}
	hero := s.party[0]
	mx, my := ebiten.CursorPosition()
	item := 0
	switch spot := s.tables.HUDHotspot800(mx, my); {
	case spot >= 1 && spot <= len(gamestate.ButtonTooltips):
		return s.tables.CString(gamestate.ButtonTooltips[spot-1])
	case s.inv.showStrip() && s.inv.strip.hit(mx, my):
		item = hero.Bag[((mx-s.inv.strip.x0)/s.inv.strip.cellW+s.inv.scroll)&31]
	case s.inv.open:
		item = gamestate.HoveredItem(hero, spot)
	}
	if item == 0 {
		return nil
	}
	return s.game.ItemDescription(s.tables, item)
}

// windowClick handles a left click on the character window's doll boxes
// (hotspots 8–18, click handler 0x00422864; 19, the in-hand box, is not
// ported) and returns a message VA or 0.
func (s *startScene) windowClick(hero *gamestate.Entity, spot int) int {
	switch {
	case spot >= 8 && spot <= 13:
		slot := [6]int{gamestate.EquipHelmet, -1, gamestate.EquipShield, gamestate.EquipArmour,
			gamestate.WeaponBow, gamestate.EquipAmmo}[spot-8]
		if slot < 0 {
			slot = gamestate.HandSlot(&s.hand, *hero, s.game)
		}
		return s.game.ClickEquipment(&s.hand, hero, s.tables, slot)
	case spot >= 14 && spot <= 18:
		return s.game.ClickJewellery(&s.hand, hero, s.tables, spot-14)
	}
	return 0
}

// messageTicks is how long a status-line message stays: 0x0042FA0C sets the
// counter 0x00A3E84C to 12, decremented once per main-loop pass (0x0043774E);
// the port counts animation steps.
const messageTicks = 12 * ticksPerStep

func (s *startScene) showMessage(m []byte) { s.msg, s.msgEnd = m, s.frame+messageTicks }

// cursorSprite is the GRAPH.RES Type 5 cursor to draw: the hand's sprite
// (0x00A3E790, 0x00428AC0) over the HUD, the window and the bag strip, while
// holding an item or digging; on the map the rules of 0x00428A50
// (characterCursor, piles, occupied cells).
func (s *startScene) cursorSprite() int {
	mx, my := ebiten.CursorPosition()
	if !inView(mx, my) || s.inv.open || s.hand.Holding() || s.hand.Digging() || (s.inv.showStrip() && my >= s.inv.strip.y-1) {
		return s.hand.Cursor
	}
	if s.hovered >= 0 && len(s.party) > 0 {
		return s.characterCursor(s.entity(s.hovered))
	}
	c, ok := s.pileUnderCursor()
	switch {
	case ok && !s.occ[c]:
		return gamestate.CursorTake // 0x00428D7F: visible pile on a free cell
	case s.occ[mapdata.CellAt(mx+s.camX, my+s.camY)]:
		return gamestate.CursorBlocked // 0x00428DB7: someone stands there
	}
	return gamestate.CursorPointer
}

// characterCursor is 0x00428BDE: the cursor over character e. Party members
// keep the hand's cursor; anyone else shows the attack cursor while a living
// selected member (the port's selection: the hero) is in the stance, the
// talk cursor when e has a dialogue, and the barred circle otherwise. The
// port shows the corpse of a dead character like an occupied cell (the
// game's hit test for the dead is not traced). The flame cursor 6 (bow and
// fire arrows over a settlement building, 0x00428AF3) is not ported.
func (s *startScene) characterCursor(e gamestate.Entity) int {
	hero := s.party[0]
	switch {
	case e.Group == 0:
		return s.hand.Cursor
	case e.Dead():
		return gamestate.CursorBlocked
	case !hero.Dead() && hero.InStance():
		return gamestate.CursorAttack
	case e.Dialogue != gamestate.NoDialogue:
		return gamestate.CursorTalk
	}
	return gamestate.CursorBlocked
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
	s.frame++
	s.hud.now = s.frame
	mx, my := ebiten.CursorPosition()
	if s.hero != nil {
		if inpututil.IsKeyJustPressed(ebiten.KeyC) { // port key; the game has none
			s.follow = !s.follow
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
			s.hideHelp = !s.hideHelp
		}
		s.updateSpeed()
		s.updateControls(mx, my)
	}
	// One drawn frame runs 1 + speed game ticks (0x00437849–0x004378CC);
	// the extra ticks stop when the hero is dead.
	for k := 0; k <= s.speed; k++ {
		if k > 0 && len(s.party) > 0 && s.party[0].Dead() {
			break
		}
		s.step()
	}
	if s.hero != nil {
		if s.follow {
			x, y := s.hero.position()
			s.centerOn(int(x), int(y))
			return
		}
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || mx < edgeScrollZone {
		s.camX -= scrollSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) || mx >= ScreenWidth-edgeScrollZone {
		s.camX += scrollSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) || my < edgeScrollZone {
		s.camY -= scrollSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) || my >= ScreenHeight-edgeScrollZone {
		s.camY += scrollSpeed
	}
	s.clampCamera()
}

// step advances the world by one game tick at the port's 60 Hz rate:
// characters, the pick-up on arrival, map exits, pile respawn and the NPC AI.
func (s *startScene) step() {
	s.tick++
	if s.hero != nil {
		moving := s.hero.walking || len(s.hero.path) > 0
		if s.hero.act == 0 {
			s.hero.update()
		}
		stopped := !s.hero.walking && len(s.hero.path) == 0
		switch {
		case s.pickIntent && stopped:
			s.pickIntent = false
			s.arrive()
		case moving && stopped:
			// 0x00422346: a walk that ends without the pick-up intent on an
			// exit cell takes the exit (0x00421BB8).
			if x, ok := s.game.ExitAt(s.mapNumber, s.hero.cell.Row, s.hero.cell.Col); ok {
				s.changeMap(x)
				return
			}
		}
	}
	if s.tick%walkTicksPerStep == 0 {
		s.activatePiles()
		s.combatTick()
	}
	s.updateNPCs()
}

// Game speed 0x00A3E764: 0 normal, 1 double, 2 triple. Numpad + / −
// (VK_ADD 0x6B at 0x00436FD2, VK_SUBTRACT 0x6D at 0x00436FF0); the port
// also accepts = and − on the main keyboard.
const maxSpeed = 2

func (s *startScene) updateSpeed() {
	if inpututil.IsKeyJustPressed(ebiten.KeyNumpadAdd) || inpututil.IsKeyJustPressed(ebiten.KeyEqual) {
		s.speed = min(maxSpeed, s.speed+1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyNumpadSubtract) || inpututil.IsKeyJustPressed(ebiten.KeyMinus) {
		s.speed = max(0, s.speed-1)
	}
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
	s.drawSelection(screen)
	s.drawPiles(screen)
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
	if s.hero != nil {
		s.drawHelp(screen)
	}
	s.hud.Draw(screen, s.party, func(k int) bool { return k == 0 })
	if s.inv.open && len(s.party) > 0 {
		s.inv.draw(screen, s.hud, s.game, s.tables, s.party[0])
	}
	if len(s.party) > 0 {
		s.inv.drawQuick(screen, s.hud, s.game, s.tables, s.party[0])
	}
	if text := s.statusText(); text != nil {
		// The status line in the bottom bar (0x0043195C, 0x0042F9B8), small font.
		s.font.drawWrapped(screen, text, viewLeft+10, viewBottom+5, viewRight-viewLeft-15)
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

func toEbiten(img *image.RGBA) *ebiten.Image { return ebiten.NewImageFromImage(img) }
