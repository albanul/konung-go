package gamestate

import (
	"encoding/binary"

	"konung1.5/internal/resfmt"
)

// Village buildings and the voevoda's training in the settlement pass
// (0x0041D39F). See docs/konung_buildings.md.
//
// A building record (settlement +0x18 + 8r, r < extra buildings + fence
// pieces + 7) is int8 type (−1 = none), int8 state, uint16 map placement
// index, int16 timer at +6. The placement shows object ID
// BuildingObject(type) + state.

// Building states.
const (
	BuildingStarted  = 0 // states 0–2: under construction
	BuildingStanding = 3
	BuildingRuin     = 6 // states 4 and 5 burn down to the ruin
)

// voevodaPeriod is the training period in passes (0x0041804F).
const (
	voevodaTimerOff = 0x0C
	voevodaPeriod   = 720
)

// buildingCount is the number of building records the pass and the map
// loader walk: 7 + extra buildings (+0) + fence pieces (+1).
func (s *Settlement) buildingCount() int {
	return min(int(s.Raw[0])+s.Fence+7, buildingRecords)
}

// BuildingObjects maps placement index → object ID for the buildings of
// the settlements on map m: the map loader sets each building's placement
// to its type's object + state (0x0043CB4A).
func (g *Game) BuildingObjects(m int, t *resfmt.ExeTables) map[int]int {
	out := map[int]int{}
	for i := range g.Settlements {
		s := &g.Settlements[i]
		if s.Map != m || m == 0 {
			continue
		}
		for r := range s.buildingCount() {
			b := s.building(r)
			if typ := int(int8(b[0])); typ > -1 {
				out[int(binary.LittleEndian.Uint16(b[2:]))] = t.BuildingObject(typ) + int(int8(b[1]))
			}
		}
	}
	return out
}

// villageCount is the head of the settlement pass (0x0041D404): the
// village group's humans (alive, not in action 9 or 3, model < 3), how
// many of them are busy (+4 & 0x38: apprentices…), and the filled posts.
func (g *Game) villageCount(s *Settlement) (humans, busy, posted int) {
	gr := g.Groups[s.Group]
	for k := 0; k < gr.Count; k++ {
		e := g.Entities[gr.First+k]
		if e.Model >= 3 || !e.canAct() {
			continue
		}
		humans++
		if e.Flags4&0x38 != 0 {
			busy++
		}
	}
	for _, p := range s.Posts {
		if p.Entity != 0 {
			posted++
		}
	}
	return
}

// buildingPass is 0x0041D939. builders is the number of free humans (0
// while the village is alarmed). Per record with a type:
//   - a ruin with builders starts over: state 0, timer 60·build time /
//     builders;
//   - while the timer runs and the state is below 3 with builders, each
//     pass counts down; at 0 the next stage starts (or the building
//     stands at state 3);
//   - otherwise a running timer counts down and steps the state with the
//     burn time per stage, ending at the ruin (6). Without builders a
//     construction also steps on this way (and burns on past 3).
//
// started reports a stage started (the voevoda then skips the pass);
// changed reports a state change on map m (0x0041733C updates the map
// object).
func (g *Game) buildingPass(s *Settlement, t *resfmt.ExeTables, builders, m int) (started, changed bool) {
	for r := range s.buildingCount() {
		b := s.building(r)
		typ := int(int8(b[0]))
		if typ <= -1 {
			continue
		}
		state := int(int8(b[1]))
		timer := int(int16(binary.LittleEndian.Uint16(b[6:])))
		setTimer := func(v int) { binary.LittleEndian.PutUint16(b[6:], uint16(int16(v))) }
		stage := func() int { return 60 * int(int16(t.BuildTime(typ))) / builders }
		switch {
		case timer <= 0:
			if state != BuildingRuin || builders <= 0 {
				continue
			}
			started = true
			b[1] = BuildingStarted
			setTimer(stage())
		case state < BuildingStanding && builders > 0:
			timer--
			setTimer(timer)
			if timer > 0 {
				continue
			}
			state++
			b[1] = byte(state)
			if state != BuildingStanding {
				started = true
				setTimer(stage())
			}
		default:
			timer--
			setTimer(timer)
			if timer > 0 {
				continue
			}
			state++
			b[1] = byte(state)
			if state != BuildingRuin {
				setTimer(t.BurnTime(typ))
			}
		}
		if s.Map == m {
			changed = true
		}
	}
	return
}

// voevodaPass is 0x00418014: every 720 passes the voevoda (post 4)
// trains each human of the village: a level when it qualifies, then each
// base stat below the voevoda's (Лов, Хар, Сил, Вын) goes up one at the
// usual cost, paid as experience (+0x3C) instead of free experience;
// another level check; and when any stat was below, Фехтование and
// Меткость close the gap by (voevoda − member) / total stat gap + 1.
func (g *Game) voevodaPass(s *Settlement, t *resfmt.ExeTables) {
	v := s.Posts[4].Entity
	if v == 0 {
		return
	}
	n := s.i32(voevodaTimerOff) - 1
	s.setI32(voevodaTimerOff, n)
	if n > 0 {
		return
	}
	s.setI32(voevodaTimerOff, voevodaPeriod)
	gr := g.Groups[s.Group]
	vo := g.Entities[v]
	// Raise order and RaiseCost ids: Лов 2, Хар 3, Сил 5, Вын 6
	// (BaseStats is Хар, Сил, Лов, Вын).
	order := [4]struct{ stat, id int }{{2, 2}, {0, 3}, {1, 5}, {3, 6}}
	for k := 0; k < gr.Count; k++ {
		id := gr.First + k
		m := g.Entities[id]
		if !m.canAct() || id == v || m.Model >= 3 {
			continue
		}
		if g.RaiseCost(m, t, 0) != 0 {
			m.Level = int(uint8(m.Level + 1))
		}
		gap := 0
		for _, o := range order {
			if int16(m.BaseStats[o.stat]) >= int16(vo.BaseStats[o.stat]) {
				continue
			}
			gap += int(int16(vo.BaseStats[o.stat])) - int(int16(m.BaseStats[o.stat]))
			m.FreeExp += 1000
			if c := g.RaiseCost(m, t, o.id); c != 0 {
				m.Experience += c
				m.BaseStats[o.stat] = int(uint16(m.BaseStats[o.stat] + 1))
			}
			m.FreeExp -= 1000
		}
		if g.RaiseCost(m, t, 0) != 0 {
			m.Level = int(uint8(m.Level + 1))
		}
		if gap != 0 {
			m.Fencing = int(uint16(m.Fencing + (vo.Fencing-m.Fencing)/gap + 1))
			m.Marksmanship = int(uint16(m.Marksmanship + (vo.Marksmanship-m.Marksmanship)/gap + 1))
			m = g.Recompute(m, t)
		}
		g.Entities[id] = m
	}
}
