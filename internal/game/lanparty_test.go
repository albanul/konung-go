package game

import (
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
)

// A hireling (MsgParty) becomes party member 1; the inventory carries its
// record (bag, equipment) and the window follows the held item's holder;
// item clicks name the window's member; the portrait click keeps it
// (the window shows the hireling, not only the hero).
func TestLANViewHirelingItems(t *testing.T) {
	s := viewScene(t)
	var sent []netplay.OrderPayload
	s.view.order = func(o netplay.OrderPayload) { sent = append(sent, o) }
	v := -1
	for i, e := range s.Game.Entities {
		if e.Dialogue == 110 && e.Row > 0 {
			v = i
			break
		}
	}
	if v < 0 {
		t.Fatal("no villager of map 1 in GAME.RES")
	}
	ve := s.Game.Entities[v]
	s.applyChar(netplay.CharState{Entity: v, Row: ve.Row, Col: ve.Col, HP: ve.HP}) // the server reports it
	if s.ByEntity[v] == nil {
		t.Fatal("the villager is not in the view")
	}
	s.applyParty(netplay.PartyPayload{Hirelings: []int{v}})
	if len(s.Party) != 2 || s.Party[1].Index != v {
		t.Fatalf("party %d members", len(s.Party))
	}
	gift := 4100
	inv := netplay.InventoryPayload{Hero: s.Party[0], Items: map[int]netplay.ItemRecord{gift: {gamestate.CatBlade, 0xFF, 0xFF, 2}}}
	rec := s.Party[1]
	rec.Bag = [32]int{gift}
	rec.HP = 1 // the view's to set
	inv.Hirelings = []gamestate.Entity{rec}
	inv.Hand, inv.Holder = netplay.HandState{Item: gift, Cursor: 1}, v
	hp := s.Party[1].HP
	s.applyInventory(inv)
	if e := s.Party[1]; e.Bag[0] != gift || e.HP != hp || e.Group != 0 {
		t.Errorf("hireling after the inventory: bag %v HP %d group %d", e.Bag[:2], e.HP, e.Group)
	}
	if s.Cur != 1 {
		t.Errorf("the window shows member %d, want the holder", s.Cur)
	}
	s.itemOrder(netplay.ItemEquip, 9, false)
	s.lanClick(&s.Party[1], 0, 0, hotspotPortrait, false) // the hero's portrait: give
	if len(sent) != 2 || sent[0].Member != v || sent[1].Item != netplay.ItemGive || sent[1].Index != s.Party[0].Index || sent[1].Member != v {
		t.Errorf("orders %+v", sent)
	}
	// With an empty hand a portrait click selects; the window's member
	// stays the clicked one.
	s.hand.Item, s.hand.Cursor = 0, gamestate.CursorPointer
	s.inv.open = true
	sent = nil
	s.Cur = 0
	s.lanClick(&s.Party[0], 0, 0, hotspotPortrait+1, false)
	if s.Cur != 1 || len(sent) != 0 {
		t.Errorf("after the hireling's portrait the window shows %d", s.Cur)
	}
	s.lanClick(&s.Party[1], 0, 0, hotspotPortrait, false)
	if s.Cur != 0 {
		t.Errorf("after the hero's portrait the window shows %d", s.Cur)
	}
}
