package gamestate

import "testing"

// Three lines, the oldest scrolls away; a repeated line shows again (the
// port's rule).
func TestLANChat(t *testing.T) {
	var c LANChat
	for _, s := range []string{"a : 1", "b : 2", "a : 1", "a : 1"} {
		c.Add([]byte(s), s[0] == 'a')
	}
	want := []string{"b : 2", "a : 1", "a : 1"}
	if len(c.Lines) != len(want) {
		t.Fatalf("%d lines", len(c.Lines))
	}
	for k, l := range c.Lines {
		if string(l.Text) != want[k] || l.All != (want[k][0] == 'a') {
			t.Errorf("line %d: %q all %v", k, l.Text, l.All)
		}
	}
	c.Clear()
	if len(c.Lines) != 0 {
		t.Error("not cleared")
	}
}
