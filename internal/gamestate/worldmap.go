package gamestate

import "math/rand/v2"

// The world map window (window 5, resfmt.WorldMap) shows the locations the
// party knows: flag byte 0x00A3E0A8[map − 1] (Game.KnownMaps, saved with
// the game). The map loader sets the flag of every map it loads
// (0x0043C12A); misc item type 0 «Потерянная карта» sets the flag of a
// random unknown map (0x00414E95, RevealMap).

// jumpOrbType is misc item type 20 «Шар магического прыжка»: travel by the
// map window needs one in the bag (0x00434C90(0x15)).
const jumpOrbType = 20

// lostMapType is misc item type 0 «Потерянная карта» (UseMisc, RevealMap).
const lostMapType = 0

// neverRevealed is 0x0045AE90: 1 for the maps (index map − 1) the lost map
// never picks, 42–55.
var neverRevealed = func() (b [knownMapsSize]bool) {
	for i := 41; i <= 54; i++ {
		b[i] = true
	}
	return b
}()

// RevealMap is case 0 of 0x00414E64 (0x00414E95): start at rand() % 56 and
// mark the first map, wrapping around, that is neither known nor in
// 0x0045AE90; nothing when every candidate is known.
func (g *Game) RevealMap() {
	n := len(g.KnownMaps)
	r := g.rand(n)
	for range n {
		if g.KnownMaps[r] == 0 && !neverRevealed[r] {
			g.KnownMaps[r] = 1
			return
		}
		r = (r + 1) % n
	}
}

func (g *Game) rand(n int) int {
	if g.Rand != nil {
		return g.Rand(n)
	}
	return rand.IntN(n)
}

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
