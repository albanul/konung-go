package game

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

// Flames on burning map objects (docs/konung_flames.md): FLAMES.RES frames
// drawn over the object by the sprite pass (0x0042708D), alpha-blended when
// «Прозрачность» is on (0x0043D8EF), otherwise copied (0x0043E696).

// flameImages are the FLAMES.RES frames as drawn without transparency and
// premultiplied for the transparent blend (the game converts them at load,
// 0x00427D9C, and reloads them when the setting changes).
type flameImages struct {
	opaque, blended []*ebiten.Image
}

func loadFlames(path string) (*flameImages, error) {
	frames, err := resfmt.LoadType5File(path)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	if len(frames) != resfmt.FlameFrames {
		return nil, fmt.Errorf("%s has %d frames, want %d", path, len(frames), resfmt.FlameFrames)
	}
	f := &flameImages{}
	for _, fr := range frames {
		f.opaque = append(f.opaque, ebiten.NewImageFromImage(fr))
		f.blended = append(f.blended, ebiten.NewImageFromImage(resfmt.TransparentFlame(fr)))
	}
	return f, nil
}

// flame is one flame of a placed object: its world position, type and
// current frame (placement byte +0x08 + i).
type flame struct {
	x, y, typ, k int
}

// newFlames sets up an object's flames like the map loader (0x0043CCC2):
// each starts at a random frame start + rand() % (end − start + 1), or at
// start when that span is below 2.
func (s *startScene) newFlames(placed []mapdata.PlacedFlame) []flame {
	var out []flame
	for _, p := range placed {
		start, end := s.tables.FlameCycle(p.Type)
		k := start
		if span := end - start + 1; span >= 2 {
			k += s.rng.IntN(span)
		}
		out = append(out, flame{x: p.X, y: p.Y, typ: p.Type, k: k})
	}
	return out
}

// drawFlames draws an object's flames after the object (0x0042708D) and,
// once per drawn frame, advances each one, wrapping to the type's start at
// its end (0x00427185). The flames are not day/night tinted.
func (s *startScene) drawFlames(dst *ebiten.Image, o *sceneObject, advance bool) {
	if len(o.flames) == 0 || s.assets.flames == nil {
		return
	}
	frames := s.assets.flames.opaque
	if s.transparency {
		frames = s.assets.flames.blended
	}
	for i := range o.flames {
		f := &o.flames[i]
		if f.k >= 0 && f.k < len(frames) {
			img := frames[f.k]
			if s.visible(f.x, f.y, img.Bounds().Dx(), img.Bounds().Dy()) {
				drawAt(dst, img, f.x-s.camX, f.y-s.camY)
			}
		}
		if advance {
			f.k++
			if start, end := s.tables.FlameCycle(f.typ); f.k >= end {
				f.k = start
			}
		}
	}
}
