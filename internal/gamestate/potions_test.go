package gamestate

import (
	"testing"

	"konung1.5/internal/resfmt"
)

func potionGame(t *testing.T) (*Game, *resfmt.ExeTables) {
	t.Helper()
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	return g, tb
}

// herbOf returns the first GAME.0 item of herb type typ.
func herbOf(t *testing.T, g *Game, typ int) int {
	t.Helper()
	for i := 1; (i+1)*itemStride <= len(g.items); i++ {
		if it, _ := g.Item(i); it.Category == CatHerb && it.Type == typ {
			return i
		}
	}
	t.Fatalf("no herb of type %d", typ)
	return 0
}

func TestRecipesExe(t *testing.T) {
	_, tb := potionGame(t)
	r := tb.Recipes()
	// Row 6: «Пустая банка» + «Белый корень» → «Лечебный бальзам», no skill.
	if want := (resfmt.Recipe{Target: HerbJar, Held: HerbRoot, NewTarget: HerbBalm, NewHeld: 0xFF, Mode: resfmt.RecipeSet}); r[6] != want {
		t.Fatalf("recipe 6 = %+v, want %+v", r[6], want)
	}
	// Row 19: «Масло» + «Противоядие» → «Живая Вода» and an empty jar, skill +4.
	if want := (resfmt.Recipe{Target: HerbOil, Held: HerbAntidote, NewTarget: HerbLivingWater, NewHeld: HerbJar, Skill: 4, Mode: resfmt.RecipeMin}); r[19] != want {
		t.Fatalf("recipe 19 = %+v, want %+v", r[19], want)
	}
}

func TestMixGame0(t *testing.T) {
	g, tb := potionGame(t)
	volk := g.Entities[0] // healing 101, bag: amulet part, five balms of 7.0 (items 3–7)
	if volk.Skills[0] != 101 || g.Concentration(3) != 7 {
		t.Fatalf("GAME.0 Volk: healing %d, balm %v", volk.Skills[0], g.Concentration(3))
	}
	// Balm + balm: concentrations add, the held balm is used up, healing +1.
	g.Mix(&volk, tb, 3, 4)
	if it, _ := g.Item(4); g.Concentration(3) != 14 || it.Category != 0xFF || volk.Skills[0] != 102 {
		t.Fatalf("balm+balm: %v, held category %#x, healing %d", g.Concentration(3), it.Category, volk.Skills[0])
	}
	// Once more: capped at 15.
	g.Mix(&volk, tb, 3, 5)
	if g.Concentration(3) != 15 {
		t.Fatalf("cap: %v", g.Concentration(3))
	}
	// Drinking leaves an empty jar; the jar and a root make a balm of healing·0.01.
	volk.HP = 100
	if !g.UseHerb(&volk, tb, 0, 6) || volk.HP != 100+7*160 || g.ItemType(6) != HerbJar {
		t.Fatalf("drink balm: HP %d, type %d", volk.HP, g.ItemType(6))
	}
	root := herbOf(t, g, HerbRoot)
	g.Mix(&volk, tb, 6, root)
	if g.ItemType(6) != HerbBalm || g.Concentration(6) != float32(103*0.01) || volk.Skills[0] != 103 {
		t.Fatalf("jar+root: type %d %v healing %d", g.ItemType(6), g.Concentration(6), volk.Skills[0])
	}
	// No recipe: «Непонятная смесь», which halves HP when drunk.
	nut := herbOf(t, g, HerbNut)
	g.Mix(&volk, tb, 7, nut)
	if g.ItemType(7) != HerbMess {
		t.Fatalf("no recipe: type %d", g.ItemType(7))
	}
	volk.HP = 1001
	if !g.UseHerb(&volk, tb, 0, 7) || volk.HP != 500 {
		t.Fatalf("mess: HP %d", volk.HP)
	}
	// Ingredients can't be drunk.
	if g.UseHerb(&volk, tb, 0, herbOf(t, g, HerbRoot)) {
		t.Fatal("a root must not be usable")
	}
}

func TestPotionTimerGame0(t *testing.T) {
	g, tb := potionGame(t)
	volk := g.Entities[0] // base Хар 10, Сил 15, Лов 10, Вын 10
	braga := herbOf(t, g, HerbBraga)
	g.setConcentration(braga, 9)
	// n = min(Лов/3, f) = 3: Лов −9, Хар/Сил/Вын +3 for 90 passes.
	if !g.UseHerb(&volk, tb, 0, braga) || volk.PotionTimer != 90 || volk.BaseStats != [4]int{13, 18, 1, 13} {
		t.Fatalf("braga: timer %d base %v", volk.PotionTimer, volk.BaseStats)
	}
	// A second timed potion is refused while the timer runs.
	potion := herbOf(t, g, HerbPotion)
	if g.UseHerb(&volk, tb, 0, potion) || g.ItemType(potion) != HerbPotion {
		t.Fatal("potion must be refused while a timer runs")
	}
	g.Entities[0] = volk
	for range 90 {
		g.Upkeep(tb, 1, func(int) bool { return false }, func(int) {})
	}
	if v := g.Entities[0]; v.PotionTimer != 0 || v.BaseStats != [4]int{10, 15, 10, 10} {
		t.Fatalf("after the timer: %d %v", v.PotionTimer, v.BaseStats)
	}
}

func TestInHandBoxGame0(t *testing.T) {
	g, tb := potionGame(t)
	volk := g.Entities[0]
	h := NewHand()
	// Balm 3 into the box: it goes back to the end of the bag and can't be lifted.
	g.ClickBag(&h, &volk, tb, 1)
	if g.ClickBox(&h, &volk, tb); h.Box != 3 || h.Holding() || volk.Bag[5] != 3 {
		t.Fatalf("box: %+v bag %v", h, volk.Bag[:8])
	}
	g.ClickBag(&h, &volk, tb, 5)
	if h.Holding() {
		t.Fatal("the boxed item must stay in the bag")
	}
	// Pour balm 4 onto it: mixed, the hand empties.
	g.ClickBag(&h, &volk, tb, 1)
	if msg := g.ClickBox(&h, &volk, tb); msg != 0 || h.Holding() || g.Concentration(3) != 14 {
		t.Fatalf("pour: msg %#x hand %+v conc %v", msg, h, g.Concentration(3))
	}
	// Poison on the knife in the box; the jar goes to the bag front.
	h.Box = 0
	g.ClickEquipment(&h, &volk, tb, WeaponBlade)
	g.ClickBox(&h, &volk, tb)
	poison := herbOf(t, g, HerbPoison)
	BagInsert(&volk, poison)
	g.ClickBag(&h, &volk, tb, 0)
	before := int16(g.itemWord(1, 0xC))
	if msg := g.ClickBox(&h, &volk, tb); msg != 0 || volk.Bag[0] != poison || g.ItemType(poison) != HerbJar ||
		int16(g.itemWord(1, 0xC)) != before+int16(g.Concentration(poison)) {
		t.Fatalf("poison knife: msg %#x bag %v poisoning %d", msg, volk.Bag[:4], int16(g.itemWord(1, 0xC)))
	}
	// Drinking from the portrait: the jar returns to the bag front.
	volk.HP = 100
	g.ClickBag(&h, &volk, tb, 2)
	held := h.Item
	if msg := g.ClickPortrait(&h, &volk, &volk, tb); msg != 0 || h.Holding() || volk.HP <= 100 || volk.Bag[0] != held {
		t.Fatalf("portrait: msg %#x HP %d bag %v", msg, volk.HP, volk.Bag[:4])
	}
}
