package netplay

import (
	"fmt"

	"konung1.5/internal/gamestate"
)

// StartFromGameRes returns a ServerConfig.Start that only answers where a
// hero of each type starts, like the LAN server (0x0044ADB4): the hero
// model's village map, on the arrival cell of the first GAME.RES map exit
// leading there, slot 0 (tests; the dedicated server places the hero in its
// world, internal/lanserver).
func StartFromGameRes(gameRes []byte) (func(PlayerInfo, *Hero) (EnterPayload, error), error) {
	exits, err := gamestate.LANExits(gameRes)
	if err != nil {
		return nil, err
	}
	var starts [Models]EnterPayload
	for m := range starts {
		x, err := gamestate.LANStart(exits, m)
		if err != nil {
			return nil, err
		}
		starts[m] = EnterPayload{Position: Position{Map: x.To, Row: x.Row, Col: x.Col, Facing: x.Facing}}
	}
	return func(p PlayerInfo, _ *Hero) (EnterPayload, error) {
		if p.Model < 0 || p.Model >= Models {
			return EnterPayload{}, fmt.Errorf("no start for hero model %d", p.Model)
		}
		return starts[p.Model], nil
	}, nil
}
