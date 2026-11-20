package game

import (
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
)

// The viewer mirrors the server's items: the inventory (records, hero,
// hand) without touching what the view keeps; the piles of its map; the
// pile window. Item clicks become orders and change nothing here.
func TestLANViewItems(t *testing.T) {
	s := viewScene(t)
	var sent []netplay.OrderPayload
	s.view.order = func(o netplay.OrderPayload) { sent = append(sent, o) }

	// An inventory: a sword in the hand slot, a herb in the bag, the hand
	// holding the herb; the server's record keeps another cell and HP.
	inv := netplay.InventoryPayload{Hero: s.Party[0], Items: map[int]netplay.ItemRecord{}}
	inv.Hero.Row, inv.Hero.Col, inv.Hero.HP = 1, 1, 5
	inv.Hero.Gold, inv.Hero.Level = 777, 4
	inv.Hero.Bag = [32]int{}
	inv.Hero.Equipment = [6]int{}
	sword, herb := 4000, 4001
	inv.Hero.Equipment[gamestate.WeaponBlade] = sword
	inv.Hero.Bag[0] = herb
	inv.Items[sword] = netplay.ItemRecord{gamestate.CatBlade, 0xFF, 0xFF, 2}
	inv.Items[herb] = netplay.ItemRecord{gamestate.CatHerb, 0xFF, 0xFF, 3}
	inv.Hand = netplay.HandState{Item: herb, Cursor: 1}
	cell, hp := s.Hero.Cell, s.Party[0].HP
	s.applyInventory(inv)
	e := s.Party[0]
	if e.Gold != 777 || e.Level != 4 || e.Equipment[gamestate.WeaponBlade] != sword || e.Bag[0] != herb {
		t.Errorf("hero after the inventory: %+v", e)
	}
	if e.Row != cell.Row || e.Col != cell.Col || e.HP != hp {
		t.Errorf("the inventory moved the hero or its HP: (%d, %d) HP %d", e.Row, e.Col, e.HP)
	}
	if s.Game.ItemType(sword) != 2 || s.Game.ItemType(herb) != 3 || s.Game.Entities[e.Index].Gold != 777 {
		t.Error("item records or the world record not mirrored")
	}
	if s.hand.Item != herb || !s.hand.Holding() {
		t.Errorf("hand %+v", s.hand)
	}

	// Piles: the map's active list is the server's.
	pile := netplay.PilesPayload{Map: s.MapNumber, Items: map[int]netplay.ItemRecord{4002: {gamestate.CatHerb, 0xFF, 0xFF, 1}}}
	p := gamestate.Pile{Index: -1, Row: 100, Col: 30, Map: s.MapNumber}
	p.Items[0] = 4002
	pile.Piles = []netplay.PileState{{Index: 2500, Pile: p}}
	s.applyPiles(pile)
	if len(s.PileAt) != 1 || s.PileAt[mapdata.Cell{Row: 100, Col: 30}] != 2500 || s.Game.Piles[2500].Items[0] != 4002 {
		t.Errorf("piles: %v", s.PileAt)
	}
	s.applyPiles(netplay.PilesPayload{Map: s.MapNumber + 1})
	if len(s.PileAt) != 1 {
		t.Error("another map's piles were taken")
	}

	// The pile window opens, keeps its scroll, «Закрыть» closes it and
	// sends the lists.
	var lists [4][32]int
	lists[0][0], lists[2][0] = herb, 4002
	s.applyPileWindow(netplay.PileWindowPayload{Open: true, Pile: 2500, Map: s.MapNumber, Lists: lists})
	if s.trade.tr == nil || s.trade.tr.Pile != 2500 || s.trade.tr.Lists != lists {
		t.Fatalf("pile window %+v", s.trade.tr)
	}
	s.trade.tr.Move(0, 0) // the herb into the offer: the viewer's own arranging
	s.applyTrade(false)
	if s.trade.tr != nil {
		t.Error("«Закрыть» left the window open")
	}
	if len(sent) == 0 || sent[len(sent)-1].Item != netplay.ItemPile || sent[len(sent)-1].Lists == nil || sent[len(sent)-1].Lists[1][0] != herb {
		t.Errorf("pile order: %+v", sent)
	}

	// Clicks are orders for the window's member: a held item dropped on
	// the map, on the own portrait (give to the holder: use), the hand's
	// return; nothing changes here.
	sent = nil
	bag := s.Party[0].Bag
	s.lanClick(&s.Party[0], viewLeft+200, viewTop+200, 0, false)
	s.lanClick(&s.Party[0], 0, 0, hotspotPortrait, false)
	s.returnHand(&s.Party[0])
	hero := s.Party[0].Index
	if len(sent) != 3 || sent[0].Item != netplay.ItemDrop || sent[1].Item != netplay.ItemGive || sent[1].Index != hero ||
		sent[2].Item != netplay.ItemReturn || sent[0].Member != hero {
		t.Errorf("orders %+v", sent)
	}
	if s.Party[0].Bag != bag || !s.hand.Holding() {
		t.Error("a click changed the items here")
	}
}
