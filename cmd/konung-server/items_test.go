package main

import (
	"path/filepath"
	"testing"

	"konung1.5/internal/lanserver"
	"konung1.5/internal/lanstore"
	"konung1.5/internal/netplay"
)

// Items go through the store: saved, loaded back, an amulet part dropped
// on load (SAVITEMS.DAT's rule); a new account loads nothing (its hero
// keeps the starting gear).
func TestItemsThroughStore(t *testing.T) {
	store, err := lanstore.Open(filepath.Join(t.TempDir(), "konung.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.Login(netplay.JoinInfo{Name: "Volk", Password: "p"})
	if got := loadItems(store, "Volk"); len(got) != 0 {
		t.Fatalf("a new account has items: %+v", got)
	}
	items := []lanserver.Item{
		{Slot: lanserver.SlotEquip, Index: 2, Category: 2, Enchantment: -1, Type: 3, Condition: 40, MaxCondition: 50},
		{Slot: lanserver.SlotArrows, Category: 12, Type: 1, Count: 30},
		{Slot: lanserver.SlotBag, Index: 0, Category: 11, Type: 12}, // an amulet part
	}
	saveHero(store, "Volk", &netplay.Hero{Level: 2, Gold: 300}, items)
	got := loadItems(store, "Volk")
	if len(got) != 2 || got[0] != items[0] && got[1] != items[0] {
		t.Errorf("loaded %+v", got)
	}
	if h, _ := store.Hero("Volk"); h.Level != 2 || h.Gold != 300 {
		t.Errorf("hero %+v", h)
	}
}
