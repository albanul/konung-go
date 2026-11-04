package gamestate

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"

	"konung1.5/internal/resfmt"
)

// Hover texts in the status line (hover handler 0x00423FBA, switch on the
// hotspot at 0x004244B3).

// ButtonTooltips are the CP866 strings shown over HUD panel buttons 0–6
// (hotspots 1–7): «Сменить оружие (1, 2, 3)», «Достать/Убрать оружие
// (Пробел)», «Карта (M)», «Все ко мне (F)», «Атаковать (A)», «Информация к
// размышлению (Q)», «Панель персонажа (I)». Button 2 shows another text
// while window 5 is open (0x004240D0, not ported).
var ButtonTooltips = [7]int{0x0045E17C, 0x0045D77C, 0x0045DF7C, 0x0045D97C, 0x0045D57C, 0x0045DB7C, 0x0045DD7C}

// HoveredItem returns the item a character-window hotspot (8–18) stands
// for, as the hover cases read it: 8 helmet, 9 the hand box (axe, or the
// blade when it is the active weapon or there is no axe; 0x00424375), 10
// shield, 11 armour, 12 bow, 13 ammunition (+0xCC), 14–18 jewellery. 0 for
// an empty box or another hotspot.
func HoveredItem(e Entity, spot int) int {
	switch spot {
	case 8:
		return e.Equipment[EquipHelmet]
	case 9:
		axe := e.Equipment[WeaponAxe]
		if axe == 0 || (e.WeaponMode == WeaponBlade && e.Equipment[WeaponBlade] != 0) {
			return e.Equipment[WeaponBlade]
		}
		return axe
	case 10:
		return e.Equipment[EquipShield]
	case 11:
		return e.Equipment[EquipArmour]
	case 12:
		return e.Equipment[WeaponBow]
	case 13:
		return e.Quiver
	}
	if spot >= 14 && spot <= 18 {
		return e.Extra[spot-14]
	}
	return 0
}

// Item description strings (CP866, DGROUP).
const (
	descUnknownWeapon = 0x00450391 // «Неведомое оружие»
	descUnknownBow    = 0x004503A2 // «Неведомый лук»
	descUnknownItem   = 0x004503B0 // «Неведомый предмет»
	descDamage        = 0x004503C2 // «, убойная сила »
	descArmour        = 0x004503D2 // «, броня: »
	descWear          = 0x004503DC // «, износ »
	descWeight        = 0x004503E7 // «, вес »
	descPoison        = 0x004503FC // «, отравление: »
	descRequires      = 0x00450425 // «, требует: »
	descQuiver        = 0x00450433 // «Колчан стрел: »
	descUnknownArrows = 0x00450442 // « неведомые стрелы»
	descConcentration = 0x004504C9 // «, концентрация »
)

// ItemDescription is the status-line text for an item, as 0x004305E4 builds
// it (switch on the category, table 0x0043138C):
//
//   - gear (0–5): name, «убойная сила» (weapons 0–2) or «броня:» value,
//     «износ» current/max condition (floats +4/+8 truncated; 0 shows as 1),
//     weight, poisoning (+0xC, weapons only), special property, enchantment
//     effects and the type's requirement;
//   - arrows (12): «Колчан стрел:» count, name, damage, weight × count,
//     poisoning, special property;
//   - jewellery (6–8): name, weight, enchantment effects;
//   - herbs (9): name, «концентрация» (float +4, when record +0x78 < 0),
//     weight; misc (11): name, weight. Category 10 and others: nothing.
//
// Unidentified items (+0xE bit 0) show only «Неведомое оружие / лук /
// предмет» (arrows: the count and «неведомые стрелы»). Weights print as
// "%4.2f" kg; the game draws the text wrapped in the small font at the
// status line and sets the message counter 0x00A3E84C to 12.
func (g *Game) ItemDescription(t *resfmt.ExeTables, item int) []byte {
	it, ok := g.Item(item)
	if !ok {
		return nil
	}
	r := g.items[item*itemStride:][:itemStride]
	f32 := func(off int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(r[off:])) }
	poison := s16(r, 0xC)
	var s []byte
	add := func(b []byte) { s = append(s, b...) }
	str := func(va int) { add(t.CString(va)) }
	num := func(v int) { add([]byte(strconv.Itoa(v))) }
	kg := func(grams float64) { add([]byte(fmt.Sprintf("%4.2f", grams*0.001))) }
	enchant := func(e int) {
		en := t.Enchant(e)
		for _, fx := range en.Effects {
			if fx.Stat == 0 {
				continue
			}
			add(t.StatName(fx.Stat))
			if fx.Mask == 0 {
				add([]byte("="))
			} else if fx.Value > 0 {
				add([]byte("+"))
			}
			num(fx.Value)
		}
		if en.ReqStat != 0 {
			str(descRequires)
			add(t.StatName(en.ReqStat))
			add([]byte(":"))
			num(en.ReqMin)
		}
	}
	switch c := it.Category; {
	case c <= 5:
		if it.Flagged {
			switch c {
			case 0, 2:
				str(descUnknownWeapon)
			case 1:
				str(descUnknownBow)
			default:
				str(descUnknownItem)
			}
			return s
		}
		add(t.ItemName(resfmt.ItemTableGear, it.Type))
		if c <= 2 {
			str(descDamage)
		} else {
			str(descArmour)
		}
		num(t.GearValue(it.Type))
		str(descWear)
		num(max(1, int(f32(4))))
		add([]byte("/"))
		num(int(f32(8)))
		str(descWeight)
		kg(float64(t.ItemWeight(resfmt.ItemTableGear, it.Type)))
		if (c == 0 || c == 2) && poison != 0 {
			str(descPoison)
			num(poison)
		}
		if it.Special >= 0 {
			add(t.ItemSpecialName(it.Special))
		}
		if it.Enchant >= 0 {
			enchant(it.Enchant)
		}
		if stat, minimum := t.GearRequirement(it.Type); stat != 0 {
			str(descRequires)
			add(t.StatName(stat))
			add([]byte(":"))
			num(minimum)
		}
	case c == 12:
		str(descQuiver)
		num(it.Count)
		if it.Flagged {
			str(descUnknownArrows)
			return s
		}
		add(t.ItemName(resfmt.ItemTableGear, it.Type))
		str(descDamage)
		num(t.GearValue(it.Type))
		str(descWeight)
		kg(float64(t.ItemWeight(resfmt.ItemTableGear, it.Type) * it.Count))
		if poison != 0 {
			str(descPoison)
			num(poison)
		}
		if it.Special >= 0 {
			add(t.ItemSpecialName(it.Special))
		}
	case c <= 8:
		if it.Flagged {
			return t.CString(descUnknownItem)
		}
		add(t.ItemName(resfmt.ItemTableJewel, it.Type))
		str(descWeight)
		kg(float64(t.ItemWeight(resfmt.ItemTableJewel, it.Type)))
		if it.Enchant >= 0 {
			enchant(it.Enchant)
		}
	case c == 9:
		add(t.ItemName(resfmt.ItemTableHerb, it.Type))
		if t.ItemField16(resfmt.ItemTableHerb, it.Type, 0x78) < 0 {
			str(descConcentration)
			add([]byte(fmt.Sprintf("%5.2f", f32(4))))
		}
		str(descWeight)
		kg(float64(t.ItemWeight(resfmt.ItemTableHerb, it.Type)))
	case c == 11:
		if it.Flagged {
			return t.CString(descUnknownItem)
		}
		add(t.ItemName(resfmt.ItemTableMisc, it.Type))
		str(descWeight)
		kg(float64(t.ItemWeight(resfmt.ItemTableMisc, it.Type)))
	}
	return s
}
