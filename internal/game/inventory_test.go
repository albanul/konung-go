package game

import "testing"

func TestBagStripGeometry(t *testing.T) {
	// Intr800r.res: cell slot 17 is 70x70, arrows 20x70.
	b := newBagStrip(70, 20, 70)
	if b.cellW != 69 || b.visible != 9 || b.x0 != 159 || b.y != 486 {
		t.Fatalf("strip %+v", b)
	}
}
