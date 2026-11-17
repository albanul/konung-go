//go:build js

package game

import (
	"syscall/js"

	"konung1.5/internal/gamefs"
	"konung1.5/internal/resfmt"
)

// webFilm is a film played by the browser: the pure-Go AV1 decoder is far
// too slow in WebAssembly, so the page shows a <video> element over the
// game's FilmRect (konungFilm in cmd/web-build/web/konungfs.js) while the
// frozen screen is drawn around it.
type webFilm struct{ v js.Value }

// openWebFilm starts the film's WebM if the build served one; otherwise
// nil, and the film is decoded in Go.
func openWebFilm(gameDir, name string) *webFilm {
	path := resfmt.FilmWebM(gameDir, name)
	api := js.Global().Get("konungFilm")
	if api.IsUndefined() || !gamefs.Exists(path) {
		return nil
	}
	r := resfmt.FilmRect
	return &webFilm{v: api.Call("open", path, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), ScreenWidth, ScreenHeight)}
}

// ended reports that the film played to its end or failed to play.
func (w *webFilm) ended() bool { return js.Global().Get("konungFilm").Call("ended", w.v).Bool() }

func (w *webFilm) close() { js.Global().Get("konungFilm").Call("close", w.v) }
