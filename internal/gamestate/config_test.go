package gamestate

import (
	"bytes"
	"testing"
)

// The installed KONUNG.CFG is the original 28-byte file (seven dwords, so
// Sound keeps its zero value) or the 32 bytes the settings screen writes;
// the game rewrites it, so only its shape is checked.
func TestParseConfigInstalled(t *testing.T) {
	data := readAsset(t, ConfigFile)
	if len(data) != 28 && len(data) != ConfigSize {
		t.Fatalf("size %d", len(data))
	}
	c := ParseConfig(data)
	if c.Video < 0 || c.Video > 2 {
		t.Fatalf("video %d", c.Video)
	}
	for _, v := range []int32{c.Transparency, c.DayNight, c.Lighting, c.Music, c.Tiling, c.Shadows, c.Sound} {
		if v != 0 && v != 1 {
			t.Fatalf("setting %d in %+v", v, c)
		}
	}
	if len(data) == 28 && c.Sound != 0 {
		t.Fatal("a 28-byte file leaves Sound at 0")
	}
}

func TestConfigRoundTrip(t *testing.T) {
	c := Config{Video: 0, Transparency: 1, DayNight: 0, Lighting: 1, Music: 0, Tiling: 1, Shadows: 0, Sound: 1}
	data := c.Marshal()
	if len(data) != ConfigSize {
		t.Fatalf("size %d", len(data))
	}
	if got := ParseConfig(data); got != c {
		t.Fatalf("got %+v, want %+v", got, c)
	}
	if ParseConfig(nil) != DefaultConfig() {
		t.Fatal("no file: want defaults")
	}
	bad := Config{Video: 7}.Marshal()
	if ParseConfig(bad).Video != 0 {
		t.Fatal("video mode 7 must become 0")
	}
	if !bytes.Equal(DefaultConfig().Marshal()[:4], []byte{1, 0, 0, 0}) {
		t.Fatal("video first")
	}
}
