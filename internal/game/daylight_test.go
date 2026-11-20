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

// TestLitShaderCompiles: the lit terrain shader (litShaderSrc) is valid Kage.
func TestLitShaderCompiles(t *testing.T) {
	if _, err := loadLitShader(); err != nil {
		t.Fatal(err)
	}
}
