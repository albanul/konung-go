package netplay

// The amulet of the dragon (internal/lanserver amulets.go): the server
// tells every player in the world where the three amulet items are and how
// many amulets each nation delivered (MsgAmulets) whenever that changes;
// the original's 0x16 (0x00448200) every 16 ticks, client 0x004222E8.

// AmuletPart is one of the three amulet items: Kind 1–3 the right, left and
// bottom part, 4 the amulet, 5–7 two parts joined (gamestate.AmuletKind), 0
// none (joined into another); Holder the player slot whose hero or
// hireling carries it, −1 on the ground.
type AmuletPart struct {
	Kind   int `json:"kind"`
	Holder int `json:"holder"`
}

// AmuletsPayload is the amulets' state: the three items (the joined ones
// first, as the original keeps them), the amulets delivered by nation (0
// славяне, 1 викинги, 2 византийцы; the hero types) and how many win the
// session (0: none).
type AmuletsPayload struct {
	Parts   [3]AmuletPart `json:"parts"`
	Nations [3]int        `json:"nations"`
	Limit   int           `json:"limit,omitempty"`
}
