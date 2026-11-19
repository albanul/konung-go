package lanserver

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/resfmt"
	"konung1.5/internal/world"
)

// frameKey is what an animation's frame count depends on: the model (and
// whether it is an animated OBJECTS.RES model), the action type (the
// animation set's base + kind, as the game's loader) and the facing.
type frameKey struct {
	animated        bool
	model, typ, dir int
}

// frameCache counts animation frames by decoding the sprites once
// (resfmt.LoadEntityAnimation), like the game counts its loaded frames.
type frameCache struct {
	graph, objects []byte
	n              map[frameKey]int
}

// counter is a walker's world.FrameCounter; live is its entity now (the
// stance moves the action type to the set 0–5).
func (f *frameCache) counter(live func() gamestate.Entity) world.FrameCounter {
	return func(_ world.Look, kind, dir int) int {
		e := live()
		spec := e.ModelSpec()
		spec.Type, spec.Variant, spec.Step = world.AnimBase(e.Type)+kind*world.WalkTypeOffset, dir, 0
		k := frameKey{spec.Animated, spec.Model, spec.Type, spec.Variant}
		if n, ok := f.n[k]; ok {
			return n
		}
		steps, err := resfmt.LoadEntityAnimation(f.objects, f.graph, spec)
		n := 0
		if err == nil {
			n = len(steps)
		}
		f.n[k] = n
		return n
	}
}

// Char is a character on a map as clients draw it (the original's 0x19
// and the appearance parts of 0x1A).
type Char struct {
	Entity  int  // index in the world's entities (players: their slot)
	Player  bool // a player's hero
	Owner   int  // a hireling: its player's slot + 1; 0 none
	Row     int  // the cell held (while walking: the one being entered)
	Col     int
	FromRow int // while walking: the cell being left
	FromCol int
	Walking bool
	Step    int // walk step of the cell step in progress
	Facing  int
	Act     int // action animation (gamestate.Act*), 0 = idle or walking
	ActStep int
	HP      int
	Dead    bool
	Stance  bool
	Weapon  int
}

// MapState is a map's characters at a tick.
type MapState struct {
	Map   int
	Tick  int
	Chars []Char
}

// Snapshot is map m's characters now: the players' heroes and every other
// character on it.
func (w *World) Snapshot(m int) MapState {
	out := MapState{Map: m, Tick: w.Tick}
	if m < 1 || m > Maps {
		return out
	}
	s := w.Maps[m]
	add := func(wk *world.Walker, e *gamestate.Entity, player bool) {
		owner := 0
		if player && wk.Entity >= world.LANPlayers {
			player, owner = false, e.Group+1 // a hireling: drawn as its GAME.RES record
		}
		out.Chars = append(out.Chars, Char{
			Entity: wk.Entity, Player: player, Owner: owner, Row: wk.Cell.Row, Col: wk.Cell.Col,
			FromRow: wk.From.Row, FromCol: wk.From.Col, Walking: wk.Walking, Step: wk.StepT, Facing: int(wk.Dir),
			Act: wk.Act, ActStep: wk.ActStep, HP: e.HP, Dead: e.Dead(), Stance: e.InStance(), Weapon: e.WeaponMode,
		})
	}
	for _, mem := range s.Members {
		add(&mem.Walker, &s.Party[mem.K], true)
	}
	for _, n := range s.NPCs {
		add(&n.Walker, &s.Game.Entities[n.Entity], false)
	}
	return out
}

// EventKind says what an Event is.
type EventKind int

const (
	EventSound     EventKind = iota + 1 // a sound at a cell (Sound)
	EventAction                         // a character started an action (Act; its sound is the client's choice)
	EventMapChange                      // a player's hero (Entity = slot) took an exit from map From to Map
	EventKill                           // a player's hero (Entity = slot) killed Victim (VictimPlayer: another player's hero)
)

// Event is something that happened in a tick, for the clients on Map.
// Hits and deaths show in the snapshots (HP, Act, Dead).
type Event struct {
	Kind     EventKind
	Map      int
	From     int
	Entity   int
	Sound    int
	Act      int
	Row, Col int
	Facing   int

	Victim         int
	VictimPlayer   bool
	VictimHireling bool // a player's hireling
}
