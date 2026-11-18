package lanserver

import (
	"os/exec"
	"strings"
	"testing"
)

// The server world stays engine-free: the dedicated server runs without a
// display.
func TestNoEngine(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skip("go list:", err)
	}
	for _, p := range strings.Fields(string(out)) {
		if strings.Contains(p, "ebiten") {
			t.Errorf("internal/lanserver depends on %s", p)
		}
	}
}
