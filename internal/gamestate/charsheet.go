package gamestate

import (
	"fmt"
	"strconv"

	"konung1.5/internal/resfmt"
)

// Character window (window 3, drawn by 0x0042E04C; opened with I or the HUD
// bag button). Layout tables are in resfmt.CharWindow; this file holds what
// is shown.

// Number of the 25 character-window values (resfmt.CharWindow.Stats).
const (
	SheetExperience   = 0  // +0x3C «Опыт»
	SheetLevel        = 1  // +0x33 «Уровень»
	SheetStatsNow     = 2  // 2..5: current Хар, Сил, Лов, Вын («сейчас»)
	SheetHealth       = 6  // (HP + 15) / 16 «Здоровье»
	SheetStatsBase    = 7  // 7..10: base Хар, Сил, Лов, Вын («базовая»)
	SheetMarksmanship = 11 // +0x44 / 100 «Меткость»
	SheetFencing      = 12 // +0x40 / 100 «Фехтование»
	SheetSkills       = 13 // 13..17: +0x4A, +0x42, +0x48, +0x4C, +0x46
	SheetWeight       = 18 // «Текущий вес», %4.1f
	SheetMaxWeight    = 19 // «Максим. вес», %4.1f
	SheetPoison       = 20 // +0x50 «Отравление»
	SheetArmour       = 21 // «Броня»
	SheetDamage       = 22 // «Удар»
	SheetGold         = 23 // +0x34 «Золото»
	SheetFreeExp      = 24 // +0x38 «Свободный опыт»
	SheetValues       = 25
)

// SheetEquipment reports which equipment slots the window draws
// (0x0042E260): the axe/club (slot 0) and the blade (slot 2) share the hand
// box. The axe is shown unless the blade is the active weapon and present;
// the blade is shown only when the axe isn't.
func SheetEquipment(e Entity) (shown [6]bool) {
	axeShown := false
	for slot := range e.Equipment {
		if e.Equipment[slot] == 0 {
			continue
		}
		switch slot {
		case WeaponAxe:
			if e.WeaponMode == WeaponBlade && e.Equipment[WeaponBlade] != 0 {
				continue
			}
			axeShown = true
		case WeaponBlade:
			if axeShown {
				continue
			}
		}
		shown[slot] = true
	}
	return shown
}

// CharSheet returns the window's 25 numbers as the game prints them
// (0x0042E6B1–0x0042F21D): integers with itoa, the two weights with
// "%4.1f" of value·0.001.
func (g *Game) CharSheet(e Entity, t *resfmt.ExeTables) [SheetValues]string {
	e = g.Recompute(e, t) // the window calls 0x0041C750 when it opens (0x0042E074)
	var v [SheetValues]string
	itoa := strconv.Itoa
	v[SheetExperience] = itoa(e.Experience)
	v[SheetLevel] = itoa(e.Level)
	for i := range 4 {
		v[SheetStatsNow+i] = itoa(e.Stats[i])
		v[SheetStatsBase+i] = itoa(e.BaseStats[i])
	}
	v[SheetHealth] = itoa((e.HP + 15) / 16) // signed, truncating like the game's sar/sbb
	v[SheetMarksmanship] = itoa(e.Marksmanship / 100)
	v[SheetFencing] = itoa(e.Fencing / 100)
	// Skills holds healing, smithing, trade, building, curios; the window
	// prints curios, trade, healing, smithing, building.
	for i, k := range [5]int{4, 2, 0, 1, 3} {
		v[SheetSkills+i] = itoa(e.Skills[k])
	}
	v[SheetWeight] = weight(g.CarriedWeight(e, t))
	v[SheetMaxWeight] = weight(MaxWeight(e))
	v[SheetPoison] = itoa(e.Poison)
	v[SheetArmour] = itoa(g.Armour(e, t))
	v[SheetDamage] = itoa(g.SheetDamage(e, t))
	v[SheetGold] = itoa(e.Gold)
	v[SheetFreeExp] = itoa(e.FreeExp)
	return v
}

// MaxWeight is «Максим. вес» in grams: Вын·1000/3 + 10000 (0x0042EF48).
func MaxWeight(e Entity) int { return e.Stats[3]*1000/3 + 10000 }

func weight(grams int) string { return fmt.Sprintf("%4.1f", float64(grams)*0.001) }
