package lanserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

// LingerTime is how long a hero stays in the world after its player left
// it away from the hero's home village (the original server: "оставался
// там неконтролируем 3 минуты").
const LingerTime = 3 * time.Minute

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
	// OnAmulet is called (without the lock) when a player delivered the
	// amulet for its nation (0x00437B69: 25 points to the player); OnWin
	// when a nation reached the win limit (0x00434438): the session is to
	// end now (the dedicated server ends it as at its time).
	OnAmulet func(p netplay.PlayerInfo, nation int)
	OnWin    func(nation int)

	// Linger is how long a hero stays after its player left away from its
	// home village (LingerTime when 0); now is the clock (time.Now when
	// nil), for tests.
	Linger time.Duration
	now    func() time.Time
	// lingering are the heroes whose players left, by account name, until
	// their time is up; the player cannot enter again meanwhile.
	lingering map[string]*hostPlayer
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
	Killer         netplay.PlayerInfo
	KillerHero     *netplay.Hero
	KillerLevel    int
	Victim         netplay.PlayerInfo // set when VictimPlayer
	VictimPlayer   bool
	VictimHireling bool // a player's hireling (counted, 0 points)
	VictimAmulet   bool // the victim hero carried an amulet item: double points
	VictimLevel    int
}

// delivery is an OnAmulet call to make after the lock.
type delivery struct {
	p      netplay.PlayerInfo
	nation int
}

type hostPlayer struct {
	info  netplay.PlayerInfo
	hero  *netplay.Hero
	slot  int
	dead  bool      // died; kept by Died, not saved again
	until time.Time // lingering after its player left: removed then
	view  Viewer
	looks map[int]netplay.Look // players' heroes sent to this player, as sent
	// The items, piles, pile window, dialogue and party as last sent
	// (JSON), so only changes go out.
	inv, piles, pileWin, dialogue, party, amulets []byte
	// The building states sent (by map and placement), the map and the
	// buildings version checked last (buildings.go).
	buildings      map[buildingKey]int
	bmap, bversion int
}

// NewHost connects w; send is the session's Server.Send.
func NewHost(w *World, send func(id string, m *netplay.Message) error) *Host {
	return &Host{W: w, send: send, players: map[string]*hostPlayer{}, bySlot: map[int]*hostPlayer{},
		lingering: map[string]*hostPlayer{}}
}

func (h *Host) clock() time.Time {
	if h.now != nil {
		return h.now()
	}
	return time.Now()
}

func (h *Host) linger() time.Duration {
	if h.Linger > 0 {
		return h.Linger
	}
	return LingerTime
}

// Start is ServerConfig.Start: the player's hero (hero: the account's;
// p.Model the type picked at «Ok») in the first free slot, on the type's
// start. A dead hero's player entering again gets a fresh one.
func (h *Host) Start(p netplay.PlayerInfo, hero *netplay.Hero) (netplay.EnterPayload, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lingering[p.Name] != nil {
		// Its hero is still in the world (the original's message 8).
		return netplay.EnterPayload{}, netplay.ErrHeroLeaving
	}
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
	hp.inv, hp.piles, hp.pileWin, hp.dialogue, hp.party, hp.amulets = nil, nil, nil, nil, nil, nil
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

// Leave is player id leaving (ServerConfig.OnLeave): in its hero's home
// village (map model·3 + 1), or dead, the hero goes at once (saved unless
// dead: Died kept it); elsewhere it stays in the world uncontrolled for
// Linger (it can be attacked and killed) and goes then (expire). Its
// player cannot enter again meanwhile (Start: ErrHeroLeaving).
func (h *Host) Leave(id string) {
	h.mu.Lock()
	var saves []save
	if hp := h.players[id]; hp != nil {
		delete(h.players, id)
		if hp.dead || h.W.PlayerMap(hp.slot) == gamestate.LANStartMap(hp.info.Model) {
			saves = h.remove(hp, saves)
		} else {
			hp.until = h.clock().Add(h.linger())
			h.lingering[hp.info.Name] = hp
		}
	}
	h.mu.Unlock()
	h.save(saves)
}

// remove takes hp's hero out of the world, saving it unless dead; h.mu
// held.
func (h *Host) remove(hp *hostPlayer, saves []save) []save {
	if !hp.dead {
		saves = append(saves, h.saveOf(hp))
	}
	h.W.RemovePlayer(hp.slot)
	delete(h.bySlot, hp.slot)
	if h.lingering[hp.info.Name] == hp {
		delete(h.lingering, hp.info.Name)
	}
	return saves
}

// expire removes the lingering heroes whose time is up; h.mu held.
func (h *Host) expire(saves []save) []save {
	now := h.clock()
	for _, hp := range h.lingering {
		if !now.Before(hp.until) {
			saves = h.remove(hp, saves)
		}
	}
	return saves
}

// Lingering reports whether the hero of account name is still in the
// world after its player left.
func (h *Host) Lingering(name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lingering[name] != nil
}

// EndSession is the session's end (0x00434438, then 0x0044687C and the
// next session's start 0x00448BB8): the heroes left behind by their
// players go (saved, as 0x00443C1C), the world is built anew from GAME.RES
// with every hero in the world on its start, alive, healed, with its items
// (World.Reset), and those heroes are saved. Each player then gets its map
// whole again: a map change when its hero was elsewhere, every character,
// the piles, its items and party.
func (h *Host) EndSession() error {
	h.mu.Lock()
	var saves []save
	for _, hp := range h.lingering {
		saves = h.remove(hp, saves)
	}
	err := h.W.Reset()
	for _, hp := range h.players {
		hp.dead = false
		hp.view, hp.looks = Viewer{}, map[int]netplay.Look{}
		hp.inv, hp.piles, hp.pileWin, hp.dialogue, hp.party, hp.amulets = nil, nil, nil, nil, nil, nil
		saves = append(saves, h.saveOf(hp))
	}
	h.mu.Unlock()
	h.save(saves)
	return err
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
// whoever or whatever killed them; lingering ones too) and returns their
// Died saves: the worn items only (DropLoot put the bag into the pile); a
// dead hero that came back («Слова Воскрешения») is saved alive (Save,
// which clears the account's dead flag); h.mu held.
func (h *Host) deaths() []save {
	var out []save
	for _, hp := range h.bySlot {
		if hp.dead {
			if e, ok := h.W.Hero(hp.slot); ok && e.HP > 0 && !e.Dead() {
				hp.dead = false
				out = append(out, h.saveOf(hp))
			}
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
		h.W.Walk(hp.slot, o.Sel, o.Row, o.Col)
	case netplay.OrderRight:
		h.W.Right(hp.slot, o.Sel, o.Target, o.Row, o.Col)
	case netplay.OrderStance:
		h.W.Stance(hp.slot, o.Sel, o.On)
	case netplay.OrderWeapon:
		h.W.Weapon(hp.slot, o.Sel, o.Mode)
	case netplay.OrderAttack:
		h.W.Attack(hp.slot)
	case netplay.OrderGather:
		h.W.Gather(hp.slot)
	case netplay.OrderItem:
		h.W.Item(hp.slot, o)
	case netplay.OrderTalk:
		h.W.Talk(hp.slot, o.Target)
	case netplay.OrderReply:
		h.W.Reply(hp.slot, o.Index)
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
	kills, delivered, saves := h.report()
	saves = append(saves, h.deaths()...)
	saves = h.expire(saves)
	nation, won := h.W.TakeWin()
	h.mu.Unlock()
	if h.OnKill != nil {
		for _, k := range kills {
			h.OnKill(k)
		}
	}
	if h.OnAmulet != nil {
		for _, d := range delivered {
			h.OnAmulet(d.p, d.nation)
		}
	}
	h.save(saves)
	if won && h.OnWin != nil {
		h.OnWin(nation)
	}
	return n
}

// report sends the tick's events, views and item changes; h.mu held. It
// returns the kills for OnKill, the deliveries for OnAmulet and the dead
// players' saves.
func (h *Host) report() ([]Kill, []delivery, []save) {
	var kills []Kill
	var delivered []delivery
	var saves []save
	byMap := map[int][]netplay.Event{}
	for _, ev := range h.W.TakeEvents() {
		switch ev.Kind {
		case EventSound:
			byMap[ev.Map] = append(byMap[ev.Map], netplay.Event{Kind: netplay.EventSound, Sound: ev.Sound, Row: ev.Row, Col: ev.Col})
		case EventAction:
			byMap[ev.Map] = append(byMap[ev.Map], netplay.Event{Kind: netplay.EventAction, Entity: ev.Entity, Act: ev.Act, Row: ev.Row, Col: ev.Col})
		case EventArrow:
			a := ev.Arrow
			byMap[ev.Map] = append(byMap[ev.Map], netplay.Event{Kind: netplay.EventArrow, Entity: ev.Entity, Tick: ev.Tick,
				Arrow: &netplay.Arrow{X: a.X, Y: a.Y, VX: a.VX, VY: a.VY, Shooter: a.Shooter, Facing: a.Facing, Life: a.Life, Special: a.Special}})
		case EventMapChange:
			if hp := h.bySlot[ev.Entity]; hp != nil {
				st := h.W.state(hp.slot)
				st.pile, st.talk, st.trade = nil, nil, nil // the windows stay behind
				hp.party = nil                             // the client's new scene gets its party again
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
			k.VictimHireling, k.VictimAmulet = ev.VictimHireling, ev.VictimAmulet
			kills = append(kills, k)
		case EventAmulet:
			if hp := h.bySlot[ev.Entity]; hp != nil {
				delivered = append(delivered, delivery{p: hp.info, nation: ev.Victim})
			}
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
		if hp.bmap != view.Map || hp.bversion != h.W.buildingsVersion {
			if hp.buildings == nil {
				hp.buildings = map[buildingKey]int{}
			}
			hp.bmap, hp.bversion = view.Map, h.W.buildingsVersion
			evs = append(evs, h.W.buildingUpdates(view.Map, hp.buildings)...)
		}
		for _, text := range h.W.TakeMessages(hp.slot) {
			evs = append(evs, netplay.Event{Kind: netplay.EventMsg, Text: text})
		}
		if len(evs) > 0 {
			h.sendTo(hp, netplay.MsgEvents, netplay.EventsPayload{Events: evs})
		}
		h.sendTo(hp, netplay.MsgView, view)
	}
	amulets := h.W.Amulets()
	for _, hp := range h.players {
		h.sendItems(hp)
		if b, err := json.Marshal(amulets); err == nil && !bytes.Equal(b, hp.amulets) {
			hp.amulets = b
			h.sendTo(hp, netplay.MsgAmulets, amulets)
		}
	}
	return kills, delivered, saves
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
	if d := h.W.Dialogue(hp.slot); changed(&hp.dialogue, d) {
		h.sendTo(hp, netplay.MsgDialogue, d)
	}
	if p := (netplay.PartyPayload{Hirelings: h.W.Party(hp.slot)}); changed(&hp.party, p) {
		h.sendTo(hp, netplay.MsgParty, p)
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
