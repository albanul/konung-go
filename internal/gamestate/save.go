package gamestate

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

// Saved games, written by 0x00424E34 and read by 0x004249AC; see
// docs/konung_save_format.md. KONUNG.SAV holds the ten slot names,
// KONUNG.SA0 … KONUNG.SA9 the slots: a raw dump of the globals below.
const (
	SaveSlots      = 10
	SlotNameSize   = 50 // 0x00674640: 10 × 50 bytes, CP866, zero-terminated
	SlotNamesFile  = "KONUNG.SAV"
	EmptySlotVA    = 0x0045070F // «Пустая строка», the name of a slot never saved
	LocationNameVA = 0x0045B3F4 // map names, 0x28 bytes each, index map − 1
	locationStride = 0x28

	QuestFlagsSize   = 0x4B0 // 0x00625068: 300 dwords, bit 0x80 tested by the dialogue code
	QuestFlagsOffset = 337800 - QuestFlagsSize
	QuestFileSize    = 337800 // QUESTR.RES: seven blocks read by 0x0043BDD5, the flags last
	knownMapsSize    = 0x38   // 0x00A3E0A8

	maxArrows      = 100 // 0x0082C1F8: 100 × 32 bytes, count 0x00A3E7D8
	arrowStride    = 0x20
	maxActivePiles = 200 // 0x007A6CDC: 200 × 0x40, count 0x00A3E7E0
	runtimePile    = 0xFFFF
	arrowImage     = 0xB8 // INTR slot of facing 0; +2 per facing

	saveHeaderSize = 0x10
	// SaveSize is the size of a KONUNG.SA<n> file.
	SaveSize = saveHeaderSize + 8 + QuestFlagsSize + maxArrows*arrowStride + 0x20000 +
		maxActivePiles*pileStride + 0x1900 + 0x7D00 + 0x7D000 + 0x5E24 + knownMapsSize
	// saveEntityOffset is where the entity array starts in a slot file; the
	// loader seeks there to read the hero record first (0x00424B1E).
	saveEntityOffset = saveHeaderSize + 0x2D938
)

// SlotFile is the file name of save slot i (0x00424EE3: the last digit of
// "KONUNG.SA0" plus the slot).
func SlotFile(i int) string { return fmt.Sprintf("KONUNG.SA%c", '0'+byte(i)) }

// ActivePile is an entry of the current map's active pile list: a copy of
// the pile with its record index (+0x0E, -1 for a runtime pile), the
// image's screen position (+0/+4: cell centre minus half the image) and
// its INTR image slot (+0x0C).
type ActivePile struct {
	Pile
	X, Y, Image int
}

// SaveState is what a slot holds besides the GAME.N records and the Game's
// own saved fields.
type SaveState struct {
	Time   int     // 0x00A3E770, game ticks
	Arrows []Arrow // projectiles in flight
	Active []ActivePile
}

// MarshalSave returns a KONUNG.SA<n> file (0x00424E34).
func (g *Game) MarshalSave(st SaveState) ([]byte, error) {
	img, err := g.Marshal()
	if err != nil {
		return nil, err
	}
	if len(st.Arrows) > maxArrows || len(st.Active) > maxActivePiles {
		return nil, fmt.Errorf("gamestate: %d arrows, %d active piles", len(st.Arrows), len(st.Active))
	}
	var b bytes.Buffer
	w32 := func(v int) { binary.Write(&b, binary.LittleEndian, int32(v)) }
	w32(int(g.Options[0]))
	w32(int(g.Options[1]))
	w32(st.Time)
	w32(g.Groups[0].First) // hero index (0x00A3E694 − 0x007AB378) / 256
	w32(len(st.Arrows))
	w32(len(st.Active))
	b.Write(g.Quests[:])
	arrows := make([]byte, maxArrows*arrowStride)
	for i, a := range st.Arrows {
		a.encode(arrows[i*arrowStride:][:arrowStride])
	}
	b.Write(arrows)
	b.Write(img[itemOffset : itemOffset+0x20000])
	active := make([]byte, maxActivePiles*pileStride)
	for i, p := range st.Active {
		r := active[i*pileStride:][:pileStride]
		p.encode(r)
		put32(r, 0, p.X)
		put32(r, 4, p.Y)
		put16(r, 0x0C, p.Image)
		idx := p.Index
		if idx < 0 {
			idx = runtimePile
		}
		put16(r, 0x0E, idx)
	}
	b.Write(active)
	b.Write(img[groupOffset : groupOffset+0x1900])
	b.Write(img[pileOffset : pileOffset+0x7D00])
	b.Write(img[entityOffset : entityOffset+0x7D000])
	b.Write(img[settlementOffset : settlementOffset+0x5E24])
	b.Write(g.KnownMaps[:])
	if b.Len() != SaveSize {
		return nil, fmt.Errorf("gamestate: save is %d bytes, want %d", b.Len(), SaveSize)
	}
	return b.Bytes(), nil
}

// SaveHeroModel returns the hero's model (+0xD6) from a slot file. The
// loader reads it first (0x00424B1E) to reopen GAME.<model>, whose map
// exits a save does not hold (0x0043BB6C).
func SaveHeroModel(save []byte) (int, error) {
	if len(save) != SaveSize {
		return 0, fmt.Errorf("save has size %d, want %d", len(save), SaveSize)
	}
	hero := int(int32(binary.LittleEndian.Uint32(save[12:])))
	if hero < 0 || hero >= entityCount {
		return 0, fmt.Errorf("save hero index %d", hero)
	}
	return int(int8(save[saveEntityOffset+hero*entityStride+0xD6])), nil
}

// ParseSave reads a slot file over gameN, the GAME.<hero model> file
// (0x004249AC). The hero entity must be group 0's first (as in every
// GAME.N); runtime piles of the active list come back as Index -1 piles.
func ParseSave(save, gameN []byte) (*Game, SaveState, error) {
	var st SaveState
	if len(save) != SaveSize {
		return nil, st, fmt.Errorf("save has size %d, want %d", len(save), SaveSize)
	}
	if len(gameN) != FileSize {
		return nil, st, fmt.Errorf("GAME file has size %d, want %d", len(gameN), FileSize)
	}
	i32 := func(off int) int { return int(int32(binary.LittleEndian.Uint32(save[off:]))) }
	options := [2]int32{int32(i32(0)), int32(i32(4))}
	st.Time = i32(8)
	hero := i32(12)
	nArrows, nActive := i32(16), i32(20)
	if nArrows < 0 || nArrows > maxArrows || nActive < 0 || nActive > maxActivePiles {
		return nil, st, fmt.Errorf("save has %d arrows, %d active piles", nArrows, nActive)
	}
	off := 24
	next := func(n int) []byte { b := save[off : off+n]; off += n; return b }
	quests := next(QuestFlagsSize)
	arrows := next(maxArrows * arrowStride)
	img := append([]byte(nil), gameN...)
	copy(img[itemOffset:], next(0x20000))
	active := next(maxActivePiles * pileStride)
	copy(img[groupOffset:], next(0x1900))
	copy(img[pileOffset:], next(0x7D00))
	copy(img[entityOffset:], next(0x7D000))
	copy(img[settlementOffset:], next(0x5E24))
	misc := next(knownMapsSize)
	g, err := Parse(img)
	if err != nil {
		return nil, st, err
	}
	if hero != g.Groups[0].First {
		return nil, st, fmt.Errorf("save hero %d is not group 0's first entity %d", hero, g.Groups[0].First)
	}
	g.Options = options
	copy(g.Quests[:], quests)
	copy(g.KnownMaps[:], misc)
	for i := range nArrows {
		st.Arrows = append(st.Arrows, decodeArrow(arrows[i*arrowStride:][:arrowStride]))
	}
	for i := range nActive {
		r := active[i*pileStride:][:pileStride]
		p := ActivePile{
			Pile:  Pile{Index: u16(r, 0x0E), Row: u16(r, 0x08), Col: u16(r, 0x0A), Gold: s16(r, 0x10), Map: u16(r, 0x12)},
			X:     int(int32(binary.LittleEndian.Uint32(r))),
			Y:     int(int32(binary.LittleEndian.Uint32(r[4:]))),
			Image: u16(r, 0x0C),
		}
		copy(p.Items[:], words(r[0x14:], pileMaxItems))
		if p.Index == runtimePile {
			p.Index = -1
		} else if p.Index < len(g.Piles) {
			// The active copy is the live one; keep the record's timer.
			p.Taken, p.TakenRef = g.Piles[p.Index].Taken, g.Piles[p.Index].TakenRef
		}
		st.Active = append(st.Active, p)
	}
	return g, st, nil
}

func (a Arrow) encode(b []byte) {
	binary.LittleEndian.PutUint32(b[0:], math.Float32bits(float32(a.VX)))
	binary.LittleEndian.PutUint32(b[4:], math.Float32bits(float32(a.VY)))
	binary.LittleEndian.PutUint32(b[8:], math.Float32bits(float32(a.X)))
	binary.LittleEndian.PutUint32(b[0xC:], math.Float32bits(float32(a.Y)))
	dx, dy := a.SpriteOffset()
	put32(b, 0x10, dx)
	put16(b, 0x14, a.Shooter)
	put16(b, 0x16, a.Bow)
	put16(b, 0x18, a.Poison)
	put16(b, 0x1A, dy)
	b[0x1C], b[0x1D], b[0x1E], b[0x1F] = byte(arrowImage+2*(a.Facing&7)), byte(a.Life), byte(a.Special), byte(a.Type)
}

func decodeArrow(b []byte) Arrow {
	f := func(off int) float64 { return float64(math.Float32frombits(binary.LittleEndian.Uint32(b[off:]))) }
	return Arrow{
		VX: f(0), VY: f(4), X: f(8), Y: f(0xC),
		Shooter: u16(b, 0x14), Bow: u16(b, 0x16), Poison: u16(b, 0x18),
		Facing: (int(b[0x1C]) - arrowImage) / 2 & 7, Life: int(b[0x1D]), Special: int(int8(b[0x1E])), Type: int(b[0x1F]),
	}
}

// SetQuestFlags takes the initial quest flags from QUESTR.RES (its last
// QuestFlagsSize bytes; the new-game load 0x0043BDD5).
func (g *Game) SetQuestFlags(questr []byte) error {
	if len(questr) != QuestFileSize {
		return fmt.Errorf("QUESTR.RES has size %d, want %d", len(questr), QuestFileSize)
	}
	copy(g.Quests[:], questr[QuestFlagsOffset:])
	return nil
}

// SlotNames decodes KONUNG.SAV (0x0043A9A7); without the file every slot
// is named empty (the string at EmptySlotVA).
func SlotNames(data []byte, empty []byte) [SaveSlots][]byte {
	var out [SaveSlots][]byte
	for i := range out {
		if len(data) < SaveSlots*SlotNameSize {
			out[i] = append([]byte(nil), empty...)
			continue
		}
		r := data[i*SlotNameSize:][:SlotNameSize]
		if n := bytes.IndexByte(r, 0); n >= 0 {
			r = r[:n]
		}
		out[i] = append([]byte(nil), r...)
	}
	return out
}

// MarshalSlotNames encodes KONUNG.SAV; names are cut to 49 bytes.
func MarshalSlotNames(names [SaveSlots][]byte) []byte {
	out := make([]byte, SaveSlots*SlotNameSize)
	for i, n := range names {
		copy(out[i*SlotNameSize:][:SlotNameSize-1], n)
	}
	return out
}

// SaveName is the name a new save gets (0x00437A25): the location name of
// map m, a space and the C ctime of t without the weekday, seconds and
// year, e.g. «Родники Jun  5 21:49». loc is the CP866 name.
func SaveName(loc []byte, t time.Time) []byte {
	out := append(append([]byte(nil), loc...), ' ')
	return append(out, fmt.Sprintf("%s %2d %02d:%02d", t.Format("Jan"), t.Day(), t.Hour(), t.Minute())...)
}

// LocationName returns the address of map m's name (m ≥ 1).
func LocationName(m int) int { return LocationNameVA + (m-1)*locationStride }
