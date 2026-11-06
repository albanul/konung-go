package gamestate

import (
	"math/rand/v2"

	"konung1.5/internal/resfmt"
)

// Dialogues (QUESTR.RES, resfmt.Quests); see docs/konung_dialogue.md.
//
// A right click on a character with a dialogue (+0xD0 ≠ 0xFF) opens it at
// once, wherever the hero stands (0x00423B6E → 0x004364F0). The window
// (window 1, 0x0042DC50) lists the NPC's lines and the player's replies; a
// left click on a reply (hotspot k<<8 | 0x64, 0x00423501) runs its actions
// and moves to its next node (0x0041F18C). The dialogue only ends through
// a reply (Escape is ignored in window 1, 0x00437451).

// MaxDialogueRows is the size of the line and reply lists (0x007AA0DC,
// 0x007A9EDC: 16 × 32 bytes each).
const MaxDialogueRows = 16

// DialogueReply is an entry of the reply list 0x007A9EDC.
type DialogueReply struct {
	Node int    // +0: the reply's node record
	Back int    // +4: the node a QuestBack reply returns to (0x00A5398C when listed)
	Text uint32 // +8
}

// Dialogue is the open dialogue: the globals 0x00A3E7A4 (node), 0x00A5398C
// (previous node), 0x00A3E69C (the NPC) and the two lists.
type Dialogue struct {
	NPC     int // entity index of the character talked to
	Node    int // current node
	Prev    int // node the current one was entered from
	Lines   []uint32
	Replies []DialogueReply
}

// Talk is the context the script functions run in: the hero (0x00A3E694),
// the NPC (0x00A3E69C), the current map and its settlement (0x00A3E6A8).
// The hero and the party are read and written in g.Entities; the caller
// syncs its copies before and after.
type Talk struct {
	G      *Game
	Q      *resfmt.Quests
	T      *resfmt.ExeTables
	Hero   int // entity index of the hero
	Map    int
	NPC    int             // entity talked to (0x00A3E69C), set by OpenDialogue
	Time   int             // game time 0x00A3E770 (apprentices, 291 and 354)
	Rand   func(n int) int // rand() % n; nil = math/rand
	Events TalkEvents
	// Drop puts an item that does not fit into the bag on the ground at
	// the hero's cell (0x004138C8); nil drops nothing.
	Drop func(item int)
	// lastGreeting is 0x00A3E7C0, the last greeting sound (no repeat).
	lastGreeting *int
}

// TalkEvents report what the scripts changed beyond the records, so the
// scene can follow: party membership, fights, closing the window.
type TalkEvents struct {
	PartyChanged bool   // a character joined or left the party
	Hostile      bool   // a group was alarmed (fight, 274/275)
	Greeting     int    // greeting sound to play on open, 0 = none
	Trade        *Trade // script 275 opened the trade window (window 7)
	Unported     []int
}

func (t *Talk) rand(n int) int {
	if t.Rand != nil {
		return t.Rand(n)
	}
	return rand.IntN(n)
}

func (t *Talk) hero() *Entity { return &t.G.Entities[t.Hero] }

func (t *Talk) settlement() *Settlement { return t.G.settlementOn(t.Map) }

// Quest flags (0x00625068): 300 dwords; byte 0 bit 0x80 = set, uint16 +2 =
// the quest-log text (0xFFFF = none).
const (
	QuestFlagCount = QuestFlagsSize / 4
	questFlagBit   = 0x80
)

// QuestFlag reports flag i.
func (g *Game) QuestFlag(i int) bool {
	return i >= 0 && i < QuestFlagCount && g.Quests[4*i]&questFlagBit != 0
}

// SetQuestFlag sets or clears flag i.
func (g *Game) SetQuestFlag(i int, on bool) {
	if i < 0 || i >= QuestFlagCount {
		return
	}
	if on {
		g.Quests[4*i] |= questFlagBit
	} else {
		g.Quests[4*i] &^= questFlagBit
	}
}

// QuestLog is what the quest log (window 2, 0x0042DEDB) lists: the text of
// every set flag that has one, in flag order.
func (g *Game) QuestLog() []uint32 {
	var out []uint32
	for i := range QuestFlagCount {
		if g.QuestFlag(i) {
			if id := int(g.Quests[4*i+2]) | int(g.Quests[4*i+3])<<8; id != resfmt.QuestNone {
				out = append(out, uint32(id))
			}
		}
	}
	return out
}

// Greeting sounds (0x004365EF): men 12–15 at random without repeating the
// last; women and children by +4: 0x51 → 17, 0x52 → 18, 0x53 → 19, other
// +4 & 0x40 → 16.
const (
	SoundGreetingMan   = 12
	SoundGreetingWoman = 16
)

// OpenDialogue is 0x004364F0: start npc's dialogue (+0xD0). It returns nil
// when the dialogue has no replies to offer; t.Events.Greeting is the sound
// to play (0 when a line has speech, SPEECHS.RES, which is not installed).
func (t *Talk) OpenDialogue(npc int, lastGreeting *int) *Dialogue {
	t.lastGreeting = lastGreeting
	e := t.G.Entities[npc]
	if e.Dialogue == NoDialogue || e.Dialogue >= resfmt.QuestDialogues {
		return nil
	}
	t.NPC = npc
	start := t.Q.Starts[e.Dialogue]
	d := &Dialogue{NPC: npc, Node: start, Prev: start}
	sound := 0
	if e.Flags4&0x40 != 0 {
		switch e.Flags4 {
		case 0x51:
			sound = 17
		case 0x52:
			sound = 18
		case 0x53:
			sound = 19
		default:
			sound = SoundGreetingWoman
		}
	} else {
		sound = t.rand(4) + SoundGreetingMan
		if lastGreeting != nil {
			if sound == *lastGreeting {
				if sound++; sound == SoundGreetingWoman {
					sound = SoundGreetingMan
				}
			}
			*lastGreeting = sound
		}
	}
	r := t.advance(d)
	if r == 0 {
		return nil
	}
	if r == 1 {
		t.Events.Greeting = sound
	}
	return d
}

// advance is 0x00435E78: resolve branch nodes, then list the NPC's lines
// (running their actions) and the replies whose conditions hold. 0 when
// the node offers no reply (the window closes), −1 when a line has speech,
// else 1.
func (t *Talk) advance(d *Dialogue) int {
	d.Lines, d.Replies = nil, nil
	n := d.Node
	for {
		h := t.Q.Node(n)
		if h.Action != resfmt.QuestRedirect {
			break
		}
		k := 0
		for ; k < h.Lines; k++ {
			if r := t.Q.Node(n + k); r.Cond == resfmt.QuestNone || t.condition(r.Cond) {
				break
			}
		}
		if k == h.Lines {
			return 0
		}
		next := t.Q.Node(n + k).Next
		if next == resfmt.QuestEnd || next == resfmt.QuestNone {
			return 0
		}
		n = next
	}
	d.Node = n
	h := t.Q.Node(n)
	ret := 1
	for k := 1; k <= h.Lines; k++ {
		r := t.Q.Node(n + k)
		if r.Cond != resfmt.QuestNone && !t.condition(r.Cond) {
			continue
		}
		if r.Action != resfmt.QuestNone {
			t.actions(r.Action)
		}
		if r.Text != resfmt.QuestNoText && len(d.Lines) < MaxDialogueRows {
			d.Lines = append(d.Lines, r.Text)
			if r.Speech != resfmt.QuestNone {
				ret = -1
			}
		}
	}
	for k := h.Lines + 1; k <= h.Lines+h.Options; k++ {
		r := t.Q.Node(n + k)
		if r.Text == resfmt.QuestNoText || len(d.Replies) >= MaxDialogueRows {
			continue
		}
		if r.Cond != resfmt.QuestNone && !t.condition(r.Cond) {
			continue
		}
		d.Replies = append(d.Replies, DialogueReply{Node: n + k, Back: d.Prev, Text: r.Text})
	}
	if len(d.Replies) == 0 {
		return 0
	}
	d.Prev = d.Node
	return ret
}

// Choose is 0x0041F18C: the player picks reply k. It runs the reply's
// actions and moves to its next node: QuestBack returns to the node the
// current one was entered from, QuestEnd closes. It reports whether the
// dialogue stays open. (A reply with speech first plays it and acts when
// it ends, 0x00423538; the port has no SPEECHS.RES and acts at once.)
func (t *Talk) Choose(d *Dialogue, k int) bool {
	if k < 0 || k >= len(d.Replies) {
		return true
	}
	rep := d.Replies[k]
	r := t.Q.Node(rep.Node)
	if r.Action != resfmt.QuestNone {
		t.actions(r.Action)
	}
	switch r.Next {
	case resfmt.QuestEnd:
		return false
	case resfmt.QuestNone:
		d.Node = rep.Back
	default:
		d.Node = r.Next
	}
	return t.advance(d) != 0
}

// condition is 0x00436248: terms left to right, starting true. An
// AND-term (0x4000) is evaluated only while the result is true and
// replaces it; an OR-term only while it is false. Skipped terms don't run
// their script function.
func (t *Talk) condition(list int) bool {
	res := true
	for _, term := range t.Q.Condition(list) {
		and := term&resfmt.QuestTermAnd != 0
		if and != res {
			continue
		}
		var v bool
		if term&resfmt.QuestTermFunc != 0 {
			v = t.script(term&resfmt.QuestTermIndex) != 0
		} else {
			v = t.G.QuestFlag(term & resfmt.QuestTermIndex)
		}
		if term&resfmt.QuestTermNot != 0 {
			v = !v
		}
		res = v
	}
	return res
}

// actions is 0x00435DBC: run each term: a script function, or set (0x8000)
// / clear a quest flag.
func (t *Talk) actions(list int) {
	for _, term := range t.Q.Actions(list) {
		i := term & resfmt.QuestTermIndex
		switch {
		case term&resfmt.QuestTermFunc != 0:
			t.script(i)
		case term&resfmt.QuestTermNot != 0:
			t.G.SetQuestFlag(i, true)
		default:
			t.G.SetQuestFlag(i, false)
		}
	}
}

// Script function id ranges of 0x0043259C: five families of 56 ids each,
// then the pointer table 0x004657F4 (scriptFuncs).
const (
	scriptGroupHurt  = 0x00 // 0x00434160(map = id + 1)
	scriptMapCleared = 0x38 // 0x004351FC(map = id − 0x37)
	scriptTakeQuest  = 0x70 // 0x00434B60(type + 1 = id − 0x6F)
	scriptGiveQuest  = 0xA6 // 0x004359D4
	scriptHasQuest   = 0xDC // 0x00434C90
	scriptTable      = 0x112
	CatQuest         = 11
)

// script is 0x0043259C(id): a condition's value or an action (the
// return value is then ignored).
func (t *Talk) script(id int) int {
	switch {
	case id < scriptMapCleared:
		return b2i(t.groupHurt(id + 1))
	case id < scriptTakeQuest:
		return b2i(t.mapCleared(id - scriptMapCleared + 1))
	case id < scriptGiveQuest:
		return b2i(t.takeQuestItem(id - scriptTakeQuest))
	case id < scriptHasQuest:
		return b2i(t.giveQuestItem(id - scriptGiveQuest))
	case id < scriptTable:
		return b2i(t.hasQuestItem(id - scriptHasQuest))
	}
	if f, ok := scriptFuncs[id]; ok {
		return f(t)
	}
	t.Events.Unported = append(t.Events.Unported, id)
	return 0
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// groupHurt is 0x00434160(m): a creature group (+0x1E bit 0x40) on map m
// is below its population target or has a dead member.
func (t *Talk) groupHurt(m int) bool {
	for gi, gr := range t.G.Groups {
		if gr.Map != m || gr.Kind&0x40 == 0 {
			continue
		}
		if gr.Count < gr.Target {
			return true
		}
		for k := range gr.Count {
			if t.G.Entities[t.G.Groups[gi].First+k].Dead() {
				return true
			}
		}
		return false // the routine returns after the first matching group
	}
	return false
}

// mapCleared is 0x004351FC(m): every member of the creature groups on map
// m is dead (or of type 3 or 9).
func (t *Talk) mapCleared(m int) bool {
	for _, gr := range t.G.Groups {
		if gr.Map != m || gr.Kind&0x40 == 0 {
			continue
		}
		for k := range gr.Count {
			e := t.G.Entities[gr.First+k]
			if !e.Dead() && e.Type != 9 && e.Type != 3 {
				return false
			}
		}
	}
	return true
}

// questBagIndex finds a quest item (category 11) of type typ in the hero's bag.
func (t *Talk) questBagIndex(typ int) int {
	for k, i := range t.hero().Bag {
		if it, ok := t.G.Item(i); ok && i != 0 && it.Category == CatQuest && it.Type == typ {
			return k
		}
	}
	return -1
}

// takeQuestItem is 0x00434B60: free the first quest item of type typ in
// the hero's bag and close the gap.
func (t *Talk) takeQuestItem(typ int) bool {
	k := t.questBagIndex(typ)
	if k < 0 {
		return false
	}
	e := t.hero()
	t.G.items[e.Bag[k]*itemStride] = 0xFF
	copy(e.Bag[k:], e.Bag[k+1:])
	e.Bag[len(e.Bag)-1] = 0
	return true
}

// giveQuestItem is 0x004359D4: a new quest item of type typ goes to the
// front of the hero's bag, or onto the ground when the bag is full.
func (t *Talk) giveQuestItem(typ int) bool {
	i := t.G.newItem()
	if i == 0 {
		return false
	}
	t.G.setItem(i, CatQuest, typ)
	if !BagInsert(t.hero(), i) && t.Drop != nil {
		t.Drop(i)
	}
	return true
}

// hasQuestItem is 0x00434C90.
func (t *Talk) hasQuestItem(typ int) bool { return t.questBagIndex(typ) >= 0 }

// ScriptFunc runs script function id in t's context (0x0043259C), for
// tools and tests.
func ScriptFunc(t *Talk, id int) int { return t.script(id) }
