package lanserver

import (
	"konung1.5/internal/lanfight"
	"konung1.5/internal/netplay"
)

// Viewer is what one player was last sent of its hero's map, so the next
// view carries only the changes (netplay.ViewPayload).
type Viewer struct {
	m    int
	sent map[int]netplay.CharState
}

// View is slot's hero's map now, for its client: every character when the
// map is new to v (entering, a map change), else the characters that
// changed since the last view and the ones gone. ok is false when slot is
// not in the world or nothing changed.
func (w *World) View(slot int, v *Viewer) (out netplay.ViewPayload, ok bool) {
	m, in := w.players[slot]
	if !in {
		return out, false
	}
	out = netplay.ViewPayload{Map: m, Tick: w.Tick}
	if v.m != m || v.sent == nil {
		v.m, v.sent = m, map[int]netplay.CharState{}
		out.Full = true
	}
	now := map[int]bool{}
	for _, c := range w.Snapshot(m).Chars {
		cs := charState(c)
		now[cs.Entity] = true
		if old, seen := v.sent[cs.Entity]; out.Full || !seen || old != cs {
			out.Chars = append(out.Chars, cs)
			v.sent[cs.Entity] = cs
		}
	}
	for e := range v.sent {
		if !now[e] {
			out.Gone = append(out.Gone, e)
			delete(v.sent, e)
		}
	}
	return out, out.Full || len(out.Chars) > 0 || len(out.Gone) > 0
}

func charState(c Char) netplay.CharState {
	cs := netplay.CharState{
		Entity: c.Entity, Player: c.Player, Row: c.Row, Col: c.Col, Walking: c.Walking, Facing: c.Facing,
		Act: c.Act, ActStep: c.ActStep, HP: c.HP, Dead: c.Dead, Stance: c.Stance, Weapon: c.Weapon,
	}
	if c.Walking {
		cs.FromRow, cs.FromCol, cs.Step = c.FromRow, c.FromCol, c.Step
	}
	return cs
}

// PlayerLook is slot's hero's look: stance, weapon and the equipment's
// item kinds (lanfight.LookOf).
func (w *World) PlayerLook(slot int) (netplay.Look, bool) {
	e, ok := w.Hero(slot)
	if !ok {
		return netplay.Look{}, false
	}
	return lanfight.LookOf(w.Game, e), true
}

// Progress is slot's hero's values after a kill (the experience: free
// points and the combat skills), for the account and the 0x5C report.
func (w *World) Progress(slot int, h netplay.Hero) netplay.Hero {
	if e, ok := w.Hero(slot); ok {
		h.FreePoints, h.Fencing, h.Archery = e.FreeExp, e.Fencing, e.Marksmanship
	}
	return h
}

// Level is entity i's level (a player's hero: its record on its map).
func (w *World) Level(i int) int {
	if e, ok := w.Hero(i); ok {
		return e.Level
	}
	if i >= 0 && i < len(w.Game.Entities) {
		return w.Game.Entities[i].Level
	}
	return 1
}
