package lanserver

import (
	"sync"
	"testing"
	"time"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
)

// The original server's death rules over the network: a new hero carries
// only the knife; a killed hero's bag lies in a pile where it fell, its
// worn items are kept with the dead flag (Died), and entering again before
// the session's end comes naked.
func TestDeathRules(t *testing.T) {
	w := load(t)
	ss := startSession(t, w)
	var mu sync.Mutex
	saved := map[string][]Item{}
	dead := map[string]bool{}
	var died []Item
	ss.host.Save = func(p netplay.PlayerInfo, _ *netplay.Hero, items []Item) {
		mu.Lock()
		saved[p.Name] = items
		mu.Unlock()
	}
	ss.host.Died = func(p netplay.PlayerInfo, _ *netplay.Hero, worn []Item) {
		mu.Lock()
		saved[p.Name], dead[p.Name] = worn, true
		if p.Name == "Olaf" {
			died = worn
		}
		mu.Unlock()
	}
	// The store's rules (cmd/konung-server loadItems): never seen = a new
	// hero (the knife); dead = naked; else the saved items.
	ss.host.Load = func(p netplay.PlayerInfo) ([]Item, bool) {
		mu.Lock()
		defer mu.Unlock()
		if dead[p.Name] {
			delete(dead, p.Name)
			saved[p.Name] = nil
			return nil, false
		}
		items, ok := saved[p.Name]
		return items, !ok
	}
	a := enter(t, ss, "Ivar", 1)
	b := enter(t, ss, "Olaf", 1)

	// A new hero: only the knife.
	waitUntil(t, "Olaf's inventory", func() bool { inv, _, _ := b.items(); return inv.Hero.Equipment[gamestate.WeaponBlade] != 0 })
	inv, _, _ := b.items()
	for k, i := range inv.Hero.Equipment {
		if i != 0 && k != gamestate.WeaponBlade {
			t.Errorf("a new hero wears more than the knife: slot %d", k)
		}
	}
	for _, i := range inv.Hero.Bag {
		if i != 0 {
			t.Fatalf("a new hero's bag is not empty: %v", inv.Hero.Bag)
		}
	}

	// Olaf carries two herbs and 500 gold; Ivar kills him, abroad.
	abroad(t, ss, w, a, b)
	ss.host.mu.Lock()
	_, e := w.Maps[w.PlayerMap(b.at.Slot)].Member(b.at.Slot)
	herbs := [2]int{w.Game.SetItemKind(0, gamestate.CatHerb, 1), w.Game.SetItemKind(0, gamestate.CatHerb, 2)}
	e.Bag[0], e.Bag[1] = herbs[0], herbs[1]
	e.Gold = 500
	ss.host.mu.Unlock()
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderStance, On: true})
	a.c.Order(netplay.OrderPayload{Kind: netplay.OrderRight, Target: b.at.Slot, Row: b.at.Row, Col: b.at.Col})
	waitUntil(t, "Olaf's death", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return died != nil
	})
	mu.Lock()
	worn := died
	mu.Unlock()
	if len(worn) != 1 || worn[0].Slot != SlotEquip || worn[0].Index != gamestate.WeaponBlade {
		t.Errorf("Died kept %+v, want the knife only (the bag lies on the ground)", worn)
	}
	// The bag lies where Olaf fell, for anyone; Ivar sees the pile.
	_, _, chars := a.state()
	fell := mapdata.Cell{Row: chars[b.at.Slot].Row, Col: chars[b.at.Slot].Col}
	waitUntil(t, "the bag on the ground", func() bool {
		_, p, _ := a.items()
		return pileItem(p, fell, herbs[0]) && pileItem(p, fell, herbs[1])
	})
	ss.host.mu.Lock()
	if e, _ := w.Hero(b.at.Slot); e.Gold != 500 {
		t.Errorf("the dead hero's gold %d, want 500 kept", e.Gold)
	}
	ss.host.mu.Unlock()

	// Olaf leaves (his game over) and enters again: naked.
	b.c.Close()
	waitUntil(t, "Olaf gone", func() bool {
		_, ok := ss.host.Slot(b.c.PlayerID)
		return !ok
	})
	b2 := enter(t, ss, "Olaf", 1)
	time.Sleep(300 * time.Millisecond)
	waitUntil(t, "Olaf's new inventory", func() bool { inv, _, _ := b2.items(); return inv.Hero.Level != 0 || inv.Items != nil })
	inv, _, _ = b2.items()
	if inv.Hero.Equipment != ([6]int{}) || inv.Hero.Bag != ([32]int{}) {
		t.Errorf("entered again after death not naked: equipment %v, bag %v", inv.Hero.Equipment, inv.Hero.Bag[:4])
	}
}
