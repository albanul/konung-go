package netplay

// Characters' equipment (a port message; the original sent a hireling's
// look change with 0x58 to the players on its map): the server tells a
// player the equipment of the world's characters on its map (villagers,
// creatures, the other players' hirelings: entities 16 and up) whose
// equipment differs from what that player's GAME.RES copy says, and again
// whenever it changes. Stance and weapon come with the view (CharState).

// LooksPayload is MsgLooks: entity → its equipment now.
type LooksPayload struct {
	Equip map[int][EquipSlots]EquipLook `json:"equip"`
}
