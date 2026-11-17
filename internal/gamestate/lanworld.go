package gamestate

import "fmt"

// The LAN version's world file GAME.RES (client loader 0x0041DC84): GAME.N's
// six blocks in the same order, smaller (docs/konung_lan.md#world-gameres):
// 8192 items (as GAME.N), 80 groups (as GAME.N; groups 0–15 are the player
// slots), 2100 ground piles (0x40 bytes, a guess by size), 250 map exits
// (as GAME.N), 320 entities (a LAN layout of its own) and 3 settlements.
const (
	LANWorldSize  = 365509
	lanExitOffset = 0x41700
)

// LANExits reads GAME.RES's map exits.
func LANExits(gameRes []byte) ([]MapExit, error) {
	if len(gameRes) != LANWorldSize {
		return nil, fmt.Errorf("GAME.RES has %d bytes, want %d", len(gameRes), LANWorldSize)
	}
	return parseExitsAt(gameRes, lanExitOffset), nil
}

// LANStartMap is a LAN hero's start map: its model's village, model·3 + 1
// (0x0044401C). The hero model is also the team.
func LANStartMap(model int) int { return model*3 + 1 }

// LANStart is where the server puts an entering hero (0x0044ADB4): the start
// map, on the arrival cell and facing of the first map exit that leads
// there. The shipped GAME.RES gives model 0 → map 1 (110, 32), 1 → map 4
// (66, 57), 2 → map 7 (65, 8).
func LANStart(exits []MapExit, model int) (MapExit, error) {
	m := LANStartMap(model)
	for _, x := range exits {
		if x.To == m {
			return x, nil
		}
	}
	return MapExit{}, fmt.Errorf("GAME.RES: no map exit leads to start map %d", m)
}

// EnterLANWorld turns g, a single-player GAME.N used as the hero's template,
// into a LAN world holding only the hero: group 0 with its first entity on
// map m at (row, col) facing facing; every other group, settlement, pile
// and map exit is removed (the LAN world's own characters come from
// GAME.RES, whose entity layout is not ported yet).
func (g *Game) EnterLANWorld(m, row, col, facing int) {
	if len(g.Groups) == 0 {
		return
	}
	for i := range g.Groups[1:] {
		gr := &g.Groups[i+1]
		gr.Count, gr.Map, gr.Kind = 0, 0, 0
	}
	g0 := &g.Groups[0]
	g0.Count, g0.Map, g0.Row, g0.Col = 1, m, row, col
	if h := g0.First; h >= 0 && h < len(g.Entities) {
		e := &g.Entities[h]
		e.Row, e.Col, e.Variant = row, col, facing&7
	}
	g.Settlements, g.Piles, g.Exits = nil, nil, nil
}
