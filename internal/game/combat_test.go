package game

import (
	"math/rand/v2"
	"os"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

// TestHeroKillsAnt runs the melee controller headless: Volk next to giant
// ant 921 swings until it dies; the ant plays its death, is marked dead and
// frees its cell, and Volk gains the kill's experience.
func TestHeroKillsAnt(t *testing.T) {
	gameData, err := os.ReadFile("../../assets/Konung/GAME.0")
	if err != nil {
		t.Skip("GAME.0 not available:", err)
	}
	exe, err := os.ReadFile("../../assets/Konung/konung.exe")
	if err != nil {
		t.Skip("konung.exe not available:", err)
	}
	g, err := gamestate.Parse(gameData)
	if err != nil {
		t.Fatal(err)
	}
	tb, err := resfmt.NewExeTables(exe)
	if err != nil {
		t.Fatal(err)
	}
	volk := g.Entities[g.Groups[0].First]
	volk.Flags3 |= 0x04 // stance
	antCell := mapdata.Cell{Row: 100, Col: 40}
	heroCell := mapdata.Neighbors(antCell)[0].Cell
	ant := &npc{walker: walker{entity: 921, cell: antCell}}
	s := &startScene{
		game: g, tables: tb, party: []gamestate.Entity{volk},
		hero:     &walker{entity: volk.Index, cell: heroCell, blocked: func(mapdata.Cell) bool { return false }},
		npcs:     []*npc{ant},
		byEntity: map[int]*npc{921: ant},
		occ:      map[mapdata.Cell]bool{antCell: true, heroCell: true},
		rng:      rand.New(rand.NewPCG(1, 2)),
		target:   -1,
	}
	s.startFight(921)
	for i := 0; i < 1000 && !g.Entities[921].Dead(); i++ {
		s.combatTick()
	}
	if !g.Entities[921].Dead() || g.Entities[921].HP != 0 {
		t.Fatalf("ant not dead: HP %d", g.Entities[921].HP)
	}
	if s.occ[antCell] || ant.act != gamestate.ActDeath || s.target != -1 {
		t.Errorf("after the kill: cell held %v, ant action %d, hero target %d", s.occ[antCell], ant.act, s.target)
	}
	if s.party[0].FreeExp != volk.FreeExp+1 || s.party[0].Fencing != volk.Fencing+4 {
		t.Errorf("Volk free exp %d → %d, Фехтование %d → %d", volk.FreeExp, s.party[0].FreeExp, volk.Fencing, s.party[0].Fencing)
	}
	if s.hero.dir != facing(heroCell, antCell) {
		t.Errorf("Volk faces %d, want %d", s.hero.dir, facing(heroCell, antCell))
	}
}

// TestHeroChasesTarget checks the approach: from a few cells away the hero
// walks up to the target, stops before its cell and starts the swing.
func TestHeroChasesTarget(t *testing.T) {
	gameData, err := os.ReadFile("../../assets/Konung/GAME.0")
	if err != nil {
		t.Skip("GAME.0 not available:", err)
	}
	g, err := gamestate.Parse(gameData)
	if err != nil {
		t.Fatal(err)
	}
	antCell := mapdata.Cell{Row: 100, Col: 40}
	ant := &npc{walker: walker{entity: 921, cell: antCell}}
	occ := map[mapdata.Cell]bool{antCell: true}
	hero := &walker{entity: 0, cell: mapdata.Cell{Row: 92, Col: 36}, searchCap: pathSearchLimit}
	hero.blocked = func(c mapdata.Cell) bool { return occ[c] }
	s := &startScene{game: g, party: []gamestate.Entity{g.Entities[0]}, hero: hero, npcs: []*npc{ant},
		byEntity: map[int]*npc{921: ant}, occ: occ, target: -1}
	s.startFight(921)
	for i := 0; i < 5000 && hero.act != gamestate.ActAttack; i++ {
		hero.update()
		if hero.cell == antCell {
			t.Fatal("hero walked into the target's cell")
		}
		if i%walkTicksPerStep == 0 {
			s.heroTick()
		}
	}
	if hero.act != gamestate.ActAttack || !adjacent(hero.cell, antCell) {
		t.Fatalf("hero at %v (action %d), want swinging next to %v", hero.cell, hero.act, antCell)
	}
}

// TestCreatureFightsBack puts the hero inside a giant ant group's
// territory next to one ant: the alarm check alarms the group, the ant goes
// for the hero, the idle hero strikes back (0x0041EC0C), and the fight ends
// with one of them dead.
func TestCreatureFightsBack(t *testing.T) {
	gameData, err := os.ReadFile("../../assets/Konung/GAME.0")
	if err != nil {
		t.Skip("GAME.0 not available:", err)
	}
	exe, err := os.ReadFile("../../assets/Konung/konung.exe")
	if err != nil {
		t.Skip("konung.exe not available:", err)
	}
	g, err := gamestate.Parse(gameData)
	if err != nil {
		t.Fatal(err)
	}
	tb, err := resfmt.NewExeTables(exe)
	if err != nil {
		t.Fatal(err)
	}
	antE := g.Entities[921]
	grp := antE.Group
	area, _ := g.Groups[grp].WanderArea()
	antCell := mapdata.Cell{Row: (area.RowMin + area.RowMax) / 2, Col: (area.ColMin + area.ColMax) / 2}
	heroCell := mapdata.Neighbors(antCell)[0].Cell
	occ := map[mapdata.Cell]bool{antCell: true, heroCell: true}
	free := func(c mapdata.Cell) bool { return occ[c] }
	volk := g.Entities[g.Groups[0].First]
	ant := &npc{walker: walker{entity: 921, cell: antCell, blocked: free, searchCap: pathSearchLimit},
		mode: npcCreature, group: grp, target: -1}
	s := &startScene{
		game: g, tables: tb, party: []gamestate.Entity{volk},
		hero:     &walker{entity: volk.Index, cell: heroCell, blocked: free, searchCap: pathSearchLimit},
		npcs:     []*npc{ant},
		byEntity: map[int]*npc{921: ant},
		occ:      occ,
		rng:      rand.New(rand.NewPCG(3, 4)),
		target:   -1,
	}
	s.setStopAt(ant)
	attacked, struck := false, false
	for i := 0; i < 2000 && !g.Entities[921].Dead() && !s.party[0].Dead(); i++ {
		s.tick = i * walkTicksPerStep
		s.combatTick()
		attacked = attacked || (ant.act == gamestate.ActAttack && ant.target == volk.Index)
		struck = struck || s.target == 921
	}
	if g.Groups[grp].Alarm != gamestate.AlarmAttack {
		t.Errorf("group %d alarm %#x, want attack", grp, g.Groups[grp].Alarm)
	}
	if !attacked || !struck {
		t.Fatalf("ant attacked %v, hero struck back %v", attacked, struck)
	}
	if !g.Entities[921].Dead() && !s.party[0].Dead() {
		t.Fatalf("no one died: ant HP %d, hero HP %d", g.Entities[921].HP, s.party[0].HP)
	}
}

// TestCreatureReturnsHome: an ant outside its territory with no party
// member inside gets the return alarm and walks back in (0x004132FE).
func TestCreatureReturnsHome(t *testing.T) {
	gameData, err := os.ReadFile("../../assets/Konung/GAME.0")
	if err != nil {
		t.Skip("GAME.0 not available:", err)
	}
	g, err := gamestate.Parse(gameData)
	if err != nil {
		t.Fatal(err)
	}
	grp := g.Entities[921].Group
	area, _ := g.Groups[grp].WanderArea()
	start := mapdata.Cell{Row: area.RowMax + 6, Col: area.ColMax + 3}
	occ := map[mapdata.Cell]bool{start: true}
	ant := &npc{walker: walker{entity: 921, cell: start, searchCap: pathSearchLimit}, mode: npcCreature, group: grp, target: -1,
		rng: rand.New(rand.NewPCG(5, 6)).IntN}
	ant.blocked = func(c mapdata.Cell) bool { return occ[c] }
	ant.reserve = func(c mapdata.Cell) bool {
		if occ[c] {
			return false
		}
		occ[c] = true
		return true
	}
	ant.release = func(c mapdata.Cell) { delete(occ, c) }
	s := &startScene{game: g, npcs: []*npc{ant}, byEntity: map[int]*npc{921: ant}, occ: occ, target: -1}
	s.walk = &mapdata.WalkGrid{}
	s.worldW, s.worldH = 1<<20, 1<<20
	for i := 0; i < 20000 && !area.Contains(ant.cell.Row, ant.cell.Col); i++ {
		s.tick = i
		ant.update()
		if i%walkTicksPerStep == 0 {
			s.combatTick()
		}
	}
	if g.Groups[grp].Alarm != gamestate.AlarmReturn || !area.Contains(ant.cell.Row, ant.cell.Col) {
		t.Fatalf("alarm %#x, ant at %v; want return and inside %+v", g.Groups[grp].Alarm, ant.cell, area)
	}
}
