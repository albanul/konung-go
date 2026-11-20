package netplay

// The player list of the Q window in a LAN world (docs/konung_lan.md#player-list).
// LAN_KONG.EXE's window 2 (state 2 of 0x00425594, from 0x004258C3) is the
// quest log followed by the players: every player slot with a hero in the
// world (group +0x1B ≠ 0), in slot order, «name[ клан clan][ Владелец
// амулета]», then «Счет команд» and the nations' amulet counts. The client
// knows the names and clans from the players' records the server sends to
// everyone when a hero enters (0x1A, 0x0044B6D8; client 0x00422C9C keeps
// +0x61 name and +0x81 clan per slot at 0x00872ED8 / 0x00872CD8), the
// amulet holders and counts from 0x16 (MsgAmulets here), and its quest log
// from its own quest flags. The port's server sends the list (MsgPlayers)
// to every player when it changes, with the player's own quest log, since
// the quest flags live on the server.

// ListedPlayer is one player in the world: its slot, account name, clan,
// nation (hero type: 0 славяне, 1 викинги, 2 византийцы) and the map its
// hero is on (the original's player group +0x1B, which the map window's
// counts and hover list read: 0x0041E8EC, 0x00416B5E).
type ListedPlayer struct {
	Slot   int    `json:"slot"`
	Name   string `json:"name"`
	Clan   string `json:"clan,omitempty"`
	Nation int    `json:"nation"`
	Map    int    `json:"map"`
}

// PlayersPayload is the player list (by slot) and the receiving player's
// quest log (QUESTR.RES text ids of its set quest flags, in flag order).
type PlayersPayload struct {
	Players []ListedPlayer `json:"players"`
	Log     []uint32       `json:"log,omitempty"`
}
