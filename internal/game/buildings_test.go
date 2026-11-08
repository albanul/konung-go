package game

import (
	"encoding/binary"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
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

// TestArrowsSetBuildingOnFire: the hero shooting at the elder's house of
// «Родники» (the burn order of incendiary arrows) hits its image, angers
// and alarms the village, and on a 21 % roll sets the house burning; the
// settlement pass then turns it into a burning sprite.
func TestArrowsSetBuildingOnFire(t *testing.T) {
	s := newVolkScene(t)
	g := s.game
	st := g.SettlementOnMap(s.mapNumber)
	b := st.Raw[0x18 : 0x18+8] // record 0, the elder's house
	p := int(binary.LittleEndian.Uint16(b[2:]))
	o := s.buildingObject(p)
	if o == nil {
		t.Fatalf("placement %d is not a building object", p)
	}
	f := o.frames[0]
	bb := o.building.Bounds()
	cx, cy := f.x+bb.Dx()/2, f.y+bb.Dy()*3/4
	if !o.buildingHit(cx, cy) {
		t.Fatal("no opaque pixel at the house's centre")
	}
	target := mapdata.CellAt(cx, cy)
	bow, arrows := 0, 0
	for i := 1; i < 8192 && (bow == 0 || arrows == 0); i++ {
		if it, ok := g.Item(i); ok && it.Category == gamestate.CatBow && bow == 0 {
			bow = i
		} else if ok && it.Category == gamestate.CatArrows && it.Count >= 20 && arrows == 0 {
			arrows = i
		}
	}
	hero := &s.party[0]
	hero.Equipment[gamestate.WeaponBow], hero.Quiver = bow, arrows
	gamestate.SelectWeapon(hero, gamestate.WeaponBow)
	gamestate.EnterStance(hero)
	s.hero.cell = mapdata.Cell{Row: target.Row + 14, Col: target.Col}
	s.hero.path, s.hero.walking = nil, false
	s.hero.look = lookOf(*hero)

	// Without incendiary arrows a right click on the house does nothing.
	s.hoveredBuilding = p + 1
	from := s.hero.cell
	s.click(hero, viewLeft+100, viewTop+100, 0, true)
	h := s.lead()
	if h.burnTarget != 0 || s.hero.walking || len(s.hero.path) > 0 || s.hero.cell != from {
		t.Fatal("plain arrows: the click gave an order")
	}

	s.endFight(h)
	h.burnTarget, h.intent, h.targetCell = p+1, gamestate.IntentShoot, target
	shots := 0
	for i := 0; i < 4000 && h.shoot() && binary.LittleEndian.Uint16(b[6:]) == 0; i++ {
		n := len(s.arrows)
		s.flyArrows()
		s.memberTick(h)
		if len(s.arrows) > n {
			shots++
		}
	}
	if shots == 0 {
		t.Fatalf("no arrow was shot (shoot %v, act %d, cell %v, target %v)", h.shoot(), s.hero.act, s.hero.cell, target)
	}
	if st.Flags&0x15 != 0x15 || g.Groups[st.Group].Alarm != gamestate.AlarmAttack {
		t.Errorf("village not angered: flags %#x alarm %d", st.Flags, g.Groups[st.Group].Alarm)
	}
	if b[1] != gamestate.BuildingStanding || binary.LittleEndian.Uint16(b[6:]) != 1 {
		t.Fatalf("after %d shots: record %v (shoot %v)", shots, b, h.shoot())
	}
	t.Logf("caught fire after %d shots", shots)
	img := o.frames[0].img
	s.game.SettlementPass(s.tables, hero.Index, s.mapNumber, false)
	s.updateBuildings()
	if b[1] != 4 || s.buildingObject(p) == nil || s.buildingObject(p).frames[0].img == img || len(s.buildingObject(p).flames) == 0 {
		t.Errorf("not burning after the pass: %v", b)
	}
}

// TestAnimatedObjects: on KONUNG.42 the cracked ground, the waterfall and
// the pit cycle their variants, one step per game tick (0x0042480C); still
// objects keep their one frame.
func TestAnimatedObjects(t *testing.T) {
	v := newVolkScene(t)
	s, err := v.assets.buildScene(v.game, 42, v, nil)
	if err != nil {
		t.Fatal(err)
	}
	var anim []*sceneObject
	for i := range s.objects {
		o := &s.objects[i]
		switch {
		case o.period > 0:
			if len(o.frames) < 2 || o.counter != o.period || o.variant >= len(o.frames) {
				t.Fatalf("placement %d: %d frames, variant %d, counter %d/%d", o.placement, len(o.frames), o.variant, o.counter, o.period)
			}
			anim = append(anim, o)
		case len(o.frames) != 1 || o.counter != 0:
			t.Fatalf("still placement %d has %d frames", o.placement, len(o.frames))
		}
	}
	if len(anim) != 11 {
		t.Fatalf("%d animated objects, want 11", len(anim))
	}
	o := anim[0]
	start, period := o.variant, o.period
	for range period - 1 {
		s.animateObjects()
	}
	if o.variant != start {
		t.Fatalf("variant changed after %d ticks", period-1)
	}
	s.animateObjects()
	if o.variant != (start+1)%len(o.frames) || o.counter != period {
		t.Errorf("after %d ticks: variant %d (from %d), counter %d", period, o.variant, start, o.counter)
	}
	// The step runs in the game tick.
	before := o.counter
	for range walkTicksPerStep {
		s.step()
	}
	if o.counter == before {
		t.Error("a game tick did not step the animation")
	}
}
