package netplay

// Hirelings (internal/lanserver party.go): a player's hero hires plain
// villagers through their dialogue; they follow the hero and take the
// player's orders when selected (OrderPayload.Sel), and «Все ко мне»
// (OrderGather) calls them back. The server tells the player its party
// (MsgParty) whenever it changes and after a map change; the hirelings'
// states come with the view like every character's (CharState.Owner).

// PartyPayload is the own hero's hirelings: entity indices (GAME.RES
// records), in hiring order.
type PartyPayload struct {
	Hirelings []int `json:"hirelings"`
}
