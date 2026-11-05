package gamestate

import (
	"encoding/binary"
	"fmt"
)

// Encoders: the inverse of Parse. Each writes the decoded fields back over
// the record's original bytes, so fields the port does not decode keep
// their values. Marshal(Parse(f)) == f for an unmodified GAME.N (test).

func put16(b []byte, off, v int) { binary.LittleEndian.PutUint16(b[off:], uint16(v)) }
func put32(b []byte, off, v int) { binary.LittleEndian.PutUint32(b[off:], uint32(v)) }

func putWords(b []byte, off int, v []int) {
	for i, w := range v {
		put16(b, off+i*2, w)
	}
}

// Encode returns the 256-byte entity record.
func (e Entity) Encode() [entityStride]byte {
	r := e.Raw
	b := r[:]
	b[0], b[1], b[2], b[3], b[4] = byte(e.Type), byte(e.Variant), e.Flags2, e.Flags3, e.Flags4
	put16(b, 0x08, e.Row)
	put16(b, 0x0A, e.Col)
	b[0x10], b[0x11], b[0x12] = byte(e.Step), byte(e.Throttle), byte(e.WeaponMode)
	b[0x33] = byte(e.Level)
	put32(b, 0x34, e.Gold)
	put32(b, 0x38, e.FreeExp)
	put32(b, 0x3C, e.Experience)
	put16(b, 0x40, e.Fencing)
	put16(b, 0x44, e.Marksmanship)
	for i, off := range [5]int{0x48, 0x4C, 0x42, 0x46, 0x4A} {
		put16(b, off, e.Skills[i])
	}
	put16(b, 0x4E, e.ArmourBase)
	put16(b, 0x50, e.Poison)
	put16(b, 0x52, e.PoisonDealt)
	for i, off := range [4]int{0x58, 0x5E, 0x54, 0x62} {
		put16(b, off, e.Stats[i])
	}
	for i, off := range [4]int{0x5A, 0x60, 0x56, 0x64} {
		put16(b, off, e.BaseStats[i])
	}
	put16(b, 0x5C, e.HP)
	putWords(b, 0x66, e.Equipment[:])
	putWords(b, 0x72, e.Bag[:])
	putWords(b, 0xB2, e.Extra[:])
	for i, off := range [4]int{0xBC, 0xBE, 0xC0, 0xC2} {
		put16(b, off, e.SavedBase[i])
	}
	for i, off := range [3]int{0xC4, 0xC6, 0xC8} {
		put16(b, off, e.Bonus[i])
	}
	put16(b, 0xCA, e.HitBonus)
	put16(b, 0xCC, e.Quiver)
	put16(b, 0xCE, e.PotionTimer)
	b[0xD0], b[0xD1] = byte(e.Dialogue), e.BonusFlags
	b[0xD2], b[0xD3] = byte(e.Enchants[0]), byte(e.Enchants[1])
	b[0xD4], b[0xD5], b[0xD6] = byte(e.NameIndex), byte(e.Nickname), byte(e.Model)
	binary.LittleEndian.PutUint32(b[0xD8:], e.PaletteOffset)
	return r
}

// Encode returns the 32-byte group record.
func (gr Group) Encode() [groupStride]byte {
	r := gr.Raw
	b := r[:]
	put16(b, 0x00, gr.First)
	put32(b, 0x02, gr.Debt)
	put16(b, 0x06, gr.Countdown)
	put16(b, 0x08, gr.Row)
	put16(b, 0x10, gr.Col)
	b[0x18], b[0x19], b[0x1A], b[0x1B] = byte(gr.Target), byte(gr.MaxPop), byte(gr.Enemy), byte(gr.Map)
	b[0x1C], b[0x1D], b[0x1E] = byte(gr.Count), byte(gr.Alarm), gr.Kind
	return r
}

// encode writes the pile's fields over a 0x40-byte record.
func (p Pile) encode(b []byte) {
	put32(b, 0x00, p.Taken)
	put32(b, 0x04, p.TakenRef)
	put16(b, 0x08, p.Row)
	put16(b, 0x0A, p.Col)
	put16(b, 0x10, p.Gold)
	put16(b, 0x12, p.Map)
	putWords(b, 0x14, p.Items[:])
}

// encode writes the settlement's fields over a 0x4B5-byte record.
func (s Settlement) encode(b []byte) {
	b[1], b[2], b[3] = byte(s.Fence), byte(s.Group), byte(s.Map)
	b[0x4B2] = s.Flags
	for k, p := range s.Posts {
		r := b[postOffset+k*8:]
		r[0], r[1] = byte(p.Facing), byte(p.Dialogue)
		put16(r, 2, p.Entity)
		put16(r, 4, p.Row)
		put16(r, 6, p.Col)
	}
}

// Marshal returns the game state in GAME.N layout (FileSize bytes): the
// loaded file with every decoded record written back. Section 3 gets the
// record piles (Index ≥ 0); runtime piles live only in the save's active
// list (SaveState.Active), as in the game.
func (g *Game) Marshal() ([]byte, error) {
	if len(g.data) != FileSize || len(g.Groups) != groupCount || len(g.Entities) != entityCount {
		return nil, fmt.Errorf("gamestate: game was not parsed from a GAME.N file")
	}
	out := append([]byte(nil), g.data...)
	copy(out[itemOffset:], g.items)
	for i, gr := range g.Groups {
		r := gr.Encode()
		copy(out[groupOffset+i*groupStride:], r[:])
	}
	for _, p := range g.Piles {
		if p.Index >= 0 && p.Index < pileCount {
			p.encode(out[pileOffset+p.Index*pileStride:][:pileStride])
		}
	}
	for i, e := range g.Entities {
		r := e.Encode()
		copy(out[entityOffset+i*entityStride:], r[:])
	}
	for i, s := range g.Settlements {
		s.encode(out[settlementOffset+i*settlementStride:][:settlementStride])
	}
	return out, nil
}
