//go:build !js

package game

// webFilm exists only in the browser build (film_js.go).
type webFilm struct{}

func openWebFilm(string, string) *webFilm { return nil }
func (*webFilm) ended() bool              { return true }
func (*webFilm) close()                   {}
