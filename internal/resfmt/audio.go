package resfmt

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Music and sound helpers (docs/konung_sound.md).

// MusicTracks is the number of Ogg Vorbis tracks in MUSICS.RES.
const MusicTracks = 9

// LoadMusicTracks splits MUSICS.RES (loader 0x00438E04): the 8-byte header
// (tag 1 big-endian, data size little-endian), a table of 9 × (offset,
// size) at 0x006324B8, then the data the offsets are relative to. Each
// track is a complete Ogg Vorbis stream.
func LoadMusicTracks(data []byte) ([][]byte, error) {
	const dataStart = 8 + MusicTracks*8
	if len(data) < dataStart || binary.BigEndian.Uint32(data) != 1 {
		return nil, fmt.Errorf("MUSICS.RES: bad header")
	}
	body := data[dataStart:]
	if size := binary.LittleEndian.Uint32(data[4:]); int(size) != len(body) {
		return nil, fmt.Errorf("MUSICS.RES: data size %d, file holds %d", size, len(body))
	}
	tracks := make([][]byte, MusicTracks)
	for i := range tracks {
		off := int(binary.LittleEndian.Uint32(data[8+8*i:]))
		size := int(binary.LittleEndian.Uint32(data[12+8*i:]))
		if off < 0 || size < 0 || off+size > len(body) {
			return nil, fmt.Errorf("MUSICS.RES: track %d out of range", i)
		}
		tracks[i] = body[off : off+size]
	}
	return tracks, nil
}

// DSGain converts a DirectSound attenuation in hundredths of a decibel
// (0 … −10000) to a linear amplitude factor.
func DSGain(hundredthsDB int) float64 {
	if hundredthsDB <= -10000 {
		return 0
	}
	return math.Pow(10, float64(hundredthsDB)/2000)
}

// DSPanGains returns the left and right factors for a DirectSound pan
// (−10000 … 10000): the side away from the pan is attenuated by |pan|.
func DSPanGains(pan int) (left, right float64) {
	switch {
	case pan > 0:
		return DSGain(-pan), 1
	case pan < 0:
		return 1, DSGain(pan)
	}
	return 1, 1
}

// RenderSound converts a SOUNDS.RES sample (mono 16-bit little-endian PCM)
// played at srcRate to 16-bit little-endian stereo at dstRate, with linear
// interpolation and the given channel factors. Playing a sample at another
// rate than it was recorded changes its pitch, which is how the game varies
// voices (three wave formats, 0x00A4E368).
func RenderSound(pcm []byte, srcRate, dstRate int, left, right float64) []byte {
	n := len(pcm) / 2
	if n == 0 || srcRate <= 0 || dstRate <= 0 {
		return nil
	}
	sample := func(i int) float64 { return float64(int16(binary.LittleEndian.Uint16(pcm[2*i:]))) }
	frames := int(int64(n) * int64(dstRate) / int64(srcRate))
	out := make([]byte, frames*4)
	clamp := func(v float64) uint16 { return uint16(int16(math.Max(-32768, math.Min(32767, math.Round(v))))) }
	for k := range frames {
		pos := float64(k) * float64(srcRate) / float64(dstRate)
		i := int(pos)
		v := sample(i)
		if i+1 < n {
			v += (sample(i+1) - v) * (pos - float64(i))
		}
		binary.LittleEndian.PutUint16(out[4*k:], clamp(v*left))
		binary.LittleEndian.PutUint16(out[4*k+2:], clamp(v*right))
	}
	return out
}
