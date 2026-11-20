package gamestate

// The amulet of the dragon in the LAN version: the network game's objective
// (LAN_KONG.EXE, docs/konung_lan.md#amulets). Three parts lie at their
// places; a player joins them into the amulet and drops it on the spot of
// its nation (the hero type), which scores the player and the nation.
//
// Item numbers: LAN_KONG.EXE has its own misc item table (0x0048222C,
// 17 × 64-byte records: name, then the single-player record's tail: price,
// weight, icon, ground image). The port keeps the single-player numbers for
// the misc items in a LAN world too (konung.exe's tables draw and weigh
// them); LANMiscTypes maps the LAN numbers to them.

// LANMiscTypes maps LAN_KONG.EXE's misc item types (category 11) to the
// single-player types with the same name: 0 «Труды Гиппократа», 1 «Трактат
// о торговле», 2 «Слова Воскрешения», 3 «Свиток ведуна», 4 «Сын Луны»,
// 5 «Вино», 6 «Факел», 7 «Кукла», 8 «Заячий хвост», 9 «Дудочка», 10–12 the
// right, left and bottom part of the amulet, 13 «Амулет Дракона», 14–16 the
// left and right, right and bottom, left and bottom parts joined.
var LANMiscTypes = [17]int{
	MiscHippocrates, MiscTradeTract, MiscWords, MiscSeerScroll, 11, MiscWine, MiscTorch,
	MiscDoll, 34, MiscPipe, MiscAmuletRight, MiscAmuletLeft, MiscAmuletBottom,
	MiscAmulet, MiscAmuletLR, MiscAmuletRB, MiscAmuletLB,
}

// AmuletKind is the LAN server's kind of a misc item type (LAN type − 9,
// 0x00439194): 1–3 the right, left and bottom part, 4 the amulet, 5–7 the
// left and right, right and bottom, left and bottom parts joined; 0 for
// any other item.
func AmuletKind(typ int) int {
	for k, t := range LANMiscTypes[10:] {
		if t == typ {
			return k + 1
		}
	}
	return 0
}

// AmuletKindType is the misc item type of amulet kind k (1–7).
func AmuletKindType(k int) int {
	if k < 1 || k > 7 {
		return -1
	}
	return LANMiscTypes[9+k]
}

// AmuletWhole is the kind of the whole amulet, the one that is delivered.
const AmuletWhole = 4

// AmuletParts are the parts (0 right, 1 left, 2 bottom: placed at points
// 0–2) an amulet item of kind k is made of: what is placed again when it
// vanishes (0x0043B7FC).
func AmuletParts(k int) []int {
	switch k {
	case 1, 2, 3:
		return []int{k - 1}
	case AmuletWhole:
		return []int{0, 1, 2}
	case 5:
		return []int{0, 1}
	case 6:
		return []int{0, 2}
	case 7:
		return []int{1, 2}
	}
	return nil
}

// LANAmuletPoints are the six places of the SERVER file (map, row, col on
// the fine grid; 0x004826CC, read by 0x0043153C): 0–2 where the right,
// left and bottom parts are put (0x004349FC), 3–5 the delivery spots of
// the nations 0 (Волк: славяне), 1 (Драгомир: викинги), 2 (Михаил:
// византийцы) (0x00437B24: the hero's model + 3). Without a SERVER file
// the exe's own values hold: DefaultAmuletPoints.
type LANAmuletPoints [6][3]int

// DefaultAmuletPoints are the values LAN_KONG.EXE starts with
// (0x004826CC), all on map 4.
var DefaultAmuletPoints = LANAmuletPoints{
	{4, 66, 55}, {4, 64, 57}, {4, 62, 57}, {4, 68, 59}, {4, 66, 59}, {4, 66, 59},
}

// AmuletPointsFrom are the six places of SERVER file f, its lines over the
// defaults (the original's fscanf leaves a value it cannot read as it was).
func AmuletPointsFrom(f ServerFile) LANAmuletPoints {
	p := DefaultAmuletPoints
	for k := 0; k < len(p) && k < len(f.Points); k++ {
		p[k] = f.Points[k]
	}
	return p
}

// DefaultAmuletLimit is the delivered amulets that win without a SERVER
// file (0x0043162C); with one, its second number in TEAM mode, else none.
const DefaultAmuletLimit = 15

// AmuletLimitFrom is the win limit of SERVER file f (0 = none).
func AmuletLimitFrom(f ServerFile) int {
	if f.Mode != "TEAM" {
		return 0
	}
	return max(f.Amulets, 0)
}

// AmuletGroundPasses is how many settlement passes (16 ticks each,
// 0x00440532) an amulet item may lie on the ground after it was dropped
// before it vanishes (0x00437D75: 90; 0x00440549: the item becomes an
// empty jar and its parts are placed again).
const AmuletGroundPasses = 90

// HerbEmptyJar is «Пустая банка» (herb type 3, the same in both exes): what a
// vanished amulet item on the ground becomes (0x0044062A).
const HerbEmptyJar = 3

// AmuletSound is the sound a client plays when a nation's count changes
// (0x00422347: 0x15).
const AmuletSound = 0x15

// SessionEndSound is the sound a client plays at the session's end (the
// server's end line, 0x004225AA: 0x14).
const SessionEndSound = 0x14

// SessionEndText starts the server's session-end line (0x00470844); a
// nation's win follows it (NationWins).
const SessionEndText = "ИГРА ОКОНЧЕНА! "

// NationWins are the session-end texts of a nation's win (0x00470824,
// 0x00470833, 0x00470810), by nation.
var NationWins = [3]string{"Победа славян!", "Победа викингов!", "Победа византийцев!"}

// NationNames are the player list's team names (0x004703E5…0x004703F5).
var NationNames = [3]string{"Русские", "Викинги", "Византийцы"}
