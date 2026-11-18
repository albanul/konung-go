package game

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"sort"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// The golden determinism test pins single-player behaviour while the
// simulation moves to internal/world: a scripted game (walk, stance,
// attack, a map change into creature land, fights) runs thousands of ticks
// and the whole simulation state is hashed at checkpoints. The hashes must
// never change unless a rule deliberately changes.

// goldenHashes are the expected checkpoint hashes (first 16 hex digits).
var goldenHashes = []string{
	"78a211dd04f97df6", "63b8617d77ea24d6", "71c92771e0a71bf3", "b3aaa966167f8f24",
	"29118a76e4c4c9d1", "5d50e5b61a00dcea", "8677491f5a949141",
}

func goldenScenario(t *testing.T) []string {
	t.Helper()
	s := newVolkScene(t)
	var out []string
	check := func() { out = append(out, simHash(s)) }
	lead := func() *member { return s.Lead() }

	run(s, 300)
	check()
	if m := lead(); m != nil {
		s.WalkOrder(m, freeCellAway(s, m.Cell, 15), false)
	}
	run(s, 300)
	check()
	s.ToggleSelectionStance()
	run(s, 100)
	s.attackOrder()
	run(s, 600)
	check()
	// Into creature land: the first exit of the start map to another map.
	var exit *gamestate.MapExit
	for i := range s.Game.Exits {
		x := s.Game.Exits[i]
		if x.From == s.MapNumber && x.To != s.MapNumber && x.To > 0 {
			exit = &x
			break
		}
	}
	if exit == nil {
		t.Fatal("no exit from the start map")
	}
	s.changeMap(*exit)
	run(s, 200)
	check()
	if !s.Party[0].InStance() {
		s.ToggleSelectionStance()
	}
	// God Mode: the hero survives and kills (loot, experience, piles).
	s.Game.GodMode = true
	s.attackOrder()
	run(s, 1500)
	check()
	// Walk around and keep fighting whatever comes.
	if m := lead(); m != nil {
		s.WalkOrder(m, freeCellAway(s, m.Cell, -20), false)
	}
	run(s, 1500)
	check()
	// Without god mode the creatures may win: the hero's death.
	s.Game.GodMode = false
	s.attackOrder()
	run(s, 2000)
	check()
	return out
}

// TestGoldenSimulation checks the scripted game against the recorded
// hashes (and that two runs agree, so the simulation is deterministic).
func TestGoldenSimulation(t *testing.T) {
	a := goldenScenario(t)
	b := goldenScenario(t)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("checkpoint %d differs between two runs: %s / %s (not deterministic)", i, a[i], b[i])
		}
	}
	for i, h := range a {
		if goldenHashes[i] == "" {
			t.Logf("checkpoint %d: %s (not recorded)", i, h)
			continue
		}
		if h != goldenHashes[i] {
			t.Errorf("checkpoint %d: hash %s, want %s", i, h, goldenHashes[i])
		}
	}
}

// simHash hashes the simulation state: the game (GAME.N layout), the party
// copies, every walker, the occupancy, the active piles, the arrows and the
// clock.
func simHash(s *startScene) string {
	h := sha256.New()
	p := func(format string, args ...any) { fmt.Fprintf(h, format, args...) }
	data, err := s.Game.Marshal()
	if err != nil {
		p("marshal error %v\n", err)
	}
	h.Write(data)
	p("tick %d map %d speed %d\n", s.Tick, s.MapNumber, s.speed)
	for _, e := range s.Party {
		p("party %+v\n", e)
	}
	for _, m := range s.Members {
		if m == nil {
			p("member nil\n")
			continue
		}
		hashWalker(h, &m.Walker)
		p("member k=%d intent=%d target=%d tc=%v burn=%d pick=%v resume=%v/%v\n",
			m.K, m.Intent, m.Target, m.TargetCell, m.BurnTarget, m.PickIntent, m.Resume, m.ResumeSet)
	}
	for _, n := range s.NPCs {
		hashWalker(h, &n.Walker)
		p("npc mode=%d live=%d area=%v post=%v posted=%v moveAt=%d group=%d target=%d tc=%v intent=%d pause=%d\n",
			n.Mode, n.LiveMode, n.Area, n.Post, n.Posted, n.MoveAt, n.Group, n.Target, n.TargetCell, n.Intent, n.Pause)
	}
	var occ []mapdata.Cell
	for c, on := range s.Occ {
		if on {
			occ = append(occ, c)
		}
	}
	sort.Slice(occ, func(i, j int) bool {
		if occ[i].Row != occ[j].Row {
			return occ[i].Row < occ[j].Row
		}
		return occ[i].Col < occ[j].Col
	})
	p("occ %v\n", occ)
	var piles []string
	for c, i := range s.PileAt {
		piles = append(piles, fmt.Sprintf("%v=%d", c, i))
	}
	sort.Strings(piles)
	p("piles %v\n", piles)
	for _, a := range s.Arrows {
		p("arrow %+v\n", a)
	}
	p("sel %v cur %d aiNext %d fightNext %d\n", s.Sel, s.Cur, s.AINext, s.FightNext)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

func hashWalker(h hash.Hash, w *walker) {
	l := w.Look // formatted as the original look{typ, weapon, stance, equipment}
	look := fmt.Sprintf("{typ:%d weapon:%d stance:%v equipment:%v}", l.Typ, l.Weapon, l.Stance, l.Equipment)
	fmt.Fprintf(h, "walker e=%d cell=%v from=%v path=%v goal=%v dir=%d walking=%v stepT=%d idle=%d look=%s act=%d step=%d\n",
		w.Entity, w.Cell, w.From, w.Path, w.Goal, w.Dir, w.Walking, w.StepT, w.Idle, look, w.Act, w.ActStep)
}
