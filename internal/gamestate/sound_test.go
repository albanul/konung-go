package gamestate

import "testing"

func TestSoundRules(t *testing.T) {
	if SoundVolume(0, 0) != 0 || SoundVolume(2049, 0) != -10000 || SoundVolume(1024, 0) != -5000 {
		t.Fatal("volume falls off linearly to −10000 at 2048 px")
	}
	if SoundVolume(819, 0) < SoundMinVolume || SoundVolume(820, 0) >= SoundMinVolume {
		t.Fatalf("audible range ends at 820 px: %d %d", SoundVolume(819, 0), SoundVolume(820, 0))
	}
	if SoundPan(10) != 625 || SoundPan(-160) != -10000 {
		t.Fatal("pan is 62.5 per column")
	}
	never := func(int) int { return 0 }
	always := func(n int) int { return n - 1 }
	creature := Entity{Flags4: 0x45} // animated model 5
	base := 0x45*8 - 0x1D0           // 8·5 + 48
	if ActionSound(creature, ActAttack, false, never) != base || ActionSound(creature, ActDeath, false, never) != base+2 ||
		ActionSound(creature, ActIdle, false, never) != 0 || ActionSound(creature, ActIdle, false, always) != base+3 ||
		WakeSound(creature) != base+1 {
		t.Fatal("creature sounds")
	}
	human := Entity{Model: 1, WeaponMode: WeaponBlade}
	if ActionSound(human, ActDeath, false, never) != 42 || ActionSound(human, ActFlinch, false, never) != 44 ||
		ActionSound(human, ActAttack, false, never) != SoundSwingBlade || ActionSound(human, ActWalk, true, never) != SoundFootstep ||
		ActionSound(human, ActWalk, false, never) != 0 || ActionSound(human, ActShoot, false, never) != SoundShoot {
		t.Fatal("human sounds")
	}
	if AmbientSound(1, never) != -1 || AmbientSound(4, always) != 0x100+3*8+7 {
		t.Fatal("ambient sounds")
	}
}
