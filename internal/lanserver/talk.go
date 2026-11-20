package lanserver

import (
	"log"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

// Dialogues and trade on the server (docs/konung_lan.md#dialogues-and-trade).
// The original's client runs the dialogue and the trade window on its copy
// of the world and tells the server what changed (0x53 script functions,
// 0x42 the finished trade, which the server takes as sent); the port runs
// both on the server with the single-player engine (gamestate.Talk with the
// LAN script rules, gamestate.Trade) and the client only shows them and
// sends the player's choices. Per player: the open dialogue, the open trade,
// the quest flags (the original keeps them in each client's memory) and,
// per settlement, the shops (the LAN settlement record keeps one stock per
// player slot).

// TalkRange is how far (world pixels, horizontally and vertically) a
// character may be from the hero to be talked to: one 800×600 screen (the
// original's client only sends 0x43 for a character under the cursor; its
// server does not check; a port rule).
const TalkRange = 800

// settlementOf is the index of map m's settlement, −1 when it has none.
func (w *World) settlementOf(m int) int {
	for i, s := range w.Game.Settlements {
		if s.Map == m && m != 0 {
			return i
		}
	}
	return -1
}

// startTalk resets slot's dialogue state for a hero entering the world:
// the quest flags as QUESTR.RES starts them, and the slot's merchant
// restocked in every village.
func (w *World) startTalk(slot int) {
	st := w.state(slot)
	st.talk, st.trade = nil, nil
	if w.Quests != nil {
		copy(st.quests[:], w.Quests.Flags())
	}
	w.restockAll(slot)
}

// restockAll is 0x0044B971: slot's merchant stock in every village is freed
// and made anew (0x0043C764, the single-player 0x00418F0C with the slot's
// hero's level).
func (w *World) restockAll(slot int) {
	for si := range w.shops {
		w.Game.FreeMerchant(&w.shops[si][slot])
		w.restock(si, slot)
	}
}

func (w *World) restock(si, slot int) {
	m := w.Game.Settlements[si].Map
	if m < 1 || m > Maps || w.Tables == nil {
		return
	}
	w.Game.WithShop(si, &w.shops[si][slot], func() {
		w.Game.Restock(m, slot, w.Tables, w.Maps[m].Rng.IntN)
	})
}

// settlementPass is the LAN settlement pass (every 16 ticks, 0x00440C70)
// of every village (gamestate.LANSettlementPass): the alarm against its
// aggressors among the players on its map, the buildings, the shops of
// every player slot and the voevoda's training. A building that changed
// state bumps buildingsVersion for the players' updates (Host).
func (w *World) settlementPass() {
	if w.Tables == nil {
		return
	}
	for si := range w.shops {
		m := w.Game.Settlements[si].Map
		if m < 1 || m > Maps {
			w.Game.LANShopPass(si, w.shops[si][:], w.Tables)
			continue
		}
		s := w.Maps[m]
		s.StoreWorld()
		if w.Game.LANSettlementPass(si, w.shops[si][:], w.Tables, s.PartyOnCells()) {
			w.buildingsVersion++
		}
		s.SyncParty()
	}
}

// Talk is a right click on character npc out of the stance (the
// original's 0x43): its dialogue opens when it is a character of the
// world (not a player's hero) on the hero's map, alive, with a dialogue
// the LAN world opens (gamestate.LANTalks),
// within TalkRange, and no other window is open.
func (w *World) Talk(slot, npc int) {
	s, mem, e := w.member(slot)
	if mem == nil || w.Quests == nil || e.InStance() {
		return
	}
	st := w.state(slot)
	if st.talk != nil || st.trade != nil || st.pile != nil {
		return
	}
	if npc < world.LANPlayers || npc >= len(w.Game.Entities) {
		return
	}
	if w.hireling(slot, npc) {
		// An own hireling (Ctrl + right click, its portrait too): its
		// dialogue (gamestate.LANMemberDialogue) wherever it stands on the
		// hero's map, as single player's party members'.
		if hm, he := s.Member(npc); hm == nil || !world.Alive(&hm.Walker, he) {
			return
		}
	} else {
		n := s.ByEntity[npc]
		if n == nil {
			return
		}
		ne := w.Game.Entities[npc]
		if ne.Dead() || !gamestate.LANTalks(ne.Dialogue) {
			return
		}
		ax, ay := mapdata.CellCenter(mem.Cell)
		bx, by := mapdata.CellCenter(n.Cell)
		if abs(ax-bx) > TalkRange || abs(ay-by) > TalkRange {
			return
		}
	}
	w.returnHand(slot)
	w.runTalk(slot, s, func(tk *gamestate.Talk) *gamestate.Dialogue {
		d := tk.OpenDialogue(npc, &st.lastGreeting)
		st.greeting = tk.Events.Greeting
		return d
	})
}

// Reply is a click on reply k of slot's dialogue.
func (w *World) Reply(slot, k int) {
	s, mem, _ := w.member(slot)
	st := w.state(slot)
	if mem == nil || st.talk == nil {
		return
	}
	d := st.talk
	if k < 0 || k >= len(d.Replies) {
		return // not offered: ignored, the dialogue stays
	}
	if wk, _ := s.Combatant(d.NPC); wk == nil || w.Game.Entities[d.NPC].Dead() {
		st.talk = nil // the character died or left the map meanwhile
		return
	}
	w.runTalk(slot, s, func(tk *gamestate.Talk) *gamestate.Dialogue {
		tk.NPC = d.NPC
		if !tk.Choose(d, k) {
			return nil
		}
		return d
	})
}

// runTalk runs f's dialogue step for slot's hero on map s with the LAN
// script rules: the hero's record and its group, its quest flags, its loan
// bit and its shops; then keeps the dialogue, or the trade that script 275
// opened in its place.
func (w *World) runTalk(slot int, s *world.World, f func(*gamestate.Talk) *gamestate.Dialogue) {
	st := w.state(slot)
	g := w.Game
	si := w.settlementOf(s.MapNumber)
	s.StoreWorld()
	saved := g.Quests
	g.Quests = st.quests
	tk := &gamestate.Talk{G: g, Q: w.Quests, T: w.Tables, Hero: slot, Map: s.MapNumber, Time: s.GameTime(),
		Rand: s.Rng.IntN, LAN: true, Party: slot, Slot: slot, PartySize: w.PartySize(slot)}
	var d *gamestate.Dialogue
	run := func() { d = f(tk) }
	if si >= 0 {
		tk.Loans = &w.loans[si]
		g.WithShop(si, &w.shops[si][slot], run)
	} else {
		run()
	}
	st.quests = g.Quests
	g.Quests = saved
	s.SyncParty()
	if mem, e := s.Member(slot); mem != nil {
		mem.Look = world.LookOf(*e)
	}
	if len(tk.Events.Unported) > 0 {
		log.Printf("lan dialogue: script functions not ported: %v", tk.Events.Unported)
	}
	for _, i := range tk.Events.Hired {
		w.hire(slot, i)
	}
	for _, i := range tk.Events.Dismissed {
		w.dismiss(slot, i)
	}
	st.talk = d
	if tk.Events.Trade != nil {
		st.talk = nil // window 7 replaces the dialogue
		gamestate.LANShopFilter(g, tk.Events.Trade)
		st.trade = tk.Events.Trade
	}
}

// openTrade is script 275 again for slot's trade (the window reopens after
// «Ok», and the server checks a deal against it): the hero's bag and the
// trader's goods as they are now.
func (w *World) openTrade(slot, si int, tr *gamestate.Trade) *gamestate.Trade {
	var out *gamestate.Trade
	open := func() {
		out = w.Game.OpenTrade(slot, slot, tr.NPC, tr.Map)
		gamestate.LANShopFilter(w.Game, out)
	}
	if si >= 0 {
		w.Game.WithShop(si, &w.shops[si][slot], open)
	} else {
		open()
	}
	return out
}

// tradeOrder is the trade's «Ok» (accept) or «Закрыть» with the client's
// lists: they must move only the bag's items (lists 0 and 1) and the
// goods as they are now (2 and 3), else nothing happens; the coins are
// balanced and the deal applied with the single-player rules
// (gamestate.Game.Apply: the bag and weight limits, «Слишком мало
// даешь!»). An accepted deal with the healer or the merchant restocks the
// slot's shop (0x0044A255); the window then shows the new state, «Закрыть»
// closes it.
func (w *World) tradeOrder(s *world.World, slot int, accept bool, lists *[netplay.PileLists][netplay.PileListSlots]int) int {
	st := w.state(slot)
	tr := st.trade
	if tr == nil {
		return 0
	}
	if wk, _ := s.Combatant(tr.NPC); wk == nil || w.Game.Entities[tr.NPC].Dead() || tr.Map != s.MapNumber {
		st.trade = nil
		return 0
	}
	si := w.settlementOf(s.MapNumber)
	fresh := w.openTrade(slot, si, tr)
	if lists != nil {
		if !samePileItems(fresh.Lists, *lists) {
			st.trade = fresh // out of date: show the window as it is now
			return 0
		}
		fresh.Lists = *lists
	}
	msg, ok := 0, false
	apply := func() {
		w.Game.Balance(w.Tables, fresh)
		s.StoreWorld()
		msg, ok, _ = w.Game.Apply(w.Tables, fresh, accept, s.GameTime())
		s.SyncParty()
	}
	if si >= 0 {
		w.Game.WithShop(si, &w.shops[si][slot], apply)
	} else {
		apply()
	}
	if !ok {
		if !accept {
			st.trade = nil
		}
		return msg
	}
	if !accept {
		st.trade = nil
		return 0
	}
	if si >= 0 && (fresh.Mode == 2 || fresh.Mode == -2 || fresh.Mode == 3 || fresh.Mode == -3) {
		w.restock(si, slot)
	}
	st.trade = w.openTrade(slot, si, fresh)
	return 0
}

// Dialogue is slot's dialogue window (MsgDialogue).
func (w *World) Dialogue(slot int) netplay.DialoguePayload {
	st := w.state(slot)
	d := st.talk
	if d == nil {
		return netplay.DialoguePayload{}
	}
	out := netplay.DialoguePayload{Open: true, NPC: d.NPC, Lines: d.Lines, Greeting: st.greeting}
	for _, r := range d.Replies {
		out.Replies = append(out.Replies, r.Text)
	}
	return out
}

// tradeWindow is slot's trade as window 7's payload.
func (w *World) tradeWindow(st *itemState) netplay.PileWindowPayload {
	tr := st.trade
	out := netplay.PileWindowPayload{Open: true, Pile: -1, Map: tr.Map, Lists: tr.Lists, Items: map[int]netplay.ItemRecord{}}
	out.Trader = &netplay.TraderState{NPC: tr.NPC, Mode: tr.Mode, HeroMoney: tr.HeroMoney, OtherMoney: tr.OtherMoney,
		Skill: w.Game.Entities[tr.NPC].Skills[2]}
	for _, l := range tr.Lists {
		for _, i := range l {
			if i > 0 {
				out.Items[i] = netplay.ItemRecord(w.Game.ItemRecord(i))
			}
		}
	}
	return out
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// QuestLog is slot's quest log (the Q window): the texts of its set quest
// flags (gamestate.QuestLogOf on the player's flags).
func (w *World) QuestLog(slot int) []uint32 {
	if _, ok := w.players[slot]; !ok {
		return nil
	}
	return gamestate.QuestLogOf(w.state(slot).quests[:])
}
