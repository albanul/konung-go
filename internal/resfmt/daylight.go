package resfmt

import "encoding/binary"

// Tint is a light tint for the palette tables (0x0043FD18 convention): a
// signed percentage per channel, negative darkens. The game packs it as
// R<<16 | G<<8 | B with signed bytes (0x0051EC7C).
type Tint struct{ R, G, B int }

// UnpackTint splits a packed tint.
func UnpackTint(v uint32) Tint {
	return Tint{R: int(int8(v >> 16)), G: int(int8(v >> 8)), B: int(int8(v))}
}

// Zero reports whether the tint leaves colours unchanged; the game then
// uses the raw palette (0x0043ED09).
func (t Tint) Zero() bool { return t == Tint{} }

const (
	dayThresholdsVA = 0x0045AE70 // 12 int16 times of day (0x00429330 reads them as dword>>16 from 0x0045AE6E)
	mapLightFlagVA  = 0x0045AE8F // byte per map: fixed light (0x0043C108, into 0x00A3E70C)
	mapLightTintVA  = 0x0045AD40 // packed tint per map, used when the flag is set
)

// DayThresholds returns the times of day (ticks mod 21600) at which the
// day/night routine 0x00429330 changes segment.
func (t *ExeTables) DayThresholds() [12]int {
	var th [12]int
	b := t.at(dayThresholdsVA)
	for i := range th {
		th[i] = int(int16(binary.LittleEndian.Uint16(b[2*i:])))
	}
	return th
}

// MapLight returns a map's fixed light: maps with the flag set (the caves,
// KONUNG.42 and up) ignore the time of day and use their table tint.
func (t *ExeTables) MapLight(mapNumber int) (Tint, bool) {
	if mapNumber < 0 || mapNumber > 255 || t.at(mapLightFlagVA)[mapNumber] == 0 {
		return Tint{}, false
	}
	return UnpackTint(binary.LittleEndian.Uint32(t.at(mapLightTintVA + 4*mapNumber))), true
}
