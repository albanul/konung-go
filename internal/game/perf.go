package game

import (
	"fmt"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// perf is a port addition for measuring (e.g. the browser build against the
// native one): F3 shows FPS, TPS, the CPU time of Update and Draw averaged
// over the last second, and the scene's draw calls in the last frame.
var perf perfStats

type perfStats struct {
	shown                 bool
	upd, drw              time.Duration // sums over the current second
	nUpd, nDrw            int
	since                 time.Time
	updMs, drwMs          float64 // averages of the last full second
	draws, tinted         int     // counted during the current Draw
	lastDraws, lastTinted int
}

func (p *perfStats) update(start time.Time) {
	if inpututil.IsKeyJustPressed(ebiten.KeyF3) {
		p.shown = !p.shown
	}
	p.upd += time.Since(start)
	p.nUpd++
}

func (p *perfStats) draw(start time.Time) {
	p.drw += time.Since(start)
	p.nDrw++
	p.lastDraws, p.lastTinted = p.draws, p.tinted
	p.draws, p.tinted = 0, 0
	if now := time.Now(); now.Sub(p.since) >= time.Second {
		if p.nUpd > 0 {
			p.updMs = float64(p.upd.Microseconds()) / 1000 / float64(p.nUpd)
		}
		if p.nDrw > 0 {
			p.drwMs = float64(p.drw.Microseconds()) / 1000 / float64(p.nDrw)
		}
		p.upd, p.drw, p.nUpd, p.nDrw, p.since = 0, 0, 0, 0, now
	}
}

func (p *perfStats) overlay(screen *ebiten.Image) {
	if !p.shown {
		return
	}
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf(
		"FPS %.1f  TPS %.1f\nUpdate %.2f ms  Draw %.2f ms (CPU)\ndraws %d (tinted %d)",
		ebiten.ActualFPS(), ebiten.ActualTPS(), p.updMs, p.drwMs, p.lastDraws, p.lastTinted), 4, 4)
}
