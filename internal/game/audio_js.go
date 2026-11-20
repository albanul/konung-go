//go:build js

package game

import "syscall/js"

// webMusic is a MUSICS.RES track played by the browser (konungMusic in
// cmd/web-build/web/konungfs.js). Decoding and resampling the Ogg in Go
// runs on the game's only WebAssembly thread all the time, as the music
// keeps playing even when switched off (setMusic changes the volume).
type webMusic struct{ v js.Value }

// newWebMusic starts track (Ogg Vorbis) at volume 0 or returns nil
// without the page's helper (the Go decoder plays it then).
func newWebMusic(track []byte) musicPlayer {
	api := js.Global().Get("konungMusic")
	if api.IsUndefined() {
		return nil
	}
	data := js.Global().Get("Uint8Array").New(len(track))
	js.CopyBytesToJS(data, track)
	return &webMusic{v: api.Call("open", data)}
}

func (m *webMusic) SetVolume(v float64) { js.Global().Get("konungMusic").Call("volume", m.v, v) }

// IsPlaying is false once the track has ended or failed; a track waiting
// for the first user gesture (autoplay policy) counts as playing.
func (m *webMusic) IsPlaying() bool {
	return js.Global().Get("konungMusic").Call("playing", m.v).Bool()
}

func (m *webMusic) Close() error {
	js.Global().Get("konungMusic").Call("close", m.v)
	return nil
}
