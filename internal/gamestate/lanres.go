package gamestate

import (
	"encoding/binary"
	"fmt"
)

// The LAN world file GAME.RES as the LAN server loads it (LAN_KONG.EXE
// 0x00449451; the session reset 0x00446900 reads it again, all but the map
// exits and the player entities 0–15). Six sections in GAME.N's order,
// docs/konung_lan.md#world-gameres:
//
//	0x00000  0x20000  8192 × 16-byte items, as GAME.N           → 0xC4E1B8
//	0x20000    0xA00    80 × 32-byte groups, as GAME.N          → 0xC6E1B8
//	0x20A00  0x20D00  2100 × 64-byte piles, 100 per map (LAN)   → 0xC216B0
//	0x41700   0x109A   250 × 17-byte map exits, as GAME.N       → 0xE24724
//	0x4279A  0x14000   320 × 256-byte entities (LAN layout)     → 0xE0BFE0
//	0x5679A   0x2C2B     3 × 0xEB9 settlements (LAN layout)     → 0xE20C60
//
// ParseLANWorld converts the LAN records into the GAME.N types, so the
// game's rules run on them unchanged.
const (
	lanItemOffset       = 0x00000
	lanGroupOffset      = 0x20000
	LANGroups           = 80
	LANPlayerSlots      = 16 // groups and entities 0–15: one per player slot
	lanPileOffset       = 0x20A00
	lanPileRecords      = 2100
	LANPilesPerMap      = 100 // record = map·100 + k (0x00434AEF: map·0x1900)
	lanEntityOffset     = 0x4279A
	LANEntities         = 320
	lanSettlementOffset = 0x5679A
	LANSettlements      = 3
	lanSettlementStride = 0xEB9
)

// LAN entity record offsets (a packed form of the GAME.N record: the same
// fields in the same order; the base stats moved to +0xC5; docs).
const (
	lanEntGroup     = 0x05 // byte: owning group (GAME.N derives it from the groups)
	lanEntRow       = 0x06 // u16 (GAME.N +0x08)
	lanEntCol       = 0x08 // u16 (+0x0A)
	lanEntStep      = 0x0A // byte (+0x10), runtime; by order (guess)
	lanEntThrottle  = 0x0B // byte (+0x11), runtime; by order (guess)
	lanEntWeapon    = 0x0C // byte (+0x12): 0x0044660D, draw 0x004197A2
	lanEntLevel     = 0x0D // byte (+0x33)
	lanEntGold      = 0x0E // i32 (+0x34)
	lanEntFreeExp   = 0x12 // i32 (+0x38): record +0x38 (0x004475F8)
	lanEntExp       = 0x16 // i32 (+0x3C): record +0x4C
	lanEntSkills    = 0x1A // u16 ×7: Фехтование, Торговля, Меткость, Плотницкое, Знахарство, Идентификация, Кузнечное (+0x40…+0x4C)
	lanEntArmour    = 0x28 // i16 (+0x4E): record +0x62
	lanEntPoison    = 0x2A // i16 (+0x50)
	lanEntPoisonHit = 0x2C // i16 (+0x52)
	lanEntStats     = 0x2E // u16 ×5: Лов, Хар, HP, Сил, Вын (+0x54, +0x58, +0x5C, +0x5E, +0x62)
	lanEntQuiver    = 0x38 // u16 (+0xCC): 0x0044662A
	lanEntDialogue  = 0x3A // byte (+0xD0)
	lanEntEnchants  = 0x3B // int8 ×2 (+0xD2, +0xD3)
	lanEntName      = 0x3D // byte (+0xD4)
	lanEntNickname  = 0x3E // byte (+0xD5)
	lanEntModel     = 0x3F // int8 (+0xD6)
	lanEntPalette   = 0x41 // u32 (+0xD8)
	lanEntEquipment = 0x45 // u16 ×6 (+0x66)
	lanEntBag       = 0x51 // u16 ×32 (+0x72)
	lanEntJewellery = 0x91 // u16 ×5 (+0xB2, Entity.Extra)
	lanEntTarget    = 0xBB // u16: target index + 1 (server orders), runtime
	lanEntIndex     = 0xBD // u16: own index
	lanEntBaseStats = 0xC5 // u16 ×4: base Лов, Хар, Сил, Вын (+0x56, +0x5A, +0x60, +0x64)
	lanEntWorldX    = 0xE5 // world pixel x, runtime
	lanEntWorldY    = 0xED // world pixel y, runtime
)

// LAN settlement record offsets: the GAME.N settlement with a longer
// header and the shop stocks kept per player slot.
const (
	lanSetBuildings   = 0x54  // 119 × 8-byte building records (GAME.N +0x18)
	lanSetPosts       = 0x40C // 5 × 8-byte posts (+0x3D0)
	lanSetApprentice  = 0x434 // 5 × 4-byte apprentice times (+0x3F8)
	lanSetHealer      = 0x448 // 16 × 0x38 healer flags and stock, one per player (+0x40C)
	lanSetMerchant    = 0x7C8 // 16 × 0x40 merchant stock (+0x444)
	lanSetSmith       = 0xBC8 // 16 × 0x2E smith stock (+0x484)
	lanSetFlags       = 0xEA8 // byte (+0x4B2); the session reset sets 0x10
	lanSetSmithJob    = 0xEA9 // 16 bytes, one per player (+0x4B4)
	lanSetLoans       = 0x4C  // u16: player slots with a loan (282/284/333, 0x0044BD9A)
	lanSetAggressors  = 0x4E  // u16: player slots that attacked the village (0x446AC1 clears it)
	spSetBuildings    = 0x18
	spSetBuildingsLen = 119 * 8
	spSetPostsLen     = 5 * 8
	spSetApprentice   = 0x3F8
	spSetSmithJob     = 0x4B4
)

// LAN pile record offsets (GAME.N: gold +0x10, cell +0x08/+0x0A).
const (
	lanPileGold  = 0x00 // i32: a non-zero value makes the pile icon (0x00444058)
	lanPileRow   = 0x0C // u16
	lanPileCol   = 0x0E // u16
	lanPileImage = 0x10 // i16: ground image, recomputed by 0x00444058 (0xA3 = several things)
	lanPileMap   = 0x12 // byte: map, 0 = unused record
	lanPileFree  = 0x13 // bit 0: emptied record, reusable (0x00448096)
	lanPileItems = 0x14 // u16 ×22, as GAME.N
)

// LANWorld is GAME.RES decoded. Game holds the records in GAME.N's types:
// the 8192 items, the 80 groups, the 320 entities, the piles in use
// (Pile.Index = record index), the map exits and the 3 settlements (GAME.N
// layout; the per-player stocks keep slot 0's). Settlements keeps each LAN
// settlement record as loaded. Game is not a GAME.N image: Marshal fails.
type LANWorld struct {
	Game        *Game
	Settlements [LANSettlements][lanSettlementStride]byte
}

// ParseLANWorld decodes GAME.RES.
func ParseLANWorld(data []byte) (*LANWorld, error) {
	if len(data) != LANWorldSize {
		return nil, fmt.Errorf("GAME.RES has %d bytes, want %d", len(data), LANWorldSize)
	}
	data = append([]byte(nil), data...)
	g := &Game{items: data[lanItemOffset : lanItemOffset+0x20000], Options: [2]int32{1, 1}, LAN: true}
	for i := range LANGroups {
		g.Groups = append(g.Groups, parseGroup(i, data[lanGroupOffset+i*groupStride:][:groupStride]))
	}
	for i := range LANEntities {
		g.Entities = append(g.Entities, lanEntity(i, data[lanEntityOffset+i*entityStride:][:entityStride]))
	}
	for i := range lanPileRecords {
		r := data[lanPileOffset+i*pileStride:][:pileStride]
		if r[lanPileMap] == 0 || r[lanPileFree]&1 != 0 {
			continue
		}
		p := Pile{Index: i, Row: u16(r, lanPileRow), Col: u16(r, lanPileCol), Map: int(r[lanPileMap]),
			Gold: int(int32(binary.LittleEndian.Uint32(r[lanPileGold:])))}
		copy(p.Items[:], words(r[lanPileItems:], pileMaxItems))
		g.Piles = append(g.Piles, p)
	}
	g.Exits = parseExitsAt(data, lanExitOffset)
	w := &LANWorld{Game: g}
	for i := range LANSettlements {
		r := data[lanSettlementOffset+i*lanSettlementStride:][:lanSettlementStride]
		copy(w.Settlements[i][:], r)
		st := settlementFrom(i, lanSettlementToGameN(r))
		st.Aggressors = uint16(u16(r, lanSetAggressors))
		g.Settlements = append(g.Settlements, st)
	}
	return w, nil
}

// lanEntity converts a LAN entity record into an Entity (Raw in GAME.N's
// layout, built by Encode).
func lanEntity(i int, r []byte) Entity {
	sk := func(k int) int { return u16(r, lanEntSkills+2*k) }
	st := func(k int) int { return u16(r, lanEntStats+2*k) }
	base := func(k int) int { return u16(r, lanEntBaseStats+2*k) }
	e := Entity{
		Index:         i,
		Type:          int(int8(r[0])),
		Variant:       int(int8(r[1])),
		Flags2:        r[2],
		Flags3:        r[3],
		Flags4:        r[4],
		Group:         int(r[lanEntGroup]),
		Row:           u16(r, lanEntRow),
		Col:           u16(r, lanEntCol),
		Step:          int(r[lanEntStep]),
		Throttle:      int(r[lanEntThrottle]),
		WeaponMode:    int(r[lanEntWeapon]),
		Level:         int(r[lanEntLevel]),
		Gold:          int(int32(binary.LittleEndian.Uint32(r[lanEntGold:]))),
		FreeExp:       int(int32(binary.LittleEndian.Uint32(r[lanEntFreeExp:]))),
		Experience:    int(int32(binary.LittleEndian.Uint32(r[lanEntExp:]))),
		Fencing:       sk(0),
		Marksmanship:  sk(2),
		Skills:        [5]int{sk(4), sk(6), sk(1), sk(3), sk(5)}, // healing, smithing, trade, building, curios
		ArmourBase:    s16(r, lanEntArmour),
		Poison:        s16(r, lanEntPoison),
		PoisonDealt:   s16(r, lanEntPoisonHit),
		Stats:         [4]int{st(1), st(3), st(0), st(4)}, // Хар, Сил, Лов, Вын
		HP:            s16(r, lanEntStats+4),
		BaseStats:     [4]int{base(1), base(2), base(0), base(3)},
		Quiver:        u16(r, lanEntQuiver),
		Dialogue:      int(r[lanEntDialogue]),
		Enchants:      [2]int{int(int8(r[lanEntEnchants])), int(int8(r[lanEntEnchants+1]))},
		NameIndex:     int(r[lanEntName]),
		Nickname:      int(r[lanEntNickname]),
		Model:         int(int8(r[lanEntModel])),
		PaletteOffset: binary.LittleEndian.Uint32(r[lanEntPalette:]),
		Equipment:     [6]int(words(r[lanEntEquipment:], 6)),
		Bag:           [32]int(words(r[lanEntBag:], 32)),
		Extra:         [5]int(words(r[lanEntJewellery:], 5)),
	}
	e.Raw = e.Encode()
	return e
}

// lanSettlementToGameN lays a LAN settlement record out as GAME.N's: the
// header bytes +0…+3, the building records, posts and apprentice times,
// the flags and player slot 0's smith job. The runtime header fields
// (+0x04…+0x53) and the per-player stocks are zero in the file.
func lanSettlementToGameN(r []byte) []byte {
	out := make([]byte, settlementStride)
	copy(out[:4], r[:4])
	copy(out[spSetBuildings:spSetBuildings+spSetBuildingsLen], r[lanSetBuildings:])
	copy(out[postOffset:postOffset+spSetPostsLen], r[lanSetPosts:])
	copy(out[spSetApprentice:spSetApprentice+5*4], r[lanSetApprentice:])
	out[0x4B2] = r[lanSetFlags]
	out[0x4B3] = 0xFF
	out[spSetSmithJob] = r[lanSetSmithJob]
	return out
}
