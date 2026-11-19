package main

import (
	"path/filepath"
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/lanserver"
	"konung1.5/internal/lanstore"
	"konung1.5/internal/netplay"
)

// Items go through the store with the original server's rules: a new
// account gets the knife once (starter), an account without items gets
// nothing; saved items come back, an amulet part dropped on load
// (SAVITEMS.DAT's rule); a hero that died enters again naked; one that
// was dead at a session's end gets its worn items back; «Новый герой» gives
// the knife again.
func TestItemsThroughStore(t *testing.T) {
	store, err := lanstore.Open(filepath.Join(t.TempDir(), "konung.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.Login(netplay.JoinInfo{Name: "Volk", Password: "p"})
	if got, starter := loadItems(store, "Volk"); !starter || len(got) != 0 {
		t.Fatalf("a new account: starter %v, items %+v", starter, got)
	}
	if got, starter := loadItems(store, "Volk"); starter || len(got) != 0 {
		t.Fatalf("the knife twice, or items from nowhere: starter %v, %+v", starter, got)
	}
	sword := lanserver.Item{Slot: lanserver.SlotEquip, Index: 2, Category: 2, Enchantment: -1, Type: 3, Condition: 40, MaxCondition: 50}
	arrows := lanserver.Item{Slot: lanserver.SlotArrows, Category: 12, Type: 1, Count: 30}
	items := []lanserver.Item{sword, arrows,
		{Slot: lanserver.SlotBag, Index: 0, Category: gamestate.CatQuest, Type: gamestate.MiscAmuletBottom}, // an amulet part
	}
	saveHero(store, "Volk", &netplay.Hero{Level: 2, Gold: 300}, items)
	got, _ := loadItems(store, "Volk")
	if len(got) != 2 || got[0] != sword && got[1] != sword {
		t.Errorf("loaded %+v", got)
	}
	if h, _ := store.Hero("Volk"); h.Level != 2 || h.Gold != 300 {
		t.Errorf("hero %+v", h)
	}

	// Died, entered again before the session's end: naked.
	heroDied(store, "Volk", &netplay.Hero{Level: 2, Gold: 300}, []lanserver.Item{sword, arrows})
	if got, starter := loadItems(store, "Volk"); starter || len(got) != 0 {
		t.Errorf("after death: starter %v, items %+v (want naked)", starter, got)
	}
	// Died, the session ended: the worn items are back.
	heroDied(store, "Volk", nil, []lanserver.Item{sword})
	if err := store.EndSession(); err != nil {
		t.Fatal(err)
	}
	if got, _ := loadItems(store, "Volk"); len(got) != 1 || got[0] != sword {
		t.Errorf("after the session's end: %+v (want the sword)", got)
	}
	// «Новый герой»: the knife again.
	store.Login(netplay.JoinInfo{Name: "Volk", Password: "p", NewHero: &netplay.HeroChoice{Class: 5, Stats: [4]int{3, 15, 3, 10}}})
	if got, starter := loadItems(store, "Volk"); !starter || len(got) != 0 {
		t.Errorf("rebuilt hero: starter %v, items %+v", starter, got)
	}
}
