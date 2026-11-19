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
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
	"konung1.5/internal/world"
)

const (
	// scrollSpeed is the port's scroll step per update (480 px/s), between
	// the game's 16 px per 60 ms frame (267 px/s) and the port's earlier
	// 720 px/s. The edge zone and the arrow keys are port choices; the game
	// scrolls only with the mouse on the screen's edge pixel (0x00436610).
	scrollSpeed    = 8
	edgeScrollZone = 8

	// The game's camera (map format: camera): the view's world left and top
	// stay at or beyond these, and a followed character's sprite centre is
	// kept followMarginX / followMarginY inside the view's edges.
	cameraMinX    = 58
	cameraMinY    = 32
	followMarginX = 60
	followMarginY = 50
	viewW         = viewRight - viewLeft + 1 // 660 at 800×600 (0x00A3E85C)
	viewH         = viewBottom - viewTop + 1 // 556 (0x00A3E858)
)

// gameFrame is the game's frame count: one frame (main-loop pass) every
// 60 ms (0x00A3E6F4), i.e. updates · 5/18 at 60 updates/s.
func (s *startScene) gameFrame() int { return s.frame * 5 / 18 }

// sceneFrame is one drawable image with its shadow, in world pixels.
type sceneFrame struct {
	img, shadow *ebiten.Image
	x, y        int  // image top-left
	sx, sy      int  // shadow top-left
	raw         bool // untinted by day/night (self-lit object, character on a lit cell)
}

// sceneObject is a static map object (one frame) or an entity (its idle
// animation steps), ready to draw.
type sceneObject struct {
	placement int // KONUNG.N section-3 record (mapdata.Placement.Index)
	// building is the decoded image of a placement with flag bit 0x01
	// (settlement buildings), for the pixel tests of the hover
	// (0x00A3E6A4), the line of fire and the arrows (0x0043F89C); nil for
	// other objects.
	building *image.RGBA
	// frames holds one frame per variant: one for a still object, the
	// header's variant count for an animated one (mapdata.ObjectVariantSprites).
	frames []sceneFrame
	// variant and counter are placement +0x23 and +0x1C; period is header
	// +0x18 (mapdata.StartObject, mapdata.StepObject).
	variant, counter, period int
	// depth is the bottom-edge sort key of variant 0. The variants of every
	// placed animated object have the same size and offsets, so the order
	// does not change as they cycle.
	depth  int
	flames []flame
}

func (o *sceneObject) frame() *sceneFrame { return &o.frames[o.variant] }

// animateObjects is 0x0042480C, once per game tick after the AI: every
// animated object counts down and shows its next variant.
func (s *startScene) animateObjects() {
	for i := range s.objects {
		o := &s.objects[i]
		o.variant, o.counter = mapdata.StepObject(o.variant, o.counter, len(o.frames), o.period)
	}
}

type startScene struct {
	assets        *sceneAssets
	m             *mapdata.Map
	tiles         []*ebiten.Image
	tileMarginX   int
	tileMarginY   int
	decorations   []mapdata.Decoration
	objects       []sceneObject       // objects and entities, in depth order
	placements    []mapdata.Placement // KONUNG.N section 3, building objects updated
	shadowLayer   *ebiten.Image
	shadows       bool // setting 0x00A3E750: draw the shadow pass (0x004266C4)
	dayNight      bool // setting 0x00A3E71C: tint the world by the time of day
	moon          bool // 0x00A3E738: this night is moonlit
	lit           *mapdata.LitCells
	terrainLights *mapdata.TerrainLights
	lighting      bool // setting 0x00A3E734: the lit terrain path at night
	litPath       bool // this frame draws lit terrain cells (0x0042565D)
	flicker       resfmt.LitTerrainLight
	litCells      map[[2]int]*litCell // composed lit terrain cells, relit each frame
	litTables     *resfmt.LitTables
	litBuf        *image.RGBA
	transparency  bool           // setting 0x00A3E700: flames are blended (0x0043D8EF)
	flameFrame    int            // game frame (60 ms) of the last flame advance
	tiling        bool           // setting 0x00A3E744 («T»): windows hide the world behind a tiled background
	soundFrame    int            // game frame (60 ms) of the last soundTick
	tint          resfmt.Tint    // 0x0051EC7C, this frame's light
	tintU         map[string]any // shader tables for tint
	tintSh        *ebiten.Shader // nil if the shader failed to compile
	camX, camY    int
	frame         int // drawn frames (Update calls)
	speed         int // 0x00A3E764

	// World is the simulation (internal/world): the party, the
	// characters, the occupancy, piles, arrows and the clock.
	*world.World
	follow bool // camera follows the hero (0x00A3E68C; Ctrl+S, off at start)
	// camW, camH are the camera's right and bottom limits
	// (mapdata.Map.CameraLimits).
	camW, camH int

	hud *hud

	font   *gameFont // small status-line font (resfmt.FontSmallVA)
	inv    *charWindow
	talk   *talkWindow     // dialogue (window 1) and quest log (window 2), dialogue.go
	trade  *tradeWindow    // trade and pile window (window 7), trade.go
	wmap   *worldMapWindow // world map (window 5), worldmap.go
	hand   gamestate.Hand  // item being moved in the character window
	msg    []byte          // status-line message (0x0042F9B8) and when it ends
	msgEnd int

	hovered int // entity under the cursor in the last frame, -1 = none

	// hoveredBuilding is placement + 1 of the front-most settlement
	// building under the cursor (0x00A3E6A4, set by the object draw
	// pass), 0 = none.
	hoveredBuilding int
	hideHelp        bool // F1 hides the key help (help.go)
	lan             bool // a LAN world (lanview.go)
	// view is a LAN world's link to the server (lanview.go): the scene
	// shows the server's world and simulates nothing.
	view *lanView
	// clock is the server's clock (lanclock.go), nil until it arrives;
	// the menu sets it every frame.
	clock *serverClock
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
	glow           *ebiten.Image  // LIGHTS.RES glow map (resfmt.GlowMask); nil if the file is missing
	arrowGlow      *ebiten.Image  // LIGHTS.RES incendiary arrow glow (resfmt.ArrowGlow); nil if missing
	lightMasks     [][]byte       // LIGHTS.RES light masks for the lit terrain path; nil if missing
	flames         *flameImages   // FLAMES.RES; nil if missing
	quests         *resfmt.Quests // QUESTR.RES dialogues; nil if missing (no one talks)
	// lanMaps, when set, are the LAN version's maps (MAPS.RES; map n =
	// entry n−1) used instead of KONUNG.N and LIGHTS.N.
	lanMaps []mapdata.MapsResEntry
	// view: scenes are LAN viewers (lanview.go); viewState is the link the
	// first one starts with.
	view      bool
	viewState *lanView
	lanMap    *lanWorldMap // the LAN map window (M); nil in single player
}

// mapFile returns map n's KONUNG.N and LIGHTS.N bytes (nil lights when the
// file is missing) and a name for messages.
func (a *sceneAssets) mapFile(n int) (data, lights []byte, name string, err error) {
	if a.lanMaps != nil {
		if n < 1 || n > len(a.lanMaps) {
			return nil, nil, "", fmt.Errorf("MAPS.RES has no map %d", n)
		}
		e := a.lanMaps[n-1]
		return e.Map, e.Lights, fmt.Sprintf("MAPS.RES #%d", n), nil
	}
	name = filepath.Join(a.mapDir, fmt.Sprintf("KONUNG.%d", n))
	if data, err = os.ReadFile(name); err != nil {
		return nil, nil, "", err
	}
	lights, _ = os.ReadFile(filepath.Join(a.mapDir, fmt.Sprintf("LIGHTS.%d", n)))
	return data, lights, name, nil
}

// loadSceneAssets loads the resources a game needs (they stay loaded across
// map changes, new games and loaded saves). mapDir holds KONUNG.N, GAME.N,
// LIGHTS.N and QUESTR.RES.
func loadSceneAssets(graphPath, objectsPath, mapDir, exePath, intrPath string) (*sceneAssets, error) {
	a := &sceneAssets{mapDir: mapDir}
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
	// LIGHTS.RES sits next to GRAPH.RES (0x0043B7FC); without it piles don't glow.
	lightsPath := filepath.Join(filepath.Dir(graphPath), "LIGHTS.RES")
	if data, err := os.ReadFile(lightsPath); err != nil {
		log.Printf("%s: %v", lightsPath, err)
	} else if glow, err := resfmt.LightsGlow(data); err != nil {
		log.Printf("%s: %v", lightsPath, err)
	} else {
		a.glow = ebiten.NewImageFromImage(resfmt.GlowMask(glow, resfmt.GlowWidth))
		if ag, err := resfmt.ArrowGlow(data); err == nil {
			a.arrowGlow = ebiten.NewImageFromImage(resfmt.GlowMask(ag, resfmt.ArrowGlowSize))
		}
		a.lightMasks, _ = resfmt.LightMasks(data)
	}
	// QUESTR.RES next to GAME.N (0x0043BDD5): dialogues and quest texts.
	if data, err := readGameFile(mapDir, "QUESTR.RES"); err != nil {
		log.Printf("QUESTR.RES: %v", err)
	} else if a.quests, err = resfmt.ParseQuests(data); err != nil {
		log.Printf("QUESTR.RES: %v", err)
	}
	// FLAMES.RES sits next to GRAPH.RES (0x0043BA50); without it nothing burns.
	if a.flames, err = loadFlames(filepath.Join(filepath.Dir(graphPath), "FLAMES.RES")); err != nil {
		log.Print(err)
	}
	return a, nil
}

// newGame is the game start 0x00438F3F with the hero made on the
// character-creation screen: GAME.<model> is loaded, the hero takes the
// template's choices (gamestate.Game.StartHero) and the scene is built for
// the start map KONUNG.<model·3 + 1>.
func (a *sceneAssets) newGame(hero gamestate.Entity) (*startScene, error) {
	a.gamePath = filepath.Join(a.mapDir, fmt.Sprintf("GAME.%d", hero.Model))
	gameData, err := os.ReadFile(a.gamePath)
	if err != nil {
		return nil, err
	}
	game, err := gamestate.Parse(gameData)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", a.gamePath, err)
	}
	game.StartHero(hero)
	game.InitSettlementFlags(hero.Model) // 0x0043BD5C
	// 0x0043BDD5: QUESTR.RES next to GAME.N; the game state takes the
	// initial quest flags (the dialogues stay in sceneAssets.quests).
	if questr, err := readGameFile(a.mapDir, "QUESTR.RES"); err != nil {
		log.Printf("QUESTR.RES: %v", err)
	} else if err := game.SetQuestFlags(questr); err != nil {
		log.Printf("QUESTR.RES: %v", err)
	}
	return a.buildScene(game, game.Groups[0].Map, nil, nil)
}

// changeMap takes map exit x (0x00421CCD, then the main loop's map change at
// 0x004377C6; the world's ChangeMap hook). An exit or jump to the map the
// party is on only places the party again (World.PlaceAgain). Otherwise
// the world leaves the map (World.Leave) and the scene is rebuilt for the
// target; if it fails to load, Leave's undo restores the records.
func (s *startScene) changeMap(x gamestate.MapExit) {
	if x.To == s.MapNumber {
		s.PlaceAgain(x)
		s.centerHero() // 0x00437832 → 0x00428F90
		return
	}
	undo := s.Leave(x)
	ns, err := s.assets.buildScene(s.Game, x.To, s, nil)
	if err != nil {
		log.Printf("map exit %d to KONUNG.%d: %v", x.Index, x.To, err)
		undo()
		return
	}
	*s = *ns
	s.installHooks() // the hooks act on the live scene
}

// buildScene builds the scene for map mapNumber: terrain, decorations and
// static objects from the KONUNG.N file, characters and creatures from game.
// prev, when set, is the scene being left: the clock, speed, camera mode,
// held item and windows carry over. restore, when set, is a loaded save:
// like the map loader's restore mode 0x0043C0FC(1) it keeps the groups'
// alarm, skips placement and respawn (EnterMap) and takes the active piles,
// the clock and the arrows in flight from the save.
func (a *sceneAssets) buildScene(game *gamestate.Game, mapNumber int, prev *startScene, restore *gamestate.SaveState) (*startScene, error) {
	mapData, lightsData, mapPath, err := a.mapFile(mapNumber)
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
	game.Lit = false        // 0x0043C11B: the map loader puts the torch out
	game.KnowMap(mapNumber) // 0x0043C12A: the map window shows it from now on
	s := &startScene{
		World: &world.World{
			Walk:      walk,
			Game:      game,
			Tables:    tables,
			ByEntity:  map[int]*npc{},
			Occ:       map[mapdata.Cell]bool{},
			MapNumber: mapNumber,
			PileAt:    map[mapdata.Cell]int{},
			Rng:       rand.New(rand.NewPCG(1, uint64(mapNumber))),
		},
		assets:      a,
		m:           m,
		tiles:       a.tiles,
		shadowLayer: ebiten.NewImage(ScreenWidth, ScreenHeight),
		hud:         a.hud,
		font:        a.font,
		hovered:     -1,
		hand:        gamestate.NewHand(),
		lan:         a.lanMaps != nil, // also after a map change
	}
	if s.lit, s.terrainLights, err = parseLights(lightsData); err != nil {
		log.Printf("LIGHTS.%d: %v", mapNumber, err)
	}
	if s.tintSh, err = loadTintShader(); err != nil {
		log.Print(err)
	}
	if prev != nil {
		s.moon, s.dayNight, s.shadows, s.lighting = prev.moon, prev.dayNight, prev.shadows, prev.lighting
		s.transparency = prev.transparency
		s.inv, s.hand, s.follow, s.speed = prev.inv, prev.hand, prev.follow, prev.speed
		s.talk, s.trade, s.wmap = prev.talk, prev.trade, prev.wmap
		s.Tick, s.frame, s.msg, s.msgEnd = prev.Tick, prev.frame, prev.msg, prev.msgEnd
		s.shadowLayer = prev.shadowLayer
		s.Sel = prev.Sel
	} else {
		s.inv = newCharWindow(a.hud, tables, a.largeFont, a.font)
		s.talk = newTalkWindow(tables, a.quests, a.font)
		s.trade = newTradeWindow(tables, s.inv.strip, a.font, a.largeFont)
		s.wmap = newWorldMapWindow(tables)
		s.wmap.lan = a.lanMap
		if a.lanMaps != nil && a.lanMap == nil {
			s.wmap.lan = &lanWorldMap{} // a LAN world without the map data: the bare parchment
		}
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
	s.WorldW, s.WorldH = m.WorldSize()
	s.camW, s.camH = m.CameraLimits()

	// Village buildings show their state (0x0043CB4A).
	if tables != nil {
		for i, id := range game.BuildingObjects(mapNumber, tables) {
			if i < len(placements) {
				placements[i].ID = id
			}
		}
	}
	s.placements = placements
	upload := newUploader()
	s.objects = append(s.objects, s.placeObjects(placements, mapPath, upload)...)
	// Characters and creatures are walkers with lazily loaded idle/walk
	// loops. The player (group 0's first entity) is controllable; villagers
	// run the ported village AI, creatures stand still while calm (see npc).
	camX, camY := s.WorldW/2, s.WorldH/2
	uploadSprite := func(sp *resfmt.EntitySprite) (*ebiten.Image, *ebiten.Image) { return upload(sp.Image, sp.Shadow) }
	logErr := func(err error) { log.Printf("%s: %v", gamePath, err) }
	attach := func(w *walker, p gamestate.Placed, live func() gamestate.Entity) {
		withSprites(w, entityLoader(game, p, live, objectsData, graphData, tables, uploadSprite, logErr))
	}
	if a.view {
		// A LAN viewer: only the own hero; the server reports the rest.
		s.view = a.viewState
		if prev != nil {
			s.view = prev.view
		}
		s.view.attach = attach
		s.placeOwn(attach)
	} else {
		s.Populate(restore != nil, attach)
	}
	if s.Hero != nil {
		camX, camY = gamestate.WorldPos(s.Hero.Cell.Row, s.Hero.Cell.Col)
	}
	sort.SliceStable(s.objects, func(i, j int) bool { return s.objects[i].depth < s.objects[j].depth })
	prevCur := -1
	if prev != nil {
		prevCur = prev.Cur
	}
	s.InitSelection(prevCur)
	s.installHooks()
	// 0x0043CB8B: the map's settlement restocks (healer jars, merchant);
	// the loader does this on every load, a restored save included.
	if (prev == nil || prev.MapNumber != mapNumber) && !a.view {
		game.Restock(mapNumber, game.Groups[0].First, tables, s.Rng.IntN)
	}
	if restore != nil {
		s.restoreSaved(*restore)
	}
	s.ActivatePiles()
	if prev != nil {
		s.clock = prev.clock
	}
	s.centerOn(camX, camY)
	s.centerHero()
	return s, nil
}

// updateControls handles the player's keys and clicks. Orders go to the
// selection list (0x0083B93C, party.go); item moves work on the character
// window's character (0x00A3E688, startScene.current).
//
//   - Space / HUD stance button: the selection's stance (0x0042179C); 1/2/3
//     and the HUD weapon button: the first selected member's weapon, the
//     whole selection's with Shift (0x0042192C).
//   - I / HUD bag button: character window (0x00424610); ~: the bag strip
//     alone (0x004371D2). The bag arrows are hotspots 120/121.
//   - Left click (0x00422933): HUD hotspots, portraits (select, the bag
//     strip), window boxes and bag cells move items; on the map the
//     selection walks there without picking up (0x004229F7), or a held item
//     is dropped on the ground at its owner's cell (0x004229BC → 0x004138C8).
//   - Right click (0x004238EE): returns a held item; on a bag cell uses the
//     item (gamestate.UseBagItem); on a portrait deselects (0x00423D02); on
//     a party member selects it (0x00423A5C); on a character with a
//     dialogue (out of the stance; a party member with Ctrl) opens it
//     without walking (0x00423B6E, 0x00423A41); otherwise every selected
//     member takes the order (rightOrder: shoot, attack, walk and pick up).
//   - F / HUD button 4: «Все ко мне» (gatherOrder); A / HUD button 5:
//     attack (attackOrder).
//   - Q / HUD button 5: quest log (0x0042459C). While a dialogue is open
//     only its replies take clicks.
//
// The stance, weapon and window routines first return a held item
// (0x00429194(1) at 0x004217BA, 0x00421948, 0x00424635); any click ends the
// shovel's dig mode first.
func (s *startScene) updateControls(mx, my int) {
	if len(s.Party) == 0 || s.Lead() == nil || s.Party[0].Dead() || s.Hero.Act == gamestate.ActDeath {
		return
	}
	cur := s.Current()
	left := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	right := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
	if s.trade.open() {
		// Window 7: its strips and buttons (and the portraits) are the
		// only hotspots (0x004392F4).
		if left {
			s.tradeClick(mx, my)
		}
		return
	}
	if s.talk.d != nil {
		// Window 1: the hit test offers only the replies (0x0043968F); the
		// click handler ignores everything else (0x00422EF5).
		if left {
			if k := s.talk.replyAt(mx, my); k >= 0 {
				s.chooseReply(k)
			}
		}
		return
	}
	button, spot := -1, 0
	if left || right {
		spot = s.Tables.HUDHotspot800(mx, my)
	}
	if left {
		button = buttonAt(s.Tables, mx, my)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || button == hudStanceButton {
		s.returnHand(cur)
		if s.view != nil {
			// The first selected decides, as ToggleSelectionStance.
			s.viewOrder(netplay.OrderPayload{Kind: netplay.OrderStance, On: !s.ButtonEntity().InStance()})
		} else {
			s.ToggleSelectionStance()
		}
		s.hud.press(hudStanceButton, gamestate.HUDButtonSlots(*s.ButtonEntity())[hudStanceButton])
	}
	weapon := func(mode int) {
		s.returnHand(cur)
		before := gamestate.HUDButtonSlots(*s.ButtonEntity())[hudWeaponButton]
		if s.view != nil {
			// The first selected member, the whole selection with Shift
			// (SelectWeapon).
			sel := append([]int(nil), s.Sel...)
			if !shiftHeld() && len(sel) > 1 {
				sel = sel[:1]
			}
			s.viewOrder(netplay.OrderPayload{Kind: netplay.OrderWeapon, Mode: mode, Sel: sel})
		} else {
			s.selectWeapon(mode)
		}
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
		if s.view != nil {
			s.returnHand(cur)
			s.hud.press(hudGatherButton, hudGatherImage)
			s.viewOrder(netplay.OrderPayload{Kind: netplay.OrderGather}) // the server's hirelings
		} else {
			s.gatherOrder()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyA) || button == hudAttackButton {
		if s.view != nil {
			s.hud.press(hudAttackButton, hudAttackImage)
			s.viewOrder(netplay.OrderPayload{Kind: netplay.OrderAttack})
		} else {
			s.attackOrder()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyM) || button == hudMapButton {
		s.toggleMap(cur) // 0x00436EB9, 0x00422C01
	}
	if (inpututil.IsKeyJustPressed(ebiten.KeyQ) || button == hudLogButton) && !s.inv.open && !s.wmap.open {
		s.talk.toggleLog() // 0x0042459C
		s.hud.press(hudLogButton, hudLogImage)
	}
	if (inpututil.IsKeyJustPressed(ebiten.KeyI) || button == hudBagButton) && !s.talk.log && !s.wmap.open {
		s.returnHand(cur)
		s.inv.toggle()
		if s.inv.open {
			s.clearBox() // 0x004246D1
		}
		s.hud.press(hudBagButton, hudBagImage)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackquote) && !s.inv.open && !s.talk.open() {
		s.inv.toggleQuick()
		if s.inv.quick {
			s.clearBox() // 0x00437253: showing the strip empties the in-hand box
		}
	}
	switch {
	case s.talk.log:
		// Window 2: clicks only scroll (hotspots 0xCA/0x1CA, 0x00422A83).
		if left {
			s.talk.scrollLog(mx, my)
		}
	case s.wmap.open:
		// Window 5: the map area's hotspots (0x0043946E); other clicks in
		// the view are ignored (0x00422EDF), and a right click only
		// returns a held item (0x0042393D needs window 0).
		if _, inArea := s.wmap.regionAt(s.Game, mx, my); left && inArea {
			s.mapClick(cur, mx, my)
		} else if right {
			s.returnHand(cur)
		} else if left && !inView(mx, my) {
			s.click(cur, mx, my, spot, false)
		}
	case left || right:
		s.click(s.Current(), mx, my, spot, right)
	}
	for _, m := range s.Members {
		if m != nil {
			m.Look = lookOf(s.Party[m.K])
		}
	}
}

// click routes one mouse click (button already handled above for the HUD);
// cur is the character window's character, who holds the hand's item.
func (s *startScene) click(cur *gamestate.Entity, mx, my, spot int, right bool) {
	if s.hand.Digging() {
		s.hand.Cursor = gamestate.CursorPointer
		if s.inv.strip.hit(mx, my) && s.inv.showStrip() && !right {
			return // 0x00423412: a bag click only ends dig mode
		}
	}
	if right && s.hand.Holding() {
		s.returnHand(cur) // 0x00423919
		return
	}
	if s.view != nil {
		// A LAN viewer: item clicks, map clicks and talking are orders
		// (lanitems.go, lanview.go).
		s.lanClick(cur, mx, my, spot, right)
		return
	}
	msg := 0
	portrait := -1
	if k := spot - hotspotPortrait; k >= 0 && k < len(portraitCells) && k < len(s.Party) {
		portrait = k
	}
	switch {
	case portrait >= 0 && right:
		s.portraitRightClick(portrait) // 0x00423D02
	case portrait >= 0 && s.hand.Holding():
		// 0x00422E21: a held item dropped on a portrait is used on (or given
		// to) that member.
		msg = s.Game.ClickPortrait(&s.hand, cur, &s.Party[portrait], s.Tables)
		if m, _ := s.PartyMember(s.Party[portrait].Index); m != nil {
			m.Look = lookOf(s.Party[portrait])
		}
	case portrait >= 0:
		s.portraitClick(portrait) // 0x00422EA8: select, the bag strip
	case !inView(mx, my):
		return // HUD: buttons were handled by updateControls
	case s.inv.showStrip() && (spot == hotspotBagLeft || spot == hotspotBagRight):
		if !right {
			dir := -1
			if spot == hotspotBagRight {
				dir = 1
			}
			s.inv.scrollBy(*cur, dir, s.frame)
		}
	case s.inv.showStrip() && s.inv.strip.hit(mx, my):
		i := (mx-s.inv.strip.x0)/s.inv.strip.cellW + s.inv.scroll
		if right {
			msg = s.Game.UseBagItem(&s.hand, cur, s.Tables, i)
		} else {
			msg = s.Game.ClickBag(&s.hand, cur, s.Tables, i)
		}
	case s.inv.open:
		if !right {
			msg = s.windowClick(cur, spot)
		}
	case !right && s.hand.Holding():
		s.dropHeld(cur)
	case right && s.hovered >= 0 && s.Entity(s.hovered).Group == 0:
		// 0x004239EE: a party member — its dialogue with Ctrl, else the
		// selection.
		if s.canTalk(s.Party[0]) {
			s.talkTo(s.hovered)
		} else {
			s.selectInWorld(s.hovered)
		}
	case right && s.canTalk(s.Party[0]):
		s.talkTo(s.hovered) // 0x00423B6E: at once, wherever the hero stands
	default:
		c := mapdata.CellAt(mx+s.camX, my+s.camY)
		for _, m := range s.Selected() {
			if right {
				s.rightOrder(m, s.hovered, c)
			} else {
				s.WalkOrder(m, c, false)
			}
		}
	}
	if msg != 0 {
		s.showMessage(s.Tables.CString(msg))
	}
}

// dropHeld puts the held item on the ground at its owner's cell (e,
// 0x004138C8 with 0x00A3E520's cell). The game ignores a failure (full
// pile, 200 active piles): the item is lost.
func (s *startScene) dropHeld(e *gamestate.Entity) {
	item := s.hand.Item
	s.hand.Cursor = gamestate.CursorPointer
	cell := mapdata.Cell{Row: e.Row, Col: e.Col}
	if m, _ := s.PartyMember(e.Index); m != nil {
		cell = m.Cell
	}
	at, on := s.PileAt[cell]
	if !on {
		if len(s.PileAt) >= gamestate.MaxActivePiles {
			return
		}
		at = -1
	}
	if i, ok := s.Game.DropItem(at, s.MapNumber, cell.Row, cell.Col, item); ok {
		s.PileAt[cell] = i
	}
}

// drawCorpses draws the dead flat in the terrain pass, before the ground
// items (0x00425C23 draws only entities with +4 bit 0x80; the depth-sorted
// pass 0x00426C07 skips them), so a loot pouch lies on top of its body.
func (s *startScene) drawCorpses(dst *ebiten.Image) {
	draw := func(w *walker) {
		if f, ok := walkerFrame(w, s.Tick); ok && s.visible(f.x, f.y, f.img.Bounds().Dx(), f.img.Bounds().Dy()) {
			s.drawTinted(dst, f.img, f.x-s.camX, f.y-s.camY, s.lit.Lit(w.Cell.Row, w.Cell.Col))
		}
	}
	for _, m := range s.Members {
		if m != nil && s.Party[m.K].Dead() {
			draw(&m.Walker)
		}
	}
	for _, n := range s.NPCs {
		if s.Game.Entities[n.Entity].Dead() {
			draw(&n.Walker)
		}
	}
}

// drawSelection draws the selection ellipses (0x00426146): under every
// selected member on the map, before the ground items, flat, coloured by
// HP and following the walk animation.
func (s *startScene) drawSelection(dst *ebiten.Image) {
	for _, i := range s.Sel {
		m, e := s.PartyMember(i)
		if m == nil || e.Dead() {
			continue
		}
		img := s.hud.slot(gamestate.SelectionMarker(*e))
		if img == nil {
			continue
		}
		step := min(m.StepT, m.WalkSteps()-1)
		dx, dy := gamestate.SelectionOffset(int(m.Dir), step, m.Walking)
		x, y := mapdata.CellCenter(m.Anchor()) // +0xE8/+0xF0: the cell being left while walking
		drawAt(dst, img, x+dx-s.camX, y+dy-s.camY)
	}
}

// statusText is what the status line shows: a recent message, then (with no
// window open) a pile or a character under the mouse, then the hotspot under
// the mouse (0x004244B3): a panel button's tooltip, a party member's status
// line over its portrait, an item in the character window or in the bag
// strip (0x004305E4).
func (s *startScene) statusText() []byte {
	if s.gameFrame() < s.msgEnd {
		return s.msg
	}
	if s.trade.open() {
		// 0x004241A1: the item under the mouse in window 7.
		mx, my := ebiten.CursorPosition()
		if i := s.trade.itemAt(mx, my); i != 0 {
			return s.Game.ItemDescription(s.Tables, i)
		}
		return nil
	}
	if s.talk.open() {
		return nil // 0x00424054 shows the hover line only with no window open
	}
	if s.wmap.open {
		// 0x004240D0: over a known location its name (0x0045B3F4); over
		// the rest of the area nothing.
		mx, my := ebiten.CursorPosition()
		if m, inArea := s.wmap.regionAt(s.Game, mx, my); m != 0 {
			return s.Tables.CString(gamestate.LocationName(m))
		} else if inArea {
			return nil
		}
	}
	if !s.inv.open && !s.wmap.open {
		if _, ok := s.pileUnderCursor(); ok {
			return s.Tables.CString(gamestate.MsgSomethingOnGround)
		}
		if t, ok := s.viewStatus(s.hovered); ok {
			return t
		}
		if s.hovered >= 0 {
			return s.Game.StatusLine(s.Entity(s.hovered), s.Tables)
		}
	}
	if len(s.Party) == 0 {
		return nil
	}
	hero := *s.Current()
	mx, my := ebiten.CursorPosition()
	item := 0
	spot := s.Tables.HUDHotspot800(mx, my)
	switch k := spot - hotspotPortrait; {
	case spot >= 1 && spot <= len(gamestate.ButtonTooltips):
		return s.Tables.CString(gamestate.ButtonTooltips[spot-1])
	case k >= 0 && k < len(portraitCells) && k < len(s.Party):
		return s.portraitStatus(k)
	case s.inv.showStrip() && s.inv.strip.hit(mx, my):
		item = hero.Bag[((mx-s.inv.strip.x0)/s.inv.strip.cellW+s.inv.scroll)&31]
	case s.inv.open && spot == hotspotInHand:
		item = s.hand.Box
	case s.inv.open:
		item = gamestate.HoveredItem(hero, spot)
	}
	if item == s.hand.Box && spot != hotspotInHand {
		return nil // the boxed item is described only over the box (0x00424294, 0x004243C6, 0x00424403)
	}
	if item == 0 {
		return nil
	}
	return s.Game.ItemDescription(s.Tables, item)
}

// portraitStatus is the hover over portrait k (hotspot 0x1E with the cell
// in the high byte, 0x00424131): the member's status line (0x0043195C),
// nothing while it is dead or dying.
func (s *startScene) portraitStatus(k int) []byte {
	e := s.Party[k]
	if m, _ := s.PartyMember(e.Index); e.Dead() || (m != nil && m.Act == gamestate.ActDeath) {
		return nil
	}
	return s.Game.StatusLine(e, s.Tables)
}

// hotspotPortrait is the first portrait cell's hotspot id (30–39 = cells 0–9).
const hotspotPortrait = 30

// hotspotInHand is the character window's in-hand box (0x00A3E7A0).
const hotspotInHand = 19

// windowClick handles a left click on the character window's doll boxes
// (hotspots 8–18), the in-hand box (19, where herbs are mixed and applied),
// the raise buttons (40–44) and the auto-level toggle (45), as the click
// handler 0x00422864 does, and returns a message VA or 0.
func (s *startScene) windowClick(hero *gamestate.Entity, spot int) int {
	switch {
	case spot >= 8 && spot <= 13:
		slot := [6]int{gamestate.EquipHelmet, -1, gamestate.EquipShield, gamestate.EquipArmour,
			gamestate.WeaponBow, gamestate.EquipAmmo}[spot-8]
		if slot < 0 {
			slot = gamestate.HandSlot(&s.hand, *hero, s.Game)
		}
		return s.Game.ClickEquipment(&s.hand, hero, s.Tables, slot)
	case spot >= 14 && spot <= 18:
		return s.Game.ClickJewellery(&s.hand, hero, s.Tables, spot-14)
	case spot == hotspotInHand:
		return s.Game.ClickBox(&s.hand, hero, s.Tables)
	case spot >= gamestate.HotspotRaise && spot < gamestate.HotspotRaise+gamestate.RaiseRows:
		s.Game.Raise(hero, s.Tables, spot-gamestate.HotspotRaise)
	case spot == gamestate.HotspotAutoLevel:
		s.Game.ToggleAutoLevel(hero, s.Tables)
	}
	return 0
}

// messageFrames is how long a status-line message stays: 0x0042FA0C sets
// the counter 0x00A3E84C to 12, decremented once per main-loop pass
// (0x0043774E), so 12 game frames = 0.72 s.
const messageFrames = 12

func (s *startScene) showMessage(m []byte) { s.msg, s.msgEnd = m, s.gameFrame()+messageFrames }

// cursorSprite is the GRAPH.RES Type 5 cursor to draw: the hand's sprite
// (0x00A3E790, 0x00428AC0) over the HUD, the window and the bag strip, while
// holding an item or digging; on the map the rules of 0x00428A50
// (characterCursor, piles, occupied cells).
func (s *startScene) cursorSprite() int {
	mx, my := ebiten.CursorPosition()
	if !inView(mx, my) || s.inv.open || s.talk.open() || s.trade.open() || s.hand.Holding() || s.hand.Digging() || (s.inv.showStrip() && my >= s.inv.strip.y-1) {
		return s.hand.Cursor
	}
	if s.fireReady() && s.hoveredBuilding != 0 {
		return gamestate.CursorFire // 0x00428AF3, before the character cursor
	}
	if s.hovered >= 0 && len(s.Party) > 0 {
		return s.characterCursor(s.Entity(s.hovered))
	}
	c, ok := s.pileUnderCursor()
	switch {
	case ok && !s.Occ[c]:
		return gamestate.CursorTake // 0x00428D7F: visible pile on a free cell
	case s.Occ[mapdata.CellAt(mx+s.camX, my+s.camY)]:
		return gamestate.CursorBlocked // 0x00428DB7: someone stands there
	}
	return gamestate.CursorPointer
}

// characterCursor is 0x00428BDE: the cursor over character e. Party members
// keep the hand's cursor; anyone else shows the attack cursor while a living
// selected member is in the stance, the
// talk cursor when e has a dialogue, and the barred circle otherwise. The
// selection is the port's list (s.Sel). The port shows the corpse of a dead character like an occupied cell (the
// game's hit test for the dead is not traced). The flame cursor 6 over a
// settlement building comes first (cursorSprite, fireReady).
func (s *startScene) characterCursor(e gamestate.Entity) int {
	stance := false
	for _, m := range s.Selected() {
		stance = stance || s.Party[m.K].InStance()
	}
	switch {
	case e.Group == 0 && ebiten.IsKeyPressed(ebiten.KeyControl) && e.Dialogue != gamestate.NoDialogue:
		return gamestate.CursorTalk // 0x00A3E714: Ctrl held over a party member with a dialogue
	case e.Group == 0:
		return s.hand.Cursor
	case e.Dead():
		return gamestate.CursorBlocked
	case stance:
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

// centerHero is 0x00428F90 after a map change, a load or a new game: the
// hero's sprite top-left (+0xE4/+0xEC) goes to the screen centre, then the
// limits apply.
func (s *startScene) centerHero() {
	if s.Hero == nil {
		return
	}
	if f, ok := walkerFrame(s.Hero, s.Tick); ok {
		s.camX, s.camY = f.x-ScreenWidth/2, f.y-ScreenHeight/2
		s.clampCamera()
	}
}

// clampCamera applies the game's camera limits (0x00428F90, 0x00436610):
// the view's world left at least cameraMinX, else its right edge at most
// camW; its top at least cameraMinY, else its bottom at most camH. The
// minimum is tested first, so on a map smaller than the view the maximum
// wins.
func (s *startScene) clampCamera() {
	c, t := s.camX+viewLeft, s.camY+viewTop
	if c < cameraMinX {
		c = cameraMinX
	} else if c+viewW > s.camW {
		c = s.camW - viewW
	}
	if t < cameraMinY {
		t = cameraMinY
	} else if t+viewH > s.camH {
		t = s.camH - viewH
	}
	s.camX, s.camY = c-viewLeft, t-viewTop
}

// followHero is the follow part of 0x00436610, right after the scroll
// while following: the camera moves just enough to keep the hero's follow
// point followMarginX inside the view's left and right edges and
// followMarginY inside its top and bottom, then the limits apply.
func (s *startScene) followHero() {
	x, y, ok := s.followPoint(s.Hero)
	if !ok {
		return
	}
	s.pushCamera(x, y)
}

func (s *startScene) pushCamera(x, y int) {
	c, t := s.camX+viewLeft, s.camY+viewTop
	if c+followMarginX > x {
		c = x - followMarginX
	} else if c+viewW-followMarginX <= x {
		c = x - viewW + followMarginX
	}
	if t+followMarginY > y {
		t = y - followMarginY
	} else if t+viewH-followMarginY <= y {
		t = y - viewH + followMarginY
	}
	s.camX, s.camY = c-viewLeft, t-viewTop
	s.clampCamera()
}

// followPoint is the point 0x00436610 follows (0x004366C6–0x004368C8), in
// world pixels: the sprite's centre, rounded to even; while walking with an
// east or west part (facing not N or S) x is the cell being left plus 5 px
// per walk frame east/west or 2.5 px diagonally.
func (s *startScene) followPoint(w *walker) (x, y int, ok bool) {
	f, ok := walkerFrame(w, s.Tick)
	if !ok || f.img == nil {
		return 0, 0, false
	}
	even := func(v int) int { return (v + 1) / 2 * 2 }
	b := f.img.Bounds()
	x, y = even(f.x+b.Dx()/2), even(f.y+b.Dy()/2)
	if w.Walking && w.Dir != 2 && w.Dir != 6 {
		ax, _ := mapdata.CellCenter(w.Anchor())
		step := w.StepT
		switch w.Dir {
		case 0:
			x = ax - 5*step
		case 4:
			x = ax + 5*step
		case 1, 7:
			x = ax - (2*step + step/2)
		case 3, 5:
			x = ax + (2*step + step/2)
		}
	}
	return x, y, true
}

// escape handles Escape in the game (0x00437447): return a held item
// (0x00429194(1)), then close an open window (0x00A3E740 = 0, redraw
// 0x004292E4). Only with no window open does it report true, and the caller
// opens the main menu (state 7 → menu, 0x00A3E728 = 1). Escape is ignored
// in window 1 (the dialogue) and closes the quest log (window 2).
func (s *startScene) escape() bool {
	if len(s.Party) > 0 {
		s.returnHand(s.Current())
	}
	switch {
	case s.trade.open():
		if s.view != nil {
			s.itemOrder(netplay.ItemClose, 0, false) // the server closes its window 7 too
		}
		s.trade.tr = nil // closes without applying: nothing changed yet
		return false
	case s.talk.d != nil:
		return false // 0x00437451: Escape does nothing in window 1
	case s.talk.log:
		s.talk.toggleLog()
		return false
	case s.wmap.open:
		s.wmap.open = false
		return false
	case s.inv.open:
		s.inv.toggle()
		return false
	}
	return true
}

func (s *startScene) Update() {
	s.Game.World = s.Access()
	if s.Hero != nil && godModeKey() {
		godMode = !godMode
	}
	s.Game.GodMode = godMode
	s.frame++
	if f := s.frame * 5 / 18; f != s.soundFrame { // one game frame per 60 ms
		s.soundFrame = f
		s.soundTick()
	}
	s.hud.now = s.frame
	mx, my := ebiten.CursorPosition()
	if s.Hero != nil {
		if inpututil.IsKeyJustPressed(ebiten.KeyS) && ebiten.IsKeyPressed(ebiten.KeyControl) { // 0x00436F2B
			s.follow = !s.follow
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
			s.hideHelp = !s.hideHelp
		}
		if s.clock == nil {
			s.updateSpeed() // a LAN world has the server's speed
		}
		s.updateControls(mx, my)
	}
	if s.miscRequests() {
		return
	}
	if s.Game.ReviveAll {
		s.Game.ReviveAll = false
		s.ReviveAll()
	}
	if s.Game.OpenMap {
		// 0x004153B0: the jump orb hides the bag strip (0x00A3E730 = 0)
		// and opens window 5 in place of any other.
		s.Game.OpenMap = false
		s.inv.open, s.inv.quick = false, false
		s.wmap.open = true
	}
	// The game draws a frame every 60 ms (0x00A3E6F4), one per 3.6 updates
	// at 60 Hz, and each frame runs 1 + speed game ticks
	// (0x00437849–0x004378CC); the extra ticks stop when the hero is dead.
	if s.view != nil {
		// A LAN viewer: the server runs the world; only the idle loops
		// animate here, one frame per 60 ms.
		if s.clock != nil {
			s.speed = s.clock.c.Speed
		}
		if s.frame*5/18 != (s.frame-1)*5/18 {
			s.viewFrame()
		}
	} else if s.frame*5/18 != (s.frame-1)*5/18 {
		for k := 0; k <= s.speed; k++ {
			if k > 0 && len(s.Party) > 0 && s.Party[0].Dead() {
				break
			}
			s.Step()
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
	// 0x00436610 scrolls, then follows, in the same pass and before the
	// draw, so the follow stops a scroll at the margin. The port does both
	// on every update; following only once per 60 ms frame let the scroll
	// show for a few updates and then snap back (the camera shook).
	if s.follow && s.Hero != nil {
		s.followHero()
	}
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
	s.updateLight()
	window := s.inv.open || s.talk.open() || s.trade.open() || s.wmap.open
	if s.tiling && window {
		// 0x0042AE68: with background tiling («T», 0x00A3E744) an open
		// window hides the world; the window tiles INTR slot 5 behind it.
		s.hovered, s.hoveredBuilding = -1, 0
	} else {
		chars, owners := s.drawWorld(screen)
		s.hovered = s.hitTest(chars, owners)
		s.hoveredBuilding = s.buildingUnderCursor()
		if window {
			s.hovered, s.hoveredBuilding = -1, 0 // the window covers the world
		}
	}
	if s.Hero != nil {
		s.drawHelp(screen)
		s.drawSessionLeft(screen)
	}
	if len(s.Party) > 0 {
		s.hud.Draw(screen, s.Party, *s.ButtonEntity(),
			func(k int) bool { return gamestate.Selected(s.Sel, s.Party[k].Index) },
			func(k int) bool { m := s.Members[k]; return m != nil && m.Act == gamestate.ActFlinch })
	} else {
		s.hud.Draw(screen, nil, gamestate.Entity{}, nil, nil)
	}
	s.inv.tiled = s.tiling
	if s.inv.open && len(s.Party) > 0 {
		s.inv.draw(screen, s.hud, s.Game, s.Tables, *s.Current(), s.hand.Box)
	}
	if len(s.Party) > 0 {
		s.inv.drawQuick(screen, s.hud, s.Game, s.Tables, *s.Current(), s.hand.Box)
	}
	s.talk.draw(screen, s.hud, s.Game, s.tiling, s.inv.strip.y)
	if s.trade.open() {
		s.trade.draw(screen, s.hud, s.Game, s.Tables, s.tiling)
	}
	if s.lan && s.wmap.open {
		s.wmap.counts = s.playerCounts()
	}
	s.wmap.draw(screen, s.hud, s.Game, s.MapNumber, s.tiling, s.font)
	if text := s.statusText(); text != nil {
		// The status line in the bottom bar (0x0043195C, 0x0042F9B8), small font.
		s.font.drawWrapped(screen, text, viewLeft+10, viewBottom+5, viewRight-viewLeft-15)
	}

}

// drawWorld is the scene draw 0x0042894C: terrain, decorations, piles,
// shadows, objects with their flames, characters and arrows. It returns
// the drawn characters and their entities for the hit test.
func (s *startScene) drawWorld(screen *ebiten.Image) ([]sceneFrame, []int) {
	// Same order as the game's terrain pass (VA 0x0042562F): rows top to
	// bottom, columns left to right, base layer then overlay layer.
	rowMin := max(0, (s.camY-s.tileMarginY)/mapdata.CellHeight)
	rowMax := min(mapdata.Rows-1, (s.camY+ScreenHeight)/mapdata.CellHeight)
	colMin := max(0, (s.camX-s.tileMarginX)/mapdata.CellWidth)
	colMax := min(mapdata.Cols-1, (s.camX+ScreenWidth)/mapdata.CellWidth)
	for r := rowMin; r <= rowMax; r++ {
		for c := colMin; c <= colMax; c++ {
			x, y := mapdata.CellOrigin(r, c)
			if s.litPath && s.drawLitCell(screen, r, c, x-s.camX, y-s.camY) {
				continue
			}
			for _, b := range s.m.Cells[r][c] {
				if b != 0 && int(b) <= len(s.tiles) && s.tiles[b-1] != nil {
					s.drawTinted(screen, s.tiles[b-1], x-s.camX, y-s.camY, false)
				}
			}
		}
	}
	// Flat decorations follow the terrain pass (VA 0x00425A95).
	for _, d := range s.decorations {
		t := s.tiles[d.Tile]
		if s.visible(d.X, d.Y, t.Bounds().Dx(), t.Bounds().Dy()) {
			s.drawTinted(screen, t, d.X-s.camX, d.Y-s.camY, false)
		}
	}
	s.drawSelection(screen)
	s.drawCorpses(screen)
	s.drawPiles(screen)
	// Shadows are unioned, then every covered pixel is halved once (0x0043E5DF).
	s.shadowLayer.Clear()
	drawShadow := func(f *sceneFrame) {
		if f.shadow != nil && s.visible(f.sx, f.sy, f.shadow.Bounds().Dx(), f.shadow.Bounds().Dy()) {
			drawAt(s.shadowLayer, f.shadow, f.sx-s.camX, f.sy-s.camY)
		}
	}
	for i := range s.objects {
		drawShadow(s.objects[i].frame())
	}
	// Characters move, so they are depth-sorted each frame and merged into
	// the static objects' order.
	var chars []sceneFrame
	var owners []int // entity index per chars entry
	addChar := func(w *walker) {
		if f, ok := walkerFrame(w, s.Tick); ok && s.visible(f.x-64, f.y-64, f.img.Bounds().Dx()+128, f.img.Bounds().Dy()+128) {
			f.raw = s.lit.Lit(w.Cell.Row, w.Cell.Col) // 0x00427285
			chars = append(chars, f)
			owners = append(owners, w.Entity)
			drawShadow(&chars[len(chars)-1])
		}
	}
	for _, m := range s.Members {
		if m != nil && !s.Party[m.K].Dead() {
			addChar(&m.Walker)
		}
	}
	for _, n := range s.NPCs {
		if !s.Game.Entities[n.Entity].Dead() { // corpses: drawCorpses
			addChar(&n.Walker)
		}
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
	if s.shadows {
		op := &ebiten.DrawImageOptions{}
		op.ColorScale.Scale(0, 0, 0, 0.5)
		screen.DrawImage(s.shadowLayer, op)
	}

	// Static objects are depth-sorted by their bottom edge; characters are
	// merged in where their own bottom edge falls.
	drawFrame := func(f *sceneFrame) {
		if s.visible(f.x, f.y, f.img.Bounds().Dx(), f.img.Bounds().Dy()) {
			s.drawTinted(screen, f.img, f.x-s.camX, f.y-s.camY, f.raw)
		}
	}
	// The flames advance once per drawn frame (0x00427185). The game draws
	// a frame every 60 ms (0x00A3E6F4, set at 0x0042B9C4, paced with
	// timeGetTime at 0x0042BD97); the port updates 60 times a second, so
	// the game's frame count is updates · (1000/60) / 60 = updates · 5/18.
	gameFrame := s.frame * 5 / 18
	advanceFlames := gameFrame != s.flameFrame
	s.flameFrame = gameFrame
	ci := 0
	for i := range s.objects {
		for ci < len(chars) && depth(&chars[ci]) < s.objects[i].depth {
			drawFrame(&chars[ci])
			ci++
		}
		drawFrame(s.objects[i].frame())
		s.drawFlames(screen, &s.objects[i], advanceFlames)
	}
	for ; ci < len(chars); ci++ {
		drawFrame(&chars[ci])
	}
	s.drawArrows(screen)
	return chars, owners
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
func (s *startScene) Entity(i int) gamestate.Entity {
	for _, e := range s.Party {
		if e.Index == i {
			return e
		}
	}
	return s.Game.Entities[i]
}

func toEbiten(img *image.RGBA) *ebiten.Image { return ebiten.NewImageFromImage(img) }

// installHooks connects the simulation to this scene (world.Hooks): sounds,
// the status line, the windows, the buildings' sprites, the map change,
// and the animated objects.
func (s *startScene) installHooks() {
	s.Hooks = world.Hooks{
		PlaySound:        s.playAt,
		ActionSound:      s.actionSound,
		Message:          s.showMessage,
		WindowOpen:       func() bool { return s.inv != nil && s.inv.open },
		TradeOpen:        func() bool { return s.trade.open() },
		OpenPileWindow:   s.openPileWindow,
		BuildingsChanged: s.updateBuildings,
		BuildingHit: func(p int) func(x, y int) bool {
			if o := s.buildingObject(p); o != nil {
				return o.buildingHit
			}
			return nil
		},
		ChangeMap: s.changeMap,
		Animate:   s.animateObjects,
	}
}
