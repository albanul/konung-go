package gamestate

import "bytes"

// The chat in a LAN world (LAN_KONG.EXE; docs/konung_lan.md#chat): the
// client keeps the last three lines (0x00A35F94, 3 × 0x74 bytes: a flag
// byte and the text) and draws them at the view's top left (0x0041EC48).

// LANChatLines is how many chat lines the view shows.
const LANChatLines = 3

// FieldChat is the chat input's limit in bytes including the cursor, the
// prompt «>» not counted: the buffer 0x00A6E3E4 holds 0x4F bytes with both
// (0x0042CC2E).
const FieldChat = 0x4F - 1

// LANChatLine is a shown chat line: its text (CP866: «name : text», or a
// server's text alone) and whether it went to everyone (flag 0x40: the
// small font; a team line is drawn in the highlight font).
type LANChatLine struct {
	Text []byte
	All  bool
}

// LANChat is the shown chat, oldest line first.
type LANChat struct {
	Lines []LANChatLine
}

// Add shows a line (0x0042240C); with three lines shown the oldest scrolls
// away (0x00422560). The original ignores a text equal to a shown line's
// (0x004224FC, likely against its network layer's repeated deliveries);
// the port shows every line (a port rule, the user's choice: the port's
// connection delivers each line once, and a repeated «да» should show).
func (c *LANChat) Add(text []byte, all bool) {
	if len(c.Lines) == LANChatLines {
		c.Lines = append(c.Lines[:0], c.Lines[1:]...)
	}
	c.Lines = append(c.Lines, LANChatLine{Text: bytes.Clone(text), All: all})
}

// Clear empties the chat (entering the world 0x0042F6C2; the server's
// message with the flag byte 0xFF, 0x00422459).
func (c *LANChat) Clear() { c.Lines = nil }
