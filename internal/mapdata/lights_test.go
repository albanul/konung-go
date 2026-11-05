package mapdata

import (
	"os"
	"testing"
)

// LIGHTS.1 is 64000 bytes; its fine-grid part marks a few hundred lit cells.
func TestParseLitCells(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/LIGHTS.1")
	if err != nil {
		t.Skip("LIGHTS.1 not available:", err)
	}
	l, err := ParseLitCells(data)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for r := range WalkRows {
		for c := range WalkCols {
			if l.Lit(r, c) {
				n++
			}
		}
	}
	if n < 100 || n > 2000 {
		t.Fatalf("%d lit cells", n)
	}
	if l.Lit(-1, 0) || l.Lit(0, WalkCols) {
		t.Fatal("off-grid cells must be unlit")
	}
	empty, err := ParseLitCells(nil)
	if err != nil || empty.Lit(0, 0) {
		t.Fatal("no file: all unlit")
	}
}
