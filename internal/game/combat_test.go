package game

import (
	"github.com/hajimehoshi/ebiten/v2"
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
		pileAt:   map[mapdata.Cell]int{},
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
		byEntity: map[int]*npc{921: ant}, occ: occ, target: -1, pileAt: map[mapdata.Cell]int{}}
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
		pileAt:   map[mapdata.Cell]int{},
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
	exe, err := os.ReadFile("../../assets/Konung/konung.exe")
	if err != nil {
		t.Skip("konung.exe not available:", err)
	}
	tb, err := resfmt.NewExeTables(exe)
	if err != nil {
		t.Fatal(err)
	}
	s := &startScene{game: g, tables: tb, npcs: []*npc{ant}, byEntity: map[int]*npc{921: ant}, occ: occ, target: -1,
		pileAt: map[mapdata.Cell]int{}}
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

// TestAttackOrderFindsNextEnemy is the A key: with the melee intent and no
// target the hero takes the nearest enemy in range, kills it, takes the next
// one, and drops the intent when none is left in range.
func TestAttackOrderFindsNextEnemy(t *testing.T) {
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
	grp := g.Entities[921].Group
	if g.Entities[922].Group != grp || g.Entities[923].Group != grp {
		t.Skip("ants 921-923 not in one group")
	}
	heroCell := mapdata.Cell{Row: 100, Col: 40}
	near := mapdata.Neighbors(heroCell)
	cells := map[int]mapdata.Cell{921: near[0].Cell, 922: near[4].Cell, 923: {Row: 100, Col: 40 + attackRangeCols*2}}
	occ := map[mapdata.Cell]bool{heroCell: true}
	s := &startScene{game: g, tables: tb, byEntity: map[int]*npc{}, occ: occ, target: -1,
		rng: rand.New(rand.NewPCG(7, 8)), pileAt: map[mapdata.Cell]int{}}
	for i, c := range cells {
		n := &npc{walker: walker{entity: i, cell: c}, target: -1} // calm: they don't move
		s.npcs = append(s.npcs, n)
		s.byEntity[i] = n
		occ[c] = true
	}
	volk := g.Entities[g.Groups[0].First]
	volk.Flags3 |= 0x04
	s.party = []gamestate.Entity{volk}
	s.hero = &walker{entity: volk.Index, cell: heroCell, blocked: func(c mapdata.Cell) bool { return occ[c] }}
	s.melee = true
	for i := 0; i < 5000 && s.melee; i++ {
		s.heroTick()
		for _, n := range s.npcs {
			s.actTick(&n.walker, &g.Entities[n.entity], n)
		}
	}
	if s.melee || !g.Entities[921].Dead() || !g.Entities[922].Dead() || g.Entities[923].Dead() {
		t.Fatalf("intent %v; dead: 921 %v, 922 %v, 923 (out of range) %v", s.melee,
			g.Entities[921].Dead(), g.Entities[922].Dead(), g.Entities[923].Dead())
	}
}

// TestHeroShootsAnt: Volk with a bow and arrows from GAME.0's item records
// shoots an ant six cells away until it dies; arrows fly as projectiles.
func TestHeroShootsAnt(t *testing.T) {
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
	bow, arrows := 0, 0
	for i := 1; i < 8192 && (bow == 0 || arrows == 0); i++ {
		if it, ok := g.Item(i); ok && it.Category == gamestate.CatBow && bow == 0 {
			bow = i
		} else if ok && it.Category == gamestate.CatArrows && it.Count >= 5 && arrows == 0 {
			arrows = i
		}
	}
	if bow == 0 || arrows == 0 {
		t.Skip("no bow or arrows in GAME.0")
	}
	frames := func(int, int) []sceneFrame { return make([]sceneFrame, 12) }
	heroCell, antCell := mapdata.Cell{Row: 100, Col: 40}, mapdata.Cell{Row: 100, Col: 46}
	occ := map[mapdata.Cell]bool{heroCell: true, antCell: true}
	ant := &npc{walker: walker{entity: 921, cell: antCell, load: frames}, target: -1}
	volk := g.Entities[g.Groups[0].First]
	volk.Flags3 |= 0x04
	volk.Equipment[gamestate.WeaponBow], volk.Quiver, volk.WeaponMode = bow, arrows, gamestate.WeaponBow
	s := &startScene{game: g, tables: tb, party: []gamestate.Entity{volk}, npcs: []*npc{ant},
		byEntity: map[int]*npc{921: ant}, occ: occ, target: 921, shoot: true, walk: &mapdata.WalkGrid{},
		hero: &walker{entity: volk.Index, cell: heroCell, load: frames,
			blocked: func(c mapdata.Cell) bool { return occ[c] }},
		rng: rand.New(rand.NewPCG(9, 9)), pileAt: map[mapdata.Cell]int{}}
	shots := 0
	for i := 0; i < 3000 && !g.Entities[921].Dead() && s.shoot; i++ {
		before := len(s.arrows)
		s.flyArrows()
		s.heroTick()
		if len(s.arrows) > before {
			shots++
		}
		s.actTick(&ant.walker, &g.Entities[921], ant)
	}
	if shots == 0 {
		t.Fatal("no arrow was shot")
	}
	if !g.Entities[921].Dead() && s.party[0].Quiver != 0 {
		t.Fatalf("after %d shots the ant has HP %d (shoot intent %v)", shots, g.Entities[921].HP, s.shoot)
	}
	if s.party[0].Marksmanship <= volk.Marksmanship && g.Entities[921].Dead() {
		t.Error("the kill gave no Меткость")
	}
}

// TestAngryVillagerAttacks: in an alarmed, hostile village a man next to the
// hero takes the stance, then goes for the hero (0x00416DA6).
func TestAngryVillagerAttacks(t *testing.T) {
	gameData, err := os.ReadFile("../../assets/Konung/GAME.0")
	if err != nil {
		t.Skip("GAME.0 not available:", err)
	}
	g, err := gamestate.Parse(gameData)
	if err != nil {
		t.Fatal(err)
	}
	st := g.SettlementOnMap(1)
	gr := g.Groups[st.Group]
	man := -1
	for k := 0; k < gr.Count; k++ {
		if e := g.Entities[gr.First+k]; e.Model < 3 && e.Flags4 != 0x51 && e.Flags4 != 0x52 {
			man = gr.First + k
			break
		}
	}
	if man < 0 {
		t.Skip("no man in the village")
	}
	g.Anger(st)
	cell := mapdata.Cell{Row: 100, Col: 40}
	heroCell := mapdata.Neighbors(cell)[0].Cell
	n := &npc{walker: walker{entity: man, cell: cell}, mode: npcVillager, group: st.Group, target: -1}
	volk := g.Entities[g.Groups[0].First]
	s := &startScene{game: g, party: []gamestate.Entity{volk}, npcs: []*npc{n}, byEntity: map[int]*npc{man: n},
		hero: &walker{entity: volk.Index, cell: heroCell}, target: -1, pileAt: map[mapdata.Cell]int{}, aiBudget: 3}
	s.villagerTick(n)
	if e := g.Entities[man]; !e.InStance() || e.Type >= 6 || lookOf(e).typ != 0 {
		// The stance moves the action type to the set 0–5; with type 6 the
		// swing would ask for the missing type 11.
		t.Fatalf("after the stance: in stance %v, type %d", e.InStance(), e.Type)
	}
	s.villagerTick(n)
	if n.target != volk.Index || n.act != gamestate.ActAttack {
		t.Fatalf("villager target %d, action %d; want the hero, swing", n.target, n.act)
	}
	if !s.fighting(n) {
		t.Error("the village AI would still run for a fighting villager")
	}
}

// TestStanceSwingFrames: a villager who took the stance at runtime must get
// the swing of the stance set (type 5) from its loader, not type 11 of the
// peaceful set (the loader reads the live record, liveEntity).
func TestStanceSwingFrames(t *testing.T) {
	gameData, err := os.ReadFile("../../assets/Konung/GAME.0")
	if err != nil {
		t.Skip("GAME.0 not available:", err)
	}
	objects, err1 := os.ReadFile("../../assets/Konung/OBJECTS.RES")
	graph, err2 := os.ReadFile("../../assets/Konung/GRAPH.RES")
	exe, err3 := os.ReadFile("../../assets/Konung/konung.exe")
	if err1 != nil || err2 != nil || err3 != nil {
		t.Skip("assets not available")
	}
	g, err := gamestate.Parse(gameData)
	if err != nil {
		t.Fatal(err)
	}
	tb, err := resfmt.NewExeTables(exe)
	if err != nil {
		t.Fatal(err)
	}
	const elder = 355
	var p gamestate.Placed
	for _, q := range g.OnMap(1, nil) {
		if q.Index == elder {
			p = q
		}
	}
	var loadErr error
	load := entityLoader(g, p, liveEntity(g, elder), objects, graph, tb,
		func(*resfmt.EntitySprite) (*ebiten.Image, *ebiten.Image) { return nil, nil },
		func(err error) { loadErr = err })
	gamestate.EnterStance(&g.Entities[elder])
	if f := load(gamestate.ActAttack, 0); loadErr != nil || len(f) != 12 {
		t.Fatalf("swing after the stance: %d frames, error %v", len(f), loadErr)
	}
}

// TestAlarmCutsWalks: the alarm 0x004248A0 ends every member's walk after
// the current step (+0x14 = 0xFF), so villagers on a calm walk decide at once.
func TestAlarmCutsWalks(t *testing.T) {
	gameData, err := os.ReadFile("../../assets/Konung/GAME.0")
	if err != nil {
		t.Skip("GAME.0 not available:", err)
	}
	g, err := gamestate.Parse(gameData)
	if err != nil {
		t.Fatal(err)
	}
	st := g.SettlementOnMap(1)
	long := []mapdata.Cell{{Row: 90, Col: 40}, {Row: 92, Col: 40}, {Row: 94, Col: 40}, {Row: 96, Col: 40}}
	walking := &npc{walker: walker{entity: 360, cell: mapdata.Cell{Row: 88, Col: 40}, walking: true,
		path: append([]mapdata.Cell(nil), long...)}, mode: npcVillager, group: st.Group, target: -1}
	idle := &npc{walker: walker{entity: 361, cell: mapdata.Cell{Row: 70, Col: 40}, path: long[2:]},
		mode: npcVillager, group: st.Group, target: -1}
	s := &startScene{game: g, npcs: []*npc{walking, idle}}
	s.provoke(st.Group)
	if st.Flags&0x15 != 0x15 || g.Groups[st.Group].Alarm != gamestate.AlarmAttack {
		t.Fatalf("village flags %#x alarm %#x", st.Flags, g.Groups[st.Group].Alarm)
	}
	if len(walking.path) != 1 || walking.path[0] != long[0] || len(idle.path) != 0 {
		t.Errorf("paths after the alarm: walking %v, not yet stepping %v", walking.path, idle.path)
	}
}

// TestHunterShootsAgain: a village hunter (bow, arrows in the bag) shoots
// from range; when the hero is next to him he fights in melee; when the hero
// steps back into bow range he shoots again (the AI tests the gear, not the
// active weapon: 0x00416F65).
func TestHunterShootsAgain(t *testing.T) {
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
	const hunter = 357 // bow equipped, arrows in the bag
	st := g.SettlementOnMap(1)
	frames := func(int, int) []sceneFrame { return make([]sceneFrame, 12) }
	cell := mapdata.Cell{Row: 100, Col: 40}
	n := &npc{walker: walker{entity: hunter, cell: cell, load: frames, dir: 4}, mode: npcVillager, group: st.Group, target: -1}
	volk := g.Entities[g.Groups[0].First]
	volk.HP = 60000
	far := mapdata.Cell{Row: 100, Col: 45} // 5 cells east, inside bow range
	s := &startScene{game: g, tables: tb, party: []gamestate.Entity{volk}, npcs: []*npc{n}, byEntity: map[int]*npc{hunter: n},
		hero: &walker{entity: volk.Index, cell: far, load: frames}, target: -1, pileAt: map[mapdata.Cell]int{},
		walk: &mapdata.WalkGrid{}, rng: rand.New(rand.NewPCG(4, 4)), occ: map[mapdata.Cell]bool{}}
	s.provoke(st.Group)
	shoot := func() bool {
		for range 40 {
			s.aiBudget = 3
			s.villagerTick(n)
			if n.act == gamestate.ActShoot {
				return true
			}
		}
		return false
	}
	if !shoot() {
		t.Fatalf("the hunter did not shoot from 5 cells (mode %d, quiver %d)", g.Entities[hunter].WeaponMode, g.Entities[hunter].Quiver)
	}
	for n.act == gamestate.ActShoot {
		s.villagerTick(n)
	}
	s.hero.cell = mapdata.Neighbors(cell)[4].Cell // right next to him: too close to shoot
	s.aiBudget = 3
	s.villagerTick(n)
	if n.act != gamestate.ActAttack {
		t.Fatalf("next to the hero the hunter's action is %d, want the swing", n.act)
	}
	s.hero.cell = far // the hero steps back: the swing stops, the bow comes out again
	s.villagerTick(n)
	if !shoot() {
		t.Fatalf("after melee the hunter never shot again (mode %d)", g.Entities[hunter].WeaponMode)
	}
}
