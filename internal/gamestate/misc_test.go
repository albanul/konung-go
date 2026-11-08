package gamestate

import (
	"encoding/binary"
	"slices"
	"testing"
)

// codeAt returns the konung.exe bytes at BEGTEXT VA va (section at file
// offset 0x400, VA 0x00410000).
func codeAt(exe []byte, va int) []byte { return exe[va-0x410000+0x400:] }

// miscCase returns the jump-table target of misc type typ (0x00415ADC).
func miscCase(exe []byte, typ int) int {
	return int(binary.LittleEndian.Uint32(codeAt(exe, 0x415ADC+4*typ)))
}

// pushesBeforeCall finds the first call to target from va on and returns
// the immediates of the n pushes right before it, in argument order.
func pushesBeforeCall(t *testing.T, exe []byte, va, target, n int) []int {
	t.Helper()
	b := codeAt(exe, va)
	for k := 0; k+5 <= 0x200; k++ {
		if b[k] != 0xE8 || va+k+5+int(int32(binary.LittleEndian.Uint32(b[k+1:]))) != target {
			continue
		}
		var args []int
		for p := k; len(args) < n; {
			switch {
			case p >= 2 && b[p-2] == 0x6A:
				args = append(args, int(int8(b[p-1])))
				p -= 2
			case p >= 5 && b[p-5] == 0x68:
				args = append(args, int(int32(binary.LittleEndian.Uint32(b[p-4:]))))
				p -= 5
			default:
				t.Fatalf("no push before the call at %#x", va+k)
			}
		}
		return args // pushed last = first argument
	}
	t.Fatalf("no call to %#x after %#x", target, va)
	return nil
}

// cmpImm8 returns the immediates of the `cmp eax, imm8` (83 F8 xx)
// instructions in [va, end).
func cmpImm8(exe []byte, va, end int) []int {
	var out []int
	b := codeAt(exe, va)
	for k := 0; k+3 <= end-va; k++ {
		if b[k] == 0x83 && b[k+1] == 0xF8 {
			out = append(out, int(b[k+2]))
		}
	}
	return out
}

// TestMiscTablesMatchExe checks the port's case tables against konung.exe:
// the types without a use, the orb and key jumps and the amulet joins.
func TestMiscTablesMatchExe(t *testing.T) {
	exe := load(t, "konung.exe")
	const noUse = 0x415ACD
	var none []int
	for typ := range 54 {
		if miscCase(exe, typ) == noUse {
			none = append(none, typ)
		}
	}
	if want := []int{11, 29, 32, 34, 40, 45, 46, 47, 49, 50, 51}; !slices.Equal(none, want) {
		t.Errorf("types without a use %v, want %v", none, want)
	}
	if b := codeAt(exe, 0x415BBA); b[0] != 0x80 || b[3] != 53 {
		t.Errorf("type bound at 0x00415BBA: % x", b[:4])
	}
	for typ, x := range orbJumps {
		a := pushesBeforeCall(t, exe, miscCase(exe, typ), 0x4106E0, 4)
		if got := (MapExit{To: a[0], Row: a[1], Col: a[2], Facing: a[3]}); got != x {
			t.Errorf("orb %d jumps to %+v, want %+v", typ, x, got)
		}
	}
	for typ, d := range keyDoors {
		va := miscCase(exe, typ)
		a := pushesBeforeCall(t, exe, va, 0x4106E0, 4)
		if got := (MapExit{To: a[0], Row: a[1], Col: a[2], Facing: a[3]}); got != d.To {
			t.Errorf("key %d jumps to %+v, want %+v", typ, d.To, got)
		}
		if c := cmpImm8(exe, va, va+0x40); len(c) < 3 || c[0] != d.Map || c[1] != d.Row || c[2] != d.Col {
			t.Errorf("key %d door %d (%d,%d), exe compares %v", typ, d.Map, d.Row, d.Col, c)
		}
	}
	// 0x00410010: per join cmp a, cmp b, cmp b, cmp a, then mov [ebp-4], to.
	c := cmpImm8(exe, 0x410010, 0x4101AA)[1:] // the first compares the category (11)
	b := codeAt(exe, 0x410010)
	var tos []int
	for k := 0; k < 0x19A; k++ {
		if b[k] == 0xC7 && b[k+1] == 0x45 && b[k+2] == 0xFC && b[k+3] != 0 {
			tos = append(tos, int(b[k+3]))
		}
	}
	if len(c) != 4*len(amuletParts) || len(tos) != len(amuletParts) {
		t.Fatalf("amulet compares %v, results %v", c, tos)
	}
	for k, p := range amuletParts {
		if c[4*k] != p.a || c[4*k+1] != p.b || c[4*k+2] != p.b || c[4*k+3] != p.a || tos[k] != p.to {
			t.Errorf("join %d = %+v, exe %v → %d", k, p, c[4*k:4*k+4], tos[k])
		}
	}
}

// fakeWorld is a World over the GAME.N records.
type fakeWorld struct {
	g      *Game
	killed []int
	walks  [][3]int
	jumps  []MapExit
	enemy  int
}

func (w *fakeWorld) Hero() *Entity         { return &w.g.Entities[w.g.Groups[0].First] }
func (w *fakeWorld) Entity(i int) *Entity  { return &w.g.Entities[i] }
func (w *fakeWorld) Cell(i int) (int, int) { return w.g.Entities[i].Row, w.g.Entities[i].Col }
func (w *fakeWorld) Alive(i int) bool      { return !w.g.Entities[i].Dead() }
func (w *fakeWorld) Kill(i, _ int)         { w.killed = append(w.killed, i) }
func (w *fakeWorld) Walk(i, r, c int)      { w.walks = append(w.walks, [3]int{i, r, c}) }
func (w *fakeWorld) Blocked(int, int) bool { return false }
func (w *fakeWorld) EnemyGroup() int       { return w.enemy }
func (w *fakeWorld) Jump(x MapExit)        { w.jumps = append(w.jumps, x) }

// miscGame returns GAME.0 with an empty-bagged hero, a fake world and a
// function that makes a fresh misc item of a type.
func miscGame(t *testing.T) (*Game, *Entity, *fakeWorld, func(cat, typ int) int) {
	t.Helper()
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	w := &fakeWorld{g: g, enemy: -1}
	g.World = w
	hero := &g.Entities[g.Groups[0].First]
	hero.Bag = [32]int{}
	return g, hero, w, func(cat, typ int) int {
		i := g.newItem()
		g.setItem(i, cat, typ)
		return i
	}
}

func TestUseMiscBooks(t *testing.T) {
	g, hero, _, item := miscGame(t)
	hero.Skills = [5]int{450, 7, 95, 3, 60}
	for _, c := range []struct{ typ, msg int }{
		{MiscHippocrates, MsgHippocrates}, {MiscPyramids, MsgPyramids}, {MiscSmithScroll, MsgSmithScroll},
		{MiscTradeTract, MsgTradeTract}, {MiscThirdEye, MsgThirdEye},
	} {
		book := item(CatQuest, c.typ)
		hero.Bag[0] = book
		ok, msg := g.UseMisc(hero, nil, 0, book)
		if !ok || msg != c.msg || hero.Bag[0] != 0 {
			t.Errorf("book %d: ok %v msg %#x bag %v", c.typ, ok, msg, hero.Bag[:2])
		}
		if it, _ := g.Item(book); it.Category != 0xFF {
			t.Errorf("book %d not used up", c.typ)
		}
	}
	// healing +100 ≤ 500, smithing +4, trade +10 ≤ 100, building +5, curios +100 ≤ 100.
	if want := [5]int{500, 11, 100, 8, 100}; hero.Skills != want {
		t.Errorf("skills %v, want %v", hero.Skills, want)
	}
	if n := len(g.Sounds); n != 5 || g.Sounds[0] != SoundUse {
		t.Errorf("sounds %v", g.Sounds)
	}
}

func TestUseMiscNoUse(t *testing.T) {
	g, hero, _, item := miscGame(t)
	for _, typ := range []int{11, 29, 32, 34, 40, 45, 51, 54, 200} {
		i := item(CatQuest, typ)
		hero.Bag[0] = i
		if ok, _ := g.UseMisc(hero, nil, 0, i); ok || hero.Bag[0] != i {
			t.Errorf("type %d was used", typ)
		}
	}
	if len(g.Sounds) != 0 {
		t.Errorf("a refusal played %v", g.Sounds)
	}
}

func TestUseMiscCombine(t *testing.T) {
	tb := exeTables(t)
	g, hero, _, item := miscGame(t)
	// The hilt needs the blade in the box; the boxed part becomes the sword.
	hilt, blade := item(CatQuest, MiscHilt), item(CatQuest, MiscBlade)
	hero.Bag[0], hero.Bag[1] = hilt, 5
	if ok, _ := g.UseMisc(hero, tb, 0, hilt); ok {
		t.Fatal("the hilt was used without a target")
	}
	free := hero.FreeExp
	if ok, _ := g.UseMisc(hero, tb, blade, hilt); !ok {
		t.Fatal("hilt on blade refused")
	}
	sw, _ := g.Item(blade)
	if sw.Category != CatBlade || sw.Type != ancientSwordType || sw.Enchant != -1 || sw.Special != ancientSwordSpecial ||
		g.Concentration(blade) != float32(tb.ItemField16(0, ancientSwordType, 0x2E)) {
		t.Errorf("sword %+v condition %v", sw, g.Concentration(blade))
	}
	if hero.FreeExp != free+30 || hero.Bag[0] != 5 {
		t.Errorf("free exp %d (was %d), bag %v", hero.FreeExp, free, hero.Bag[:2])
	}
	// Amulet: right + left → left-and-right, then + bottom → the amulet.
	r, l, b := item(CatQuest, MiscAmuletRight), item(CatQuest, MiscAmuletLeft), item(CatQuest, MiscAmuletBottom)
	if ok, _ := g.UseMisc(hero, tb, r, MiscAmuletRight); ok {
		t.Error("a part joined a non-part")
	}
	if ok, _ := g.UseMisc(hero, tb, l, r); !ok {
		t.Fatal("right on left refused")
	}
	if it, _ := g.Item(l); it.Type != MiscAmuletLR {
		t.Errorf("joined type %d", it.Type)
	}
	if ok, _ := g.UseMisc(hero, tb, l, b); !ok {
		t.Fatal("bottom on left-and-right refused")
	}
	if it, _ := g.Item(l); it.Type != MiscAmulet {
		t.Errorf("amulet type %d", it.Type)
	}
	// The duplicator becomes a copy of the boxed item, at the bag front.
	hero.Bag = [32]int{}
	dup := item(CatQuest, MiscDuplicator)
	if ok, _ := g.UseMisc(hero, tb, blade, dup); !ok {
		t.Fatal("duplicator refused")
	}
	if got, _ := g.Item(dup); got.Category != CatBlade || got.Type != ancientSwordType || hero.Bag[0] != dup {
		t.Errorf("duplicate %+v, bag %v", got, hero.Bag[:2])
	}
}

func TestUseMiscSelf(t *testing.T) {
	g, hero, _, item := miscGame(t)
	use := func(typ int) {
		t.Helper()
		if ok, _ := g.UseMisc(hero, nil, 0, item(CatQuest, typ)); !ok {
			t.Fatalf("type %d refused", typ)
		}
	}
	hero.BaseStats = [4]int{20, 30, 16, 40}
	hero.PotionTimer = 0
	use(MiscWine) // n = 16/3 = 5
	if want := [4]int{25, 35, 1, 45}; hero.BaseStats != want || hero.PotionTimer != 150 ||
		hero.SavedBase != [4]int{16, 20, 30, 40} {
		t.Errorf("wine: base %v timer %d saved %v", hero.BaseStats, hero.PotionTimer, hero.SavedBase)
	}
	hero.BaseStats = [4]int{20, 30, 16, 40}
	use(MiscFalconFoot)
	use(MiscGoblet)
	use(MiscToys)
	if want := [4]int{35, 33, 16, 40}; hero.BaseStats != want {
		t.Errorf("base %v, want %v", hero.BaseStats, want)
	}
	use(MiscRelics)
	if hero.Enchants[1] != enchantRelics {
		t.Errorf("relics enchantment %d", hero.Enchants[1])
	}
	use(MiscBirchBark)
	if !g.QuestFlag(0) {
		t.Error("birch bark did not set quest flag 0")
	}
	// The seer's scroll identifies the user's items; the hero learns.
	hero.Skills[4] = 99
	a, b := item(CatRing, 6), item(CatBlade, 3)
	g.items[a*itemStride+0xE], g.items[b*itemStride+0xE] = 1, 1
	hero.Bag[3], hero.Equipment[0] = a, b
	use(MiscSeerScroll)
	ia, _ := g.Item(a)
	ib, _ := g.Item(b)
	if ia.Flagged || ib.Flagged || hero.Skills[4] != 100 {
		t.Errorf("identify: %v %v curios %d", ia.Flagged, ib.Flagged, hero.Skills[4])
	}
}

func TestUseMiscMapWide(t *testing.T) {
	g, hero, w, item := miscGame(t)
	m := g.Groups[0].Map
	var others []int
	for gi, gr := range g.Groups {
		if gi > 0 && gr.Map == m {
			for k := range gr.Count {
				others = append(others, gr.First+k)
			}
		}
	}
	if len(others) == 0 {
		t.Skip("no other group on the start map")
	}
	// Magnet: every other character's gold to the hero, bags from the back.
	v := &g.Entities[others[0]]
	v.Gold, v.Bag = 40, [32]int{}
	v.Bag[0], v.Bag[1] = item(CatHerb, 1), item(CatHerb, 2)
	want := hero.Gold
	for _, i := range others {
		want += g.Entities[i].Gold
	}
	use := func(typ int) {
		t.Helper()
		if ok, _ := g.UseMisc(hero, nil, 0, item(CatQuest, typ)); !ok {
			t.Fatalf("type %d refused", typ)
		}
	}
	use(MiscMagnet)
	if hero.Gold != want || v.Gold != 0 || v.Bag[0] != 0 || v.Bag[1] != 0 {
		t.Errorf("magnet: hero gold %d want %d, victim %d %v", hero.Gold, want, v.Gold, v.Bag[:2])
	}
	// Doll on everyone of the other groups.
	use(MiscDoll)
	for _, i := range others {
		if e := g.Entities[i]; !e.Dead() && e.Enchants[0] != enchantDoll {
			t.Errorf("entity %d not cursed", i)
		}
	}
	// The lamp kills the living non-village, non-0x53 characters.
	use(MiscSkeletonLamp)
	for _, i := range others {
		gr := g.Groups[g.Entities[i].Group]
		should := gr.Kind&0x80 == 0 && gr.Kind != 0x53
		if got := slices.Contains(w.killed, i); got != should {
			t.Errorf("lamp on %d (group kind %#x): killed %v", i, gr.Kind, got)
		}
		if should && g.Entities[i].HP != 0 {
			t.Errorf("lamp victim %d has HP %d", i, g.Entities[i].HP)
		}
	}
	// Keys open only on their spot; orbs always jump; both are kept.
	key := item(CatQuest, MiscWaterKey)
	hero.Bag = [32]int{}
	hero.Bag[0] = key
	g.UseMisc(hero, nil, 0, key)
	g.Groups[0].Map, hero.Row, hero.Col = 50, 34, 11
	g.UseMisc(hero, nil, 0, key)
	orb := item(CatQuest, 18)
	g.UseMisc(hero, nil, 0, orb)
	if len(w.jumps) != 2 || w.jumps[0] != keyDoors[MiscWaterKey].To || w.jumps[1].To != 3 || hero.Bag[0] != key {
		t.Errorf("jumps %+v, bag %v", w.jumps, hero.Bag[:2])
	}
	g.Groups[0].Map = m
	// The mirror digs up the buried piles of the map.
	buried := -1
	for i, p := range g.Piles {
		if p.Map == m && p.Gold < 0 {
			buried = i
		}
	}
	if buried >= 0 {
		gold := g.Piles[buried].Gold
		use(MiscMirror)
		if g.Piles[buried].Gold != -gold {
			t.Errorf("mirror: pile gold %d, was %d", g.Piles[buried].Gold, gold)
		}
	}
}

func TestUseMiscWhistle(t *testing.T) {
	g, hero, w, item := miscGame(t)
	gi := -1
	for i, gr := range g.Groups {
		if i > 0 && gr.Kind&0x80 == 0 && gr.Count > 0 {
			if e := g.Entities[gr.First]; e.Flags4&0x40 != 0 && !slices.Contains(unscared, e.Flags4) {
				gi = i
				break
			}
		}
	}
	if gi < 0 {
		t.Skip("no animal group in GAME.0")
	}
	w.enemy = gi
	for k := range g.Groups[gi].Count {
		g.Entities[g.Groups[gi].First+k].Row, g.Entities[g.Groups[gi].First+k].Col = 100, 50
	}
	g.Rand = func(int) int { return 3 } // right: two view widths
	whistle := item(CatQuest, MiscWhistle)
	if ok, _ := g.UseMisc(hero, nil, 0, whistle); !ok {
		t.Fatal("whistle refused")
	}
	e := g.Entities[g.Groups[gi].First]
	if len(w.walks) == 0 || w.walks[0] != [3]int{e.Index, e.Row - VillageRowRange, e.Col + 2*VillageColRange} {
		t.Errorf("walks %v, entity at (%d,%d)", w.walks, e.Row, e.Col)
	}
	if it, _ := g.Item(whistle); it.Category != CatQuest {
		t.Error("the whistle was used up")
	}
}

func TestFleeOffset(t *testing.T) {
	wall := func(r, c int) bool { return c < 12 } // columns 0–11 blocked
	dr, dc, ok := FleeOffset(func(int) int { return 0 }, 50, 10, 20, 9, wall)
	if !ok || dr != -40 || dc != 2 {
		t.Errorf("up: %d %d %v", dr, dc, ok)
	}
	if _, _, ok := FleeOffset(func(int) int { return 2 }, 50, 10, 20, 9, wall); ok {
		t.Error("left into the wall succeeded")
	}
	dr, dc, ok = FleeOffset(func(int) int { return 1 }, 10, 40, 20, 9, wall)
	if !ok || dr != 40 || dc != -9 {
		t.Errorf("down: %d %d %v", dr, dc, ok)
	}
}
