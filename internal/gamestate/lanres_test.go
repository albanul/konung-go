package gamestate

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"konung1.5/internal/mapdata"
)

func lanWorld(t *testing.T) (*LANWorld, []byte) {
	t.Helper()
	data, err := os.ReadFile("../../assets/Konung/GAME.RES")
	if err != nil {
		t.Skip("GAME.RES not available:", err)
	}
	w, err := ParseLANWorld(data)
	if err != nil {
		t.Fatal(err)
	}
	return w, data
}

// The six sections the server reads (0x00449451) are the whole file.
func TestLANWorldSections(t *testing.T) {
	sizes := []int{0x20000, LANGroups * groupStride, lanPileRecords * pileStride, 0x109A, LANEntities * entityStride, LANSettlements * lanSettlementStride}
	offsets := []int{lanItemOffset, lanGroupOffset, lanPileOffset, lanExitOffset, lanEntityOffset, lanSettlementOffset}
	at := 0
	for k, n := range sizes {
		if offsets[k] != at {
			t.Errorf("section %d at %#x, want %#x", k, offsets[k], at)
		}
		at += n
	}
	if at != LANWorldSize {
		t.Errorf("sections end at %d, file has %d", at, LANWorldSize)
	}
}

// Groups, entities and settlements agree with each other: every member of
// a used group names that group and its own index, the player slots are
// one entity each, every settlement post holds a villager of its village
// on the entity's own cell, facing and dialogue.
func TestLANWorldConsistent(t *testing.T) {
	w, data := lanWorld(t)
	g := w.Game
	members, next := 0, 0
	for _, gr := range g.Groups {
		if gr.Count == 0 {
			continue
		}
		// The groups reserve MaxPop consecutive slots each (creature
		// groups keep a spare for respawns), from entity 0 to the end.
		if gr.First != next || gr.MaxPop < gr.Count {
			t.Errorf("group %d: first %d (want %d), count %d of %d", gr.Index, gr.First, next, gr.Count, gr.MaxPop)
		}
		next = gr.First + gr.MaxPop
		if gr.Raw[0x1F] != byte(gr.Index) {
			t.Errorf("group %d: +0x1F = %d", gr.Index, gr.Raw[0x1F])
		}
		for k := range gr.Count {
			i := gr.First + k
			e := g.Entities[i]
			own := u16(data[lanEntityOffset+i*entityStride:], lanEntIndex)
			if e.Group != gr.Index || own != i {
				t.Errorf("entity %d: group %d, own index %d; in group %d", i, e.Group, own, gr.Index)
			}
			members++
		}
		if gr.Index < LANPlayerSlots && (gr.Count != 1 || gr.First != gr.Index) {
			t.Errorf("player group %d: first %d count %d", gr.Index, gr.First, gr.Count)
		}
	}
	// 319 entities reserved (285 used); the last record is outside every
	// group and empty.
	if next != LANEntities-1 || members != 285 || data[lanEntityOffset+(LANEntities-1)*entityStride] != 0 ||
		g.Entities[LANEntities-1].Model != 0 || g.Entities[LANEntities-1].Level != 0 {
		t.Errorf("groups reserve %d entities with %d members, want %d and 285", next, members, LANEntities-1)
	}
	if len(g.Settlements) != LANSettlements {
		t.Fatalf("%d settlements", len(g.Settlements))
	}
	for _, s := range g.Settlements {
		vg := g.Groups[s.Group]
		if !vg.VillageGroup() || vg.Map != s.Map {
			t.Errorf("settlement %d: group %d (kind %#x, map %d), map %d", s.Index, s.Group, vg.Kind, vg.Map, s.Map)
		}
		for _, p := range s.Posts {
			if p.Entity == 0 {
				continue
			}
			e := g.Entities[p.Entity]
			if e.Group != s.Group || e.Row != p.Row || e.Col != p.Col || e.Variant != p.Facing || e.Dialogue != p.Dialogue {
				t.Errorf("settlement %d post %d: entity %d %+v", s.Index, p.Slot, p.Entity, p)
			}
		}
		// The building records the map loader walks (7 + extra + fence)
		// hold buildings: type ≥ 0 in the first record.
		if n := s.buildingCount(); n < 7 || int8(s.Raw[spSetBuildings]) < 0 {
			t.Errorf("settlement %d: %d building records, first type %d", s.Index, n, int8(s.Raw[spSetBuildings]))
		}
	}
	// Items the entities and piles hold are records in use.
	check := func(who string, items []int) {
		for _, it := range items {
			if it != 0 {
				if _, ok := g.Item(it); !ok || int8(g.items[it*itemStride]) < 0 {
					t.Errorf("%s holds free item %d", who, it)
				}
			}
		}
	}
	for _, e := range g.Entities {
		check(fmt.Sprint("entity ", e.Index), append(append(e.Equipment[:], e.Bag[:]...), e.Extra[:]...))
	}
	for _, p := range g.Piles {
		check(fmt.Sprint("pile ", p.Index), p.Items[:])
		if p.Index/LANPilesPerMap != p.Map {
			t.Errorf("pile %d on map %d: not in its map's block", p.Index, p.Map)
		}
	}
}

// Every character and pile the file places stands on a walkable cell of
// its MAPS.RES map, except the village post holders (elder, healer,
// merchant, smith, voevoda) and the kind-0x53 characters (animated objects
// with a dialogue), which stand inside their building's or object's
// footprint (0x4F cells). Characters at (0, 0) are placed by the server
// inside their group's rectangle at run time.
func TestLANWorldOnWalkableCells(t *testing.T) {
	w, _ := lanWorld(t)
	data, err := os.ReadFile("../../assets/Konung/MAPS.RES")
	if err != nil {
		t.Skip("MAPS.RES not available:", err)
	}
	maps, err := mapdata.SplitMapsRes(data)
	if err != nil {
		t.Fatal(err)
	}
	grids := map[int]*mapdata.WalkGrid{}
	grid := func(m int) *mapdata.WalkGrid {
		if grids[m] == nil {
			if grids[m], err = mapdata.ParseWalkGrid(maps[m-1].Map); err != nil {
				t.Fatal(err)
			}
		}
		return grids[m]
	}
	g := w.Game
	placed, ok, inside, runtime := 0, 0, 0, 0
	census := map[int]map[string]int{}
	for _, e := range g.Entities[LANPlayerSlots:] {
		gr := g.Groups[e.Group]
		if gr.Count == 0 || e.Index < gr.First || e.Index >= gr.First+gr.Count {
			continue
		}
		kind := "villager"
		switch {
		case gr.Kind&0x40 != 0 && e.Animated():
			kind = "creature"
		case gr.Kind&0x40 != 0:
			kind = "lone"
		}
		if census[gr.Map] == nil {
			census[gr.Map] = map[string]int{}
		}
		census[gr.Map][kind]++
		if e.Row == 0 && e.Col == 0 {
			runtime++
			continue
		}
		placed++
		if grid(gr.Map).Blocked(e.Row, e.Col) {
			_, posted := g.PostOf(e)
			if b := grid(gr.Map)[e.Row][e.Col]; (!posted && gr.Kind != 0x53) || b != 0x4F {
				t.Errorf("entity %d (group %d, map %d, posted %v) on blocked cell (%d, %d) = %#x", e.Index, e.Group, gr.Map, posted, e.Row, e.Col, b)
			}
			inside++
			continue
		}
		ok++
	}
	piles, pilesOK := 0, 0
	for _, p := range g.Piles {
		piles++
		if grid(p.Map).Blocked(p.Row, p.Col) {
			t.Errorf("pile %d (map %d) on blocked cell (%d, %d)", p.Index, p.Map, p.Row, p.Col)
			continue
		}
		pilesOK++
	}
	t.Logf("characters on walkable cells: %d of %d placed, %d posts/objects inside footprints (%d placed at run time); piles: %d of %d", ok, placed, inside, runtime, pilesOK, piles)
	if ok+inside != placed || inside != 17 {
		t.Errorf("%d walkable + %d inside of %d placed", ok, inside, placed)
	}
	var ms []int
	for m := range census {
		ms = append(ms, m)
	}
	sort.Ints(ms)
	var lines []string
	for _, m := range ms {
		lines = append(lines, fmt.Sprintf("map %d %v", m, census[m]))
	}
	t.Log(strings.Join(lines, "; "))
}
