package game

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
)

// Arrows in a LAN world (docs/konung_lan.md#arrows-in-flight). The server
// reports every arrow as it leaves the bow (netplay.EventArrow, the
// original's 0x12 that starts the shot on the clients) and the viewer flies
// it with the single-player flight (world.World.FlyShown: FlyArrows' path
// without its effects): one flight per server tick from the tick after its
// release, stopped by a building or the first living character on its way
// other than the shooter. What it hits is the server's (damage, flinch and
// death come with the views); the viewer only draws it (drawArrows, the
// incendiary glow included).

// viewArrowCatchUp is the most server ticks the viewer flies its arrows at
// once (a view after a long gap starts them afresh).
const viewArrowCatchUp = 50

// viewArrow is an arrow the server released at tick, waiting for its
// first flight.
type viewArrow struct {
	a    gamestate.Arrow
	tick int
}

// addViewArrow takes an EventArrow.
func (s *startScene) addViewArrow(e netplay.Event) {
	if s.view == nil || e.Arrow == nil {
		return
	}
	a := e.Arrow
	s.view.arrows = append(s.view.arrows, viewArrow{tick: e.Tick, a: gamestate.Arrow{
		X: a.X, Y: a.Y, VX: a.VX, VY: a.VY, Shooter: a.Shooter, Facing: a.Facing & 7, Life: a.Life, Special: a.Special}})
}

// flyViewArrows flies the arrows through the server ticks up to tick
// (the view's), on the characters' cells as the last view left them: the
// server's flight of tick t also runs before its characters move.
func (s *startScene) flyViewArrows(tick int) {
	v := s.view
	if v.arrowTick == 0 || tick-v.arrowTick > viewArrowCatchUp {
		v.arrowTick = tick - 1
	}
	for t := v.arrowTick + 1; t <= tick; t++ {
		// Arrows released before tick t fly from t on.
		kept := v.arrows[:0]
		for _, p := range v.arrows {
			if p.tick < t {
				s.Arrows = append(s.Arrows, p.a)
			} else {
				kept = append(kept, p)
			}
		}
		v.arrows = kept
		s.Arrows = s.FlyShown(s.Arrows)
	}
	// Arrows released at the view's tick are drawn where they leave the
	// bow; they fly from the next tick.
	kept := v.arrows[:0]
	for _, p := range v.arrows {
		if p.tick <= tick {
			s.Arrows = append(s.Arrows, p.a)
		} else {
			kept = append(kept, p)
		}
	}
	v.arrows = kept
	if tick > v.arrowTick {
		v.arrowTick = tick
	}
}
