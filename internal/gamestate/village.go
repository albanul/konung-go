package gamestate

import "encoding/binary"

// Village AI, ported from the per-tick dispatcher at VA 0x00421DA4: groups
// whose kind (+0x1E) has bit 0x80 (and not 0x40) run 0x00416058, which asks
// 0x00416934 what each idle member does next. See docs/konung_game_format.md.

const (
	settlementOffset = 0xA769A // 20 x 0x4B5 bytes -> VA 0x00835B18
	settlementCount  = 20
	settlementStride = 0x4B5
	postOffset       = 0x3D0 // 5 x 8-byte posts
	postCount        = 5

	// VillageRowRange and VillageColRange are the random-step half-ranges
	// of 0x00413A84 in 800x600 mode (globals 0x00A3E818 and 0x00A3E7DC, set
	// at 0x00427C10 from the view size: 556/32+3 rows, 660/116+4 columns).
	VillageRowRange = 20
	VillageColRange = 9
	// VillagePathBudget is how many path searches the AI may start per tick
	// (counter 0x00A3E720, checked by 0x004116CC); members are served
	// round-robin.
	VillagePathBudget = 3
)

// Post is a settlement slot that pins a named villager (the elder and the
// role characters) to a cell and facing: 8 bytes at settlement +0x3D0.
type Post struct {
	Slot     int // 1-based slot number (as returned by 0x0041182C)
	Facing   int // +0: facing (variant numbering, 0 W .. 7 SW)
	Dialogue int // +1: the entity's dialogue ID (+0xD0), a copy
	Entity   int // +2 uint16: entity index; 0 = empty slot
	Row, Col int // +4, +6 uint16: fine-grid cell
}

// Settlement is a 0x4B5-byte record of GAME.N section 6; the map loader
// selects the one whose map byte matches (0x0043CAC6, pointer 0x00A3E6A8).
type Settlement struct {
	Index int
	Group int // +2: village group
	Map   int // +3: KONUNG.N number; 0 = unused record
	Posts [postCount]Post
	Fence int  // +1: fence pieces (non-zero = fortified)
	Flags byte // +0x4B2: SettlementHostile, SettlementStrangers, SettlementIndependent, … (anger.go)
}

func parseSettlements(data []byte) []Settlement {
	var out []Settlement
	for i := 0; i < settlementCount; i++ {
		r := data[settlementOffset+i*settlementStride:][:settlementStride]
		s := Settlement{Index: i, Group: int(r[2]), Map: int(r[3]), Fence: int(r[1]), Flags: r[0x4B2]}
		for k := range s.Posts {
			p := r[postOffset+k*8:]
			s.Posts[k] = Post{
				Slot:     k + 1,
				Facing:   int(p[0]),
				Dialogue: int(p[1]),
				Entity:   int(binary.LittleEndian.Uint16(p[2:])),
				Row:      int(binary.LittleEndian.Uint16(p[4:])),
				Col:      int(binary.LittleEndian.Uint16(p[6:])),
			}
		}
		out = append(out, s)
	}
	return out
}

// VillageGroup reports whether group g runs the village AI (kind bit 0x80
// without 0x40; 0x40 is tested first by the dispatcher).
func (g Group) VillageGroup() bool { return g.Kind&0x40 == 0 && g.Kind&0x80 != 0 }

// PostOf returns e's post in the settlement of its group's map, like
// 0x0041182C: the first of the 5 slots whose entity word equals e's index.
func (g *Game) PostOf(e Entity) (Post, bool) {
	if e.Group < 0 {
		return Post{}, false
	}
	m := g.Groups[e.Group].Map
	for _, s := range g.Settlements {
		if s.Map != m || s.Map == 0 {
			continue
		}
		for _, p := range s.Posts {
			if p.Entity != 0 && p.Entity == e.Index {
				return p, true
			}
		}
		return Post{}, false
	}
	return Post{}, false
}

// VillageMoveThreshold is the 0..99 roll at or above which an idle villager
// without a post starts a random walk (0x00416934): women and children
// (entity +4 = 0x52 / 0x51) move on 75..99, everyone else on 50..99.
func VillageMoveThreshold(e Entity) int {
	if e.Flags4 == 0x51 || e.Flags4 == 0x52 {
		return 75
	}
	return 50
}

// VillageStepTarget picks a random walk target like 0x00413A84. The base is
// the entity's cell when it lies inside the group rectangle (bounds
// inclusive), otherwise the rectangle centre. A random quadrant is chosen and
// the base moved by rand()%(2*rowRange) rows and rand()%(2*colRange) columns
// in that quadrant's direction. The target must lie strictly inside the
// rectangle and be free; up to 10 tries. intn(n) returns a value in [0, n).
func VillageStepTarget(row, col int, a Area, rowRange, colRange int, intn func(int) int, blocked func(row, col int) bool) (int, int, bool) {
	off := func(n int) int {
		if n < 2 {
			return 0
		}
		return intn(n)
	}
	for try := 0; try < 10; try++ {
		r, c := row, col
		if !a.Contains(row, col) {
			r, c = (a.RowMin+a.RowMax)/2, (a.ColMin+a.ColMax)/2
		}
		q := intn(4) // jump table 0x00413CE4: 0 (-,-), 1 (-,+), 2 (+,-), 3 (+,+)
		dr, dc := off(2*rowRange), off(2*colRange)
		if q < 2 {
			r -= dr
		} else {
			r += dr
		}
		if q%2 == 0 {
			c -= dc
		} else {
			c += dc
		}
		if r > a.RowMin && r < a.RowMax && c > a.ColMin && c < a.ColMax && !blocked(r, c) {
			return r, c, true
		}
	}
	return 0, 0, false
}
