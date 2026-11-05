package resfmt

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"testing"
)

func TestLoadMusicTracks(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/MUSICS.RES")
	if err != nil {
		t.Skip("MUSICS.RES not available:", err)
	}
	tracks, err := LoadMusicTracks(data)
	if err != nil {
		t.Fatal(err)
	}
	for i, tr := range tracks {
		if !bytes.HasPrefix(tr, []byte("OggS")) || len(tr) < 1_000_000 {
			t.Errorf("track %d: %d bytes, prefix %q", i, len(tr), tr[:4])
		}
	}
}

func TestRenderSound(t *testing.T) {
	pcm := make([]byte, 8)
	for i, v := range []int16{0, 1000, 2000, 3000} {
		binary.LittleEndian.PutUint16(pcm[2*i:], uint16(v))
	}
	out := RenderSound(pcm, 22050, 44100, 1, 0.5)
	if len(out) != 8*4 {
		t.Fatalf("%d bytes", len(out))
	}
	frame := func(k int) (int16, int16) {
		return int16(binary.LittleEndian.Uint16(out[4*k:])), int16(binary.LittleEndian.Uint16(out[4*k+2:]))
	}
	if l, r := frame(1); l != 500 || r != 250 {
		t.Fatalf("frame 1 = %d,%d, want 500,250 (interpolated, right halved)", l, r)
	}
	if g := DSGain(-2000); math.Abs(g-0.1) > 1e-9 || DSGain(-10000) != 0 {
		t.Fatal("DSGain")
	}
	if l, r := DSPanGains(2000); math.Abs(l-0.1) > 1e-9 || r != 1 {
		t.Fatal("pan right attenuates the left channel")
	}
}
