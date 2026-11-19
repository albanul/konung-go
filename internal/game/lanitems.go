package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

// Items in a LAN world (netplay/items.go): the server keeps them; the
// viewer sends every item click as an order (OrderItem) and mirrors what
// comes back: the own hero's record, items and hand (MsgInventory), the
// piles on its map (MsgPiles) and the pile window (MsgPileWindow). Nothing
// here changes an item itself; only the scroll of the bag strip and of the
// pile window's lists are the viewer's own.

// itemOrder sends an item click.
func (s *startScene) itemOrder(act string, index int, on bool) {
	s.viewOrder(netplay.OrderPayload{Kind: netplay.OrderItem, Item: act, Index: index, On: on})
}

// returnHand puts the held item back (gamestate.Hand.Return): in a LAN
// world the server does it.
func (s *startScene) returnHand(cur *gamestate.Entity) {
	if s.view != nil {
		if s.hand.Holding() {
			s.itemOrder(netplay.ItemReturn, 0, false)
		}
		return
	}
	s.hand.Return(cur)
}

// clearBox empties the in-hand box (the window or the strip opened,
// 0x004246D1, 0x00437253).
func (s *startScene) clearBox() {
	s.hand.Box = 0
	if s.view != nil {
		s.itemOrder(netplay.ItemBoxClear, 0, false)
	}
}

// lanClick is a click in a LAN world (click's single-player cases as
// orders): the held item on the own portrait, the bag strip, the character
// window, a drop on the map; otherwise an order on the map (viewClick).
func (s *startScene) lanClick(cur *gamestate.Entity, mx, my, spot int, right bool) {
	switch {
	case spot == hotspotPortrait:
		if s.hand.Holding() && !right {
			s.itemOrder(netplay.ItemSelf, 0, false) // 0x00422E21: use or drink
		}
	case !inView(mx, my):
		// HUD: the buttons were handled by updateControls.
	case s.inv.showStrip() && (spot == hotspotBagLeft || spot == hotspotBagRight):
		if !right {
			dir := -1
			if spot == hotspotBagRight {
				dir = 1
			}
			s.inv.scrollBy(*cur, dir, s.frame)
		}
	case s.inv.showStrip() && s.inv.strip.hit(mx, my):
		s.itemOrder(netplay.ItemBag, (mx-s.inv.strip.x0)/s.inv.strip.cellW+s.inv.scroll, right)
	case s.inv.open:
		if !right {
			s.lanWindowClick(spot)
		}
	case !right && s.hand.Holding():
		s.itemOrder(netplay.ItemDrop, 0, false)
	default:
		s.viewClick(mx, my, right)
	}
}

// lanWindowClick is a left click in the character window (windowClick) as
// an order.
func (s *startScene) lanWindowClick(spot int) {
	switch {
	case spot >= 8 && spot <= 13:
		s.itemOrder(netplay.ItemEquip, spot, false)
	case spot >= 14 && spot <= 18:
		s.itemOrder(netplay.ItemJewel, spot-14, false)
	case spot == hotspotInHand:
		s.itemOrder(netplay.ItemBox, 0, false)
	case spot >= gamestate.HotspotRaise && spot < gamestate.HotspotRaise+gamestate.RaiseRows:
		s.itemOrder(netplay.ItemRaise, spot-gamestate.HotspotRaise, false)
	case spot == gamestate.HotspotAutoLevel:
		s.itemOrder(netplay.ItemAuto, 0, false)
	}
}

// lanPileApply is window 7's «Ok» or «Закрыть» (applyTrade: the pile
// window or a trade) as an order with the lists as the player arranged
// them; «Закрыть» closes the
// window at once, «Ok» waits for the server's window.
func (s *startScene) lanPileApply(accept bool) {
	tr := s.trade.tr
	if tr == nil {
		return
	}
	lists := tr.Lists
	act := netplay.ItemPile
	if tr.NPC >= 0 {
		act = netplay.ItemTrade // a trade (talk.go on the server)
	}
	s.viewOrder(netplay.OrderPayload{Kind: netplay.OrderItem, Item: act, On: accept, Lists: &lists})
	if !accept {
		s.trade.tr = nil
	}
}

// applyInventory mirrors the own hero's items (MsgInventory): the item
// records, the hero's record except what the view keeps (position, facing,
// action, HP, the stance and death bits), and the hand.
func (s *startScene) applyInventory(inv netplay.InventoryPayload) {
	if len(s.Party) == 0 {
		return
	}
	for i, r := range inv.Items {
		s.Game.SetItemRecord(i, r)
	}
	e := &s.Party[0]
	keep := *e
	*e = inv.Hero
	e.Raw, e.Index, e.Group = keep.Raw, keep.Index, keep.Group
	e.Row, e.Col, e.Variant, e.Type, e.Step, e.HP = keep.Row, keep.Col, keep.Variant, keep.Type, keep.Step, keep.HP
	e.Flags3 = e.Flags3&^0x04 | keep.Flags3&0x04
	e.Flags4 = e.Flags4&^0x80 | keep.Flags4&0x80
	s.Game.Entities[e.Index] = *e
	s.hand.Item, s.hand.Cursor, s.hand.Box = inv.Hand.Item, inv.Hand.Cursor, inv.Hand.Box
	if m, _ := s.PartyMember(e.Index); m != nil {
		m.Look = lookOf(*e)
	}
}

// applyPiles mirrors the piles on the map (MsgPiles): their records at the
// server's pile indices, and the map's active list.
func (s *startScene) applyPiles(p netplay.PilesPayload) {
	if p.Map != s.MapNumber {
		return
	}
	for i, r := range p.Items {
		s.Game.SetItemRecord(i, r)
	}
	s.PileAt = map[mapdata.Cell]int{}
	for _, ps := range p.Piles {
		for len(s.Game.Piles) <= ps.Index {
			s.Game.Piles = append(s.Game.Piles, gamestate.Pile{Index: -1})
		}
		s.Game.Piles[ps.Index] = ps.Pile
		s.PileAt[mapdata.Cell{Row: ps.Pile.Row, Col: ps.Pile.Col}] = ps.Index
	}
}

// applyPileWindow opens, updates or closes the pile window as the server
// has it (MsgPileWindow); the lists' scroll stays while the same pile is
// shown.
func (s *startScene) applyPileWindow(pw netplay.PileWindowPayload) {
	if !pw.Open || len(s.Party) == 0 {
		s.trade.tr = nil
		return
	}
	hero := s.Party[0].Index
	tr := &gamestate.Trade{NPC: -1, Shown: hero, Hero: hero, Pile: pw.Pile, Mode: pw.Pile, Map: pw.Map, Lists: pw.Lists}
	tr.HeroMoney = s.Party[0].Gold
	if t := pw.Trader; t != nil {
		// A trade: the goods' records, the trader's trade skill (the
		// prices) and both sides' money as the server has them.
		if t.NPC < 0 || t.NPC >= len(s.Game.Entities) {
			return
		}
		for i, r := range pw.Items {
			s.Game.SetItemRecord(i, r)
		}
		s.Game.Entities[t.NPC].Skills[2] = t.Skill
		tr.NPC, tr.Pile, tr.Mode = t.NPC, -1, t.Mode
		tr.HeroMoney, tr.OtherMoney = t.HeroMoney, t.OtherMoney
	}
	if old := s.trade.tr; old != nil && old.Pile == tr.Pile && old.NPC == tr.NPC {
		tr.Scroll = old.Scroll
	}
	s.trade.tr = tr
	s.talk.show(nil) // window 7 replaces the dialogue
}

// lanCanTalk is the LAN client's right-click test (0x00416536,
// 0x00416689): a character of the world (group ≥ 16: not a player's hero)
// with a dialogue, the hero out of the stance, Ctrl up; the port also
// skips the dead and opens only the roles' dialogues (gamestate.LANTalks).
func (s *startScene) lanCanTalk() bool {
	if s.hovered < world.LANPlayers || s.assets == nil || s.assets.quests == nil || len(s.Party) == 0 {
		return false
	}
	e := s.Entity(s.hovered)
	return gamestate.LANTalks(e.Dialogue) && !e.Dead() && !s.Party[0].InStance() &&
		!ebiten.IsKeyPressed(ebiten.KeyControl)
}

// applyDialogue shows the dialogue as the server has it (MsgDialogue); the
// greeting plays when it opens.
func (s *startScene) applyDialogue(dp netplay.DialoguePayload) {
	if !dp.Open {
		s.talk.show(nil)
		return
	}
	d := &gamestate.Dialogue{NPC: dp.NPC, Lines: dp.Lines}
	for _, t := range dp.Replies {
		d.Replies = append(d.Replies, gamestate.DialogueReply{Text: t})
	}
	if s.talk.d == nil && dp.Greeting != 0 {
		sfx.play(dp.Greeting, 0, 0)
	}
	s.talk.show(d)
}
