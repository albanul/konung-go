package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math/rand/v2"
	"os"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
	"konung1.5/internal/world"
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
	ant := &npc{Walker: walker{Entity: 921, Cell: antCell}}
	s := &startScene{World: &world.World{Game: g, Tables: tb, Party: []gamestate.Entity{volk}, NPCs: []*npc{ant}, ByEntity: map[int]*npc{921: ant}, Occ: map[mapdata.Cell]bool{antCell: true, heroCell: true}, Rng: rand.New(rand.NewPCG(1, 2)), PileAt: map[mapdata.Cell]int{}}}
	h := setHero(s, walker{Entity: volk.Index, Cell: heroCell, Blocked: func(mapdata.Cell) bool { return false }})
	s.StartFight(h, 921)
	for i := 0; i < 1000 && !g.Entities[921].Dead(); i++ {
		s.CombatTick()
	}
	if !g.Entities[921].Dead() || g.Entities[921].HP != 0 {
		t.Fatalf("ant not dead: HP %d", g.Entities[921].HP)
	}
	if s.Occ[antCell] || ant.Act != gamestate.ActDeath || h.Target != -1 {
		t.Errorf("after the kill: cell held %v, ant action %d, hero target %d", s.Occ[antCell], ant.Act, h.Target)
	}
	if s.Party[0].FreeExp != volk.FreeExp+1 || s.Party[0].Fencing != volk.Fencing+4 {
		t.Errorf("Volk free exp %d → %d, Фехтование %d → %d", volk.FreeExp, s.Party[0].FreeExp, volk.Fencing, s.Party[0].Fencing)
	}
	if s.Hero.Dir != world.Facing(heroCell, antCell) {
		t.Errorf("Volk faces %d, want %d", s.Hero.Dir, world.Facing(heroCell, antCell))
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
	ant := &npc{Walker: walker{Entity: 921, Cell: antCell}}
	occ := map[mapdata.Cell]bool{antCell: true}
	s := &startScene{World: &world.World{Game: g, Party: []gamestate.Entity{g.Entities[0]}, NPCs: []*npc{ant}, ByEntity: map[int]*npc{921: ant}, Occ: occ, PileAt: map[mapdata.Cell]int{}}}
	m := setHero(s, walker{Entity: 0, Cell: mapdata.Cell{Row: 92, Col: 36}})
	hero := &m.Walker
	hero.Blocked = func(c mapdata.Cell) bool { return occ[c] }
	s.StartFight(m, 921)
	for i := 0; i < 5000 && hero.Act != gamestate.ActAttack; i++ {
		hero.Update()
		if hero.Cell == antCell {
			t.Fatal("hero walked into the target's cell")
		}
		s.MemberTick(m)
	}
	if hero.Act != gamestate.ActAttack || !world.Adjacent(hero.Cell, antCell) {
		t.Fatalf("hero at %v (action %d), want swinging next to %v", hero.Cell, hero.Act, antCell)
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
	ant := &npc{Walker: walker{Entity: 921, Cell: antCell, Blocked: free},
		Mode: npcCreature, Group: grp, Target: -1}
	s := &startScene{World: &world.World{Game: g, Tables: tb, Party: []gamestate.Entity{volk}, NPCs: []*npc{ant}, ByEntity: map[int]*npc{921: ant}, Occ: occ, Rng: rand.New(rand.NewPCG(3, 4)), PileAt: map[mapdata.Cell]int{}}}
	h := setHero(s, walker{Entity: volk.Index, Cell: heroCell, Blocked: free})
	s.SetStopAt(ant)
	attacked, struck := false, false
	for i := 0; i < 2000 && !g.Entities[921].Dead() && !s.Party[0].Dead(); i++ {
		s.Tick = i
		s.CombatTick()
		attacked = attacked || (ant.Act == gamestate.ActAttack && ant.Target == volk.Index)
		struck = struck || h.Target == 921
	}
	if g.Groups[grp].Alarm != gamestate.AlarmAttack {
		t.Errorf("group %d alarm %#x, want attack", grp, g.Groups[grp].Alarm)
	}
	if !attacked || !struck {
		t.Fatalf("ant attacked %v, hero struck back %v", attacked, struck)
	}
	if !g.Entities[921].Dead() && !s.Party[0].Dead() {
		t.Fatalf("no one died: ant HP %d, hero HP %d", g.Entities[921].HP, s.Party[0].HP)
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
	ant := &npc{Walker: walker{Entity: 921, Cell: start}, Mode: npcCreature, Group: grp, Target: -1,
		Rng: rand.New(rand.NewPCG(5, 6)).IntN}
	ant.Blocked = func(c mapdata.Cell) bool { return occ[c] }
	ant.Reserve = func(c mapdata.Cell) bool {
		if occ[c] {
			return false
		}
		occ[c] = true
		return true
	}
	ant.Release = func(c mapdata.Cell) { delete(occ, c) }
	exe, err := os.ReadFile("../../assets/Konung/konung.exe")
	if err != nil {
		t.Skip("konung.exe not available:", err)
	}
	tb, err := resfmt.NewExeTables(exe)
	if err != nil {
		t.Fatal(err)
	}
	s := &startScene{World: &world.World{Game: g, Tables: tb, NPCs: []*npc{ant}, ByEntity: map[int]*npc{921: ant}, Occ: occ, PileAt: map[mapdata.Cell]int{}}}
	s.Walk = &mapdata.WalkGrid{}
	s.WorldW, s.WorldH = 1<<20, 1<<20
	for i := 0; i < 20000 && !area.Contains(ant.Cell.Row, ant.Cell.Col); i++ {
		s.Tick = i
		ant.Update()
		s.CombatTick()
	}
	if g.Groups[grp].Alarm != gamestate.AlarmReturn || !area.Contains(ant.Cell.Row, ant.Cell.Col) {
		t.Fatalf("alarm %#x, ant at %v; want return and inside %+v", g.Groups[grp].Alarm, ant.Cell, area)
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
	cells := map[int]mapdata.Cell{921: near[0].Cell, 922: near[4].Cell, 923: {Row: 100, Col: 40 + world.AttackRangeCols*2}}
	occ := map[mapdata.Cell]bool{heroCell: true}
	s := &startScene{World: &world.World{Game: g, Tables: tb, ByEntity: map[int]*npc{}, Occ: occ, Rng: rand.New(rand.NewPCG(7, 8)), PileAt: map[mapdata.Cell]int{}}}
	for i, c := range cells {
		n := &npc{Walker: walker{Entity: i, Cell: c}, Target: -1} // calm: they don't move
		s.NPCs = append(s.NPCs, n)
		s.ByEntity[i] = n
		occ[c] = true
	}
	volk := g.Entities[g.Groups[0].First]
	volk.Flags3 |= 0x04
	s.Party = []gamestate.Entity{volk}
	h := setHero(s, walker{Entity: volk.Index, Cell: heroCell, Blocked: func(c mapdata.Cell) bool { return occ[c] }})
	h.Intent |= gamestate.IntentMelee
	for i := 0; i < 5000 && h.Melee(); i++ {
		s.MemberTick(h)
		for _, n := range s.NPCs {
			s.ActTick(&n.Walker, &g.Entities[n.Entity], n)
		}
	}
	if h.Melee() || !g.Entities[921].Dead() || !g.Entities[922].Dead() || g.Entities[923].Dead() {
		t.Fatalf("intent %v; dead: 921 %v, 922 %v, 923 (out of range) %v", h.Melee(),
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
	heroCell, antCell := mapdata.Cell{Row: 100, Col: 40}, mapdata.Cell{Row: 100, Col: 46}
	occ := map[mapdata.Cell]bool{heroCell: true, antCell: true}
	ant := &npc{Walker: walker{Entity: 921, Cell: antCell, Frames: fixedFrames(12)}, Target: -1}
	volk := g.Entities[g.Groups[0].First]
	volk.Flags3 |= 0x04
	volk.Equipment[gamestate.WeaponBow], volk.Quiver, volk.WeaponMode = bow, arrows, gamestate.WeaponBow
	s := &startScene{World: &world.World{Game: g, Tables: tb, Party: []gamestate.Entity{volk}, NPCs: []*npc{ant}, ByEntity: map[int]*npc{921: ant}, Occ: occ, Walk: &mapdata.WalkGrid{}, Rng: rand.New(rand.NewPCG(9, 9)), PileAt: map[mapdata.Cell]int{}}}
	h := setHero(s, walker{Entity: volk.Index, Cell: heroCell, Frames: fixedFrames(12),
		Blocked: func(c mapdata.Cell) bool { return occ[c] }})
	h.Target, h.Intent = 921, gamestate.IntentShoot
	shots := 0
	for i := 0; i < 3000 && !g.Entities[921].Dead() && h.Shoot(); i++ {
		before := len(s.Arrows)
		s.FlyArrows()
		s.MemberTick(h)
		if len(s.Arrows) > before {
			shots++
		}
		s.ActTick(&ant.Walker, &g.Entities[921], ant)
	}
	if shots == 0 {
		t.Fatal("no arrow was shot")
	}
	if !g.Entities[921].Dead() && s.Party[0].Quiver != 0 {
		t.Fatalf("after %d shots the ant has HP %d (shoot intent %v)", shots, g.Entities[921].HP, h.Shoot())
	}
	if s.Party[0].Marksmanship <= volk.Marksmanship && g.Entities[921].Dead() {
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
	n := &npc{Walker: walker{Entity: man, Cell: cell}, Mode: npcVillager, Group: st.Group, Target: -1}
	volk := g.Entities[g.Groups[0].First]
	s := &startScene{World: &world.World{Game: g, Party: []gamestate.Entity{volk}, NPCs: []*npc{n}, ByEntity: map[int]*npc{man: n}, PileAt: map[mapdata.Cell]int{}, AIBudget: 3}}
	setHero(s, walker{Entity: volk.Index, Cell: heroCell})
	s.VillagerTick(n)
	if e := g.Entities[man]; !e.InStance() || e.Type >= 6 || lookOf(e).Typ != 0 {
		// The stance moves the action type to the set 0–5; with type 6 the
		// swing would ask for the missing type 11.
		t.Fatalf("after the stance: in stance %v, type %d", e.InStance(), e.Type)
	}
	s.VillagerTick(n)
	if n.Target != volk.Index || n.Act != gamestate.ActAttack {
		t.Fatalf("villager target %d, action %d; want the hero, swing", n.Target, n.Act)
	}
	if !s.Fighting(n) {
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
	walking := &npc{Walker: walker{Entity: 360, From: mapdata.Cell{Row: 86, Col: 40}, Cell: mapdata.Cell{Row: 88, Col: 40}, Walking: true,
		Path: append([]mapdata.Cell(nil), long...)}, Mode: npcVillager, Group: st.Group, Target: -1}
	idle := &npc{Walker: walker{Entity: 361, Cell: mapdata.Cell{Row: 70, Col: 40}, Path: long[2:]},
		Mode: npcVillager, Group: st.Group, Target: -1}
	s := &startScene{World: &world.World{Game: g, NPCs: []*npc{walking, idle}}}
	s.Provoke(st.Group)
	if st.Flags&0x15 != 0x15 || g.Groups[st.Group].Alarm != gamestate.AlarmAttack {
		t.Fatalf("village flags %#x alarm %#x", st.Flags, g.Groups[st.Group].Alarm)
	}
	if !walking.Walking || len(walking.Path) != 0 || len(idle.Path) != 0 {
		t.Errorf("paths after the alarm: walking %v, not yet stepping %v", walking.Path, idle.Path)
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
	cell := mapdata.Cell{Row: 100, Col: 40}
	n := &npc{Walker: walker{Entity: hunter, Cell: cell, Frames: fixedFrames(12), Dir: 4}, Mode: npcVillager, Group: st.Group, Target: -1}
	volk := g.Entities[g.Groups[0].First]
	volk.HP = 60000
	far := mapdata.Cell{Row: 100, Col: 45} // 5 cells east, inside bow range
	s := &startScene{World: &world.World{Game: g, Tables: tb, Party: []gamestate.Entity{volk}, NPCs: []*npc{n}, ByEntity: map[int]*npc{hunter: n}, PileAt: map[mapdata.Cell]int{}, Walk: &mapdata.WalkGrid{}, Rng: rand.New(rand.NewPCG(4, 4)), Occ: map[mapdata.Cell]bool{}}}
	setHero(s, walker{Entity: volk.Index, Cell: far, Frames: fixedFrames(12)})
	s.Provoke(st.Group)
	shoot := func() bool {
		for range 40 {
			s.AIBudget = 3
			s.VillagerTick(n)
			if n.Act == gamestate.ActShoot {
				return true
			}
		}
		return false
	}
	if !shoot() {
		t.Fatalf("the hunter did not shoot from 5 cells (mode %d, quiver %d)", g.Entities[hunter].WeaponMode, g.Entities[hunter].Quiver)
	}
	for n.Act == gamestate.ActShoot {
		s.VillagerTick(n)
	}
	s.Hero.Cell = mapdata.Neighbors(cell)[4].Cell // right next to him: too close to shoot
	s.AIBudget = 3
	s.VillagerTick(n)
	if n.Act != gamestate.ActAttack {
		t.Fatalf("next to the hero the hunter's action is %d, want the swing", n.Act)
	}
	s.Hero.Cell = far // the hero steps back: the swing stops, the bow comes out again
	s.VillagerTick(n)
	if !shoot() {
		t.Fatalf("after melee the hunter never shot again (mode %d)", g.Entities[hunter].WeaponMode)
	}
}

// setHero makes w the walker of s.Party[0]: the hero as the party's only
// member, selected.
func setHero(s *startScene, w walker) *member {
	m := &member{Walker: w, Target: -1}
	s.Members = []*member{m}
	s.Hero = &m.Walker
	s.Sel = []int{s.Party[0].Index}
	return m
}

// TestKind53Stands: a kind-0x53 character (here the wizard of «Волхв у
// Родников», group 33) does nothing in the creature AI even when its group
// is alarmed (0x00412E4F).
func TestKind53Stands(t *testing.T) {
	s := newVolkScene(t)
	var wiz gamestate.Entity
	for _, e := range s.Game.Entities {
		if e.Flags4 == 0x53 && e.Group >= 0 && s.Game.Groups[e.Group].Map == 3 {
			wiz = e
			break
		}
	}
	s.changeMap(gamestate.MapExit{To: 3, Row: wiz.Row + 4, Col: wiz.Col, Facing: 2})
	n := s.ByEntity[wiz.Index]
	if n == nil || s.MapNumber != 3 {
		t.Fatal("no wizard on map 3")
	}
	godMode = true
	defer func() { godMode = false }()
	start := n.Cell
	for range 300 {
		s.Game.Groups[wiz.Group].Alarm = gamestate.AlarmAttack
		run(s, 1)
		if n.Cell != start || n.Act == gamestate.ActAttack || len(n.Path) > 0 {
			t.Fatalf("the wizard acts: at %v act %d path %d", n.Cell, n.Act, len(n.Path))
		}
	}
}

// fixedFrames counts n frames for every animation.
func fixedFrames(n int) world.FrameCounter {
	return func(world.Look, int, int) int { return n }
}
