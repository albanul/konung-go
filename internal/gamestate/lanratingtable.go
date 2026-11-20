package gamestate

import (
	"encoding/binary"

	"konung1.5/internal/resfmt"
)

// The «Рейтинг» table (LAN_KONG.EXE; docs/konung_lan.md#rating-table): the
// server builds it at a session's end (0x00446FA0) and sends every player
// in the world ten rows (message 0x06, 0x0043465F); the client keeps them
// in HISCORES.DAT (0x004173CE, read 0x004313EC) and draws them on its
// «Рейтинг» screen (0x004271AC).

// LANHiscoresFile is the client's cache of the table (in the game folder).
const LANHiscoresFile = "HISCORES.DAT"

// LANRatingRows is how many rows the table has.
const LANRatingRows = 10

// lanRatingRecord is a row's size in HISCORES.DAT and message 0x06.
const lanRatingRecord = 0x7C

// LANRatingRow is a row: a player (rank, name, clan, the counters, the
// rating) and, independently, a clan (rank, name, rating) — row k holds
// the k-th player and the k-th clan. Ratings are as sent: the account's
// rating ÷ 10 (0x0043480A); the screen divides by 10 again.
type LANRatingRow struct {
	Rank           int    `json:"rank"` // +0x78
	Name           string `json:"name,omitempty"`
	Clan           string `json:"clan,omitempty"`            // +0x20
	Amulets        int    `json:"amulets,omitempty"`         // +0x60
	KillsHeroes    int    `json:"kills_heroes,omitempty"`    // +0x64
	KillsHirelings int    `json:"kills_hirelings,omitempty"` // +0x68
	KillsMonsters  int    `json:"kills_monsters,omitempty"`  // +0x6C
	Rating         int    `json:"rating,omitempty"`          // +0x70
	ClanRank       int    `json:"clan_rank,omitempty"`       // +0x7A
	ClanName       string `json:"clan_name,omitempty"`       // +0x40
	ClanRating     int    `json:"clan_rating,omitempty"`     // +0x74
}

// LANRatingAccount is an account as the table reads it (LANUSERS.DAT: the
// name, the clan's index +0x68, the counters +0x34 and +0x28…+0x30, the
// rating +0x44).
type LANRatingAccount struct {
	Name                                                        string
	Clan                                                        int
	Amulets, KillsHeroes, KillsHirelings, KillsMonsters, Rating int
}

// LANRatingTable builds the ten rows player own gets (0x00446FA0,
// 0x0043465F): accounts (in account order) sorted by rating with the
// original's exchange sort (a later one with a higher rating swaps in);
// the clans (by index, the names in clans) by their members' mean rating
// (a whole number; a clan without members has 0), sorted the same way;
// row k the k-th of each, the ratings ÷ 10. When own is not among the top
// ten, row 9's player is own, with its rank; likewise row 9's clan is
// own's clan when that is not among the top ten.
//
// Port rules (the original's bugs): the clans start at index 0 (the
// original's loop starts at 1, so the first clan, «Византийцы», never
// shows), and own's clan's rank is its place in the sorted list (the
// original reads the sorted list at the clan's index, 0x00434797).
func LANRatingTable(accounts []LANRatingAccount, clans []string, own string) [LANRatingRows]LANRatingRow {
	players := append([]LANRatingAccount(nil), accounts...)
	for i := range players {
		for j := i + 1; j < len(players); j++ {
			if players[j].Rating > players[i].Rating {
				players[i], players[j] = players[j], players[i]
			}
		}
	}
	sum := make([]int, len(clans))
	count := make([]int, len(clans))
	for _, a := range accounts {
		if a.Clan >= 0 && a.Clan < len(clans) {
			sum[a.Clan] += a.Rating
			count[a.Clan]++
		}
	}
	mean := func(c int) int {
		if count[c] == 0 {
			return 0
		}
		return sum[c] / count[c]
	}
	var order []int
	for c, name := range clans {
		if name != "" {
			order = append(order, c)
		}
	}
	for i := range order {
		for j := i + 1; j < len(order); j++ {
			if mean(order[j]) > mean(order[i]) {
				order[i], order[j] = order[j], order[i]
			}
		}
	}
	clanName := func(c int) string {
		if c >= 0 && c < len(clans) {
			return clans[c]
		}
		return ""
	}
	setPlayer := func(r *LANRatingRow, k int) {
		a := players[k]
		r.Rank, r.Name, r.Clan = k+1, a.Name, clanName(a.Clan)
		r.Amulets, r.KillsHeroes, r.KillsHirelings, r.KillsMonsters = a.Amulets, a.KillsHeroes, a.KillsHirelings, a.KillsMonsters
		r.Rating = a.Rating / 10
	}
	setClan := func(r *LANRatingRow, k int) {
		c := order[k]
		r.ClanRank, r.ClanName, r.ClanRating = k+1, clans[c], mean(c)/10
	}
	var rows [LANRatingRows]LANRatingRow
	for k := range rows {
		rows[k].Rank = k + 1 // every row has its rank (0x00447442), names or not
		if k < len(players) {
			setPlayer(&rows[k], k)
		}
		if k < len(order) {
			setClan(&rows[k], k)
		}
	}
	last := &rows[LANRatingRows-1]
	for k, a := range players {
		if a.Name != own {
			continue
		}
		if k >= LANRatingRows {
			setPlayer(last, k)
		}
		for p, c := range order {
			if c == a.Clan && p >= LANRatingRows {
				setClan(last, p)
			}
		}
		break
	}
	return rows
}

// ParseLANHiscores reads HISCORES.DAT: ten 0x7C-byte rows (names CP866);
// a short file gives the rows it holds.
func ParseLANHiscores(b []byte) []LANRatingRow {
	var out []LANRatingRow
	for k := 0; k < LANRatingRows && (k+1)*lanRatingRecord <= len(b); k++ {
		r := b[k*lanRatingRecord:]
		i32 := func(off int) int { return int(int32(binary.LittleEndian.Uint32(r[off:]))) }
		u16 := func(off int) int { return int(binary.LittleEndian.Uint16(r[off:])) }
		out = append(out, LANRatingRow{
			Name: lanCString(r[0x00:0x20]), Clan: lanCString(r[0x20:0x40]), ClanName: lanCString(r[0x40:0x60]),
			Amulets: i32(0x60), KillsHeroes: i32(0x64), KillsHirelings: i32(0x68), KillsMonsters: i32(0x6C),
			Rating: i32(0x70), ClanRating: i32(0x74), Rank: u16(0x78), ClanRank: u16(0x7A),
		})
	}
	return out
}

// EncodeLANHiscores writes rows as HISCORES.DAT (0x004173CE: the rows as
// received).
func EncodeLANHiscores(rows []LANRatingRow) []byte {
	b := make([]byte, LANRatingRows*lanRatingRecord)
	for k, row := range rows {
		if k >= LANRatingRows {
			break
		}
		r := b[k*lanRatingRecord:]
		putLANCString(r[0x00:0x20], row.Name)
		putLANCString(r[0x20:0x40], row.Clan)
		putLANCString(r[0x40:0x60], row.ClanName)
		for off, v := range map[int]int{0x60: row.Amulets, 0x64: row.KillsHeroes, 0x68: row.KillsHirelings,
			0x6C: row.KillsMonsters, 0x70: row.Rating, 0x74: row.ClanRating} {
			binary.LittleEndian.PutUint32(r[off:], uint32(int32(v)))
		}
		binary.LittleEndian.PutUint16(r[0x78:], uint16(row.Rank))
		binary.LittleEndian.PutUint16(r[0x7A:], uint16(row.ClanRank))
	}
	return b
}

// lanCString is a NUL-terminated CP866 field as text.
func lanCString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			b = b[:i]
			break
		}
	}
	return resfmt.CP866ToUTF8(b)
}

// putLANCString writes s as a NUL-terminated CP866 field (cut to fit).
func putLANCString(dst []byte, s string) {
	n := 0
	for _, r := range s {
		c, ok := resfmt.RuneToCP866(r)
		if !ok {
			c = '?'
		}
		if n >= len(dst)-1 {
			break
		}
		dst[n] = c
		n++
	}
}
