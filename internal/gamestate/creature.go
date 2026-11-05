package gamestate

import "encoding/binary"

// Creature combat AI (0x004123BC); see docs/konung_combat.md#creature-ai.

// Group alarm states (+0x1D), rewritten by AlarmState every AlarmCheckTicks.
const (
	AlarmCalm   = 0    // members stand idle
	AlarmAttack = 1    // members go for the enemy group +0x1A
	AlarmReturn = 0x20 // members outside the territory walk home
)

// AlarmCheckTicks is how often 0x0041D0FC runs the alarm check (game time
// & 0xF == 0).
const AlarmCheckTicks = 16

// LeashArea is the group's outer rectangle (words +0x0A..+0x0E rows,
// +0x12..+0x16 cols): a chase ends when the target leaves it (0x00411FD0) and
// returning members keep fighting while a party member is inside it
// (0x0041FD74). WanderArea is the inner rectangle, the territory.
func (g Group) LeashArea() Area {
	w := func(off int) int { return int(binary.LittleEndian.Uint16(g.Raw[off:])) }
	return Area{w(0x0A), w(0x0E), w(0x12), w(0x16)}
}

// CreatureGroup reports a creature group (+0x1E bit 0x40, AI 0x004123BC).
func (g Group) CreatureGroup() bool { return g.Kind&0x40 != 0 }

// AlarmState is the check 0x00412C30: AlarmAttack when a living party
// member stands inside the territory, else AlarmReturn when a living member
// of the group stands outside it, else AlarmCalm. Cells are fine-grid
// [row, col] of the living party members and group members.
func (g Group) AlarmState(party, members [][2]int) int {
	a, _ := g.WanderArea()
	for _, p := range party {
		if a.Contains(p[0], p[1]) {
			return AlarmAttack
		}
	}
	for _, m := range members {
		if !a.Contains(m[0], m[1]) {
			return AlarmReturn
		}
	}
	return AlarmCalm
}

// Tactic is how an alarmed creature picks its target, by kind (+4) in
// 0x00412E00's switch at 0x004131CA.
type Tactic int

const (
	TacticNearest    Tactic = iota // 0x004130EE: nearest enemy (0x004202A8); humanoids first enter the stance / use the bow
	TacticAssist                   // 0x41, 0x4D, 0x4F: a group-mate's living target (0x00420178), else nearest
	TacticUnengaged                // 0x43, 0x45: nearest enemy no group-mate attacks (0x004203A0), else nearest
	TacticStationary               // 0x44, 0x57: never move; swing at an adjacent enemy (0x00412F62)
	TacticDormant                  // 0x46, 0x4C, 0x55: wake from action 4 first (0x0041309D), then as TacticUnengaged
)

// TacticOf returns the tactic for creature kind (entity +4).
func TacticOf(kind byte) Tactic {
	switch kind {
	case 0x41, 0x4D, 0x4F:
		return TacticAssist
	case 0x43, 0x45:
		return TacticUnengaged
	case 0x44, 0x57:
		return TacticStationary
	case 0x46, 0x4C, 0x55:
		return TacticDormant
	}
	return TacticNearest
}

// Revives is 0x00412944's rebirth rule: a dying kind-0x4C creature with
// +0x12 > 0 comes back with full HP as a dormant creature (action 4) and
// one revive fewer. It reports whether e revived.
func Revives(e *Entity) bool {
	if e.Flags4 != 0x4C || e.WeaponMode == 0 {
		return false
	}
	e.HP = HPFull
	e.WeaponMode--
	return true
}

// Dist is 0x00439944, the grid distance the target selectors compare:
// the larger axis difference, plus 1 when the smaller one exceeds 1.
func Dist(r1, c1, r2, c2 int) int {
	dr, dc := abs(r1-r2), abs(c1-c2)
	if dr < dc {
		dr, dc = dc, dr
	}
	if dc > 1 {
		return dr + 1
	}
	return dr
}
