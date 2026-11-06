package game

import (
	"image"
	"log"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	"konung1.5/internal/mapdata"
)

// uploader converts decoded images to ebiten images once each.
type uploader func(img *image.RGBA, sh *image.Alpha) (*ebiten.Image, *ebiten.Image)

func newUploader() uploader {
	images := map[*image.RGBA]*ebiten.Image{}
	shadows := map[*image.Alpha]*ebiten.Image{}
	return func(img *image.RGBA, sh *image.Alpha) (*ebiten.Image, *ebiten.Image) {
		ei, ok := images[img]
		if !ok {
			ei = toEbiten(img)
			images[img] = ei
		}
		if sh == nil {
			return ei, nil
		}
		es, ok := shadows[sh]
		if !ok {
			es = ebiten.NewImageFromImage(sh) // white, alpha = coverage
			shadows[sh] = es
		}
		return ei, es
	}
}

// placeObjects decodes the static objects of placements and returns them
// as scene objects in draw order (see mapdata.DrawOrder).
func (s *startScene) placeObjects(placements []mapdata.Placement, where string, upload uploader) []sceneObject {
	objs, errs := mapdata.LoadPlacedObjects(placements, s.assets.objects, s.assets.graph)
	for _, e := range errs {
		log.Printf("%s: %v", where, e)
	}
	sorted := mapdata.DrawOrder(placements, objs)
	var out []sceneObject
	for i, sp := range mapdata.ObjectSprites(sorted) {
		img, sh := upload(sp.Image, sp.Shadow)
		out = append(out, sceneObject{
			placement: sorted[i].Index,
			frames:    []sceneFrame{{img: img, shadow: sh, x: sp.X, y: sp.Y, sx: sp.SX, sy: sp.SY, raw: sp.Raw}},
			depth:     sp.Depth,
			flames:    s.newFlames(sp.Flames),
		})
	}
	return out
}

// updateBuildings is 0x0041733C for the buildings on this map whose state
// changed in the settlement pass: their placements get the new object
// (type object + state) and the sprites are replaced.
func (s *startScene) updateBuildings() {
	var changed []mapdata.Placement
	for i, id := range s.game.BuildingObjects(s.mapNumber, s.tables) {
		if i < len(s.placements) && s.placements[i].ID != id {
			s.placements[i].ID = id
			changed = append(changed, s.placements[i])
		}
	}
	if len(changed) == 0 {
		return
	}
	gone := map[int]bool{}
	for _, p := range changed {
		gone[p.Index] = true
	}
	kept := s.objects[:0]
	for _, o := range s.objects {
		if !gone[o.placement] {
			kept = append(kept, o)
		}
	}
	s.objects = append(kept, s.placeObjects(changed, "building", newUploader())...)
	// DrawOrder keeps file order on equal depths.
	sort.Slice(s.objects, func(i, j int) bool {
		a, b := s.objects[i], s.objects[j]
		return a.depth < b.depth || a.depth == b.depth && a.placement < b.placement
	})
}
