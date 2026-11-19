package game

import (
	"image"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/resfmt"
)

// Map button (hotspot 3, «Карта (M)»): 0x00422C01 and the M key 0x00436EB9
// press button 2 with image 14.
const (
	hudMapButton = 2
	hudMapImage  = 14
)

// worldMapWindow is window 5, the world map (resfmt.WorldMap): opened by
// the M key or HUD button 2 when no other window is open, closed by the
// same or Escape. Like the other windows it tiles the view with INTR slot 5
// when background tiling is on (dispatcher case 5, 0x0042F491); the draw
// 0x00428698 follows.
type worldMapWindow struct {
	open bool
	m    resfmt.WorldMap

	// overlay is the composed known regions (resfmt.WorldMap.Compose),
	// rebuilt when the known flags or the current map change.
	overlay *ebiten.Image
	known   [resfmt.WorldMapRegions]byte
	here    int
	// counts are the players per LAN map (the own hero and the remote
	// heroes, startScene.playerCounts), set before each draw.
	counts map[int]int
	// amuletIcons and amuletLine are the LAN map's amulet status (draw
	// 0x0041ED44), set before each draw: the three amulet items' icons
	// (INTR slots, −1 none) and the line "славяне : викинги : византийцы
	// minutes:seconds" (nil: no status).
	amuletIcons [3]int
	amuletLine  []byte
	// lan, when set, is the LAN version's map (draw 0x0041E8EC): its own
	// picture (INTO640R.RES slot 179) and runs, every location shown,
	// regions = map numbers, no travel by click.
	lan *lanWorldMap
}

// lanWorldMap is the LAN map window's data (gamestate.LANExe.Map and
// MapMarkers, the INTO640R.RES picture plain and tinted for the current map).
type lanWorldMap struct {
	m               resfmt.WorldMap
	markers         [gamestate.LANMaps + 1]image.Point
	reveal, current image.Image
}

// newLANWorldMap reads the LAN map picture (INTO640R.RES slot 179, drawn
// with its GRAPH.RES palette) for the LAN map tables of exe.
func newLANWorldMap(exe *gamestate.LANExe, into, graph []byte) (*lanWorldMap, error) {
	res, err := resfmt.ParseInterface(into)
	if err != nil {
		return nil, err
	}
	pal, err := resfmt.Palettes(graph)
	if err != nil {
		return nil, err
	}
	reveal, err := res.Image(resfmt.WorldMapRevealSlot, pal)
	if err != nil {
		return nil, err
	}
	current, err := res.ImageTinted(resfmt.WorldMapRevealSlot, pal, gamestate.LANMapTint, 0, 0)
	if err != nil {
		return nil, err
	}
	return &lanWorldMap{m: exe.Map, markers: exe.MapMarkers, reveal: reveal, current: current}, nil
}

func newWorldMapWindow(t *resfmt.ExeTables) *worldMapWindow {
	return &worldMapWindow{m: t.WorldMap(1)}
}

// regionAt is the window's hit test (0x0043946E): the map number under
// screen point (x, y) when the party knows it, else 0. inArea is false
// outside the map area, where the normal hotspots apply.
func (w *worldMapWindow) regionAt(g *gamestate.Game, x, y int) (m int, inArea bool) {
	if w.lan != nil {
		// The LAN hit test (0x004176AB) only names the location for the
		// status line; there is no travel and no single-player name.
		_, ok := w.lan.m.RegionAt(x-viewLeft-w.lan.m.Area.Min.X, y-viewTop-w.lan.m.Area.Min.Y)
		return 0, ok
	}
	region, ok := w.m.RegionAt(x-viewLeft-w.m.Area.Min.X, y-viewTop-w.m.Area.Min.Y)
	if !ok {
		return 0, false
	}
	if !g.MapKnown(region + 1) {
		return 0, true
	}
	return region + 1, true
}

// draw tiles the view (tiled: the T setting), draws the map picture (slot 4)
// at the view's top-left and the known locations over it, the current one
// tinted red.
func (w *worldMapWindow) draw(dst *ebiten.Image, h *hud, g *gamestate.Game, mapNumber int, tiled bool, font *gameFont) {
	if !w.open {
		return
	}
	if tiled {
		tileBackground(dst, h, viewLeft, viewTop, viewRight, viewBottom)
	}
	view := dst.SubImage(image.Rect(viewLeft, viewTop, viewRight+1, viewBottom+1)).(*ebiten.Image)
	h.draw(view, resfmt.WorldMapSlot, viewLeft, viewTop)
	if w.lan != nil {
		w.drawLAN(view, mapNumber, font)
		w.drawAmulets(dst, h, font)
		return
	}
	if w.overlay == nil || w.known != g.KnownMaps || w.here != mapNumber {
		w.known, w.here = g.KnownMaps, mapNumber
		w.overlay = nil
		reveal, err1 := h.res.Image(resfmt.WorldMapRevealSlot, h.palettes)
		current, err2 := h.res.ImageTinted(resfmt.WorldMapRevealSlot, h.palettes, resfmt.WorldMapCurrentTint, 0, 0)
		if err1 == nil && err2 == nil {
			known := func(region int) bool { return g.MapKnown(region + 1) }
			w.overlay = toEbiten(w.m.Compose(reveal, current, known, mapNumber-1))
		}
	}
	if w.overlay != nil {
		drawAt(view, w.overlay, viewLeft+w.m.Area.Min.X, viewTop+w.m.Area.Min.Y)
	}
}

// toggleMap is 0x00422C01 (M, HUD button 2): only with no window or the map
// open; returns a held item, presses the button (with the click) and opens
// or closes the window.
func (s *startScene) toggleMap(hero *gamestate.Entity) {
	if s.inv.open || s.talk.open() || s.trade.open() {
		return
	}
	s.returnHand(hero)
	s.hud.press(hudMapButton, hudMapImage)
	s.wmap.open = !s.wmap.open
}

// mapClick is a left click with the map open: on a known location other
// than the current map, with a «Шар магического прыжка» in the hero's bag,
// the orb is used up and the party travels by that map's first exit
// (gamestate.Game.Travel, 0x00422AD8); the window closes. Clicks elsewhere
// in the view are ignored (0x00422EDF).
func (s *startScene) mapClick(hero *gamestate.Entity, mx, my int) {
	m, _ := s.wmap.regionAt(s.Game, mx, my) // 0 on the LAN map: no travel
	if m == 0 {
		return
	}
	x, ok := s.Game.Travel(hero, m)
	if !ok {
		return
	}
	s.wmap.open = false
	s.changeMap(x)
}

// drawLAN is the LAN map (0x0041E8EC): every location, the current map
// tinted, and the number of players on each map at its marker (+3). The
// counts are the players' groups' maps in the original; here the own hero
// and the other players' heroes on its map (startScene.playerCounts).
func (w *worldMapWindow) drawLAN(view *ebiten.Image, mapNumber int, font *gameFont) {
	l := w.lan
	if l.reveal == nil {
		return
	}
	if w.overlay == nil || w.here != mapNumber {
		w.here = mapNumber
		all := func(int) bool { return true }
		w.overlay = toEbiten(l.m.Compose(l.reveal, l.current, all, mapNumber))
	}
	ox, oy := viewLeft+l.m.Area.Min.X, viewTop+l.m.Area.Min.Y
	drawAt(view, w.overlay, ox, oy)
	if font == nil || mapNumber < 1 || mapNumber > gamestate.LANMaps {
		return
	}
	for m, n := range w.counts {
		if m < 1 || m > gamestate.LANMaps || n == 0 {
			continue
		}
		p := l.markers[m]
		font.drawLine(view, []byte(strconv.Itoa(n)), ox+p.X+3, oy+p.Y+3)
	}
}

// drawAmulets is the LAN map window's amulet status (0x0041ED44): the
// nations' delivered amulets and the session's time left (font 0x0078C1A0)
// and the three amulet items' icons below each other (LAN_KONG.EXE's
// 800×600 places 0x0047EFE4: the line at (678, 5), the icons at x 700, y 15,
// 70, 125, right of its map area). Port rule: the single-player HUD on the
// left moves the map area 140 px right, so the line is drawn right-aligned
// at the screen's right edge and the icons in the column right of the map
// picture, at the original's heights.
func (w *worldMapWindow) drawAmulets(dst *ebiten.Image, h *hud, font *gameFont) {
	if w.amuletLine == nil || font == nil {
		return
	}
	font.drawLine(dst, w.amuletLine, viewRight-4-font.width(w.amuletLine), 5)
	for k, n := range w.amuletIcons {
		img := h.slot(n)
		if n < 0 || img == nil {
			continue
		}
		x := viewRight + 1 - 8 - img.Bounds().Dx()
		drawAt(dst, img, x, [3]int{15, 70, 125}[k])
	}
}
