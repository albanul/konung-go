package lanserver

import (
	"bytes"
	"os"
	"strconv"
	"sync"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
)

// loadAmulets loads the world with the shipped SERVER file's amulet places
// and win limit limit.
func loadAmulets(t *testing.T, limit int) (*World, AmuletRules) {
	t.Helper()
	const dir = "../../assets/Konung/"
	read := func(name string) []byte {
		b, err := os.ReadFile(dir + name)
		if err != nil {
			t.Skip("assets missing:", err)
		}
		return b
	}
	sf, err := gamestate.ParseServerFile(read("SERVER"))
	if err != nil {
		t.Fatal(err)
	}
	rules := AmuletRules{Points: gamestate.AmuletPointsFrom(sf), Limit: limit}
	f := Files{GameRes: read("GAME.RES"), MapsRes: read("MAPS.RES"), Graph: read("GRAPH.RES"), Objects: read("OBJECTS.RES"),
		Exe: read("konung.exe"), Quests: read("questr.RES"), Amulets: &rules}
	for m := range f.Games {
		f.Games[m] = read("GAME." + strconv.Itoa(m))
	}
	w, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}
	return w, rules
}

// partAt reports whether the pile on place k holds an amulet item of kind
// k+1, and returns it.
func partAt(w *World, r AmuletRules, k int) int {
	p := r.Points[k]
	at, ok := w.Maps[p[0]].PileAt[mapdata.Cell{Row: p[1], Col: p[2]}]
	if !ok {
		return 0
	}
	for _, i := range w.Game.Piles[at].Items {
		if w.amuletKind(i) == k+1 {
			return i
		}
	}
	return 0
}

// fetch takes part k from its place into slot's hero's bag (the pick-up
// itself is the single-player rule, tested elsewhere; walking there, the
// shrines' guards fight the hero).
func fetch(t *testing.T, w *World, r AmuletRules, slot, k int) int {
	t.Helper()
	item := partAt(w, r, k)
	if item == 0 {
		t.Fatalf("no part %d at its place %v", k, r.Points[k])
	}
	p := r.Points[k]
	pl := &w.Game.Piles[w.Maps[p[0]].PileAt[mapdata.Cell{Row: p[1], Col: p[2]}]]
	for i, v := range pl.Items {
		if v == item {
			pl.Items[i] = 0
		}
	}
	e := &w.Game.Entities[slot]
	if _, _, me := w.member(slot); me != nil {
		e = me
	}
	gamestate.BagInsert(e, item)
	stepAlive(w, slot)
	return item
}

// stepAlive steps the world keeping slot's hero alive: the shrines' guards
// (a flower monster and skeletons) would kill it.
func stepAlive(w *World, slot int) {
	if _, _, e := w.member(slot); e != nil {
		e.HP = gamestate.HPFull
	}
	w.Step()
}

func bagIndex(w *World, slot, item int) int {
	e, _ := w.Hero(slot)
	for k, i := range e.Bag {
		if i == item && item != 0 {
			return k
		}
	}
	return -1
}

func worldItem(w *World, slot int, kind string, index int) {
	w.Item(slot, netplay.OrderPayload{Kind: netplay.OrderItem, Item: kind, Index: index})
}

// join puts target into the in-hand box and joins part onto it (the
// window's clicks: lift, box, lift, box).
func join(t *testing.T, w *World, slot, target, part int) {
	t.Helper()
	worldItem(w, slot, netplay.ItemBag, bagIndex(w, slot, target))
	worldItem(w, slot, netplay.ItemBox, 0)
	worldItem(w, slot, netplay.ItemBag, bagIndex(w, slot, part))
	worldItem(w, slot, netplay.ItemBox, 0)
	worldItem(w, slot, netplay.ItemBoxClear, 0)
	if w.amuletKind(part) != 0 {
		t.Fatalf("part %d not used up", part)
	}
}

// The parts lie at the SERVER file's places; a hero fetches the three,
// joins them (no experience in the network game) and drops the amulet on
// its nation's spot: delivered (an EventAmulet, the nation's count), the
// parts are put out again; dropped elsewhere nothing happens; the second
// delivery reaches the limit: the nation wins (TakeWin, the status line),
// and no parts are put out until the session's end.
func TestAmuletDelivery(t *testing.T) {
	w, r := loadAmulets(t, 2)
	for k := range 3 {
		if partAt(w, r, k) == 0 {
			t.Fatalf("part %d not at its place %v", k, r.Points[k])
		}
	}
	if p := w.Amulets(); p.Parts[0].Kind != 1 || p.Parts[1].Kind != 2 || p.Parts[2].Kind != 3 || p.Parts[0].Holder != -1 || p.Limit != 2 {
		t.Fatalf("amulets %+v", p)
	}
	const slot = 0 // Михаил: nation 2, the византийцы (the shipped spots of nations 0 and 1 have a guard standing on them)
	if _, _, err := w.AddPlayer(slot, 2, nil); err != nil {
		t.Fatal(err)
	}
	deliverOnce := func() {
		t.Helper()
		right, left, bottom := fetch(t, w, r, slot, 0), fetch(t, w, r, slot, 1), fetch(t, w, r, slot, 2)
		w.Step()
		if p := w.Amulets(); p.Parts[0].Holder != slot || !w.carriesAmulet(slot) {
			t.Fatalf("carrying the parts: %+v", p)
		}
		hero, _ := w.Hero(slot)
		exp := hero.FreeExp + hero.Experience
		join(t, w, slot, right, left)
		if w.amuletKind(right) != 5 {
			t.Fatalf("right+left = kind %d", w.amuletKind(right))
		}
		w.Step()
		if p := w.Amulets(); p.Parts[0].Kind != 5 || p.Parts[1].Kind != 3 || p.Parts[2].Kind != 0 {
			t.Fatalf("joined first: %+v", p)
		}
		join(t, w, slot, right, bottom)
		if w.amuletKind(right) != gamestate.AmuletWhole {
			t.Fatalf("the amulet: kind %d", w.amuletKind(right))
		}
		if hero, _ := w.Hero(slot); hero.FreeExp+hero.Experience != exp {
			t.Error("joining gave experience")
		}
		// Dropped beside the spot: a pile, nothing delivered; picked up again.
		p := r.Points[5]
		w.MoveTo(slot, p[0], mapdata.Cell{Row: p[1] + 4, Col: p[2]})
		worldItem(w, slot, netplay.ItemBag, bagIndex(w, slot, right))
		worldItem(w, slot, netplay.ItemDrop, 0)
		if w.amuletKind(right) != gamestate.AmuletWhole || w.pileOf(right) < 0 {
			t.Fatal("dropped off the spot: not on the ground")
		}
		w.TakeEvents()
		_, mem, _ := w.member(slot)
		w.Right(slot, nil, -1, mem.Cell.Row, mem.Cell.Col)
		w.MoveTo(slot, p[0], mapdata.Cell{Row: p[1] + 6, Col: p[2]})
		w.Right(slot, nil, -1, p[1]+4, p[2])
		for range 200 {
			stepAlive(w, slot)
			if bagIndex(w, slot, right) >= 0 {
				break
			}
		}
		// On the spot: delivered.
		if !w.MoveTo(slot, p[0], mapdata.Cell{Row: p[1], Col: p[2]}) {
			t.Fatal("MoveTo the spot")
		}
		if _, mem, _ := w.member(slot); mem.Cell != (mapdata.Cell{Row: p[1], Col: p[2]}) {
			t.Fatalf("hero at %v, not on the spot %v", mem.Cell, p)
		}
		worldItem(w, slot, netplay.ItemBag, bagIndex(w, slot, right))
		worldItem(w, slot, netplay.ItemDrop, 0)
		if w.amuletKind(right) == gamestate.AmuletWhole {
			t.Fatal("the delivered amulet is still an item") // (its record may hold a new part)
		}
	}
	deliverOnce()
	if w.Nations() != [3]int{0, 0, 1} {
		t.Fatalf("nations %v", w.Nations())
	}
	got := false
	for _, ev := range w.TakeEvents() {
		got = got || ev.Kind == EventAmulet && ev.Entity == slot && ev.Victim == 2
	}
	if !got {
		t.Error("no EventAmulet")
	}
	for k := range 3 {
		if partAt(w, r, k) == 0 {
			t.Errorf("part %d not put out again", k)
		}
	}
	if _, won := w.TakeWin(); won {
		t.Fatal("won at 1 of 2")
	}
	deliverOnce()
	nation, won := w.TakeWin()
	if !won || nation != 2 {
		t.Fatalf("second delivery: won %v nation %d", won, nation)
	}
	if partAt(w, r, 0) != 0 {
		t.Error("parts put out after the win")
	}
	msgs := w.TakeMessages(slot)
	if len(msgs) == 0 || !bytes.Equal(msgs[len(msgs)-1], cp866(gamestate.NationWins[2])) {
		t.Errorf("messages %q, want the win", msgs)
	}
	if err := w.Reset(); err != nil {
		t.Fatal(err)
	}
	if w.Nations() != [3]int{} || partAt(w, r, 0) == 0 {
		t.Errorf("after the reset: nations %v, part 0 at its place %v", w.Nations(), partAt(w, r, 0) != 0)
	}
}

// A part carried out of the world vanishes and is put out again; one
// dropped on the ground vanishes after AmuletGroundPasses passes (an empty
// jar is left) and is put out again; one at its place stays.
func TestAmuletVanishes(t *testing.T) {
	w, r := loadAmulets(t, 0)
	const slot = 0
	if _, _, err := w.AddPlayer(slot, 1, nil); err != nil {
		t.Fatal(err)
	}
	part := fetch(t, w, r, slot, 1)
	if partAt(w, r, 1) != 0 {
		t.Fatal("the part is still at its place")
	}
	w.RemovePlayer(slot)
	if e, _ := w.Hero(slot); bagIndex(w, slot, part) >= 0 || e.Bag[0] != 0 {
		t.Error("the carried part is still in the bag")
	}
	if partAt(w, r, 1) == 0 {
		t.Fatal("the part not put out again after the logout")
	}

	if _, _, err := w.AddPlayer(slot, 1, nil); err != nil {
		t.Fatal(err)
	}
	part = fetch(t, w, r, slot, 2)
	p := r.Points[2]
	w.MoveTo(slot, p[0], mapdata.Cell{Row: p[1] + 6, Col: p[2]})
	worldItem(w, slot, netplay.ItemBag, bagIndex(w, slot, part))
	worldItem(w, slot, netplay.ItemDrop, 0)
	w.MoveTo(slot, p[0], mapdata.Cell{Row: p[1] + 12, Col: p[2]})
	home := partAt(w, r, 0)
	for range (gamestate.AmuletGroundPasses - 2) * gamestate.UpkeepTicks {
		w.Step()
	}
	if w.amuletKind(part) == 0 {
		t.Fatal("the dropped part vanished too early")
	}
	for range 4 * gamestate.UpkeepTicks {
		w.Step()
	}
	if c, typ := w.Game.ItemKind(part); c != gamestate.CatHerb || typ != gamestate.HerbEmptyJar {
		t.Errorf("the dropped part is %d/%d, want an empty jar", c, typ)
	}
	if partAt(w, r, 2) == 0 {
		t.Error("the part not put out again after lying on the ground")
	}
	if partAt(w, r, 0) != home || home == 0 {
		t.Error("the part at its place vanished")
	}
	if n := len(w.amulets.items); n != 3 {
		t.Errorf("%d amulet items in play, want 3", n)
	}
}

// Over the network: every player gets the amulet state (MsgAmulets) and
// sees who carries a part; a delivery calls OnAmulet with the player and
// its nation, and the limit reached calls OnWin; the players see the count.
func TestAmuletsOverNetwork(t *testing.T) {
	w, r := loadAmulets(t, 1)
	ss := startSession(t, w)
	var mu sync.Mutex
	var delivered []string
	won := -1
	ss.host.OnAmulet = func(p netplay.PlayerInfo, nation int) {
		mu.Lock()
		delivered = append(delivered, p.Name+":"+strconv.Itoa(nation))
		mu.Unlock()
	}
	ss.host.OnWin = func(nation int) {
		mu.Lock()
		won = nation
		mu.Unlock()
	}
	a := enter(t, ss, "Ivar", 2)
	b := enter(t, ss, "Olaf", 1)
	waitUntil(t, "Olaf gets the amulets", func() bool {
		p, ok := b.amuletState()
		return ok && p.Parts[0].Kind == 1 && p.Parts[0].Holder == -1 && p.Limit == 1
	})
	slot := a.at.Slot
	ss.host.mu.Lock()
	right := fetch(t, w, r, slot, 0)
	ss.host.mu.Unlock()
	waitUntil(t, "Olaf sees Ivar carry the right part", func() bool {
		p, _ := b.amuletState()
		return p.Parts[0].Kind == 1 && p.Parts[0].Holder == slot
	})
	ss.host.mu.Lock()
	left, bottom := fetch(t, w, r, slot, 1), fetch(t, w, r, slot, 2)
	join(t, w, slot, right, left)
	join(t, w, slot, right, bottom)
	p := r.Points[5]
	w.MoveTo(slot, p[0], mapdata.Cell{Row: p[1], Col: p[2]})
	worldItem(w, slot, netplay.ItemBag, bagIndex(w, slot, right))
	worldItem(w, slot, netplay.ItemDrop, 0)
	ss.host.mu.Unlock()
	waitUntil(t, "OnAmulet and OnWin", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(delivered) == 1 && delivered[0] == "Ivar:2" && won == 2
	})
	waitUntil(t, "Olaf sees the count", func() bool {
		p, _ := b.amuletState()
		return p.Nations == [3]int{0, 0, 1}
	})
}
