package game

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The tint shader compiles (Kage is compiled when the shader is created).
func TestTintShaderCompiles(t *testing.T) {
	if _, err := ebiten.NewShader([]byte(tintShaderSrc)); err != nil {
		t.Fatal(err)
	}
}
