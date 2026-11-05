package gamestate

// Sound selection (docs/konung_sound.md). The game plays a SOUNDS.RES entry
// with 0x0042A664(id, volume, pan, loop): volume and pan in DirectSound
// units (hundredths of a dB, −10000…0; pan −10000 left … 10000 right).
const (
	SoundJewellery   = 0  // lift or put on jewellery (0x0041E930)
	SoundEquipArmour = 1  // lift or put on equipment of category > 2 (0x0041E5B1, 0x0041E6E3)
	SoundEquip       = 11 // lift or put on a weapon (categories 0–2) or ammunition (0x0041E50C)
	SoundSwingAxe    = 2  // party/villager swing with axe or club (action 5)
	SoundShoot       = 3  // bow shot (action 4)
	SoundClick       = 4  // buttons, keys, window toggle, menu
	SoundDodge       = 6  // the party's blow is dodged (0x0042204B)
	SoundFootstep    = 7  // the hero starts walking (action 1)
	SoundSwingBlade  = 8  // swing with a blade (weapon mode 2)
	SoundDrink       = 9  // a herb or potion is used (0x0041E19D)
	SoundUse         = 10 // misc item use: the shovel and others (0x00414E64)

	// SoundMinVolume: quieter sounds are not played (0x0042A684).
	SoundMinVolume = -4000
	soundSilent    = -10000
	// SoundAmbientBase + (map − 1)·8 + 0…7 are a map's ambient sounds,
	// played with a 1 % chance per frame (0x004376F4).
	SoundAmbientBase   = 0x100
	SoundAmbientPerMap = 8
)

// sound queues a non-positional sound (volume 0, pan 0) for the scene to
// play; game rules that the game sounds from go through it.
func (g *Game) sound(id int) { g.Sounds = append(g.Sounds, id) }

// equipSound is 0x0041E500's choice for item i.
func (g *Game) equipSound(i int) int {
	if it, ok := g.Item(i); ok && int8(it.Category) > 2 {
		return SoundEquipArmour
	}
	return SoundEquip
}

// SoundVolume is 0x00439F4C: the volume of a sound at world offset
// (dx, dy) pixels from the hero (fine-cell centres, 0x00439BA0): −10000·d/2048
// with d the truncated distance, silent beyond 2048 px.
func SoundVolume(dx, dy int) int {
	d := isqrt(dx*dx + dy*dy)
	if d > 0x800 {
		return soundSilent
	}
	return d * soundSilent / 0x800
}

// SoundPan is 0x00439EEC: (col − hero col) · 10000 · 0.00625, truncated.
func SoundPan(dcol int) int {
	return int(float64(dcol) * 10000 * 0.00625)
}

func isqrt(v int) int {
	r := 0
	for (r+1)*(r+1) <= v {
		r++
	}
	return r
}

// SoundVariants reports whether a sound picks one of the three wave formats
// at random (22050, 19050, 25050 Hz; 0x0042A7D7): character voices
// (id ≥ 0x20) and sounds 2, 3, 6, 7, 8.
func SoundVariants(id int) bool {
	return id >= 0x20 || id == 2 || id == 3 || id == 6 || id == 7 || id == 8
}

// SoundRates are the sample rates of the three formats (0x00A4E368).
var SoundRates = [3]int{22050, 19050, 25050}

// ActionSound is the sound 0x004143DC plays when entity e starts action
// act (0 = none). Animated models (creatures, women, children) use their
// +0x04 byte: base = +0x04·8 − 0x1D0; humans their model (+0xD6): voice =
// model·8 + 0x22. rnd(100) is the game's rand() % 100.
func ActionSound(e Entity, act int, hero bool, rnd func(n int) int) int {
	if e.Animated() {
		base := int(e.Flags4)*8 - 0x1D0
		switch act { // table 0x00414500, actions 0–5
		case ActIdle:
			if rnd(100) > 95 {
				return base + 3
			}
		case ActWalk:
			if rnd(100) > 75 {
				return base + 3
			}
		case ActFlinch:
			return base + 4
		case ActDeath:
			return base + 2
		case ActAttack:
			return base
		}
		return 0
	}
	voice := e.Model*8 + 0x22
	if act < 0 || act > 9 {
		return 0
	}
	switch act % 6 { // table 0x00414768, actions 0–9 (6–9 outside the stance)
	case ActIdle:
		if rnd(100) > 95 {
			return voice + 1
		}
	case ActWalk:
		if hero {
			return SoundFootstep
		}
	case ActFlinch:
		return voice + 2
	case ActDeath:
		return voice
	case ActShoot:
		return SoundShoot
	case ActAttack:
		if e.WeaponMode == WeaponBlade {
			return SoundSwingBlade
		}
		return SoundSwingAxe
	}
	return 0
}

// WakeSound is the cry of an animated model waking from its dormant pose,
// at step 1 of action 4 (0x00412A60): base + 1.
func WakeSound(e Entity) int { return int(e.Flags4)*8 - 0x1D0 + 1 }

// AmbientSound is 0x004376F4: with rand() % 100 > 98, one of the map's
// eight ambient sounds; −1 = none this frame.
func AmbientSound(mapNumber int, rnd func(n int) int) int {
	if rnd(100) <= 98 {
		return -1
	}
	return SoundAmbientBase + (mapNumber-1)*SoundAmbientPerMap + rnd(SoundAmbientPerMap)
}
