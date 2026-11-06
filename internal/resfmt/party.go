package resfmt

// formationVA is the party formation table: int8 (row, col) offsets,
// 2 row parities × 8 directions × 12 slots (0x00C0 bytes per parity, 0x18
// per direction). The follow routine 0x0041FA5D and the party placement
// 0x004118DC read it.
const formationVA = 0x0045FF79

// FormationSlots is the number of offsets per (parity, direction).
const FormationSlots = 12

// FormationOffset returns slot k of the formation table for row parity
// (leader row & 1) and direction dir (0–7): the row and column offset from
// the leader's cell.
func (t *ExeTables) FormationOffset(parity, dir, k int) (dr, dc int) {
	p := t.at(formationVA + (parity&1)*0xC0 + (dir&7)*0x18 + k*2)
	return int(int8(p[0])), int(int8(p[1]))
}
