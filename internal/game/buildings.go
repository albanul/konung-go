package game

import (
	"image"
	"log"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	"konung1.5/internal/gamestate"
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
			building:  buildingImage(sorted[i]),
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

// buildingImage keeps the image of a settlement building placement (flag
// bit 0x01, tested by the hover 0x004270A6 and the arrows 0x0041EF56).
func buildingImage(p mapdata.SortedPlacement) *image.RGBA {
	if p.Flags&0x01 == 0 {
		return nil
	}
	return p.Object.Image
}

// buildingObject returns the scene object of placement p, or nil.
func (s *startScene) buildingObject(p int) *sceneObject {
	for i := range s.objects {
		if o := &s.objects[i]; o.placement == p && o.building != nil {
			return o
		}
	}
	return nil
}

// buildingHit is 0x0043F89C for a building: world pixel (x, y) is an
// opaque pixel of its drawn image.
func (o *sceneObject) buildingHit(x, y int) bool {
	f := &o.frames[0]
	b := o.building.Bounds()
	p := image.Pt(x-f.x+b.Min.X, y-f.y+b.Min.Y)
	return p.In(b) && o.building.RGBAAt(p.X, p.Y).A != 0
}

// buildingUnderCursor is 0x00A3E6A4 as the object draw pass leaves it: the
// last drawn (front-most) visible building with an opaque pixel under the
// cursor, as placement + 1 (0 = none).
func (s *startScene) buildingUnderCursor() int {
	mx, my := ebiten.CursorPosition()
	if !inView(mx, my) {
		return 0
	}
	found := 0
	for i := range s.objects {
		o := &s.objects[i]
		if o.building == nil {
			continue
		}
		f := &o.frames[0]
		if s.visible(f.x, f.y, f.img.Bounds().Dx(), f.img.Bounds().Dy()) && o.buildingHit(mx+s.camX, my+s.camY) {
			found = o.placement + 1
		}
	}
	return found
}

// fireReady is the cursor test of 0x00428AF3 for the selection (the
// port's: the hero): in the map's settlement, alive and able to act, in
// the stance with the bow active, a bow and incendiary arrows.
func (s *startScene) fireReady() bool {
	if len(s.party) == 0 || s.game.SettlementOnMap(s.mapNumber) == nil {
		return false
	}
	h := s.party[0]
	return !h.Dead() && h.Type != 9 && h.Type != 3 && h.InStance() && gamestate.CanShoot(h) && s.game.Incendiary(h)
}
