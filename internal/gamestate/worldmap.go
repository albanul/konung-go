package gamestate

// The world map window (window 5, resfmt.WorldMap) shows the locations the
// party knows: flag byte 0x00A3E0A8[map − 1] (Game.KnownMaps, saved with
// the game). The map loader sets the flag of every map it loads
// (0x0043C12A); misc item type 0 sets the flag of a random unknown map
// (0x00414E95, not ported).

// jumpOrbType is misc item type 20 «Шар магического прыжка»: travel by the
// map window needs one in the bag (0x00434C90(0x15)).
const jumpOrbType = 20

// KnowMap marks map m (≥ 1) as known, like the map loader (0x0043C12A).
func (g *Game) KnowMap(m int) {
	if m >= 1 && m <= len(g.KnownMaps) {
		g.KnownMaps[m-1] = 1
	}
}

// MapKnown reports whether map m (≥ 1) is known.
func (g *Game) MapKnown(m int) bool {
	return m >= 1 && m <= len(g.KnownMaps) && g.KnownMaps[m-1] != 0
}

// Travel is the map window's click on known map m (0x00422AD8, hotspot
// m << 8 | 3): nothing when the party is already on m or e carries no
// «Шар магического прыжка» (the first bag item of category 11, type 20,
// 0x00434C90). Otherwise the orb is used up (0x00421D24: removed from the
// bag, the rest shifts left) and the first map exit in record order whose
// target is m is returned, for the caller to close the window and take it
// (TakeExit). With no such exit the orb is still gone and ok is false.
func (g *Game) Travel(e *Entity, m int) (x MapExit, ok bool) {
	if g.Groups[0].Map == m {
		return MapExit{}, false
	}
	slot := -1
	for i, item := range e.Bag {
		if it, found := g.Item(item); item != 0 && found && it.Category == 11 && it.Type == jumpOrbType {
			slot = i
			break
		}
	}
	if slot < 0 {
		return MapExit{}, false
	}
	copy(e.Bag[slot:], e.Bag[slot+1:])
	e.Bag[len(e.Bag)-1] = 0 // +0xB0
	for _, x := range g.Exits {
		if x.To == m {
			return x, true
		}
	}
	return MapExit{}, false
}
