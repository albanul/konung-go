package gamestate

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"konung1.5/internal/mapdata"
)

// TestBuildingObjectsMatchMaps: in every GAME.N a building record's
// object (type object + state, 0x0043CB4A) is the ID the map file has at
// that placement, except 84 unbuilt records (state 0–2) that the files
// draw standing, most of them the fence of map 33 (a trench until dialogue
// 361 builds it).
func TestBuildingObjectsMatchMaps(t *testing.T) {
	tk := talkSetup(t)
	for n := range 3 {
		g, err := Parse(readAsset(t, fmt.Sprintf("GAME.%d", n)))
		if err != nil {
			t.Fatal(err)
		}
		checked, unbuilt := 0, 0
		for _, s := range g.Settlements {
			if s.Map == 0 {
				continue
			}
			data, err := os.ReadFile(filepath.Join("..", "..", "assets", "Konung", fmt.Sprintf("KONUNG.%d", s.Map)))
			if err != nil {
				t.Skip(err)
			}
			pl, err := mapdata.ParsePlacements(data)
			if err != nil {
				t.Fatal(err)
			}
			objs := g.BuildingObjects(s.Map, tk.T)
			for r := range s.buildingCount() {
				b := s.building(r)
				typ, state := int(int8(b[0])), int(int8(b[1]))
				if typ < 0 {
					continue
				}
				i := int(binary.LittleEndian.Uint16(b[2:]))
				id := objs[i]
				if i >= len(pl) || id != tk.T.BuildingObject(typ)+state {
					t.Fatalf("GAME.%d map %d record %d: placement %d, object %d", n, s.Map, r, i, id)
				}
				checked++
				if pl[i].ID == id {
					continue
				}
				if state >= BuildingStanding || pl[i].ID != tk.T.BuildingObject(typ)+BuildingStanding {
					t.Errorf("GAME.%d map %d record %d (type %d state %d): object %d, map has %d", n, s.Map, r, typ, state, id, pl[i].ID)
				}
				unbuilt++
			}
		}
		if checked < 500 || unbuilt != 84 {
			t.Errorf("GAME.%d: %d buildings, %d unbuilt drawn standing in the map files", n, checked, unbuilt)
		}
	}
}

// TestRuinRebuilt: a ruin in «Родники» (map 1) is rebuilt by the
// village's free humans in three stages of 60·build time/builders passes
// and then stands; the voevoda skips the passes that start a stage.
func TestRuinRebuilt(t *testing.T) {
	tk := talkSetup(t)
	g := tk.G
	s := g.SettlementOnMap(1)
	b := s.building(1)
	typ := int(int8(b[0]))
	b[1] = BuildingRuin
	humans, busy, posted := g.villageCount(s)
	builders := humans - busy - posted
	if builders <= 0 {
		t.Fatalf("builders %d (humans %d busy %d posted %d)", builders, humans, busy, posted)
	}
	stage := 60 * tk.T.BuildTime(typ) / builders
	if !g.SettlementPass(tk.T, tk.Hero, 1, false) {
		t.Error("no change reported on the current map")
	}
	if g.SettlementPass(tk.T, tk.Hero, 2, false) {
		t.Error("change reported for another map")
	}
	if b[1] != 0 || int(int16(binary.LittleEndian.Uint16(b[6:]))) != stage-1 {
		t.Fatalf("after two passes: %v, want state 0 timer %d", b, stage-1)
	}
	if id := g.BuildingObjects(1, tk.T)[int(binary.LittleEndian.Uint16(b[2:]))]; id != tk.T.BuildingObject(typ) {
		t.Errorf("object %d, want %d (state 0)", id, tk.T.BuildingObject(typ))
	}
	passes := 2
	for ; b[1] != BuildingStanding && passes < 10*stage; passes++ {
		g.SettlementPass(tk.T, tk.Hero, 1, false)
	}
	if b[1] != BuildingStanding || passes != 1+3*stage {
		t.Errorf("state %d after %d passes, want 3 after %d", b[1], passes, 1+3*stage)
	}
	for range 10 {
		g.SettlementPass(tk.T, tk.Hero, 1, false)
	}
	if b[1] != BuildingStanding {
		t.Errorf("standing building changed: %v", b)
	}
}

// TestBuildingBurns: a standing building with a running timer burns
// through states 4 and 5 (burn time per stage) to the ruin.
func TestBuildingBurns(t *testing.T) {
	tk := talkSetup(t)
	g := tk.G
	s := g.SettlementOnMap(1)
	b := s.building(0)
	typ := int(int8(b[0]))
	if b[1] != BuildingStanding {
		t.Fatalf("record 0: %v", b)
	}
	g.Groups[s.Group].Alarm = AlarmAttack // no builders
	binary.LittleEndian.PutUint16(b[6:], 1)
	burn := tk.T.BurnTime(typ)
	passes := 0
	for ; b[1] != BuildingRuin && passes < 10*burn; passes++ {
		g.SettlementPass(tk.T, tk.Hero, 1, false)
	}
	if b[1] != BuildingRuin || passes != 1+2*burn {
		t.Errorf("state %d after %d passes, want 6 after %d", b[1], passes, 1+2*burn)
	}
}

// TestVoevodaTrains: every 720 passes the voevoda raises the base stats
// below his own by one and closes part of the Фехтование gap.
func TestVoevodaTrains(t *testing.T) {
	tk := talkSetup(t)
	g := tk.G
	for i := range g.Settlements {
		s := &g.Settlements[i]
		v := s.Posts[4].Entity
		if s.Map == 0 || v == 0 {
			continue
		}
		vo := g.Entities[v]
		gr := g.Groups[s.Group]
		for k := 0; k < gr.Count; k++ {
			id := gr.First + k
			m := g.Entities[id]
			if id == v || !m.canAct() || m.Model >= 3 || m.BaseStats[1] >= vo.BaseStats[1] || m.Fencing >= vo.Fencing {
				continue
			}
			before := m
			s.setI32(voevodaTimerOff, 1)
			g.voevodaPass(s, tk.T)
			after := g.Entities[id]
			if s.i32(voevodaTimerOff) != voevodaPeriod {
				t.Errorf("timer %d", s.i32(voevodaTimerOff))
			}
			if after.Fencing <= before.Fencing || after.Fencing > vo.Fencing+1 {
				t.Errorf("Фехтование %d → %d (voevoda %d)", before.Fencing, after.Fencing, vo.Fencing)
			}
			if after.FreeExp != before.FreeExp {
				t.Errorf("free experience changed %d → %d", before.FreeExp, after.FreeExp)
			}
			if after.BaseStats[1] == before.BaseStats[1] && after.Experience == before.Experience {
				t.Logf("Сил not raised (capped at level %d): %d", after.Level, after.BaseStats[1])
			}
			t.Logf("settlement %d member %d: %+v → %+v, Фехтование %d → %d", s.Index, id, before.BaseStats, after.BaseStats, before.Fencing, after.Fencing)
			// Nothing happens until the next period.
			g.voevodaPass(s, tk.T)
			if g.Entities[id].Fencing != after.Fencing {
				t.Error("trained twice")
			}
			return
		}
	}
	t.Skip("no trainee below a voevoda")
}
