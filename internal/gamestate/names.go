package gamestate

import (
	"strconv"

	"konung1.5/internal/resfmt"
)

// Entity names and the hover status line, as CP866 bytes for the game's fonts.

const (
	classNamesVA = 0x0045AE20 // 16 bytes per class, index (+4 & 7) - 1: Храбрец, Купец, Охотник, Вождь, Воин
	roleNamesVA  = 0x0046233C // 16 bytes per post slot - 1: ", староста", ", знахарь", ", купец", ", кузнец", ", воевода"
)

// Name builds e's name like 0x0042CA90. Creatures (+4 & 0x7F in 0x41..0x50)
// use the creature table; everyone else, including women and children, the
// first-name table (+0xD4), followed by " " and the nickname (+0xD5) if set.
func Name(e Entity, t *resfmt.ExeTables) []byte {
	if f := int(e.Flags4 & 0x7F); f&0x40 != 0 && f < 0x51 {
		return append([]byte(nil), t.TableString(resfmt.CreatureNamesVA, f-0x41)...)
	}
	out := append([]byte(nil), t.TableString(resfmt.FirstNamesVA, e.NameIndex)...)
	if e.Nickname != 0 {
		out = append(out, ' ')
		out = append(out, t.TableString(resfmt.NicknamesVA, e.Nickname)...)
	}
	return out
}

// levelHealth is 0x0042C9F4: ", Уров:<level>, Здор:<(HP+15)/16>".
func levelHealth(e Entity, t *resfmt.ExeTables) []byte {
	out := append([]byte(nil), t.CString(0x00450318)...)
	out = strconv.AppendInt(out, int64(e.Level), 10)
	out = append(out, t.CString(0x00450320)...)
	return strconv.AppendInt(out, int64((e.HP+15)>>4), 10)
}

// StatusLine builds the text the game shows in the bottom bar while the
// cursor is over e (0x0043195C):
//
//   - creatures: name + level/health;
//   - women and children (+4 & 0x7F >= 0x51): name only;
//   - people: name, ", " + class (+4 & 7), level/health, then
//     party members: ", воин дружины", ", отравлен" if poisoned, and the
//     stats Хар/Сил/Лов/Вын;
//     others: ", враг" if the group is hostile (group +0x1D bit 0x01), else
//     their settlement role (post slot), else the skill phrases.
func (g *Game) StatusLine(e Entity, t *resfmt.ExeTables) []byte {
	out := Name(e, t)
	f := int(e.Flags4 & 0x7F)
	if f&0x40 != 0 {
		if f <= 0x50 {
			out = append(out, levelHealth(e, t)...)
		}
		return out
	}
	out = append(out, t.CString(0x00450540)...) // ", "
	if c := f & 7; c >= 1 && c <= 5 {
		out = append(out, t.CString(classNamesVA+(c-1)*16)...)
	}
	out = append(out, levelHealth(e, t)...)
	if e.Group == 0 {
		out = append(out, t.CString(0x00450543)...) // ", воин дружины"
		if e.Poison != 0 {
			out = append(out, t.CString(0x00450552)...) // ", отравлен"
		}
		for i, label := range [4]int{0x00465A2E, 0x00465A38, 0x00465A29, 0x00465A3D} { // Хар Сил Лов Вын
			out = append(out, t.CString(label)...)
			out = append(out, ':')
			out = strconv.AppendInt(out, int64(e.Stats[i]), 10)
		}
		return out
	}
	if e.Group > 0 && g.Groups[e.Group].Raw[0x1D]&1 != 0 {
		return append(out, t.CString(0x00450565)...) // ", враг"
	}
	if p, ok := g.PostOf(e); ok {
		return append(out, t.CString(roleNamesVA+(p.Slot-1)*16)...)
	}
	// Skill phrases: healing > 100, smithing > 4, trade > 30, building > 4,
	// curios > 10.
	limits := [5]int{100, 4, 30, 4, 10}
	phrases := [5]int{0x0045056C, 0x00450589, 0x004505A0, 0x004505B4, 0x004505C9}
	for i, lim := range limits {
		if e.Skills[i] > lim {
			out = append(out, t.CString(phrases[i])...)
		}
	}
	return out
}
