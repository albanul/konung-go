package game

import (
	"encoding/binary"
	"testing"

	"konung1.5/internal/gamestate"
)

// TestBuildingSpriteFollowsState: a building of «Родники» turned into a
// ruin gets the ruin's object when the pass reports the change, and the
// objects stay in depth order.
func TestBuildingSpriteFollowsState(t *testing.T) {
	s := newVolkScene(t)
	st := s.game.SettlementOnMap(s.mapNumber)
	b := st.Raw[0x18+8 : 0x18+16] // record 1, the healer's hut
	pl := int(binary.LittleEndian.Uint16(b[2:]))
	find := func() *sceneObject {
		for i := range s.objects {
			if s.objects[i].placement == pl {
				return &s.objects[i]
			}
		}
		return nil
	}
	before := find()
	if before == nil || s.placements[pl].ID != s.tables.BuildingObject(int(int8(b[0])))+gamestate.BuildingStanding {
		t.Fatalf("placement %d: %+v", pl, s.placements[pl])
	}
	img := before.frames[0].img
	b[1] = gamestate.BuildingRuin
	s.updateBuildings()
	after := find()
	if s.placements[pl].ID != s.tables.BuildingObject(int(int8(b[0])))+gamestate.BuildingRuin || after == nil || after.frames[0].img == img {
		t.Fatalf("sprite not replaced: %+v", s.placements[pl])
	}
	for i := 1; i < len(s.objects); i++ {
		if s.objects[i].depth < s.objects[i-1].depth {
			t.Fatal("objects out of depth order")
		}
	}
}
