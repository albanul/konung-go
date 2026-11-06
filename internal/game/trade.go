package game

import (
	"image"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

// tradeWindow is window 7 (0x00431D70): trading with a character (dialogue
// script 275) or the pile window (0x004170A0), opened when a pick-up stops
// at the bag or weight limit. Four strips (the bag, the offer, the other
// side's goods, the take), «Ok» and «Закрыть», the money of both sides.
// The rules are in gamestate (trade.go).
type tradeWindow struct {
	tr     *gamestate.Trade
	layout resfmt.TradeLayout
	strip  bagStrip // cell and arrow sizes of the bag strip (0x00A3E860, 0x00A3E864, 0x00A3E784)
	small  *gameFont
	large  *gameFont
	arrow  int // pressed arrow: ±(list + 1), 0 = none (0x00431D70's argument)
	until  int
}

// tradeVisible is the number of cells per strip: 5 + video mode (1 at
// 800×600), 0x0042CD43.
const tradeVisible = 6

// Hit-test results of window 7 (0x004392F4): a strip's arrows and cells,
// and the two buttons (hotspots 0xC8 «Закрыть», 0xC9 «Ok»).
const (
	tradeHitNone = iota
	tradeHitLeft
	tradeHitRight
	tradeHitCell
	tradeHitClose
	tradeHitOk
)

func newTradeWindow(t *resfmt.ExeTables, strip bagStrip, small, large *gameFont) *tradeWindow {
	return &tradeWindow{layout: t.TradeLayout(1), strip: strip, small: small, large: large}
}

// hit is the window-7 part of 0x004392F4: inside list k's rectangle, left
// of the first cell is its left arrow, from the end of the last cell its
// right arrow, otherwise cell (x − x1 − arrow width) / cell width; then the
// buttons.
func (w *tradeWindow) hit(x, y int) (kind, list, cell int) {
	p := image.Pt(x, y)
	for k, r := range w.layout.Lists {
		if !p.In(r) {
			continue
		}
		x0 := r.Min.X + w.strip.arrowW
		switch {
		case x < x0:
			return tradeHitLeft, k, 0
		case x >= x0+tradeVisible*w.strip.cellW:
			return tradeHitRight, k, 0
		}
		return tradeHitCell, k, (x - x0) / w.strip.cellW
	}
	if p.In(w.layout.Buttons[0]) {
		return tradeHitClose, 0, 0
	}
	if p.In(w.layout.Buttons[1]) {
		return tradeHitOk, 0, 0
	}
	return tradeHitNone, 0, 0
}

// itemAt returns the item under the mouse (the hover description, 0x004241A1).
func (w *tradeWindow) itemAt(x, y int) int {
	if kind, k, c := w.hit(x, y); kind == tradeHitCell {
		return w.tr.Lists[k][(w.tr.Scroll[k]+c)&(gamestate.TradeSlots-1)]
	}
	return 0
}

// draw is 0x00431D70: the tiled background (with T), INTR slot 6 over the
// view, the buttons, the trader's name, the four strips (0x0042CCD4) and
// the six numbers, after balancing the coins.
func (w *tradeWindow) draw(dst *ebiten.Image, h *hud, g *gamestate.Game, t *resfmt.ExeTables, tiled bool) {
	tr, l := w.tr, &w.layout
	view := dst.SubImage(image.Rect(viewLeft, viewTop, viewRight+1, viewBottom+1)).(*ebiten.Image)
	if tiled {
		tileBackground(dst, h, viewLeft, viewTop, viewRight, viewBottom)
	}
	h.draw(view, resfmt.TradeBackgroundSlot, viewLeft, viewTop)
	h.draw(view, resfmt.TradeCloseSlot, l.Buttons[0].Min.X, l.Buttons[0].Min.Y)
	h.draw(view, resfmt.TradeOkSlot, l.Buttons[1].Min.X, l.Buttons[1].Min.Y)
	if tr.NPC >= 0 {
		name := gamestate.Name(g.Entities[tr.NPC], t)
		w.large.drawLine(view, name, (viewLeft+viewRight)/2-w.large.font.Width(name)/2, l.NameY)
	}
	shown := g.Entities[tr.Shown]
	for k := range tr.Lists {
		pressed := 0
		if h.now < w.until && (w.arrow == k+1 || w.arrow == -(k+1)) {
			pressed = w.arrow
		}
		w.drawList(view, h, g, t, shown, k, pressed)
	}
	g.Balance(t, tr)
	heroTotal, otherTotal := g.Totals(t, tr)
	for k, v := range []int{tr.OtherMoney, tr.OtherPaid, otherTotal, tr.HeroPaid, tr.HeroMoney, heroTotal} {
		w.small.drawLine(view, []byte(strconv.Itoa(v)), l.Numbers[k].X, l.Numbers[k].Y)
	}
}

// drawList is 0x0042CCD4: list k from its rectangle's top-left: the left
// arrow (slot 0x12, 0x13 pressed), the visible cells (slot 17, tinted for
// the shown character, 0x0042C914) with the icons centred, the right arrow
// (0x14, 0x15 pressed) after them at the cells' height.
func (w *tradeWindow) drawList(dst *ebiten.Image, h *hud, g *gamestate.Game, t *resfmt.ExeTables, shown gamestate.Entity, k, pressed int) {
	s := &w.strip
	x, y := w.layout.Lists[k].Min.X, w.layout.Lists[k].Min.Y
	left, right := bagLeftSlot, bagRightSlot
	if pressed < 0 {
		left++
	} else if pressed > 0 {
		right++
	}
	h.draw(dst, left, x, y)
	x += s.arrowW
	cell := h.slot(bagCellSlot)
	if cell == nil {
		return
	}
	cy := y + (s.cellH-cell.Bounds().Dy())/2
	for c := range tradeVisible {
		cx := x + c*s.cellW
		item := w.tr.Lists[k][(w.tr.Scroll[k]+c)&(gamestate.TradeSlots-1)]
		it, ok := g.Item(item)
		ok = ok && item != 0
		bg := cell
		if ok {
			if r, gr, b, tint := g.CellTint(shown, t, it); tint {
				if img := h.slotTinted(bagCellSlot, [3]int{r, gr, b}); img != nil {
					bg = img
				}
			}
		}
		drawAt(dst, bg, cx, cy)
		if !ok {
			continue
		}
		if img := h.slot(itemIcon(g, t, item)); img != nil {
			b := img.Bounds()
			drawAt(dst, img, cx+(s.cellW+1-b.Dx())/2, cy+(s.cellH+1-b.Dy())/2)
		}
	}
	h.draw(dst, right, x+tradeVisible*s.cellW, cy)
}

// open reports whether window 7 is open.
func (w *tradeWindow) open() bool { return w != nil && w.tr != nil }

// openPileWindow is 0x004170A0 for the hero at pile i (the pick-up stopped
// at the bag or weight limit, 0x0041122A).
func (s *startScene) openPileWindow(i int) {
	s.storeWorld()
	hero := s.party[0].Index
	s.trade.tr = s.game.OpenPileTrade(hero, hero, i)
}

// syncParty reads the party copies back after the trade rules changed the
// records.
func (s *startScene) syncParty() {
	for k := range s.party {
		s.party[k] = s.game.Entities[s.party[k].Index]
	}
}

// tradeClick is a left click in window 7 (0x00422864: hotspots 0x32 cell,
// 0x78/0x79 arrows, 0xC8 «Закрыть», 0xC9 «Ok»). The other buttons are
// ignored while it is open.
func (s *startScene) tradeClick(mx, my int) {
	w := s.trade
	tr := w.tr
	kind, k, c := w.hit(mx, my)
	switch kind {
	case tradeHitCell:
		tr.Move(k, tr.Scroll[k]+c) // 0x004232C0
	case tradeHitLeft:
		tr.ScrollBy(k, -1, tradeVisible) // 0x0042358C
		w.arrow, w.until = -(k + 1), s.frame+pressTicks
	case tradeHitRight:
		tr.ScrollBy(k, 1, tradeVisible) // 0x00423612
		w.arrow, w.until = k+1, s.frame+pressTicks
	case tradeHitClose, tradeHitOk:
		s.applyTrade(kind == tradeHitOk)
	}
}

// applyTrade is the button action (0x004236D9, 0x004236FB): Apply; on
// failure the message shows and the window stays. «Закрыть» then closes;
// «Ok» reopens the window with the new state: the pile window while the
// pile has items (0x004170A0), the trade through script 275 (0x00432FD0).
func (s *startScene) applyTrade(accept bool) {
	tr := s.trade.tr
	s.storeWorld()
	var cell mapdata.Cell
	if tr.Pile >= 0 {
		p := s.game.Piles[tr.Pile]
		cell = mapdata.Cell{Row: p.Row, Col: p.Col}
	}
	msg, ok, removed := s.game.Apply(s.tables, tr, accept, s.gameTime())
	s.syncParty()
	if !ok {
		s.showMessage(s.tables.CString(msg))
		return
	}
	if removed {
		delete(s.pileAt, cell) // 0x0041093C
	}
	switch {
	case !accept, tr.Pile >= 0 && removed:
		s.trade.tr = nil
	case tr.Pile >= 0:
		s.trade.tr = s.game.OpenPileTrade(tr.Hero, tr.Shown, tr.Pile)
	default:
		s.trade.tr = s.game.OpenTrade(tr.Hero, tr.Shown, tr.NPC, tr.Map)
	}
}
