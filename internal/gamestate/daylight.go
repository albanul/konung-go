package gamestate

import "konung1.5/internal/resfmt"

// Day and night (0x00429330, called each frame by the scene draw 0x0042894C):
// the light tint 0x0051EC7C for the time of day. Every terrain, decoration,
// pile, object and character palette goes through it (0x0043ED09); see
// docs/konung_daylight.md.

// DayLength is the length of a day in game time units (0x00A3E770 ticks).
const DayLength = 0x5460

// MoonChance is the chance in percent that a night is moonlit: rolled as
// rand() % 100 < 90 whenever the time of day is exactly 0 (0x00429364).
const MoonChance = 90

// DayTint returns the tint for game time now with day/night on. th are the
// segment times (resfmt.ExeTables.DayThresholds); moon is 0x00A3E738.
// nightLights is 0x00A4E2A0, which lets the «Световые эффекты» setting
// switch the terrain to the LIGHTS.N path (0x0042565D).
func DayTint(now int, th [12]int, moon bool) (t resfmt.Tint, nightLights bool) {
	d := now % DayLength
	if d < 0 {
		d += DayLength
	}
	ramp := func(start, limit int) (int, bool) { // offset into a transition, halved
		v := d - start
		if v > limit {
			return 0, false
		}
		return v / 2, true
	}
	trunc := func(f float64) int { return int(f) } // 0x0043FE4C rounds toward zero
	switch {
	case d < th[0]:
		return resfmt.Tint{}, false
	case d < th[1]: // sunrise glow fades in
		v, ok := ramp(th[0], 0x18)
		if !ok {
			return resfmt.Tint{R: 12, G: 6}, false
		}
		return resfmt.Tint{R: v, G: v / 2}, false
	case d < th[2]:
		return resfmt.Tint{R: 12, G: 6}, false
	case d < th[3]: // and out
		v := d - th[2]
		if v > 0x18 {
			return resfmt.Tint{}, false
		}
		v = (0x18 - v) / 2
		return resfmt.Tint{R: v, G: v / 2}, false
	case d < th[4]:
		return resfmt.Tint{}, false
	case d < th[5]: // evening
		v, ok := ramp(th[4], 100)
		if !ok {
			return resfmt.Tint{R: 10, G: -20, B: -50}, false
		}
		return resfmt.Tint{R: trunc(float64(v) * 0.2), G: trunc(float64(-v) * 0.4), B: -v}, false
	}
	// From here 0x00A4E2A0 is set until the dawn segment ends.
	switch {
	case d < th[6]: // dusk
		v, ok := ramp(th[5], 60)
		if !ok {
			return resfmt.Tint{G: -50, B: -70}, true
		}
		return resfmt.Tint{R: trunc(10 - float64(v)*(1.0/3)), G: -20 - v, B: trunc(-50 - float64(v)*(2.0/3))}, true
	case d < th[7]: // night falls
		v, ok := ramp(th[6], 120)
		if !ok {
			return resfmt.Tint{R: -60, G: -70, B: -70}, true
		}
		return resfmt.Tint{R: -v, G: -50 - v/3, B: -70}, true
	case d < th[8]: // moonrise
		if !moon {
			return resfmt.Tint{R: -60, G: -70, B: -70}, true
		}
		v, ok := ramp(th[7], 40)
		if !ok {
			return resfmt.Tint{R: -60, G: -50, B: -70}, true
		}
		return resfmt.Tint{R: -60, G: v - 70, B: -70}, true
	case d < th[9]:
		if !moon {
			return resfmt.Tint{R: -60, G: -70, B: -70}, true
		}
		return resfmt.Tint{R: -60, G: -50, B: -70}, true
	case d < th[10]: // moonset
		if !moon {
			return resfmt.Tint{R: -60, G: -70, B: -70}, true
		}
		v, ok := ramp(th[9], 40)
		if !ok {
			return resfmt.Tint{R: -60, G: -70, B: -70}, true
		}
		return resfmt.Tint{R: -60, G: -50 - v, B: -70}, true
	case d < th[11]: // dawn
		v, ok := ramp(th[10], 100)
		if !ok {
			return resfmt.Tint{R: -30, G: -60, B: -20}, true
		}
		return resfmt.Tint{R: trunc(float64(v)*0.5988023952095808 - 60), G: v/5 - 70, B: v - 70}, true
	}
	v, ok := ramp(th[11], 120) // morning
	if !ok {
		return resfmt.Tint{}, false
	}
	return resfmt.Tint{R: v/2 - 30, G: v - 60, B: v/3 - 20}, false
}
