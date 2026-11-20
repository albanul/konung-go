package lanserver

import (
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

// plainVillager is a living villager of map m's village that holds no post.
func plainVillager(t *testing.T, w *World, m int) int {
	t.Helper()
	st := w.Game.SettlementOnMap(m)
	posted := map[int]bool{}
	for _, p := range st.Posts {
		posted[p.Entity] = true
	}
	for _, n := range w.Maps[m].NPCs {
		e := w.Game.Entities[n.Entity]
		if n.Mode == world.NPCVillager && n.Group == st.Group && !posted[n.Entity] && e.Model < 3 && !e.Dead() {
			return n.Entity
		}
	}
	t.Fatalf("no plain villager on map %d", m)
	return -1
}

// A player that hurts a villager becomes the village's aggressor; the pass
// alarms the village against that player only, and its villagers go for
// it, never for the other player standing by; the aggressor's leaving
// clears it (0x0044B0A7).
func TestVillageFightsItsAggressor(t *testing.T) {
	w := load(t)
	const ivar, olaf, m = 0, 1, 4 // both Драгомир's type, in his village
	for _, slot := range []int{ivar, olaf} {
		if _, _, err := w.AddPlayer(slot, 1, nil); err != nil {
			t.Fatal(err)
		}
	}
	st := w.Game.SettlementOnMap(m)
	gr := &w.Game.Groups[st.Group]
	v := plainVillager(t, w, m)
	nextTo(t, w, ivar, v) // Olaf stays at the village's teleport
	n := w.Maps[m].ByEntity[v]
	w.Stance(ivar, nil, true)
	w.Right(ivar, nil, v, n.Cell.Row, n.Cell.Col)
	for k := 0; k < 300 && !st.Aggressor(ivar); k++ {
		w.Step()
	}
	if !st.Aggressor(ivar) || st.Aggressor(olaf) {
		t.Fatalf("aggressors %#x, want Ivar's slot only", st.Aggressors)
	}
	for range gamestate.UpkeepTicks + 1 {
		w.Step()
	}
	if gr.Alarm != gamestate.AlarmAttack || gr.Enemy != ivar {
		t.Fatalf("alarm %d enemy %d, want an attack on slot %d", gr.Alarm, gr.Enemy, ivar)
	}
	forIvar := false
	for range 400 {
		w.Step()
		for _, n := range w.Maps[m].NPCs {
			if n.Group == st.Group && n.Target == olaf {
				t.Fatalf("villager %d went for Olaf, who did nothing", n.Entity)
			}
			forIvar = forIvar || n.Group == st.Group && n.Target == ivar
		}
	}
	if !forIvar {
		t.Fatal("no villager went for Ivar")
	}
	w.RemovePlayer(ivar)
	if st.Aggressors != 0 {
		t.Fatalf("aggressors %#x after Ivar left", st.Aggressors)
	}
	for range gamestate.UpkeepTicks + 1 {
		w.Step()
	}
	if gr.Alarm != gamestate.AlarmCalm {
		t.Fatal("the village stays alarmed after its aggressor left")
	}
}

// An attack order in the stance on a player of the village's nation makes
// the attacker an aggressor there (0x004466FA); on a player of another
// nation, or abroad, it does not.
func TestAllyAttack(t *testing.T) {
	w := load(t)
	const ivar, olaf, volk = 0, 1, 2
	for slot, model := range map[int]int{ivar: 1, olaf: 1, volk: 0} {
		if _, _, err := w.AddPlayer(slot, model, nil); err != nil {
			t.Fatal(err)
		}
	}
	home := w.Game.SettlementOnMap(4)
	// Out of the stance: nothing.
	if !w.MoveTo(ivar, 4, heroCell(w, olaf)) {
		t.Fatal("MoveTo")
	}
	w.Right(ivar, nil, olaf, 0, 0)
	if home.Aggressor(ivar) {
		t.Fatal("an aggressor without the stance")
	}
	// Volk (another nation) in Драгомир's village: attacking him is no
	// attack on the village's own.
	if !w.MoveTo(volk, 4, heroCell(w, olaf)) {
		t.Fatal("MoveTo")
	}
	w.Stance(ivar, nil, true)
	w.Right(ivar, nil, volk, 0, 0)
	if home.Aggressor(ivar) {
		t.Fatal("attacking a stranger made an aggressor")
	}
	w.Right(ivar, nil, olaf, 0, 0)
	if !home.Aggressor(ivar) || home.Aggressor(olaf) {
		t.Fatalf("aggressors %#x, want Ivar's slot", home.Aggressors)
	}
	// Abroad in Волк's village (map 1): Olaf is not of its nation.
	x, _ := w.Start(0)
	abroadCell := mapdata.Cell{Row: x.Row, Col: x.Col}
	for _, slot := range []int{ivar, olaf} {
		if !w.MoveTo(slot, x.To, abroadCell) {
			t.Fatal("MoveTo")
		}
	}
	w.Right(ivar, nil, olaf, 0, 0)
	if w.Game.SettlementOnMap(x.To).Aggressor(ivar) {
		t.Fatal("attacking a stranger abroad made an aggressor")
	}
}

func heroCell(w *World, slot int) mapdata.Cell {
	mem, _ := w.Maps[w.players[slot]].Member(slot)
	return mem.Cell
}

// No poison on weapons over the network (0x00440CC5): the herb held over
// the boxed knife goes back unused.
func TestNoPoisonOnItems(t *testing.T) {
	w := load(t)
	const slot = 0
	if _, _, err := w.AddPlayer(slot, 1, nil); err != nil {
		t.Fatal(err)
	}
	_, e := w.Maps[w.players[slot]].Member(slot)
	knife := e.Equipment[gamestate.WeaponBlade]
	if knife == 0 {
		t.Fatal("no knife")
	}
	poison := w.Game.SetItemKind(0, gamestate.CatHerb, gamestate.HerbPoison)
	e.Bag[0] = poison
	item := func(act string, index int) {
		w.Item(slot, netplay.OrderPayload{Kind: netplay.OrderItem, Item: act, Index: index})
	}
	item(netplay.ItemEquip, 9) // the knife into the hand
	item(netplay.ItemBox, 0)   // into the box
	item(netplay.ItemBag, 0)   // the poison into the hand
	item(netplay.ItemBox, 0)   // onto the knife
	if h := w.state(slot).hand; h.Box != knife || h.Holding() {
		t.Fatalf("hand %+v, want the knife boxed and the hand empty", h)
	}
	// Single player would leave an empty jar (HerbJar) in the bag.
	if w.Game.ItemType(poison) != gamestate.HerbPoison || !contains(e.Bag[:], poison) {
		t.Fatalf("poison type %d, bag %v: used on the knife", w.Game.ItemType(poison), e.Bag[:4])
	}
}

func contains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// Building states reach the players on the building's map as they change
// and a player arriving later; not the players elsewhere.
func TestBuildingStatesToPlayers(t *testing.T) {
	w := load(t)
	ss := startSession(t, w)
	a := enter(t, ss, "Ivar", 1) // map 4
	b := enter(t, ss, "Michail", 2)
	ss.host.mu.Lock()
	var p int
	for q := range w.Game.BuildingStates(4) {
		p = q
		break
	}
	w.Game.SetBuildingState(4, p, 5)
	w.buildingsVersion++
	ss.host.mu.Unlock()
	got := func(v *viewer, state int) func() bool {
		return func() bool {
			v.mu.Lock()
			defer v.mu.Unlock()
			for _, bs := range v.buildings {
				if bs.Placement == p && bs.State == state {
					return true
				}
			}
			return false
		}
	}
	waitUntil(t, "the burning building for Ivar", got(a, 5))
	b.mu.Lock()
	if len(b.buildings) != 0 {
		t.Errorf("Michail on map 7 got %v", b.buildings)
	}
	b.mu.Unlock()
	ss.host.mu.Lock()
	if !w.MoveTo(b.at.Slot, 4, heroCell(w, a.at.Slot)) {
		t.Fatal("MoveTo")
	}
	ss.host.mu.Unlock()
	waitUntil(t, "the burning building for Michail on arrival", got(b, 5))
	ss.host.mu.Lock()
	w.Game.SetBuildingState(4, p, gamestate.BuildingStanding)
	w.buildingsVersion++
	ss.host.mu.Unlock()
	waitUntil(t, "the standing building for Ivar", got(a, gamestate.BuildingStanding))
}

// The village's alarm rectangle is its area (villageBounds, a port rule),
// not GAME.RES's whole map: its aggressor outside the area leaves it calm
// and ends the villagers' chase; inside, the village attacks.
func TestVillageAlarmArea(t *testing.T) {
	w := load(t)
	const ivar, m = 0, 4 // Драгомир's type, in his village
	if _, _, err := w.AddPlayer(ivar, 1, nil); err != nil {
		t.Fatal(err)
	}
	st := w.Game.SettlementOnMap(m)
	gr := &w.Game.Groups[st.Group]
	area, ok := w.VillageArea(m)
	if got, _ := gr.WanderArea(); !ok || got != area || area.RowMax-area.RowMin >= 100 {
		t.Fatalf("group rectangle %+v, village area %+v", got, area)
	}
	st.MarkAggressor(ivar)
	pass := func() {
		for range gamestate.UpkeepTicks + 1 {
			w.Step()
		}
	}
	inside := mapdata.Cell{Row: (area.RowMin + area.RowMax) / 2, Col: (area.ColMin + area.ColMax) / 2}
	out := mapdata.Cell{Row: area.RowMax + 15, Col: inside.Col}
	if out.Row > 120 {
		out.Row = area.RowMin - 15
	}
	if !w.MoveTo(ivar, m, out) {
		t.Fatal("MoveTo")
	}
	if c := heroCell(w, ivar); area.Contains(c.Row, c.Col) {
		t.Fatalf("Ivar at %v, inside the area %+v", c, area)
	}
	pass()
	if gr.Alarm != gamestate.AlarmCalm {
		t.Fatalf("alarm %d with the aggressor outside the village", gr.Alarm)
	}
	if !w.MoveTo(ivar, m, inside) {
		t.Fatal("MoveTo")
	}
	pass()
	if gr.Alarm != gamestate.AlarmAttack || gr.Enemy != ivar {
		t.Fatalf("aggressor inside: alarm %d enemy %d", gr.Alarm, gr.Enemy)
	}
	chased := false
	for k := 0; k < 200 && !chased; k++ {
		w.Step()
		for _, n := range w.Maps[m].NPCs {
			chased = chased || n.Group == st.Group && n.Target == ivar && n.Walking
		}
	}
	if !chased {
		t.Fatal("no villager went for Ivar")
	}
	if !w.MoveTo(ivar, m, out) {
		t.Fatal("MoveTo")
	}
	pass()
	if gr.Alarm != gamestate.AlarmCalm {
		t.Fatalf("alarm %d after the aggressor left the area", gr.Alarm)
	}
	for _, n := range w.Maps[m].NPCs {
		if n.Group == st.Group && n.Target == ivar {
			t.Fatalf("villager %d still goes for Ivar outside the village", n.Entity)
		}
	}
	if !st.Aggressor(ivar) {
		t.Fatal("leaving the area forgave Ivar")
	}
}

// The A key goes for an alarmed village's villagers only when the player
// is one of its aggressors (a port rule, as world.Foe): Olaf angered the
// village, Ivar standing among its villagers attacks none of them; once
// Ivar is an aggressor too, he does.
func TestAttackKeyLeavesOthersVillage(t *testing.T) {
	w := load(t)
	const ivar, olaf, m = 0, 1, 4 // Драгомир's type, in his village
	for _, slot := range []int{ivar, olaf} {
		if _, _, err := w.AddPlayer(slot, 1, nil); err != nil {
			t.Fatal(err)
		}
	}
	st := w.Game.SettlementOnMap(m)
	gr := &w.Game.Groups[st.Group]
	v := plainVillager(t, w, m)
	nextTo(t, w, ivar, v)
	nextTo(t, w, olaf, v)
	st.MarkAggressor(olaf)
	for range gamestate.UpkeepTicks + 1 {
		w.Step()
		keepAlive(w, m, olaf)
	}
	if gr.Alarm != gamestate.AlarmAttack || gr.Enemy != olaf {
		t.Fatalf("alarm %d enemy %d, want an attack on Olaf", gr.Alarm, gr.Enemy)
	}
	villager := func(i int) bool { return i >= 0 && w.Game.Entities[i].Group == st.Group }
	w.Attack(ivar)
	for range 60 {
		w.Step()
		keepAlive(w, m, olaf)
		if mem, _ := w.Maps[m].Member(ivar); mem != nil && villager(mem.Target) {
			t.Fatalf("Ivar went for villager %d of a village angry at Olaf only", mem.Target)
		}
	}
	st.MarkAggressor(ivar)
	w.Attack(ivar)
	for k := 0; ; k++ {
		w.Step()
		keepAlive(w, m, olaf)
		keepAlive(w, m, ivar)
		if mem, _ := w.Maps[m].Member(ivar); mem != nil && villager(mem.Target) {
			break
		}
		if k == 60 {
			t.Fatal("Ivar, an aggressor, attacked no villager")
		}
	}
}

// keepAlive keeps slot's hero on map m from dying (the tests are about who
// fights whom, not the fight).
func keepAlive(w *World, m, slot int) {
	if mem, e := w.Maps[m].Member(slot); mem != nil {
		e.HP = gamestate.HPFull
	}
}
