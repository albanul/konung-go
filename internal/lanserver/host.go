package lanserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

// Host connects the world to the session (netplay.Server): it puts an
// entering player's hero into a free slot (ServerConfig.Start), takes its
// orders (ServerConfig.OnOrder), steps the world to the server clock and
// sends every player what happens on its hero's map: the characters'
// states (MsgView), the other players' heroes as they look (MsgPlayer),
// sounds (MsgEvents), its own map changes (MsgMapChange) and its values
// after a kill (MsgHero, the original's 0x5C).
type Host struct {
	mu      sync.Mutex
	W       *World
	send    func(id string, m *netplay.Message) error
	players map[string]*hostPlayer // by player ID
	bySlot  map[int]*hostPlayer

	// OnKill is called (without the lock) after a player's hero killed
	// a character: the killer's account values after the kill (its
	// experience; nil without accounts) and, for a player victim, the
	// victim. The dedicated server scores and saves (0x0043B67C).
	OnKill func(k Kill)
	// Load gives an entering player's saved items (SAVITEMS.DAT,
	// 0x0044444C; called under the lock), or starter: a new or rebuilt
	// hero, which keeps the starting knife (AddPlayer). Without Load every
	// hero starts with the knife. Save keeps a player's hero (its account
	// values, nil without accounts) and items (0x00444918; called without
	// the lock) when it leaves the world and at SaveAll (the session's
	// end). Died keeps a hero that died (HP 0, whoever or whatever killed
	// it): its account values and the items it wore (equipment,
	// jewellery, arrows; the bag lies in the pile at the death spot, open
	// to anyone, the owner too). The original server: re-entering after
	// death, the hero comes naked; a session ending while it is dead
	// revives it with the worn items and an empty bag.
	Load func(p netplay.PlayerInfo) (items []Item, starter bool)
	Save func(p netplay.PlayerInfo, hero *netplay.Hero, items []Item)
	Died func(p netplay.PlayerInfo, hero *netplay.Hero, worn []Item)
}

// save is a Save call to make after the lock.
type save struct {
	p     netplay.PlayerInfo
	hero  *netplay.Hero
	items []Item
	died  bool // Died, not Save
}

// Kill is a kill by a player's hero.
type Kill struct {
	Killer       netplay.PlayerInfo
	KillerHero   *netplay.Hero
	KillerLevel  int
	Victim       netplay.PlayerInfo // set when VictimPlayer
	VictimPlayer bool
	VictimLevel  int
}

type hostPlayer struct {
	info  netplay.PlayerInfo
	hero  *netplay.Hero
	slot  int
	dead  bool // died; kept by Died, not saved again
	view  Viewer
	looks map[int]netplay.Look // players' heroes sent to this player, as sent
	// The items, piles and pile window as last sent (JSON), so only
	// changes go out.
	inv, piles, pileWin []byte
}

// NewHost connects w; send is the session's Server.Send.
func NewHost(w *World, send func(id string, m *netplay.Message) error) *Host {
	return &Host{W: w, send: send, players: map[string]*hostPlayer{}, bySlot: map[int]*hostPlayer{}}
}

// Start is ServerConfig.Start: the player's hero (hero: the account's;
// p.Model the type picked at «Ok») in the first free slot, on the type's
// start. A dead hero's player entering again gets a fresh one.
func (h *Host) Start(p netplay.PlayerInfo, hero *netplay.Hero) (netplay.EnterPayload, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	hp := h.players[p.ID]
	if hp == nil {
		slot := -1
		for k := range world.LANPlayers {
			if h.bySlot[k] == nil {
				slot = k
				break
			}
		}
		if slot < 0 {
			return netplay.EnterPayload{}, fmt.Errorf("the world is full (%d heroes)", world.LANPlayers)
		}
		hp = &hostPlayer{slot: slot}
		h.players[p.ID], h.bySlot[slot] = hp, hp
	}
	hp.info, hp.hero, hp.dead = p, hero, false
	hp.view, hp.looks = Viewer{}, map[int]netplay.Look{}
	hp.inv, hp.piles, hp.pileWin = nil, nil, nil
	m, c, err := h.W.AddPlayer(hp.slot, p.Model, hero)
	if err != nil {
		delete(h.players, p.ID)
		delete(h.bySlot, hp.slot)
		return netplay.EnterPayload{}, err
	}
	if h.Load != nil {
		if items, starter := h.Load(p); !starter {
			h.W.LoadItems(hp.slot, items) // none: naked
		}
	}
	e, _ := h.W.Hero(hp.slot)
	return netplay.EnterPayload{Position: netplay.Position{Map: m, Row: c.Row, Col: c.Col, Facing: e.Variant}, Slot: hp.slot}, nil
}

// Slot is player id's slot, false when it has no hero.
func (h *Host) Slot(id string) (int, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if hp := h.players[id]; hp != nil {
		return hp.slot, true
	}
	return 0, false
}

// Leave takes player id's hero out of the world (ServerConfig.OnLeave).
func (h *Host) Leave(id string) {
	h.mu.Lock()
	var saves []save
	if hp := h.players[id]; hp != nil {
		if !hp.dead {
			saves = append(saves, h.saveOf(hp))
		}
		h.W.RemovePlayer(hp.slot)
		delete(h.players, id)
		delete(h.bySlot, hp.slot)
	}
	h.mu.Unlock()
	h.save(saves)
}

// SaveAll keeps every player's hero and items (the session's end,
// 0x0043459D).
func (h *Host) SaveAll() {
	h.mu.Lock()
	var saves []save
	for _, hp := range h.players {
		if !hp.dead {
			saves = append(saves, h.saveOf(hp))
		}
	}
	h.mu.Unlock()
	h.save(saves)
}

// saveOf is what Save gets for hp now; h.mu held.
func (h *Host) saveOf(hp *hostPlayer) save {
	s := save{p: hp.info, items: h.W.SavedItems(hp.slot)}
	if hp.hero != nil {
		hero := h.W.Progress(hp.slot, *hp.hero)
		hp.hero = &hero
		s.hero = &hero
	}
	return s
}

func (h *Host) save(saves []save) {
	for _, s := range saves {
		switch {
		case s.died && h.Died != nil:
			h.Died(s.p, s.hero, s.items)
		case !s.died && h.Save != nil:
			h.Save(s.p, s.hero, s.items)
		}
	}
}

// deaths finds the players' heroes that died since the last tick (HP 0,
// whoever or whatever killed them) and returns their Died saves: the worn
// items only (DropLoot put the bag into the pile); h.mu held.
func (h *Host) deaths() []save {
	var out []save
	for _, hp := range h.players {
		if hp.dead {
			continue
		}
		if e, ok := h.W.Hero(hp.slot); !ok || e.HP > 0 {
			continue
		}
		hp.dead = true
		h.W.closeItems(hp.slot)
		s := h.saveOf(hp)
		s.died = true
		worn := s.items[:0:0]
		for _, it := range s.items {
			if it.Slot != SlotBag {
				worn = append(worn, it)
			}
		}
		s.items = worn
		out = append(out, s)
	}
	return out
}

// Order is ServerConfig.OnOrder: an order for p's hero.
func (h *Host) Order(p netplay.PlayerInfo, o netplay.OrderPayload) {
	h.mu.Lock()
	defer h.mu.Unlock()
	hp := h.players[p.ID]
	if hp == nil {
		return
	}
	switch o.Kind {
	case netplay.OrderWalk:
		h.W.Walk(hp.slot, o.Row, o.Col)
	case netplay.OrderRight:
		h.W.Right(hp.slot, o.Target, o.Row, o.Col)
	case netplay.OrderStance:
		h.W.Stance(hp.slot, o.On)
	case netplay.OrderWeapon:
		h.W.Weapon(hp.slot, o.Mode)
	case netplay.OrderAttack:
		h.W.Attack(hp.slot)
	case netplay.OrderItem:
		h.W.Item(hp.slot, o)
	}
}

// Jump puts the world just behind tick target (keep ticks before it) when
// it is more than far behind, and returns the ticks jumped over.
func (h *Host) Jump(target, far, keep int) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if behind := target - h.W.Tick; behind > far {
		h.W.Tick = target - keep
		return behind - keep
	}
	return 0
}

// Snapshots are every map's characters and the tick, under the lock.
func (h *Host) Snapshots() ([Maps + 1]MapState, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out [Maps + 1]MapState
	for n := 1; n <= Maps; n++ {
		out[n] = h.W.Snapshot(n)
	}
	return out, h.W.Tick
}

// Advance steps the world toward tick target, at most max ticks, and sends
// the players what changed. It returns the ticks run.
func (h *Host) Advance(target, max int) int {
	h.mu.Lock()
	n := 0
	for ; h.W.Tick < target && n < max; n++ {
		h.W.Step()
	}
	kills, saves := h.report()
	saves = append(saves, h.deaths()...)
	h.mu.Unlock()
	if h.OnKill != nil {
		for _, k := range kills {
			h.OnKill(k)
		}
	}
	h.save(saves)
	return n
}

// report sends the tick's events, views and item changes; h.mu held. It
// returns the kills for OnKill and the dead players' saves.
func (h *Host) report() ([]Kill, []save) {
	var kills []Kill
	var saves []save
	byMap := map[int][]netplay.Event{}
	for _, ev := range h.W.TakeEvents() {
		switch ev.Kind {
		case EventSound:
			byMap[ev.Map] = append(byMap[ev.Map], netplay.Event{Kind: netplay.EventSound, Sound: ev.Sound, Row: ev.Row, Col: ev.Col})
		case EventAction:
			byMap[ev.Map] = append(byMap[ev.Map], netplay.Event{Kind: netplay.EventAction, Entity: ev.Entity, Act: ev.Act, Row: ev.Row, Col: ev.Col})
		case EventMapChange:
			if hp := h.bySlot[ev.Entity]; hp != nil {
				h.W.state(hp.slot).pile = nil // the pile window stays behind
				h.sendTo(hp, netplay.MsgMapChange, netplay.EnterPayload{
					Position: netplay.Position{Map: ev.Map, Row: ev.Row, Col: ev.Col, Facing: ev.Facing}, Slot: hp.slot})
			}
		case EventKill:
			hp := h.bySlot[ev.Entity]
			if hp == nil {
				continue
			}
			k := Kill{Killer: hp.info, KillerLevel: h.W.Level(hp.slot), VictimLevel: h.W.Level(ev.Victim)}
			if hp.hero != nil {
				hero := h.W.Progress(hp.slot, *hp.hero)
				hp.hero = &hero
				k.KillerHero = &hero
				h.sendTo(hp, netplay.MsgHero, hero)
			}
			if v := h.bySlot[ev.Victim]; v != nil && ev.VictimPlayer {
				k.Victim, k.VictimPlayer = v.info, true // its death: deaths
			}
			kills = append(kills, k)
		}
	}
	for _, hp := range h.players {
		view, ok := h.W.View(hp.slot, &hp.view)
		if !ok {
			continue
		}
		// The players' heroes on its map it has not seen, or whose look
		// changed (an item taken off or put on, worn out) even while they
		// stand still.
		for _, o := range h.players {
			if o == hp || h.W.PlayerMap(o.slot) != view.Map {
				continue
			}
			l, ok := h.W.PlayerLook(o.slot)
			if !ok {
				continue
			}
			if old, seen := hp.looks[o.slot]; !seen || old != l {
				hp.looks[o.slot] = l
				h.sendTo(hp, netplay.MsgPlayer, netplay.PlayerPayload{Slot: o.slot, Player: o.info, Hero: o.hero, Look: l})
			}
		}
		evs := byMap[view.Map]
		for _, text := range h.W.TakeMessages(hp.slot) {
			evs = append(evs, netplay.Event{Kind: netplay.EventMsg, Text: text})
		}
		if len(evs) > 0 {
			h.sendTo(hp, netplay.MsgEvents, netplay.EventsPayload{Events: evs})
		}
		h.sendTo(hp, netplay.MsgView, view)
	}
	for _, hp := range h.players {
		h.sendItems(hp)
	}
	return kills, saves
}

// sendItems sends hp its hero's items, its map's piles and its pile window
// when they changed since the last send; h.mu held.
func (h *Host) sendItems(hp *hostPlayer) {
	inv, ok := h.W.Inventory(hp.slot)
	if !ok {
		return
	}
	changed := func(last *[]byte, v any) bool {
		b, err := json.Marshal(v)
		if err != nil || bytes.Equal(b, *last) {
			return false
		}
		*last = b
		return true
	}
	if changed(&hp.inv, inv) {
		h.sendTo(hp, netplay.MsgInventory, inv)
	}
	if piles := h.W.Piles(h.W.PlayerMap(hp.slot)); changed(&hp.piles, piles) {
		h.sendTo(hp, netplay.MsgPiles, piles)
	}
	if pw := h.W.PileWindow(hp.slot); changed(&hp.pileWin, pw) {
		h.sendTo(hp, netplay.MsgPileWindow, pw)
	}
}

func (h *Host) sendTo(hp *hostPlayer, t netplay.MsgType, payload any) {
	if h.send == nil {
		return
	}
	m, err := netplay.NewMessage(t, payload)
	if err == nil {
		h.send(hp.info.ID, m)
	}
}
