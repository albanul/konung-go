package resfmt

import "encoding/binary"

// HUD hotspot maps in konung.exe: one per screen size, selected by the view
// setup (0x00427B54 for 640x480, 0x00427C10 for 800x600, pointers 0x00A3E6AC
// rows / 0x00A3E6B0 runs). Each row is an int32 byte offset into the run data
// (-1 = no hotspots); a row is a list of (id, length) byte pairs, ending at a
// zero length or the screen edge. The lookup is 0x00439A28; the main HUD
// dispatcher 0x004392F4 returns ids 1..7 for the panel buttons (0x00439584).
// Ids seen in the 800x600 map: 1..7 panel buttons in 0x0045AF00 order,
// 30..39 portrait cells, 120/121 the bag strip arrows; the character window's
// 8..19 doll boxes, 40..44 raise buttons and 45 auto-level toggle.
const (
	hotspotRows800VA = 0x004577A0
	hotspotRuns800VA = 0x004543DC
	hotspotHeight800 = 600
)

// HUDHotspot800 returns the hotspot id at screen point (x, y) of the 800x600
// HUD, 0 for none.
func (t *ExeTables) HUDHotspot800(x, y int) int {
	if x < 0 || y < 0 || y >= hotspotHeight800 {
		return 0
	}
	row := int32(binary.LittleEndian.Uint32(t.at(hotspotRows800VA + y*4)))
	if row < 0 {
		return 0
	}
	p := t.at(hotspotRuns800VA + int(row))
	acc := 0
	for i := 0; i+1 < len(p); i += 2 {
		n := int(p[i+1])
		acc += n
		if n == 0 || x < acc {
			return int(p[i])
		}
	}
	return 0
}
