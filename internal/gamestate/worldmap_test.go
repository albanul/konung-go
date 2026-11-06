package gamestate

import "testing"

func TestTravelByOrb(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	orb := 0
	for i := 1; i < 8192 && orb == 0; i++ {
		if it, ok := g.Item(i); ok && it.Category == 11 && it.Type == jumpOrbType {
			orb = i
		}
	}
	if orb == 0 {
		t.Skip("GAME.0 has no «Шар магического прыжка»")
	}
	hero := &g.Entities[g.Groups[0].First]
	hero.Bag = [32]int{}
	if _, ok := g.Travel(hero, 3); ok {
		t.Fatal("travelled without an orb")
	}
	hero.Bag[0], hero.Bag[1], hero.Bag[2] = 7, orb, 9
	if _, ok := g.Travel(hero, g.Groups[0].Map); ok || hero.Bag[1] != orb {
		t.Fatal("travel to the current map used the orb")
	}
	x, ok := g.Travel(hero, 3)
	if !ok || x.To != 3 {
		t.Fatalf("Travel(3) = %+v, %v", x, ok)
	}
	for _, e := range g.Exits {
		if e.To == 3 {
			if e != x {
				t.Errorf("took exit %d, want the first to map 3 (%d)", x.Index, e.Index)
			}
			break
		}
	}
	if hero.Bag[0] != 7 || hero.Bag[1] != 9 || hero.Bag[2] != 0 {
		t.Errorf("bag after the orb: %v", hero.Bag[:3])
	}
}

func TestKnownMaps(t *testing.T) {
	var g Game
	g.KnowMap(4)
	g.KnowMap(0)  // out of range: ignored
	g.KnowMap(57) // ditto
	if !g.MapKnown(4) || g.MapKnown(1) || g.KnownMaps[3] != 1 {
		t.Errorf("known flags %v", g.KnownMaps)
	}
}
