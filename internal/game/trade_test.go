package game

import (
	"testing"

	"konung1.5/internal/gamestate"
)

// clickCell returns a point inside cell c of trade list k.
func (w *tradeWindow) clickCell(k, c int) (int, int) {
	r := w.layout.Lists[k]
	return r.Min.X + w.strip.arrowW + c*w.strip.cellW + w.strip.cellW/2, r.Min.Y + 10
}

// TestOverweightOpensPileWindow: a pick-up over the weight limit opens the
// pile window; taking the pile's item and offering a bag item swaps them.
func TestOverweightOpensPileWindow(t *testing.T) {
	s := newVolkScene(t)
	hero := &s.party[0]
	ground := hero.Bag[4]
	hero.Bag[4] = 0
	i, _ := s.game.DropItem(-1, s.mapNumber, s.hero.cell.Row, s.hero.cell.Col, ground)
	s.pileAt[s.hero.cell] = i
	hero.Stats[3] = -40 // max weight 10000 − 13333: nothing more fits
	s.arrive(s.lead())
	if !s.trade.open() || s.trade.tr.Lists[gamestate.TradeGoods][0] != ground {
		t.Fatalf("pile window not open (%v)", s.trade.tr)
	}
	if s.statusText() == nil {
		// the message «Превышен вес» shows first
		t.Error("no message")
	}
	s.tradeClick(s.trade.clickCell(gamestate.TradeGoods, 0))
	if s.trade.tr.Lists[gamestate.TradeTake][0] != ground {
		t.Fatal("click did not take the item")
	}
	offered := s.trade.tr.Lists[gamestate.TradeBag][0]
	s.tradeClick(s.trade.clickCell(gamestate.TradeBag, 0))
	if s.trade.tr.Lists[gamestate.TradeOffer][0] != offered {
		t.Fatal("click did not offer the item")
	}
	hero.Stats[3] = 30 // strong enough now
	s.game.Entities[hero.Index].Stats[3] = 30
	ok := s.trade.layout.Buttons[1]
	s.tradeClick(ok.Min.X+5, ok.Min.Y+5)
	if s.party[0].Bag[0] != ground {
		t.Fatalf("bag after Ok: %v", s.party[0].Bag[:4])
	}
	if p := s.game.Piles[i]; p.Items[0] != offered {
		t.Errorf("pile after Ok: %v", p.Items[:3])
	}
	if !s.trade.open() {
		t.Error("the pile window closes while the pile has items")
	}
	if s.escape() || s.trade.open() {
		t.Error("Escape does not close window 7")
	}
}

// TestTradeFromDialogue: script 275 in a dialogue opens window 7 with the
// healer's (empty) shop and closes the dialogue.
func TestTradeFromDialogue(t *testing.T) {
	s := newVolkScene(t)
	tk := s.newTalk()
	d := tk.OpenDialogue(356, nil)
	gamestate.ScriptFunc(tk, 275)
	s.afterTalk(tk, d)
	if !s.trade.open() || s.talk.open() || s.trade.tr.NPC != 356 || s.trade.tr.Mode != 2 {
		t.Fatalf("trade %+v dialogue %v", s.trade.tr, s.talk.open())
	}
	c := s.trade.layout.Buttons[0]
	s.tradeClick(c.Min.X+5, c.Min.Y+5)
	if s.trade.open() {
		t.Error("«Закрыть» did not close the window")
	}
}
