package gamestate

import (
	"bytes"
	"testing"
)

// The installed KONUNG.CFG is 28 bytes: seven dwords, so Sound keeps its
// zero value.
func TestParseConfigInstalled(t *testing.T) {
	c := ParseConfig(readAsset(t, ConfigFile))
	want := Config{Video: 1, Transparency: 1, DayNight: 1, Lighting: 1, Music: 0, Tiling: 1, Shadows: 1, Sound: 0}
	if c != want {
		t.Fatalf("got %+v, want %+v", c, want)
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
