package game

import (
	"image"

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
}

func newWorldMapWindow(t *resfmt.ExeTables) *worldMapWindow {
	return &worldMapWindow{m: t.WorldMap(1)}
}

// regionAt is the window's hit test (0x0043946E): the map number under
// screen point (x, y) when the party knows it, else 0. inArea is false
// outside the map area, where the normal hotspots apply.
func (w *worldMapWindow) regionAt(g *gamestate.Game, x, y int) (m int, inArea bool) {
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
func (w *worldMapWindow) draw(dst *ebiten.Image, h *hud, g *gamestate.Game, mapNumber int, tiled bool) {
	if !w.open {
		return
	}
	if tiled {
		tileBackground(dst, h, viewLeft, viewTop, viewRight, viewBottom)
	}
	view := dst.SubImage(image.Rect(viewLeft, viewTop, viewRight+1, viewBottom+1)).(*ebiten.Image)
	h.draw(view, resfmt.WorldMapSlot, viewLeft, viewTop)
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
	s.hand.Return(hero)
	s.hud.press(hudMapButton, hudMapImage)
	s.wmap.open = !s.wmap.open
}

// mapClick is a left click with the map open: on a known location other
// than the current map, with a «Шар магического прыжка» in the hero's bag,
// the orb is used up and the party travels by that map's first exit
// (gamestate.Game.Travel, 0x00422AD8); the window closes. Clicks elsewhere
// in the view are ignored (0x00422EDF).
func (s *startScene) mapClick(hero *gamestate.Entity, mx, my int) {
	m, _ := s.wmap.regionAt(s.game, mx, my)
	if m == 0 {
		return
	}
	x, ok := s.game.Travel(hero, m)
	if !ok {
		return
	}
	s.wmap.open = false
	s.changeMap(x)
}
