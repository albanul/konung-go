package gamestate

// Party HUD rules: portrait and HP bar selection from the portrait routine
// at VA 0x0042D574, and the panel button slots set by 0x004290D8. Slot
// numbers index the INTR*R.RES table (resfmt.InterfaceRes).

const (
	// HPFull is 100 % health in entity +0x5C (the bar divides by 0x640).
	HPFull = 1600

	HUDFrameSlot     = 3   // full-screen frame with the portrait grid (0x0042DBFF)
	HUDSelectedSlot  = 173 // frame drawn over a selected character's portrait
	hudHeroPortrait  = 0xD7
	hudOtherPortrait = 0xC3
	portraitsPerBody = 25 // 5 faces x 5 damage states per Type 0 body model
	hpBarGreen       = 0xAE
	hpBarPoison      = 0xAF
	hpBarRed         = 0xB0
	hpBarYellow      = 0xB1
)

// Dead reports the dead flag (entity +4 bit 0x80) used by the HUD.
func (e Entity) Dead() bool { return e.Flags4&0x80 != 0 }

// HitTint is the palette tint (red, green, blue percentages for
// resfmt.TintPalette) of a party member's portrait while it plays its hit
// reaction (type 2 or 8): 0x0042D574 calls 0x0043FD18(0x18, 0, 0) and draws
// with the palette 0x0043F763 builds.
var HitTint = [3]int{24, 0, 0}

// PortraitState is the damage state 0 (healthy) .. 4 (dead): HP > 1200 → 0,
// > 800 → 1, > 400 → 2, otherwise 3. The thresholds are absolute.
func PortraitState(e Entity) int {
	switch {
	case e.Dead():
		return 4
	case e.HP <= 400:
		return 3
	case e.HP <= 800:
		return 2
	case e.HP <= 1200:
		return 1
	}
	return 0
}

// PortraitSlot returns the INTR slot of e's portrait, or -1 when the game
// draws none (animated OBJECTS.RES models). The block is chosen by the Type 0
// body model (+0xD6, 25 slots each). The hero uses face 4; companions use
// face (+4 & 7), which is 1, 2, 3 or 5 for GAME.0 villagers. The portrait
// table at 0x0062F3D8, used when 0x0062F3E0 is set, is not ported.
func PortraitSlot(e Entity, hero bool) int {
	if e.Animated() {
		return -1
	}
	base := hudOtherPortrait + int(e.Flags4&7)*5
	if hero {
		base = hudHeroPortrait
	}
	return base + e.Model*portraitsPerBody + PortraitState(e)
}

// HPBar returns the bar slot (3 x 40 images) and how many of its bottom
// rows are shown for a bar of full height h: HP*h/1600, at least 1. ok is
// false when HP is 0 (no bar).
func HPBar(e Entity, h int) (slot, rows int, ok bool) {
	if e.HP == 0 {
		return 0, 0, false
	}
	rows = max(1, e.HP*h/HPFull)
	switch {
	case e.Poison != 0:
		slot = hpBarPoison
	case e.HP <= 400:
		slot = hpBarRed
	case e.HP <= 800:
		slot = hpBarYellow
	default:
		slot = hpBarGreen
	}
	return slot, rows, true
}

// HUDButtonSlots returns the 7 panel button images (bytes 0x00A3E090..96),
// as 0x004290D8 sets them from the hero: weapon mode (+0x12: 0 → 8, 1 → 9,
// else 16), combat stance (+3 bit 0x04 → 10, else 15), then 14, 11, 7, 12, 13.
func HUDButtonSlots(hero Entity) [7]int {
	b := [7]int{8, 15, 14, 11, 7, 12, 13}
	switch hero.WeaponMode {
	case 0:
	case 1:
		b[0] = 9
	default:
		b[0] = 16
	}
	if hero.Flags3&0x04 != 0 {
		b[1] = 10
	}
	return b
}

// Selection marker (0x00426146): every member of the selection list
// 0x0083B93C gets an ellipse drawn flat under its feet, in the terrain pass
// before the ground items. The colour follows HP (int16 +0x5C): ≤ 400 red
// (INTR slot 172), ≤ 800 yellow (126), otherwise green (178).
const (
	selectionRed    = 172
	selectionYellow = 126
	selectionGreen  = 178
)

// SelectionMarker returns the INTR slot of e's selection ellipse.
func SelectionMarker(e Entity) int {
	switch {
	case e.HP <= 400:
		return selectionRed
	case e.HP <= 800:
		return selectionYellow
	}
	return selectionGreen
}

// SelectionOffset returns the marker's top-left relative to the entity's
// cell centre: (−29, −16), moved along the facing while the entity walks
// (type 7 or 1) by its animation step s (jump table 0x00426420): W/E 5·s,
// N/S 2·s + s/2 vertically, diagonals 2·s + s/2 across and s + s/2 down/up.
func SelectionOffset(facing, step int, walking bool) (dx, dy int) {
	dx, dy = -29, -16
	if !walking {
		return dx, dy
	}
	long, short := 2*step+step/2, step+step/2
	switch facing & 7 {
	case 0: // W
		dx -= 5 * step
	case 1: // NW
		dx, dy = dx-long, dy-short
	case 2: // N
		dy -= long
	case 3: // NE
		dx, dy = dx+long, dy-short
	case 4: // E
		dx += 5 * step
	case 5: // SE
		dx, dy = dx+long, dy+short
	case 6: // S
		dy += long
	case 7: // SW
		dx, dy = dx-long, dy+short
	}
	return dx, dy
}
