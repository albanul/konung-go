package gamestate

import "encoding/binary"

// Settings file KONUNG.CFG, read at startup by 0x0042BA60 and written by
// 0x0043D2CC when the settings screen is closed with «Ok»; see
// docs/konung_settings.md. Eight little-endian dwords in this order.
const ConfigFile = "KONUNG.CFG"

// Config holds the settings globals. Each is 0 (off) or 1 (on) except Video.
type Config struct {
	Video        int32 // 0x00A3E774: 0 = 640×480, 1 = 800×600
	Transparency int32 // 0x00A3E700: alpha-blended flames (0x0043D8EF)
	DayNight     int32 // 0x00A3E71C: day/night light level (0x004293B3); saved with a game
	Lighting     int32 // 0x00A3E734: lit terrain path with LIGHTS.N (0x0042565D); saved with a game
	Music        int32 // 0x00A3E73C: music volume (0x0042A420)
	Tiling       int32 // 0x00A3E744: background tiling (0x0043141C); not on the screen
	Shadows      int32 // 0x00A3E750: the shadow pass (0x004266C4)
	Sound        int32 // 0x00A3E75C: sound effects (0x0042A666)
}

// ConfigSize is the size of the file the game writes.
const ConfigSize = 8 * 4

// DefaultConfig is the configuration without a KONUNG.CFG (0x0042BB58):
// every setting on and 800×600.
func DefaultConfig() Config {
	return Config{Video: 1, Transparency: 1, DayNight: 1, Lighting: 1, Music: 1, Tiling: 1, Shadows: 1, Sound: 1}
}

func (c *Config) fields() [8]*int32 {
	return [8]*int32{&c.Video, &c.Transparency, &c.DayNight, &c.Lighting, &c.Music, &c.Tiling, &c.Shadows, &c.Sound}
}

// ParseConfig decodes KONUNG.CFG. A nil data (no file) gives DefaultConfig.
// The loader reads each dword separately, so a short file leaves the
// remaining settings at their zero (.bss) value. A video mode outside 0–2
// becomes 0 (0x0042BB3A).
func ParseConfig(data []byte) Config {
	if data == nil {
		return DefaultConfig()
	}
	var c Config
	for i, f := range c.fields() {
		if len(data) < 4*(i+1) {
			break
		}
		*f = int32(binary.LittleEndian.Uint32(data[4*i:]))
	}
	if c.Video < 0 || c.Video > 2 {
		c.Video = 0
	}
	return c
}

// Marshal encodes the configuration as 0x0043D2CC writes it.
func (c Config) Marshal() []byte {
	out := make([]byte, ConfigSize)
	for i, f := range c.fields() {
		binary.LittleEndian.PutUint32(out[4*i:], uint32(*f))
	}
	return out
}
