package netplay

// Dialogues and trade in the LAN world (docs/konung_lan.md#dialogues-and-trade).
// LAN_KONG.EXE's client asks the server for the trader's shop (0x43 →
// 0x0044A3BC, the slot's stock in 0x83), runs the dialogue itself and sends
// the script functions that change the hero (0x53) and the finished trade
// (0x42, which the server trusts). The port's server runs both: a client
// sends OrderTalk (a right click on a character with a dialogue),
// OrderReply (a reply) and the trade's lists (ItemTrade); the server sends
// the dialogue (MsgDialogue) and the trade window (MsgPileWindow with a
// Trader).

// DialoguePayload is the open dialogue (window 1): the character talked to,
// the QUESTR.RES texts of its lines and of the replies offered, and the
// greeting sound to play when it opened (0 none). Open false: closed.
type DialoguePayload struct {
	Open     bool     `json:"open"`
	NPC      int      `json:"npc,omitempty"`
	Lines    []uint32 `json:"lines,omitempty"`
	Replies  []uint32 `json:"replies,omitempty"`
	Greeting int      `json:"greeting,omitempty"`
}
