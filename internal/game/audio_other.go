//go:build !js

package game

// newWebMusic exists only in the browser build (audio_js.go).
func newWebMusic([]byte) musicPlayer { return nil }
