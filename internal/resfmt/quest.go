package resfmt

import (
	"encoding/binary"
	"fmt"
	"image"
)

// QUESTR.RES: dialogues and quests. The loader 0x0043BDD5 reads seven
// blocks back to back (the sizes sum to the file):
//
//	0x00000  0x1F400  0x005D2D90  8000 × 16-byte dialogue nodes (QuestNode)
//	0x1F400    0x258  0x005F2190  300 × uint16: dialogue id → first node
//	0x1F658   0xFA00  0x005F23E8  string pool: zero-terminated CP866 pieces
//	0x2F058   0x2EE0  0x00601DE8  condition lists: uint16 terms, 0xFFFF ends
//	0x31F38   0x2EE0  0x00604CC8  action lists: uint16 terms, 0xFFFF ends
//	0x34E18  0x1D4C0  0x00607BA8  texts: uint16 pool offsets, 0xFFFF ends
//	0x522D8    0x4B0  0x00625068  quest flags (gamestate.Game.Quests)
//
// Lists and texts are addressed by uint16 index into their block (a list
// id is the index of its first term).
const (
	QuestNodeCount   = 8000
	QuestDialogues   = 300
	questNodeSize    = 16
	questStartsOff   = QuestNodeCount * questNodeSize
	questPoolOff     = questStartsOff + QuestDialogues*2
	questCondsOff    = questPoolOff + 0xFA00
	questActionsOff  = questCondsOff + 0x2EE0
	questTextsOff    = questActionsOff + 0x2EE0
	questFlagsOff    = questTextsOff + 0x1D4C0
	QuestFileSizeRES = questFlagsOff + 0x4B0

	// QuestNone marks an absent field (0xFFFF); QuestEnd in Next ends the
	// dialogue, QuestBack (0xFFFF) in an option's Next returns to the node
	// the dialogue came from (0x0041F1C0).
	QuestNone = 0xFFFF
	QuestEnd  = 0xFFFE
	// QuestRedirect in a node's Action field marks a branch node
	// (0x00435EBC): its Lines records, starting with the node itself, pick
	// the next node by their conditions.
	QuestRedirect = 0xFFFE
	// QuestNoText is a record without text (+0 = −1).
	QuestNoText = 0xFFFFFFFF
)

// Condition and action term bits (0x00436248, 0x00435DBC).
const (
	QuestTermIndex = 0x01FF // flag index or script function id
	QuestTermFunc  = 0x2000 // call script function 0x0043259C(index) instead of the flag
	QuestTermAnd   = 0x4000 // condition: AND-term (else OR-term)
	QuestTermNot   = 0x8000 // condition: negate; action on a flag: set (else clear)
)

// QuestNode is one 16-byte record of the node block. A dialogue node is a
// header record followed by Lines NPC lines and Options player replies; the
// same record layout serves all three.
type QuestNode struct {
	Text    uint32 // +0: text id (QuestNoText = none)
	Next    int    // +4: next node, QuestEnd or QuestBack/QuestNone
	Cond    int    // +6: condition list, QuestNone = always
	Speech  int    // +8: SPEECHS.RES entry, QuestNone = none
	Action  int    // +0xA: action list, QuestNone = none; QuestRedirect in a header
	Options int    // +0xC (header): number of player replies after the lines
	Lines   int    // +0xE (header): number of NPC lines (redirect: branches)
}

// Quests is the decoded QUESTR.RES.
type Quests struct {
	Nodes  []QuestNode
	Starts [QuestDialogues]int
	data   []byte
}

// ParseQuests decodes QUESTR.RES.
func ParseQuests(data []byte) (*Quests, error) {
	if len(data) != QuestFileSizeRES {
		return nil, fmt.Errorf("QUESTR.RES has size %d, want %d", len(data), QuestFileSizeRES)
	}
	q := &Quests{Nodes: make([]QuestNode, QuestNodeCount), data: data}
	for i := range q.Nodes {
		r := data[i*questNodeSize:]
		q.Nodes[i] = QuestNode{
			Text:    binary.LittleEndian.Uint32(r),
			Next:    int(binary.LittleEndian.Uint16(r[4:])),
			Cond:    int(binary.LittleEndian.Uint16(r[6:])),
			Speech:  int(binary.LittleEndian.Uint16(r[8:])),
			Action:  int(binary.LittleEndian.Uint16(r[10:])),
			Options: int(binary.LittleEndian.Uint16(r[12:])),
			Lines:   int(binary.LittleEndian.Uint16(r[14:])),
		}
	}
	for i := range q.Starts {
		q.Starts[i] = int(binary.LittleEndian.Uint16(data[questStartsOff+2*i:]))
	}
	return q, nil
}

// Node returns node i, or a zero node outside the block.
func (q *Quests) Node(i int) QuestNode {
	if i < 0 || i >= len(q.Nodes) {
		return QuestNode{Text: QuestNoText, Next: QuestEnd, Cond: QuestNone, Speech: QuestNone, Action: QuestNone}
	}
	return q.Nodes[i]
}

// list reads the 0xFFFF-terminated uint16 list at index i of a block.
func (q *Quests) list(off, size, i int) []int {
	var out []int
	for p := off + 2*i; p+2 <= off+size; p += 2 {
		v := int(binary.LittleEndian.Uint16(q.data[p:]))
		if v == QuestNone {
			break
		}
		out = append(out, v)
	}
	return out
}

// Condition returns the terms of condition list i.
func (q *Quests) Condition(i int) []int { return q.list(questCondsOff, 0x2EE0, i) }

// Actions returns the terms of action list i.
func (q *Quests) Actions(i int) []int { return q.list(questActionsOff, 0x2EE0, i) }

// Text assembles text id like 0x004363E8: the id's pool pieces are joined,
// with a space before every piece but the first that starts with a letter
// (≥ 0x41, so CP866 Cyrillic too), a digit or is a lone "-".
func (q *Quests) Text(id uint32) []byte {
	if id == QuestNoText || int(id) >= 0x1D4C0/2 {
		return nil
	}
	var out []byte
	for _, off := range q.list(questTextsOff, 0x1D4C0, int(id)) {
		p := q.data[questPoolOff+off : questCondsOff]
		n := 0
		for n < len(p) && p[n] != 0 {
			n++
		}
		piece := p[:n]
		if len(out) > 0 && len(piece) > 0 {
			c := piece[0]
			if c >= 0x41 || c >= '0' && c <= '9' || c == '-' && len(piece) == 1 {
				out = append(out, ' ')
			}
		}
		out = append(out, piece...)
	}
	return out
}

// Flags returns the initial quest flags (the file's last block).
func (q *Quests) Flags() []byte { return q.data[questFlagsOff:] }

// Building type table 0x00458A30: 64-byte records, a CP866 name at +0,
// uint16 required building skill at +0x32, OBJECTS id at +0x34 and build
// time at +0x36 (the dialogue scripts address them as 0x00458A62 /
// 0x00458A66 + type·64).
const buildingTypesVA = 0x00458A30

// BuildingSkill is the building skill a village needs to start type typ.
func (t *ExeTables) BuildingSkill(typ int) int {
	return int(binary.LittleEndian.Uint16(t.at(buildingTypesVA + typ*64 + 0x32)))
}

// BuildTime is the timer a new building of type typ starts with.
func (t *ExeTables) BuildTime(typ int) int {
	return int(binary.LittleEndian.Uint16(t.at(buildingTypesVA + typ*64 + 0x36)))
}

// BuildingName is the type's CP866 name.
func (t *ExeTables) BuildingName(typ int) []byte { return t.CString(buildingTypesVA + typ*64) }

// DialogueLayout is the dialogue window's geometry (0x0042DC50) for one
// video mode, from the tables at 0x00462EE4 (8 bytes per mode, mode 1 =
// 800×600): INTR slot 1 (the frame) and slot 0 (the divider) positions,
// the top-left of the NPC's lines and of the replies, and the wrap width.
// The quest log (window 2, 0x0042DEDB) uses the frame and the line
// position.
type DialogueLayout struct {
	Frame, Divider image.Point
	Lines, Replies image.Point
	Width          int
}

// Dialogue window INTR slots and fonts.
const (
	DialogueFrameSlot   = 1
	DialogueDividerSlot = 0
	FontHighlightVA     = 0x005A2EB0 // the reply under the mouse (0x0042DE65)
)

func (t *ExeTables) i32(va int) int { return int(int32(binary.LittleEndian.Uint32(t.at(va)))) }

// DialogueLayout returns the layout for mode 0 (640×480) or 1 (800×600).
func (t *ExeTables) DialogueLayout(mode int) DialogueLayout {
	pt := func(va int) image.Point { return image.Pt(t.i32(va+8*mode), t.i32(va+8*mode+4)) }
	return DialogueLayout{
		Frame: pt(0x00462EE4), Divider: pt(0x00462EF4),
		Lines: pt(0x00462F04), Replies: pt(0x00462F14),
		Width: t.i32(0x00462F24 + 8*mode),
	}
}

// QuestLogScroll returns the quest log's two click areas for a mode
// (0x0045B0AC, 32 bytes per area: 640 then 800 rectangle, inclusive,
// tested by 0x004398C4): area 0 scrolls up, area 1 down.
func (t *ExeTables) QuestLogScroll(mode int) [2]image.Rectangle {
	var out [2]image.Rectangle
	for k := range out {
		va := 0x0045B0AC + 32*k + 16*mode
		out[k] = image.Rect(t.i32(va), t.i32(va+4), t.i32(va+8)+1, t.i32(va+12)+1)
	}
	return out
}
