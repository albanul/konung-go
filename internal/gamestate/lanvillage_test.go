package gamestate

import (
	"encoding/binary"
	"testing"

	"konung1.5/internal/resfmt"
)

// The LAN box refuses a herb held over a weapon (no poison, no oil:
// 0x00440CC5); a herb over a herb still mixes.
func TestLANClickBoxNoPoison(t *testing.T) {
	g, tb := potionGame(t)
	volk := g.Entities[0]
	h := NewHand()
	// Balm onto balm (TestInHandBoxGame0's start): mixed as in single player.
	g.ClickBag(&h, &volk, tb, 1)
	g.LANClickBox(&h, &volk, tb)
	g.ClickBag(&h, &volk, tb, 1)
	if msg := g.LANClickBox(&h, &volk, tb); msg != 0 || h.Holding() || g.Concentration(3) != 14 {
		t.Fatalf("herb on herb: msg %#x hand %+v conc %v", msg, h, g.Concentration(3))
	}
	// Poison onto the knife in the box: refused, the herb goes back.
	h.Box = 0
	g.ClickEquipment(&h, &volk, tb, WeaponBlade)
	g.LANClickBox(&h, &volk, tb)
	knife := h.Box
	poison := herbOf(t, g, HerbPoison)
	BagInsert(&volk, poison)
	g.ClickBag(&h, &volk, tb, 0)
	before := int16(g.itemWord(knife, 0xC))
	if msg := g.LANClickBox(&h, &volk, tb); msg != MsgUnusable || h.Holding() || !contains(volk.Bag[:], poison) ||
		g.ItemType(poison) != HerbPoison || int16(g.itemWord(knife, 0xC)) != before {
		t.Fatalf("poison on the knife: msg %#x bag %v type %d poisoning %d", msg, volk.Bag[:4],
			g.ItemType(poison), int16(g.itemWord(knife, 0xC)))
	}
	// Single player's box applies it.
	g.ClickBag(&h, &volk, tb, slicesIndex(volk.Bag[:], poison))
	if msg := g.ClickBox(&h, &volk, tb); msg != 0 || int16(g.itemWord(knife, 0xC)) == before {
		t.Fatalf("single player: msg %#x poisoning %d", msg, int16(g.itemWord(knife, 0xC)))
	}
}

func slicesIndex(s []int, v int) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func lanVillageGame(t *testing.T) *Game {
	t.Helper()
	lw, err := ParseLANWorld(load(t, "GAME.RES"))
	if err != nil {
		t.Fatal(err)
	}
	return lw.Game
}

// The village is alarmed against an aggressor slot with a member in its
// rectangle, the first such slot in slot order; others leave it calm.
func TestLANVillageAlarm(t *testing.T) {
	g := lanVillageGame(t)
	s := &g.Settlements[1] // Драгомир's village, map 4
	gr := &g.Groups[s.Group]
	hero := func(slot int) Entity {
		e := g.Entities[s.Posts[0].Entity] // a living record on a village cell
		e.Group, e.Index = slot, slot
		return e
	}
	party := []Entity{hero(2), hero(5)}
	g.LANVillageAlarm(s, party)
	if gr.Alarm != AlarmCalm {
		t.Fatal("alarmed without aggressors")
	}
	s.MarkAggressor(5)
	s.MarkAggressor(9) // not on the map
	g.LANVillageAlarm(s, party)
	if gr.Alarm != AlarmAttack || gr.Enemy != 5 {
		t.Fatalf("alarm %d enemy %d, want slot 5", gr.Alarm, gr.Enemy)
	}
	s.MarkAggressor(2)
	g.LANVillageAlarm(s, party)
	if gr.Enemy != 2 {
		t.Fatalf("enemy %d, want slot 2 (the first)", gr.Enemy)
	}
	dead := hero(2)
	dead.Flags4 |= 0x80
	g.LANVillageAlarm(s, []Entity{dead})
	if gr.Alarm != AlarmCalm {
		t.Fatal("alarmed by a dead aggressor")
	}
	g.ClearAggressor(2)
	g.ClearAggressor(5)
	g.ClearAggressor(9)
	if s.Aggressors != 0 {
		t.Fatalf("aggressors %#x after clearing", s.Aggressors)
	}
}

// The LAN pass burns a burning building on (state 4 → 5 → 6 at the burn
// times), reports the change, and BuildingStates/SetBuildingState see it.
func TestLANSettlementPassBuildings(t *testing.T) {
	g := lanVillageGame(t)
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	const si = 1
	s := &g.Settlements[si]
	b := s.building(0)
	p := int(binary.LittleEndian.Uint16(b[2:]))
	if st := g.BuildingStates(s.Map)[p]; st != BuildingStanding {
		t.Fatalf("GAME.RES state %d, want standing", st)
	}
	b[1] = 4
	binary.LittleEndian.PutUint16(b[6:], 1)
	shops := make([]LANShop, 16)
	if !g.LANSettlementPass(si, shops, tb, nil) || int8(b[1]) != 5 {
		t.Fatalf("pass: state %d, want 5 and a change", int8(b[1]))
	}
	ruin := false
	for range tb.BurnTime(int(int8(b[0]))) + 1 {
		g.LANSettlementPass(si, shops, tb, nil)
		ruin = ruin || g.BuildingStates(s.Map)[p] == BuildingRuin
	}
	if !ruin {
		t.Fatalf("state %d, never the ruin", g.BuildingStates(s.Map)[p])
	}
	if !g.SetBuildingState(s.Map, p, BuildingStanding) || g.BuildingStates(s.Map)[p] != BuildingStanding {
		t.Fatal("SetBuildingState")
	}
	if g.SetBuildingState(s.Map, 9999, 1) {
		t.Fatal("SetBuildingState on no building")
	}
}
