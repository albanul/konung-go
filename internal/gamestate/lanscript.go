package gamestate

import "konung1.5/internal/resfmt"

// Dialogue script functions in the LAN version (docs/konung_lan.md#dialogues-and-trade).
//
// LAN_KONG.EXE's client runs the dialogue itself (QUESTR.RES, the
// single-player engine) with its own script dispatcher 0x0042AF60: the five
// families, then the table 0x0047F418 (8 bytes per id from 0x112: the
// function at +8, a flag at +4 that also sends the id − 0x112 to the server
// as message 0x53, 0x0042CB2C). The server applies the sent ones (0x0044BCC4)
// to its hero record. The port runs the dialogue on the server with these
// rules (Talk.LAN):
//
//   - 0–111 (a group hurt, a map cleared): 0, the families are not run.
//   - 112–165 take a quest item: 0 (client stub 0x0042BCEC; the server's
//     switch has no case for the ids 0x64+ it is sent: nothing is taken).
//   - 166–219 give a quest item: 1 (client 0x0042B074 returns 1; the server
//     ignores ids 0xC8+): nothing is given.
//   - 220–273 has a quest item: the bag check (0x0042BD0C).
//   - the table's stubs (0x0042B054, return 0): lanStubs.
//   - 282, 284, 333: the loan is per player, bit slot of the settlement's
//     word +0x4C (server 0x0044BD9A, 0x0044BE1F; client 0x0042BF04).
//   - 334, 335: the server sets the hero's HP to 1600 (0x0044C374,
//     0x0044C38D); the healer's skill does not grow.
//   - 363: the client returns 1, the server's case is empty: nobody
//     vanishes.
//   - 291 hire: Events.Hired, the server moves the character into the
//     player's group (0x0044BED7, lanserver); 332 dismiss: not reached by
//     the original's LAN dialogues; the port's own hireling dialogue
//     (LANMemberDialogue) reaches it: Events.Dismissed.
//   - 349 party full: the hero and its hirelings (PartySize) against
//     LANMaxParty (0x0042BFA0).
//   - the rest: the single-player functions (scriptFuncs), on the hero's
//     own group (Talk.Party).

// lanStubs are the LAN table's ids whose function is the stub 0x0042B054.
var lanStubs = map[int]bool{
	274: true, 278: true, 279: true, 280: true, 281: true, 283: true, 292: true, 304: true, 311: true,
	314: true, 315: true, 316: true, 317: true, 319: true, 320: true, 321: true, 322: true, 323: true,
	326: true, 327: true, 328: true, 336: true, 337: true, 338: true, 339: true, 340: true, 341: true,
	342: true, 343: true, 345: true, 346: true, 347: true, 348: true, 353: true, 354: true, 355: true,
	356: true, 357: true, 358: true, 359: true, 360: true, 361: true, 365: true,
}

// LANRepayLoan is the price of 282 (server 0x0044BDEB: 0x78).
const LANRepayLoan = 120

func (t *Talk) lanScript(id int) int {
	switch {
	case id < scriptTakeQuest:
		return 0
	case id < scriptGiveQuest:
		return 0
	case id < scriptHasQuest:
		return 1
	case id < scriptTable:
		return b2i(t.hasQuestItem(id - scriptHasQuest))
	case lanStubs[id]:
		return 0
	}
	bit := uint16(1) << uint(t.Slot&15)
	switch id {
	case 282:
		if t.Loans == nil || *t.Loans&bit == 0 || t.hero().Gold < LANRepayLoan {
			return 0
		}
		*t.Loans &^= bit
		t.hero().Gold -= LANRepayLoan
		return 1
	case 284:
		if t.Loans != nil && *t.Loans&bit == 0 {
			*t.Loans |= bit
			t.hero().Gold += 100
		}
		return 0
	case 333:
		return b2i(t.Loans != nil && *t.Loans&bit != 0)
	case 334, 335:
		t.hero().HP = HPFull
		return 1
	case 363:
		return 1
	case 291:
		// The server's 0x0044BED7 moves the character into the player's
		// group; the client's 0x0042B094 returns 1. Already the player's:
		// nothing more.
		if t.npc().Group != t.Party {
			t.Events.Hired = append(t.Events.Hired, t.NPC)
		}
		return 1
	case 332:
		// Dismiss (server 0x0044C154: back to the current map's village
		// group): in the original only the roles' dialogues offer it, for
		// a role character in the party, which the LAN dialogues never
		// hire. The port offers it to the own hirelings
		// (LANMemberDialogue): Events.Dismissed, the server sends the
		// hireling home (lanserver).
		if t.NPC >= LANPlayerSlots && t.npc().Group == t.Party {
			t.Events.Dismissed = append(t.Events.Dismissed, t.NPC)
			return 1
		}
		return 0
	case 349:
		// 0x0042BFA0: the player group's member count against
		// «Макс. отряд» (0x0044D798, the hero's class and base Хар).
		h := t.hero()
		return b2i(t.PartySize >= LANMaxParty(h.Flags4&7, h.BaseStats[0]))
	}
	if f, ok := scriptFuncs[id]; ok {
		return f(t)
	}
	t.Events.Unported = append(t.Events.Unported, id)
	return 0
}

// LAN dialogues the port opens (a port rule, docs/konung_lan.md#dialogues-and-trade).
// GAME.RES gives its characters single-player dialogue numbers: the post
// roles' shared dialogues fit (100 the voevoda, 101 the healer, 102 the
// smith, 103 the merchant: training, healing, trade), but the elders (105,
// 106, 107), the guards (104), the villagers (109, 110, 111), the animals
// (108, 112) and the three objects (6) open single-player quest dialogues
// that do not belong to the network game. The installed QUESTR.RES files
// are both the single-player one, and the original opens the number as it
// is (0x0042CA18), so it would show those. The port opens the roles'
// dialogues, and for the plain villagers (109–111, the original server's
// hirelings) single player's village hire dialogue 1 (LANHireDialogue):
// the price by level and village (310), the party limit (349), hire (291).
// Elders, guards, animals and objects are silent.
const (
	LANTalkFirst = 100
	LANTalkLast  = 103
	// LANHireDialogue is single player's dialogue of a villager to hire
	// (the same as 8 and 9 up to the refusal's last words).
	LANHireDialogue = 1
	// LANMemberDialogue is what an own hireling says (Ctrl + right click,
	// a port rule: neither game lets a plain hireling go): single
	// player's dialogue 3, «Слушаю тебя, князь!», whose first reply
	// dismisses it (332) and second ends the talk.
	LANMemberDialogue = 3
)

// LANDialogue is the dialogue a character with GAME.RES dialogue d opens
// in a LAN world, NoDialogue for none.
func LANDialogue(d int) int {
	switch {
	case d >= LANTalkFirst && d <= LANTalkLast:
		return d
	case d >= 109 && d <= 111:
		return LANHireDialogue
	}
	return NoDialogue
}

// LANTalks reports whether a character with GAME.RES dialogue d can be
// talked to in a LAN world.
func LANTalks(d int) bool { return LANDialogue(d) != NoDialogue }

// LAN shops: what the port does not sell in a network game (the original
// server's rules, docs/konung_lan.md#dialogues-and-trade): the misc items
// removed from it (Яблоко познания, Дупликатор, Лопата, Медное зеркало
// колдуна, Соколиная лапка) and the herbs for poisoned weapons and
// incendiary arrows (Яд, Масло).
func LANNotSold(g *Game, i int) bool {
	it, ok := g.Item(i)
	if !ok || i == 0 {
		return false
	}
	switch it.Category {
	case CatQuest:
		switch it.Type {
		case MiscApple, MiscDuplicator, MiscShovel, MiscMirror, MiscFalconFoot:
			return true
		}
	case CatHerb:
		return it.Type == HerbPoison || it.Type == HerbOil
	}
	return false
}

// LANShopFilter takes what LANNotSold names out of a shop trade's goods
// (the list closes up); barter and pile windows are left alone.
func LANShopFilter(g *Game, tr *Trade) {
	if tr == nil || tr.NPC < 0 || tr.Mode == 6 {
		return
	}
	var keep [TradeSlots]int
	k := 0
	for _, i := range tr.Lists[TradeGoods] {
		if i != 0 && !LANNotSold(g, i) {
			keep[k] = i
			k++
		}
	}
	tr.Lists[TradeGoods] = keep
}

// LANShop is one player slot's shops in a LAN settlement (the LAN record
// keeps them per slot: healer +0x448 + 0x38·slot, merchant +0x7C8 +
// 0x40·slot, smith +0xBC8 + 0x2E·slot, the smith's job +0xEA9 + slot). The
// port runs the single-player shop code on them by laying them over the
// GAME.N layout's single shop (+0x40C, +0x444, +0x484, +0x4B4) for the
// call. SmithTimer is the smith's job timer (single player's +8); where the
// LAN server keeps it per slot was not traced (a port guess).
type LANShop struct {
	Healer     [0x38]byte
	Merchant   [0x40]byte
	Smith      [0x2E]byte
	Job        byte
	SmithTimer int32
}

// Shop is slot's shops of LAN settlement i as GAME.RES has them.
func (w *LANWorld) Shop(i, slot int) LANShop {
	r := w.Settlements[i][:]
	var sh LANShop
	copy(sh.Healer[:], r[lanSetHealer+0x38*slot:])
	copy(sh.Merchant[:], r[lanSetMerchant+0x40*slot:])
	copy(sh.Smith[:], r[lanSetSmith+0x2E*slot:])
	sh.Job = r[lanSetSmithJob+slot]
	return sh
}

// Loans is LAN settlement i's loan bits (+0x4C, one per player slot).
func (w *LANWorld) Loans(i int) uint16 {
	return uint16(u16(w.Settlements[i][:], lanSetLoans))
}

// load lays sh over s's single-player shop; save takes it back.
func (s *Settlement) load(sh *LANShop) {
	copy(s.Raw[0x40C:], sh.Healer[:])
	copy(s.Raw[0x444:], sh.Merchant[:])
	copy(s.Raw[0x484:], sh.Smith[:])
	s.Raw[smithJobOff] = sh.Job
	s.setI32(smithTimerOff, int(sh.SmithTimer))
}

func (s *Settlement) save(sh *LANShop) {
	copy(sh.Healer[:], s.Raw[0x40C:])
	copy(sh.Merchant[:], s.Raw[0x444:])
	copy(sh.Smith[:], s.Raw[0x484:])
	sh.Job = s.Raw[smithJobOff]
	sh.SmithTimer = int32(s.i32(smithTimerOff))
}

// WithShop runs f with sh laid over settlement i's shop (OpenTrade, Apply,
// Restock read and write it there), then keeps what f changed in sh.
func (g *Game) WithShop(i int, sh *LANShop, f func()) {
	s := &g.Settlements[i]
	var old LANShop
	s.save(&old)
	s.load(sh)
	f()
	s.save(sh)
	s.load(&old)
}

// LANShopPass is the LAN settlement pass's shops (0x00440C70) for
// settlement i: the healer (0x0043ABCC) and the smith (0x0043AF9C) of every
// player slot, on the slot's stock; the healer's cycle timer (+4) is the
// settlement's, counted once per pass.
func (g *Game) LANShopPass(i int, shops []LANShop, t *resfmt.ExeTables) {
	s := &g.Settlements[i]
	if s.Map == 0 {
		return
	}
	timer := s.i32(healerTimerOff)
	for k := range shops {
		s.setI32(healerTimerOff, timer)
		g.WithShop(i, &shops[k], func() {
			g.healerPass(s, t)
			g.smithPass(s, t)
		})
	}
}

// ClearShop frees sh's items and empties it (the session reset 0x00446AD1
// clears the stocks; the port also frees the items).
func (g *Game) ClearShop(sh *LANShop) {
	free := func(b []byte, n int) {
		for j := range n {
			if i := int(int16(uint16(b[2*j]) | uint16(b[2*j+1])<<8)); i > 0 {
				g.items[i*itemStride] = 0xFF
			}
		}
	}
	free(sh.Healer[6:], 25)
	free(sh.Merchant[:], 32)
	free(sh.Smith[:], 23)
	*sh = LANShop{Job: 0xFF}
}

// FreeMerchant frees the merchant's stock of sh (0x0044B9B7, before the
// slot's restock).
func (g *Game) FreeMerchant(sh *LANShop) {
	for j := range 32 {
		b := sh.Merchant[2*j:]
		if i := int(int16(uint16(b[0]) | uint16(b[1])<<8)); i > 0 {
			g.items[i*itemStride] = 0xFF
		}
		b[0], b[1] = 0, 0
	}
}
