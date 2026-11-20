package lanserver

import (
	"konung1.5/internal/lanfight"
	"konung1.5/internal/netplay"
)

// Viewer is what one player was last sent of its hero's map: each
// character's state and the tick it was reported at, so the next view
// carries only the characters whose state differs from the client's
// prediction (netplay.CharState.Predict).
type Viewer struct {
	m    int
	tick int // the last view's tick
	sent map[int]sentChar
}

type sentChar struct {
	cs   netplay.CharState
	tick int
}

// View is slot's hero's map now, for its client: every character when the
// map is new to v (entering, a map change), else the characters whose
// state is not what the client predicts from their last report, and the
// ones gone. ok is false when slot is not in the world or the world has not
// run since the last view.
func (w *World) View(slot int, v *Viewer) (out netplay.ViewPayload, ok bool) {
	m, in := w.players[slot]
	if !in {
		return out, false
	}
	out = netplay.ViewPayload{Map: m, Tick: w.Tick}
	if v.m != m || v.sent == nil {
		v.m, v.sent = m, map[int]sentChar{}
		out.Full = true
	} else if w.Tick == v.tick {
		return out, false
	}
	v.tick = w.Tick
	now := map[int]bool{}
	for _, c := range w.Snapshot(m).Chars {
		cs := charState(c)
		now[cs.Entity] = true
		if old, seen := v.sent[cs.Entity]; out.Full || !seen || !old.cs.Predict(old.tick, w.Tick).Same(cs) {
			cs = cs.Report()
			out.Chars = append(out.Chars, cs)
			v.sent[cs.Entity] = sentChar{cs, w.Tick}
		}
	}
	for e := range v.sent {
		if !now[e] {
			out.Gone = append(out.Gone, e)
			delete(v.sent, e)
		}
	}
	return out, true
}

func charState(c Char) netplay.CharState {
	cs := netplay.CharState{
		Entity: c.Entity, Player: c.Player, Owner: c.Owner, Row: c.Row, Col: c.Col, Walking: c.Walking, Facing: c.Facing,
		Act: c.Act, ActStep: c.ActStep, HP: c.HP, Dead: c.Dead, Stance: c.Stance, Weapon: c.Weapon,
		Steps: c.WalkSteps, ActSteps: c.ActSteps, Throttle: c.Throttle, Path: c.Path,
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

// Progress is slot's hero's own values now, for the account and the 0x5C
// report: level, gold, free points, the base stats and the skills (a kill's
// experience, raises, pick-ups, trades).
func (w *World) Progress(slot int, h netplay.Hero) netplay.Hero {
	if e, ok := w.Hero(slot); ok {
		h.Level, h.Gold, h.FreePoints = e.Level, e.Gold, e.FreeExp
		h.Fencing, h.Archery = e.Fencing, e.Marksmanship
		h.Charisma, h.Strength, h.Dexterity, h.Endurance = e.BaseStats[0], e.BaseStats[1], e.BaseStats[2], e.BaseStats[3]
		h.Herbalism, h.Smithing, h.Trade, h.Carpentry, h.Identification = e.Skills[0], e.Skills[1], e.Skills[2], e.Skills[3], e.Skills[4]
		h.HP, h.Poison = e.HP, e.Poison
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
