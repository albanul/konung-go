package gamestate

import (
	"encoding/binary"

	"konung1.5/internal/resfmt"
)

// Items lying on the ground ("piles"), GAME.N section 3: 500 × 0x40 records
// loaded to 0x0079EFDC. The loader counts the records up to the first with
// map 0 (0x0043BCA2, count 0x00A3E804). See docs/konung_ground_items.md.

const (
	pileOffset   = 0x21900
	pileCount    = 500
	pileStride   = 0x40
	pileMaxItems = 22 // words +0x14 .. +0x3E

	// PileRespawnTicks is how long a picked nut or root stays away, in game
	// ticks (0x00A3E770, one per AI tick): the map loader skips the pile
	// while time − pick-up time < 5400 (0x0043CF91).
	PileRespawnTicks = 5400

	// MsgSomethingOnGround is the status line over a visible pile (0x00424032).
	MsgSomethingOnGround = 0x0045C37C
	// CursorTake is the cursor sprite over a visible pile on a free cell (0x00428D7F).
	CursorTake = 7

	shovelType = 0x18 // misc item «Лопата» (0x00434C90(0x19) looks for category 11, type 0x19 − 1)
)

// Pile is a section 3 record.
type Pile struct {
	Index    int
	Taken    int // +0x00 int32: −(game time) when a respawning herb was picked
	TakenRef int // +0x04 int32: same value; the pair marks a respawn timer
	Row, Col int // +0x08, +0x0A: fine-grid cell
	// Gold (+0x10 int16) is added to the hero's gold on pick-up. Negative:
	// the pile is buried, not drawn and only taken with a shovel in the bag.
	Gold  int
	Map   int               // +0x12: KONUNG.N number
	Items [pileMaxItems]int // +0x14: item indices, packed from the front
}

func parsePiles(data []byte) []Pile {
	var out []Pile
	for i := range pileCount {
		r := data[pileOffset+i*pileStride:][:pileStride]
		p := Pile{
			Index:    i,
			Taken:    int(int32(binary.LittleEndian.Uint32(r))),
			TakenRef: int(int32(binary.LittleEndian.Uint32(r[4:]))),
			Row:      u16(r, 0x08),
			Col:      u16(r, 0x0A),
			Gold:     s16(r, 0x10),
			Map:      u16(r, 0x12),
		}
		if p.Map == 0 {
			break
		}
		copy(p.Items[:], words(r[0x14:], pileMaxItems))
		out = append(out, p)
	}
	return out
}

// Visible reports whether the pile is drawn and offered (+0x10 ≥ 0).
func (p Pile) Visible() bool { return p.Gold >= 0 }

// Active is the map loader's rule (0x0043CF5B): the pile is on map m and is
// not a respawning herb picked less than PileRespawnTicks ago.
func (p Pile) Active(m, now int) bool {
	if p.Map != m || (p.Items[0] == 0 && p.Gold == 0) { // emptied records: a port guard
		return false
	}
	if p.Taken < 0 && p.Taken == p.TakenRef && now+p.Taken < PileRespawnTicks {
		return false
	}
	return true
}

// PileImage is 0x00439DF0, the INTR slot the pile is drawn with: 0xA3 (a
// heap) for several items or gold, else the single item's ground image —
// the word after its inventory icon (resfmt.ItemGroundImage); arrows 0x99,
// categories 10 and above 12 the heap.
func (g *Game) PileImage(t *resfmt.ExeTables, p Pile) int {
	const heap, arrows = 0xA3, 0x99
	if p.Items[1] != 0 || p.Gold != 0 {
		return heap
	}
	it, ok := g.Item(p.Items[0])
	if !ok {
		return heap
	}
	switch c := it.Category; {
	case c <= 5:
		return t.ItemGroundImage(resfmt.ItemTableGear, it.Type)
	case c <= 8:
		return t.ItemGroundImage(resfmt.ItemTableJewel, it.Type)
	case c == 9:
		return t.ItemGroundImage(resfmt.ItemTableHerb, it.Type)
	case c == 11:
		return t.ItemGroundImage(resfmt.ItemTableMisc, it.Type)
	case c == 12:
		return arrows
	}
	return heap
}

// HasShovel is 0x00434C90(0x19): the hero's bag holds «Лопата».
func (g *Game) HasShovel(hero Entity) bool {
	for _, i := range hero.Bag {
		if it, ok := g.Item(i); ok && it.Category == 11 && it.Type == shovelType {
			return true
		}
	}
	return false
}

// CanPickUp is the arrival check (0x004222EF): a visible pile, or a buried
// one when the hero carries a shovel.
func (g *Game) CanPickUp(p Pile, hero Entity) bool {
	return p.Visible() || g.HasShovel(hero)
}

// PickUp is 0x004110EC for picker e (the hero's gold receives the pile's
// gold; in the port picker and hero are the same entity). Items move one by
// one to the front of e's bag until the weight limit or a full bag stops it
// (message returned; the rest stays in the pile, where the game would open
// the pile window 0x004170A0, not ported). An emptied pile is removed
// (0x0041093C); a single «Земляной орех» or «Белый корень» (category 9,
// type 1 or 2) instead gets a fresh copy of the item and a respawn timer.
// It reports a message VA (0 = none) and whether the pile was removed.
func (g *Game) PickUp(e *Entity, t *resfmt.ExeTables, pile, now int) (msg int, removed bool) {
	p := &g.Piles[pile]
	orig := *p
	e.Gold += abs(p.Gold)
	p.Gold = 0
	for p.Items[0] != 0 {
		full := true
		for _, v := range e.Bag {
			if v == 0 {
				full = false
				break
			}
		}
		if full {
			return MsgBagFull, false
		}
		if g.CarriedWeight(*e, t)+g.ItemWeight(t, p.Items[0]) > MaxWeight(*e) {
			return MsgTooHeavy, false
		}
		BagInsert(e, p.Items[0])
		copy(p.Items[:], p.Items[1:])
		p.Items[pileMaxItems-1] = 0
	}
	// 0x00410973: the record still holds what the map loader copied.
	if it, ok := g.Item(orig.Items[0]); ok && orig.Items[1] == 0 && it.Category == 9 && (it.Type == 1 || it.Type == 2) {
		if fresh := g.allocItem(orig.Items[0]); fresh != 0 {
			p.Taken, p.TakenRef = -now, -now
			p.Items[0] = fresh
			return 0, true
		}
	}
	return 0, true
}

// allocItem is 0x004399AC followed by a copy of item src: the first free
// record (category < 0, from index 1) becomes a copy of src. 0 if none.
func (g *Game) allocItem(src int) int {
	for i := 1; (i+1)*itemStride <= len(g.items); i++ {
		r := g.items[i*itemStride:]
		if int8(r[0]) < 0 {
			copy(r[:itemStride], g.items[src*itemStride:][:itemStride])
			return i
		}
	}
	return 0
}
