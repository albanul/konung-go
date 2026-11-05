package gamestate

// Combat stance and weapon selection, ported from konung.exe.
//
// Body animation types come in two sets of six: 0–5 in the combat stance,
// 6–11 out of it (0/6 idle, 1/7 walk, 3/9 dead). Entering the stance moves
// an entity to the combat set and sets +3 bit 0x04; the equipment draw then
// shows the active weapon drawn (Game.Overlays).

// Equipment slots (+0x66 words). Slots 0–2 hold the three weapon kinds that
// WeaponMode selects; 3–5 are armour, helmet and shield (seen in GAME.0:
// «Доспех кожаный», «Шлем кожаный», «Деревянный щит»).
const (
	WeaponAxe   = 0 // club / axe (keyboard 2)
	WeaponBow   = 1 // bow (keyboard 3)
	WeaponBlade = 2 // knife / sword (keyboard 1)

	EquipArmour = 3
	EquipHelmet = 4
	EquipShield = 5
)

// WeaponCycle is the SelectWeapon mode of the HUD weapon button (0x00422C69
// passes -1): step to the next weapon slot that holds an item.
const WeaponCycle = -1

// stanceBit is entity +3 bit 0x04.
const stanceBit = 0x04

// InStance reports the combat stance (entity +3 bit 0x04).
func (e Entity) InStance() bool { return e.Flags3&stanceBit != 0 }

// canAct is the check the stance and weapon routines skip entities by: not
// dead (+4 bit 0x80) and not in the death animation (type 3 / 9).
func (e Entity) canAct() bool { return !e.Dead() && e.Type != 3 && e.Type != 9 }

// CombatAction maps a combat-set animation type to the action index of the
// combat equipment tables (0x004281BD): walk (1) → 3, 5 → 2, 4 → 1, else 0.
func CombatAction(typ int) int {
	switch typ {
	case 1:
		return 3
	case 5:
		return 2
	case 4:
		return 1
	}
	return 0
}

// ToggleStance is 0x0042179C (Space, HUD stance button): the first selected
// entity decides the direction; every selected entity that can act then
// enters or leaves the stance. nil entries end the list, as in the game's
// 10-entry selection list 0x0083B93C.
func ToggleStance(sel []*Entity) {
	if len(sel) == 0 || sel[0] == nil {
		return
	}
	enter := !sel[0].InStance()
	for _, e := range sel {
		if e == nil {
			break
		}
		if !e.canAct() {
			continue
		}
		if enter {
			if !e.InStance() {
				e.Type -= 6
				e.Flags3 |= stanceBit
			}
			continue
		}
		if e.InStance() {
			if e.Type > 3 {
				e.Type = 6 // an attack (4, 5) ends in idle
			} else {
				e.Type += 6
			}
			e.Flags2 &= 0xFA
			if e.Flags2 == 0 {
				e.Flags2 = 0x40
			}
			e.Flags3 ^= stanceBit
		}
	}
}

// SelectWeapon is the per-entity part of 0x0042192C (keys 1/2/3 pass
// WeaponBlade/WeaponAxe/WeaponBow, the HUD weapon button WeaponCycle). A
// weapon kind is selected only if its slot holds an item. Cycling goes
// axe → bow → blade → axe, skipping empty slots. Leaving the bow clears
// +2 bit 0x04 (and the game calls 0x00414AB4, not traced).
func SelectWeapon(e *Entity, mode int) {
	if e == nil || !e.canAct() {
		return
	}
	has := func(slot int) bool { return e.Equipment[slot] != 0 }
	switch mode {
	case WeaponAxe, WeaponBow, WeaponBlade:
		if has(mode) {
			e.WeaponMode = mode
		}
	default:
		next := map[int][2]int{
			WeaponAxe:   {WeaponBow, WeaponBlade},
			WeaponBow:   {WeaponBlade, WeaponAxe},
			WeaponBlade: {WeaponAxe, WeaponBow},
		}
		if n, ok := next[e.WeaponMode]; ok {
			if has(n[0]) {
				e.WeaponMode = n[0]
			} else if has(n[1]) {
				e.WeaponMode = n[1]
			}
		}
	}
	if e.WeaponMode != WeaponBow && e.Flags2&0x04 != 0 {
		e.Flags2 ^= 0x04
	}
}

// EnterStance puts one entity into the combat stance the way the AI does
// (0x00413108, 0x00416F65, 0x00421777): the action type moves to the stance
// set (− 6) and +3 bit 0x04 is set. Animated models have one set; only the
// bit changes for them.
func EnterStance(e *Entity) {
	if e.InStance() {
		return
	}
	if e.Type >= 6 {
		e.Type -= 6
	}
	e.Flags3 |= stanceBit
}

// LeaveStance is the calm village AI's reset (0x00416934): out of the stance
// and back to the idle type 6.
func LeaveStance(e *Entity) {
	if !e.InStance() {
		return
	}
	e.Flags3 &^= stanceBit
	if !e.Animated() {
		e.Type = 6
	}
}
