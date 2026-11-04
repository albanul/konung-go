package game

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/resfmt"
)

// In-game HUD (800x600 layout): the INTR frame with the 2x5 party portrait
// grid, HP bars and panel buttons. Slot and state rules live in
// gamestate (portrait.go); positions are the 800x600 columns of the exe
// tables named below.
const (
	// The game view inside the frame (globals 0x00A3E874, 0x00A3E890,
	// 0x00A3E880, 0x00A3E89C set at 0x00427C10).
	viewLeft, viewTop, viewRight, viewBottom = 140, 0, 799, 555
	hpBarDX, hpBarDY, hpBarHeight            = 6, 10, 40 // 0x00465C34, 0x00462F34
)

// portraitCells are the 10 grid positions (0x0045AF70, 16 bytes per cell,
// 800x600 pair). Cell k shows the party's k-th entity.
var portraitCells = [10][2]int{
	{6, 4}, {74, 4}, {6, 71}, {74, 71}, {6, 139},
	{74, 139}, {6, 206}, {74, 206}, {6, 273}, {74, 273},
}

// buttonPos are the 7 panel buttons (0x0045AF00, same layout).
var buttonPos = [7][2]int{{3, 340}, {1, 374}, {72, 372}, {1, 455}, {78, 457}, {1, 514}, {70, 514}}

type hud struct {
	res      *resfmt.InterfaceRes
	palettes [][256]color.RGBA
	cache    map[int]*ebiten.Image
}

func newHUD(intr, graph []byte) (*hud, error) {
	res, err := resfmt.ParseInterface(intr)
	if err != nil {
		return nil, err
	}
	pals, err := resfmt.Palettes(graph)
	if err != nil {
		return nil, err
	}
	return &hud{res: res, palettes: pals, cache: map[int]*ebiten.Image{}}, nil
}

// slot returns the uploaded image of an INTR slot (nil if it can't be decoded).
func (h *hud) slot(n int) *ebiten.Image {
	if img, ok := h.cache[n]; ok {
		return img
	}
	var out *ebiten.Image
	if img, err := h.res.Image(n, h.palettes); err == nil {
		out = toEbiten(img)
	} else {
		fmt.Println("hud:", err)
	}
	h.cache[n] = out
	return out
}

func (h *hud) draw(dst *ebiten.Image, n, x, y int) {
	if img := h.slot(n); img != nil {
		drawAt(dst, img, x, y)
	}
}

// inView reports whether a screen point lies in the game view, not on the HUD.
func inView(x, y int) bool {
	return x >= viewLeft && x <= viewRight && y >= viewTop && y <= viewBottom
}

// Draw draws the frame, the buttons and one portrait per party member, like
// the HUD redraw at 0x0042DBD3: frame, buttons (0x0042DA30), then the
// portrait routine 0x0042D574 for each member. party[0] is the hero;
// selected marks members in the selection list (0x0083B93C).
func (h *hud) Draw(dst *ebiten.Image, party []gamestate.Entity, selected func(k int) bool) {
	h.draw(dst, gamestate.HUDFrameSlot, 0, 0)
	if len(party) > 0 {
		for k, n := range gamestate.HUDButtonSlots(party[0]) {
			h.draw(dst, n, buttonPos[k][0], buttonPos[k][1])
		}
	}
	for k, e := range party {
		if k >= len(portraitCells) {
			break
		}
		px, py := portraitCells[k][0], portraitCells[k][1]
		if n := gamestate.PortraitSlot(e, k == 0); n >= 0 {
			h.draw(dst, n, px, py)
		}
		if selected(k) {
			h.draw(dst, gamestate.HUDSelectedSlot, px, py)
		}
		// The bar is drawn whole and clipped to its bottom rows.
		if n, rows, ok := gamestate.HPBar(e, hpBarHeight); ok {
			if bar := h.slot(n); bar != nil {
				b := bar.Bounds()
				sub := bar.SubImage(image.Rect(b.Min.X, b.Max.Y-rows, b.Max.X, b.Max.Y)).(*ebiten.Image)
				drawAt(dst, sub, px+hpBarDX, py+hpBarDY+hpBarHeight-rows)
			}
		}
	}
}
