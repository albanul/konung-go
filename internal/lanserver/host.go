package lanserver

import (
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
	view  Viewer
	looks map[int]netplay.Look // players' heroes sent to this player, as sent
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
	hp.info, hp.hero = p, hero
	hp.view, hp.looks = Viewer{}, map[int]netplay.Look{}
	m, c, err := h.W.AddPlayer(hp.slot, p.Model, hero)
	if err != nil {
		delete(h.players, p.ID)
		delete(h.bySlot, hp.slot)
		return netplay.EnterPayload{}, err
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
	defer h.mu.Unlock()
	if hp := h.players[id]; hp != nil {
		h.W.RemovePlayer(hp.slot)
		delete(h.players, id)
		delete(h.bySlot, hp.slot)
	}
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
	kills := h.report()
	h.mu.Unlock()
	if h.OnKill != nil {
		for _, k := range kills {
			h.OnKill(k)
		}
	}
	return n
}

// report sends the tick's events and views; h.mu held. It returns the
// kills for OnKill.
func (h *Host) report() []Kill {
	var kills []Kill
	byMap := map[int][]netplay.Event{}
	for _, ev := range h.W.TakeEvents() {
		switch ev.Kind {
		case EventSound:
			byMap[ev.Map] = append(byMap[ev.Map], netplay.Event{Kind: netplay.EventSound, Sound: ev.Sound, Row: ev.Row, Col: ev.Col})
		case EventAction:
			byMap[ev.Map] = append(byMap[ev.Map], netplay.Event{Kind: netplay.EventAction, Entity: ev.Entity, Act: ev.Act, Row: ev.Row, Col: ev.Col})
		case EventMapChange:
			if hp := h.bySlot[ev.Entity]; hp != nil {
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
				k.Victim, k.VictimPlayer = v.info, true
			}
			kills = append(kills, k)
		}
	}
	for _, hp := range h.players {
		view, ok := h.W.View(hp.slot, &hp.view)
		if !ok {
			continue
		}
		// The players' heroes it has not seen (or whose look changed).
		for _, c := range view.Chars {
			if !c.Player || c.Entity == hp.slot {
				continue
			}
			o := h.bySlot[c.Entity]
			l, ok := h.W.PlayerLook(c.Entity)
			if o == nil || !ok {
				continue
			}
			if old, seen := hp.looks[c.Entity]; !seen || old != l {
				hp.looks[c.Entity] = l
				h.sendTo(hp, netplay.MsgPlayer, netplay.PlayerPayload{Slot: c.Entity, Player: o.info, Hero: o.hero, Look: l})
			}
		}
		if ev := byMap[view.Map]; len(ev) > 0 {
			h.sendTo(hp, netplay.MsgEvents, netplay.EventsPayload{Events: ev})
		}
		h.sendTo(hp, netplay.MsgView, view)
	}
	return kills
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
