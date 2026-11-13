package mapdata

import (
	"os"
	"testing"
)

// The LAN version's MAPS.RES holds 20 maps that parse as KONUNG.N + LIGHTS.N.
func TestSplitMapsRes(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/MAPS.RES")
	if err != nil {
		t.Skip("MAPS.RES not available:", err)
	}
	maps, err := SplitMapsRes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(maps) != 20 {
		t.Fatalf("got %d maps, want 20", len(maps))
	}
	for i, m := range maps {
		if _, err := Parse(m.Map); err != nil {
			t.Errorf("map %d: %v", i+1, err)
		}
		if _, err := ParsePlacements(m.Map); err != nil {
			t.Errorf("map %d placements: %v", i+1, err)
		}
		if _, err := ParseLitCells(m.Lights); err != nil {
			t.Errorf("map %d lights: %v", i+1, err)
		}
	}
	if _, err := SplitMapsRes(data[:len(data)-1]); err == nil {
		t.Error("truncated MAPS.RES accepted")
	}
}
