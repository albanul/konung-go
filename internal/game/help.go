package game

import (
	"bytes"
	"fmt"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/goregular"
)

// The key help is a port addition: a small box in the top-right corner of
// the game view, in Go Regular (crisp at native resolution, unlike the
// 6×16 debug font; Go Regular covers Cyrillic), in Russian, toggled with F1.

const helpFontSize = 11

var helpFace = func() *text.GoTextFace {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		log.Printf("help font: %v", err)
		return nil
	}
	return &text.GoTextFace{Source: src, Size: helpFontSize}
}()

// helpLines is the key help for the current state.
func (s *startScene) helpLines() []string {
	follow := "выкл"
	if s.follow {
		follow = "вкл"
	}
	return []string{
		"Левый клик: идти     Правый клик: идти и подобрать",
		"Пробел: боевая стойка     A: атаковать ближайшего врага",
		"Правый клик по персонажу в стойке: атаковать, иначе: говорить",
		"Ctrl + правый клик по члену отряда: говорить",
		"1/2/3: оружие     I: персонаж     ~: сумка     Q: журнал заданий",
		fmt.Sprintf("+/-: скорость x%d     C: следить камерой (%s)     Стрелки: прокрутка", s.speed+1, follow),
		"F: «Все ко мне» (отряд собирается)",
		"Esc: меню     F1: скрыть подсказку",
	}
}

// drawHelp draws the key help box unless it is hidden.
func (s *startScene) drawHelp(dst *ebiten.Image) {
	if s.hideHelp || helpFace == nil {
		return
	}
	lines := s.helpLines()
	lineH := helpFace.Metrics().HAscent + helpFace.Metrics().HDescent + 2
	w := 0.0
	for _, l := range lines {
		w = max(w, text.Advance(l, helpFace))
	}
	const pad = 6
	boxW, boxH := float32(w)+2*pad, float32(lineH)*float32(len(lines))+2*pad
	x0, y0 := float32(viewRight)-boxW-4, float32(viewTop)+4
	vector.FillRect(dst, x0, y0, boxW, boxH, color.RGBA{0, 0, 0, 0xA0}, false)
	op := &text.DrawOptions{}
	for i, l := range lines {
		op.GeoM.Reset()
		op.GeoM.Translate(float64(x0)+pad, float64(y0)+pad+float64(i)*lineH)
		op.ColorScale.Reset()
		op.ColorScale.ScaleWithColor(color.RGBA{0xE8, 0xE0, 0xC8, 0xFF})
		text.Draw(dst, l, helpFace, op)
	}
}
