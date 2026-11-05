package game

import (
	"fmt"
	"image"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

// Day and night (docs/konung_daylight.md). Each drawn frame the game turns
// the time of day into a light tint (0x00429330) and draws the world through
// palettes tinted with it (0x0043ED09). The tint is a per-channel lookup on
// the 5-bit colour values, so the port applies the same tables to the
// decoded sprites with a shader instead of re-decoding them.

// tintShaderSrc maps each 5-bit channel through its table (index = value).
// The loops use constant bounds so the tables are only indexed by the loop
// counter.
const tintShaderSrc = `//kage:unit pixels
package main

var R [32]float
var G [32]float
var B [32]float

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	if c.a == 0 {
		return vec4(0)
	}
	i := floor(c.rgb/c.a*31 + 0.5)
	o := vec3(0)
	for j := 0; j < 32; j++ {
		if float(j) == i.r {
			o.r = R[j]
		}
		if float(j) == i.g {
			o.g = G[j]
		}
		if float(j) == i.b {
			o.b = B[j]
		}
	}
	return vec4(o*c.a, c.a)
}
`

var tintShader *ebiten.Shader

func loadTintShader() (*ebiten.Shader, error) {
	if tintShader != nil {
		return tintShader, nil
	}
	sh, err := ebiten.NewShader([]byte(tintShaderSrc))
	if err != nil {
		return nil, fmt.Errorf("tint shader: %w", err)
	}
	tintShader = sh
	return sh, nil
}

// tintUniforms are the shader tables for a tint, as fractions of 31.
func tintUniforms(t resfmt.Tint) map[string]any {
	table := func(n int) []float32 {
		tb := resfmt.TintTable(n)
		out := make([]float32, len(tb))
		for i, v := range tb {
			out[i] = float32(v) / 31
		}
		return out
	}
	return map[string]any{"R": table(t.R), "G": table(t.G), "B": table(t.B)}
}

// loadLights reads LIGHTS.<map> next to KONUNG.<map>; the loader zero-fills
// both parts when the file is missing (0x0043C23C).
func loadLights(dir string, mapNumber int) (*mapdata.LitCells, *mapdata.TerrainLights, error) {
	data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("LIGHTS.%d", mapNumber)))
	if err != nil {
		data = nil
	}
	lit, err := mapdata.ParseLitCells(data)
	if err != nil {
		return nil, nil, err
	}
	terrain, err := mapdata.ParseTerrainLights(data)
	return lit, terrain, err
}

// updateLight computes this frame's tint like 0x00429330: the moon is rolled
// whenever the time of day is exactly 0; maps with a fixed light (caves)
// ignore the time; with day/night off the world is untinted.
func (s *startScene) updateLight() {
	now := s.gameTime()
	if now%gamestate.DayLength == 0 {
		s.moon = s.rng.IntN(100) < gamestate.MoonChance
	}
	t, night := resfmt.Tint{}, false
	if fixed, ok := s.tables.MapLight(s.mapNumber); ok {
		t = fixed
	} else if s.dayNight {
		t, night = gamestate.DayTint(now, s.tables.DayThresholds(), s.moon)
	}
	if t != s.tint || s.tintU == nil {
		s.tint, s.tintU = t, tintUniforms(t)
	}
	// 0x0042894C rolls the flicker every drawn frame: green 8–11 %, red
	// 16–23 %. The lit path (0x0042565D: night lights 0x00A4E2A0 and
	// «Световые эффекты») passes it halved, in full while the map's village
	// is alarmed (0x004257A9).
	g := s.rng.IntN(4) + 8
	r := s.rng.IntN(8) + 16
	s.litPath = night && s.lighting && s.assets.lightMasks != nil
	if st := s.game.SettlementOnMap(s.mapNumber); st == nil || s.game.Groups[st.Group].Alarm != gamestate.AlarmAttack {
		r, g = r/2, g/2
	}
	s.flicker = resfmt.LitTerrainLight{Day: t, FlickR: r, FlickG: g}
	s.litTables = nil // rebuilt on the first lit cell of the frame
}

// drawLitCell draws terrain cell (r, c) through the lit path when its
// LIGHTS.N byte is set (0x004258ED): both layers composed into the 114×64
// buffer and lit with light mask L − 1 (0x0043DFE1). It reports whether the
// cell was lit.
func (s *startScene) drawLitCell(dst *ebiten.Image, r, c, x, y int) bool {
	l := int(s.terrainLights[r][c])
	if l == 0 || l > len(s.assets.lightMasks) {
		return false
	}
	if s.litBuf == nil {
		s.litBuf = image.NewRGBA(image.Rect(0, 0, resfmt.LightMaskWidth, resfmt.LightMaskHeight))
		s.litCells = map[[2]int]*litCell{}
	}
	cell := s.litCells[[2]int{r, c}]
	if cell == nil {
		var layers [2]*image.RGBA
		for i, b := range s.m.Cells[r][c] {
			if b != 0 && int(b) <= len(s.assets.rawTiles) {
				layers[i] = s.assets.rawTiles[b-1]
			}
		}
		cell = &litCell{
			buf: resfmt.ComposeTerrainCell(layers[0], layers[1]),
			img: ebiten.NewImage(resfmt.LightMaskWidth, resfmt.LightMaskHeight),
		}
		s.litCells[[2]int{r, c}] = cell
	}
	if s.litTables == nil {
		s.litTables = resfmt.NewLitTables(s.flicker)
	}
	resfmt.LightTerrainCell(s.litBuf, cell.buf, s.assets.lightMasks[l-1], s.litTables)
	cell.img.WritePixels(s.litBuf.Pix)
	drawAt(dst, cell.img, x, y)
	return true
}

// litCell is a lit terrain cell: its composed layers and the image the
// lighting is written to each frame.
type litCell struct {
	buf *resfmt.TerrainCellBuffer
	img *ebiten.Image
}

// drawTinted draws a world image at screen (x, y) through the day/night
// tint; raw images (self-lit objects, characters on lit cells) and an
// untinted frame use plain drawing.
func (s *startScene) drawTinted(dst, img *ebiten.Image, x, y int, raw bool) {
	if raw || s.tint.Zero() || s.tintSh == nil {
		drawAt(dst, img, x, y)
		return
	}
	b := img.Bounds()
	op := &ebiten.DrawRectShaderOptions{Uniforms: s.tintU}
	op.Images[0] = img
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawRectShader(b.Dx(), b.Dy(), s.tintSh, op)
}
