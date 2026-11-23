package main

import "testing"

// The LAN client's files are served; the player's login (LAN.CFG, with a
// saved password) and the rating cache (other players' names) never are.
func TestGameFileLAN(t *testing.T) {
	for _, name := range []string{"LAN_KONG.EXE", "ME800R.RES", "PLAY800R.RES", "NEW800R.RES", "MAPS.RES",
		"GAME.RES", "INTO640R.RES", "CLANS.CFG", "GRAPH.RES", "GAME.0", "konung.exe"} {
		if !gameFile.MatchString(name) {
			t.Errorf("%s not served", name)
		}
	}
	for _, name := range []string{"LAN.CFG", "HISCORES.DAT", "Servers.cfg", "SERVER", "SERVER.LOG", "LANUSERS.DAT", "SAVITEMS.DAT", "INTR640R.RES"} {
		if gameFile.MatchString(name) {
			t.Errorf("%s served", name)
		}
	}
}
