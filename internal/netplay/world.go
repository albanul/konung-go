package netplay

import (
	"fmt"

	"konung1.5/internal/gamestate"
)

// StartFromGameRes returns a ServerConfig.Start that places heroes like the
// LAN server (0x0044ADB4): the hero model's village map, on the arrival cell
// of the first GAME.RES map exit leading there.
func StartFromGameRes(gameRes []byte) (func(PlayerInfo) (EnterPayload, error), error) {
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
		starts[m] = EnterPayload{Map: x.To, Row: x.Row, Col: x.Col, Facing: x.Facing}
	}
	return func(p PlayerInfo) (EnterPayload, error) {
		if p.Model < 0 || p.Model >= Models {
			return EnterPayload{}, fmt.Errorf("no start for hero model %d", p.Model)
		}
		return starts[p.Model], nil
	}, nil
}
