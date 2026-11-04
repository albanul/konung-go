package game

import (
	"os"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// TestStoreWorldKeepsCharacters checks that a character that moved on map 1
// is placed on its new cell, with its facing, when map 1 is entered again.
func TestStoreWorldKeepsCharacters(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/GAME.0")
	if err != nil {
		t.Skip("GAME.0 not available:", err)
	}
	g, err := gamestate.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var villager gamestate.Placed
	for _, p := range g.OnMap(1, nil) {
		if p.Group != 0 {
			villager = p
			break
		}
	}
	moved := mapdata.Cell{Row: villager.Row + 2, Col: villager.Col}
	s := &startScene{game: g, npcs: []*npc{{walker: walker{entity: villager.Index, cell: moved, dir: 3}}}}
	s.storeWorld()
	for _, p := range g.OnMap(1, nil) {
		if p.Index == villager.Index {
			if p.Row != moved.Row || p.Col != moved.Col || p.Variant != 3 {
				t.Fatalf("entity %d back at (%d,%d) facing %d, want %v facing 3", p.Index, p.Row, p.Col, p.Variant, moved)
			}
			return
		}
	}
	t.Fatalf("entity %d not on map 1 any more", villager.Index)
}
