package gamestate

import (
	"testing"

	"konung1.5/internal/resfmt"
)

func TestPilesGame0(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Piles) != 378 {
		t.Fatalf("%d pile records, want 378", len(g.Piles))
	}
	var onMap []int
	for i, p := range g.Piles {
		if p.Active(1, 0) {
			onMap = append(onMap, i)
		}
	}
	if len(onMap) != 13 {
		t.Fatalf("KONUNG.1 piles %d, want 13", len(onMap))
	}
	// Pile 355: «Земляной орех» at (92, 33), drawn with its own ground image.
	nut := g.Piles[355]
	if nut.Row != 92 || nut.Col != 33 || !nut.Visible() || g.PileImage(tb, nut) != 0xA0 {
		t.Fatalf("nut pile %+v image %#x", nut, g.PileImage(tb, nut))
	}
	// Pile 3 is buried (gold −100): hidden, needs the shovel.
	buried := g.Piles[3]
	volk := g.Entities[0]
	if buried.Visible() || g.CanPickUp(buried, volk) || g.PileImage(tb, buried) != 0xA3 {
		t.Fatalf("buried pile %+v", buried)
	}

	// Picking the nut: into the bag, the record re-armed with a fresh copy,
	// back after PileRespawnTicks.
	msg, removed := g.PickUp(&volk, tb, 355, 100)
	if msg != 0 || !removed || volk.Bag[0] != nut.Items[0] {
		t.Fatalf("pick nut: msg %#x removed %v bag %v", msg, removed, volk.Bag[:4])
	}
	re := g.Piles[355]
	fresh, _ := g.Item(re.Items[0])
	if re.Items[0] == nut.Items[0] || fresh.Category != 9 || fresh.Type != 1 || re.Taken != -100 {
		t.Fatalf("respawn record %+v item %+v", re, fresh)
	}
	if re.Active(1, 100+PileRespawnTicks-1) || !re.Active(1, 100+PileRespawnTicks) {
		t.Fatal("respawn timer")
	}

	// The shield pile (106) is emptied for good.
	if _, removed := g.PickUp(&volk, tb, 106, 200); !removed || g.Piles[106].Active(1, 1<<20) {
		t.Fatal("shield pile should be gone")
	}
	// The ring pile (105) adds its 30 gold.
	gold := volk.Gold
	g.PickUp(&volk, tb, 105, 300)
	if volk.Gold != gold+30 {
		t.Fatalf("gold %d, want %d", volk.Gold, gold+30)
	}
	// With the shovel (pile 101) the buried pile can be taken.
	g.PickUp(&volk, tb, 101, 400)
	if !g.HasShovel(volk) || !g.CanPickUp(buried, volk) {
		t.Fatal("shovel")
	}
}
