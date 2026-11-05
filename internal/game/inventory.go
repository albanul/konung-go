package game

import (
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/resfmt"
)

// charWindow is the character / inventory window (window 3), toggled with I
// or the HUD bag button (0x00424610). It shows the selected character
// (0x0083B93C[0]; the hero in the port): window art, name and title,
// equipment icons, jewellery, the 25 sheet numbers (0x0042E04C) and the bag
// strip along the bottom of the view (0x0042F50C). Items move with the hand
// (gamestate.Hand); the in-hand box shows gamestate.Hand.Box. The boxed item
// stays in its bag cell or doll box, but every draw skips it (0x0042E29F,
// 0x0042E3EF, 0x0042E4B7, 0x0042F72F), so it is shown in the box only.
//
// The game draws windows into the screen buffer once and redraws on
// changes; whether the map keeps animating underneath is not traced. The
// port draws the map, then the HUD, then the window every frame.
type charWindow struct {
	open   bool
	quick  bool // ~: the bag strip alone (0x004371D2); restored when the window closes
	scroll int  // first visible bag index (0x00A3E840), reset on open
	arrow  int  // arrow drawn pressed: -1 left, +1 right (0x0042F50C's argument)
	until  int  // tick until which the arrow stays pressed

	layout resfmt.CharWindow
	strip  bagStrip
	large  *gameFont
	small  *gameFont
}

// Bag strip INTR slots (0x0042F50C): cell background, arrows.
const (
	bagCellSlot        = 17
	bagLeftSlot        = 0x12 // 0x13 while pressed
	bagRightSlot       = 0x14 // 0x15 while pressed
	hotspotBagLeft     = 120
	hotspotBagRight    = 121
	charWindowBagSlots = 32
)

// bagStrip is the strip geometry computed at INTR load (0x0042D2D8–0x0042D396)
// from the cell (slot 17) and arrow (slot 0x12) image sizes.
type bagStrip struct {
	cellW         int // 0x00A3E860: cell width − 1 (cells overlap by a pixel)
	arrowW, cellH int // 0x00A3E864, 0x00A3E784: arrow width and height
	x0, y         int // 0x00A3E870, 0x00A3E88C: first cell x, strip top
	visible       int // cells that fit between the arrows
}

func newBagStrip(cellW, arrowW, arrowH int) bagStrip {
	b := bagStrip{cellW: cellW - 1, arrowW: arrowW, cellH: arrowH}
	width := viewRight - viewLeft + 2 - 2*arrowW
	b.visible = width / b.cellW
	b.x0 = viewLeft - 1 + arrowW + (width-b.visible*b.cellW)/2
	b.y = viewBottom - arrowH + 1
	return b
}

func newCharWindow(h *hud, t *resfmt.ExeTables, large, small *gameFont) *charWindow {
	w := &charWindow{layout: t.CharWindow800(), large: large, small: small}
	cell, arrow := h.slot(bagCellSlot), h.slot(bagLeftSlot)
	if cell != nil && arrow != nil {
		w.strip = newBagStrip(cell.Bounds().Dx(), arrow.Bounds().Dx(), arrow.Bounds().Dy())
	}
	return w
}

// toggle opens or closes the window, as 0x00424610 does. Closing restores
// the strip's state from before the window (0x00A3E730 = 0x00A3E760).
func (w *charWindow) toggle() {
	w.open = !w.open
	w.scroll, w.arrow = 0, 0
}

// toggleQuick shows or hides the bag strip alone (~, 0x004371D2: only with
// no window open; showing resets the scroll).
func (w *charWindow) toggleQuick() {
	w.quick = !w.quick
	w.scroll, w.arrow = 0, 0
}

// showStrip reports whether the bag strip is on screen.
func (w *charWindow) showStrip() bool { return w.open || w.quick }

// hit reports a point in the bag cells area as the hotspot routine tests it
// (0x00439600: right of the first cell, between strip top and view bottom).
func (b *bagStrip) hit(x, y int) bool { return x > b.x0 && y > b.y && y < viewBottom }

// scrollBy moves the bag strip one cell (hotspots 120/121, 0x004235E4 and
// 0x0042367F): left stops at 0; right only while the bag slot just past the
// last visible cell holds an item (0x0041EB9C).
func (w *charWindow) scrollBy(e gamestate.Entity, dir, now int) {
	switch {
	case dir < 0:
		w.scroll = max(0, w.scroll-1)
	case w.scroll+w.strip.visible < charWindowBagSlots && e.Bag[w.scroll+w.strip.visible] != 0:
		w.scroll++
	}
	w.arrow, w.until = dir, now+pressTicks
}

// drawQuick draws the strip alone, when the window is closed.
func (w *charWindow) drawQuick(dst *ebiten.Image, h *hud, g *gamestate.Game, t *resfmt.ExeTables, e gamestate.Entity, box int) {
	if w.quick && !w.open {
		w.drawBag(dst, h, g, t, e, box)
	}
}

// itemIcon returns the INTR slot of item i's icon by its category (0x0042C870).
func itemIcon(g *gamestate.Game, t *resfmt.ExeTables, i int) int {
	it, ok := g.Item(i)
	if !ok {
		return -1
	}
	return t.ItemIcon(it.Table(), it.Type)
}

// gearIcon is the icon the window uses for equipment and the +0xCA item
// (gear table, 0x00459BE8 + type·64).
func gearIcon(g *gamestate.Game, t *resfmt.ExeTables, i int) int {
	it, ok := g.Item(i)
	if !ok {
		return -1
	}
	return t.ItemIcon(resfmt.ItemTableGear, it.Type)
}

func (w *charWindow) draw(dst *ebiten.Image, h *hud, g *gamestate.Game, t *resfmt.ExeTables, e gamestate.Entity, box int) {
	l := &w.layout
	h.draw(dst, resfmt.CharWindowBackground, viewLeft, viewTop)
	centre := (viewLeft + viewRight) / 2
	name := gamestate.Name(e, t)
	w.large.drawLine(dst, name, centre-w.large.font.Width(name)/2, l.NameY)
	if !e.Animated() {
		title := t.CharWindowTitle(int(e.Flags4 & 7))
		w.large.drawLine(dst, title, centre-w.large.font.Width(title)/2, l.TitleY)
	}
	// Equipment: centred horizontally in a cell width, top at the table y.
	shown := gamestate.SheetEquipment(e)
	for slot, p := range l.Equipment {
		if !shown[slot] || e.Equipment[slot] == box {
			continue
		}
		if img := h.slot(gearIcon(g, t, e.Equipment[slot])); img != nil {
			drawAt(dst, img, p.X+(w.strip.cellW+1-img.Bounds().Dx())/2, p.Y)
		}
	}
	if img := h.slot(gearIcon(g, t, e.Quiver)); e.Quiver != 0 && e.Quiver != box && img != nil {
		drawAt(dst, img, l.ItemCA.X, l.ItemCA.Y)
	}
	// Jewellery: centred on the table point, icons from the jewel table.
	for i, p := range l.Extras {
		it, ok := g.Item(e.Extra[i])
		if !ok || e.Extra[i] == box {
			continue
		}
		if img := h.slot(t.ItemIcon(resfmt.ItemTableJewel, it.Type)); img != nil {
			b := img.Bounds()
			drawAt(dst, img, p.X-b.Dx()/2, p.Y-b.Dy()/2)
		}
	}
	// The in-hand box: the icon by category, centred (0x0042E5C0).
	if img := h.slot(itemIcon(g, t, box)); box != 0 && img != nil {
		b := img.Bounds()
		drawAt(dst, img, l.InHand.X-b.Dx()/2, l.InHand.Y-b.Dy()/2)
	}
	for i, v := range g.CharSheet(e, t) {
		if i != gamestate.SheetFreeExp {
			w.small.drawLine(dst, []byte(v), l.Stats[i].X, l.Stats[i].Y)
		}
	}
	w.drawRaise(dst, h, g, t, e)
	w.drawBag(dst, h, g, t, e, box)
}

// drawRaise draws the raise column (0x0042F1F5–0x0042F48F): «Свободный опыт»
// centred in the button's width; per row with a nonzero RaiseCost the
// button (and on the stat rows the cost to its left); then the auto-level
// toggle.
func (w *charWindow) drawRaise(dst *ebiten.Image, h *hud, g *gamestate.Game, t *resfmt.ExeTables, e gamestate.Entity) {
	l := &w.layout
	btn := h.slot(resfmt.RaiseButtonSlot)
	if btn == nil {
		return
	}
	free := []byte(g.CharSheet(e, t)[gamestate.SheetFreeExp])
	w.small.drawLine(dst, free, l.Raise[0].X+(btn.Bounds().Dx()-w.small.font.Width(free))/2, l.Stats[gamestate.SheetFreeExp].Y)
	for row, c := range g.RaiseCosts(e, t) {
		if c == 0 {
			continue
		}
		p, dx := l.Raise[row], 0
		if row > 0 {
			w.small.drawLine(dst, []byte(strconv.Itoa(c)), p.X, p.Y)
			dx = resfmt.RaiseButtonDX
		}
		drawAt(dst, btn, p.X+dx, p.Y+resfmt.RaiseButtonDY)
	}
	toggle := resfmt.AutoLevelOffSlot
	if e.AutoLevel() {
		toggle = resfmt.AutoLevelOnSlot
	}
	h.draw(dst, toggle, l.AutoLevel.X, l.AutoLevel.Y)
}

// drawBag draws the bag strip (0x0042F50C): arrows at both ends, then one
// cell per visible bag slot with the item icon centred in it. The in-hand
// box's item leaves a plain empty cell (0x0042F72F).
func (w *charWindow) drawBag(dst *ebiten.Image, h *hud, g *gamestate.Game, t *resfmt.ExeTables, e gamestate.Entity, box int) {
	s := &w.strip
	left, right := bagLeftSlot, bagRightSlot
	if h.now < w.until {
		if w.arrow < 0 {
			left++
		} else if w.arrow > 0 {
			right++
		}
	}
	h.draw(dst, left, viewLeft-1, s.y)
	h.draw(dst, right, viewRight-s.arrowW+1, s.y)
	cell := h.slot(bagCellSlot)
	if cell == nil {
		return
	}
	cy := s.y + (s.cellH-cell.Bounds().Dy())/2
	for k := 0; k < s.visible; k++ {
		x := s.x0 + k*s.cellW
		item := e.Bag[(w.scroll+k)&(charWindowBagSlots-1)]
		it, ok := g.Item(item)
		ok = ok && item != box
		bg := cell
		if ok {
			// The cell is tinted when the item is unusable, flagged or enchanted (0x0042C914).
			if r, gr, b, tint := g.CellTint(e, t, it); tint {
				if img := h.slotTinted(bagCellSlot, [3]int{r, gr, b}); img != nil {
					bg = img
				}
			}
		}
		drawAt(dst, bg, x, cy)
		if !ok {
			continue
		}
		if img := h.slot(itemIcon(g, t, item)); img != nil {
			b := img.Bounds()
			drawAt(dst, img, x+(s.cellW+1-b.Dx())/2, cy+(s.cellH+1-b.Dy())/2)
		}
	}
}
