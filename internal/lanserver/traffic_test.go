package lanserver

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
)

// traffic counts the bytes each player is sent, by message type, as the
// connection writes them (a JSON line: netplay.Conn).
type traffic struct {
	bytes map[string]map[netplay.MsgType]int
	msgs  map[string][]*netplay.Message
}

func newTraffic() *traffic {
	return &traffic{bytes: map[string]map[netplay.MsgType]int{}, msgs: map[string][]*netplay.Message{}}
}

func (tr *traffic) send(id string, m *netplay.Message) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if tr.bytes[id] == nil {
		tr.bytes[id] = map[netplay.MsgType]int{}
	}
	tr.bytes[id][m.Type] += len(b) + 1
	tr.msgs[id] = append(tr.msgs[id], m)
	return nil
}

func (tr *traffic) total(id string) int {
	n := 0
	for _, b := range tr.bytes[id] {
		n += b
	}
	return n
}

func (tr *traffic) report(id string, ticks int) string {
	secs := float64(ticks) * netplay.TickDuration.Seconds()
	var parts []string
	for t, b := range tr.bytes[id] {
		parts = append(parts, fmt.Sprintf("%v %.0f", t, float64(b)/secs))
	}
	sort.Strings(parts)
	return fmt.Sprintf("%.0f B/s (%s)", float64(tr.total(id))/secs, strings.Join(parts, ", "))
}

// trafficCase is a scripted minute of play for the traffic measurement.
type trafficCase struct {
	name  string
	ticks int
	// setup enters the players (ids "p1", "p2") and returns a per-tick
	// script run before each Advance.
	setup func(t *testing.T, h *Host) func(tick int)
}

func enterHost(t *testing.T, h *Host, id, name string, model int) int {
	t.Helper()
	if _, err := h.Start(netplay.PlayerInfo{ID: id, Name: name, Model: model}, nil); err != nil {
		t.Fatal(err)
	}
	slot, _ := h.Slot(id)
	return slot
}

// walkAround orders slot's hero to alternate between two cells near where
// it stands, every period ticks.
func walkAround(h *Host, id string, w *World, slot, period int) func(tick int) {
	e, _ := w.Hero(slot)
	a := mapdata.Cell{Row: e.Row, Col: e.Col}
	b := mapdata.Cell{Row: e.Row + 12, Col: e.Col + 6}
	p := netplay.PlayerInfo{ID: id}
	return func(tick int) {
		if tick%period != 0 {
			return
		}
		c := a
		if (tick/period)%2 == 1 {
			c = b
		}
		h.Order(p, netplay.OrderPayload{Kind: netplay.OrderWalk, Row: c.Row, Col: c.Col})
	}
}

// creatureCell is a living creature's cell on map m (not a player's or a
// village's character).
func creatureCell(w *World, m int) (mapdata.Cell, bool) {
	for _, c := range w.Snapshot(m).Chars {
		if !c.Player && c.Owner == 0 && !c.Dead && c.Entity >= 16 {
			if g := w.Game.Entities[c.Entity].Group; g >= 25 && w.Game.Groups[g].Map == m {
				return mapdata.Cell{Row: c.Row, Col: c.Col}, true
			}
		}
	}
	return mapdata.Cell{}, false
}

var trafficCases = []trafficCase{
	{"idle in the village", 1000, func(t *testing.T, h *Host) func(int) {
		enterHost(t, h, "p1", "Ivar", 1)
		return func(int) {}
	}},
	{"walking in the village", 1000, func(t *testing.T, h *Host) func(int) {
		slot := enterHost(t, h, "p1", "Ivar", 1)
		return walkAround(h, "p1", h.W, slot, 60)
	}},
	{"fighting creatures", 1000, func(t *testing.T, h *Host) func(int) {
		slot := enterHost(t, h, "p1", "Ivar", 1)
		const m = 2
		c, ok := creatureCell(h.W, m)
		if !ok {
			t.Fatal("no creature on map 2")
		}
		if !h.W.MoveTo(slot, m, mapdata.Cell{Row: c.Row + 6, Col: c.Col}) {
			t.Fatal("MoveTo")
		}
		p := netplay.PlayerInfo{ID: "p1"}
		return func(tick int) {
			if tick%20 == 0 {
				h.Order(p, netplay.OrderPayload{Kind: netplay.OrderAttack})
			}
		}
	}},
	{"two players walking on one map", 1000, func(t *testing.T, h *Host) func(int) {
		s1 := enterHost(t, h, "p1", "Ivar", 1)
		s2 := enterHost(t, h, "p2", "Olaf", 1)
		w1 := walkAround(h, "p1", h.W, s1, 60)
		w2 := walkAround(h, "p2", h.W, s2, 45)
		return func(tick int) { w1(tick); w2(tick) }
	}},
}

// trafficBudget is the most bytes a second a player may be sent in the
// measured cases (about 33 KB/s before the views were predicted, 3 KB/s
// after; docs/konung_lan.md#network-traffic).
const trafficBudget = 6000

// TestTraffic measures what a player is sent per second in typical
// minutes of play (go test -v -run TestTraffic ./internal/lanserver/), and
// holds it to a budget.
func TestTraffic(t *testing.T) {
	for _, tc := range trafficCases {
		t.Run(tc.name, func(t *testing.T) {
			w := load(t)
			tr := newTraffic()
			h := NewHost(w, tr.send)
			script := tc.setup(t, h)
			start := w.Tick
			for k := range tc.ticks {
				script(k)
				h.Advance(start+k+1, 1)
			}
			t.Logf("%s: p1 %s", tc.name, tr.report("p1", tc.ticks))
			secs := float64(tc.ticks) * netplay.TickDuration.Seconds()
			if bps := float64(tr.total("p1")) / secs; bps > trafficBudget {
				t.Errorf("%s: %.0f B/s, over the budget of %d", tc.name, bps, trafficBudget)
			}
		})
	}
}

// viewMirror is a client's picture of its map built only from its views
// (MsgView): each character's last report and its tick, advanced by
// netplay.CharState.Predict, as game.applyView does.
type viewMirror struct {
	m    int
	tick int
	last map[int]struct {
		cs   netplay.CharState
		tick int
	}
	reports, chars int
	acts           int // reports of a character acting (attacks, flinches, deaths)
}

func (vm *viewMirror) apply(v netplay.ViewPayload) {
	if v.Full || vm.last == nil {
		vm.last = map[int]struct {
			cs   netplay.CharState
			tick int
		}{}
	}
	vm.m, vm.tick = v.Map, v.Tick
	for _, c := range v.Chars {
		vm.last[c.Entity] = struct {
			cs   netplay.CharState
			tick int
		}{c, v.Tick}
	}
	for _, e := range v.Gone {
		delete(vm.last, e)
	}
	for _, c := range v.Chars {
		if c.Act != 0 {
			vm.acts++
		}
	}
	vm.reports += len(v.Chars)
	vm.chars += len(vm.last)
}

// check compares the mirror at its last view's tick with the server's map
// now (the view was built at this tick): the same characters, each in the
// state the server has (netplay.CharState.Same).
func (vm *viewMirror) check(w *World) []string {
	var bad []string
	actual := map[int]netplay.CharState{}
	for _, c := range w.Snapshot(vm.m).Chars {
		actual[c.Entity] = charState(c)
	}
	if len(actual) != len(vm.last) {
		bad = append(bad, fmt.Sprintf("tick %d map %d: %d characters, the server has %d", vm.tick, vm.m, len(vm.last), len(actual)))
	}
	for e, r := range vm.last {
		a, ok := actual[e]
		if !ok {
			bad = append(bad, fmt.Sprintf("tick %d: entity %d not on the server's map", vm.tick, e))
			continue
		}
		if p := r.cs.Predict(r.tick, vm.tick); !sameState(p, a) {
			bad = append(bad, fmt.Sprintf("tick %d entity %d: predicted %+v, server %+v", vm.tick, e, p, a))
		}
	}
	return bad
}

// A client that applies its views (reports, and Predict in between) has
// exactly the server's state of every character on its map at every view
// tick, through walks, a hireling following, a map change, a fight with
// creatures (actions, hits, deaths) and the session's end; and it is sent
// only a small part of the characters' states.
func TestViewPredictionExact(t *testing.T) {
	w := load(t)
	tr := newTraffic()
	h := NewHost(w, tr.send)
	s1 := enterHost(t, h, "p1", "Ivar", 2)
	s2 := enterHost(t, h, "p2", "Olaf", 2)
	mirrors := map[string]*viewMirror{"p1": {}, "p2": {}}
	seen := map[string]int{}
	walk1, walk2 := walkAround(h, "p1", w, s1, 60), walkAround(h, "p2", w, s2, 45)
	p1 := netplay.PlayerInfo{ID: "p1"}
	var bad []string
	start := w.Tick
	const ticks = 3000
	for k := range ticks {
		switch {
		case k == 300:
			vs := villagers7(w)
			if len(vs) == 0 {
				t.Fatal("no villager to hire")
			}
			chief(w, s1, 30, 1000)
			if !hireVia(t, w, s1, vs[0]) {
				t.Fatal("hire failed")
			}
			walk1 = walkAround(h, "p1", w, s1, 60)
		case k == 900:
			c, ok := creatureCell(w, 2)
			if !ok || !w.MoveTo(s1, 2, mapdata.Cell{Row: c.Row + 6, Col: c.Col}) {
				t.Fatal("no fight on map 2")
			}
			walk1 = func(tick int) {
				if tick%20 == 0 {
					h.Order(p1, netplay.OrderPayload{Kind: netplay.OrderAttack})
				}
			}
		case k == 2200:
			if err := h.EndSession(); err != nil {
				t.Fatal(err)
			}
			walk1 = walkAround(h, "p1", w, s1, 50)
		}
		walk1(k)
		walk2(k)
		h.Advance(start+k+1, 1)
		for id, vm := range mirrors {
			msgs := tr.msgs[id]
			for _, m := range msgs[seen[id]:] {
				if m.Type != netplay.MsgView {
					continue
				}
				var v netplay.ViewPayload
				if err := json.Unmarshal(m.Payload, &v); err != nil {
					t.Fatal(err)
				}
				vm.apply(v)
			}
			seen[id] = len(msgs)
			if vm.tick == w.Tick {
				bad = append(bad, vm.check(w)...)
			}
		}
		if len(bad) > 5 {
			break
		}
	}
	for _, b := range bad {
		t.Error(b)
	}
	for id, vm := range mirrors {
		t.Logf("%s: %d reports for %d character-ticks (%.1f%%)", id, vm.reports, vm.chars, 100*float64(vm.reports)/float64(max(vm.chars, 1)))
		if vm.reports*100 > vm.chars*4 {
			t.Errorf("%s: %d reports for %d character-ticks: the prediction saves too little", id, vm.reports, vm.chars)
		}
	}
	if vm := mirrors["p1"]; vm.acts == 0 {
		t.Error("no fight seen: no character acting")
	}
	if w.PlayerMap(s1) == 2 {
		t.Error("the session's end did not bring the hero home")
	}
}

// sameState is CharState.Same written independently for the tests: every
// field but Path equal (by reflection, so a new field counts), and p's path
// the start of c's.
func sameState(p, c netplay.CharState) bool {
	if len(p.Path) > len(c.Path) || !reflect.DeepEqual(p.Path, c.Path[:len(p.Path)]) && len(p.Path) > 0 {
		return false
	}
	p.Path, c.Path = nil, nil
	return reflect.DeepEqual(p, c)
}

// Same agrees with sameState on every field: each field changed alone
// makes two states differ.
func TestCharStateSame(t *testing.T) {
	base := netplay.CharState{Entity: 20, Row: 10, Col: 5, Walking: true, Steps: 8, Path: []int{12, 5, 14, 5}}
	v := reflect.ValueOf(&base).Elem()
	for i := range v.NumField() {
		c := base
		c.Path = append([]int(nil), base.Path...)
		f := reflect.ValueOf(&c).Elem().Field(i)
		switch f.Kind() {
		case reflect.Int:
			f.SetInt(f.Int() + 1)
		case reflect.Bool:
			f.SetBool(!f.Bool())
		case reflect.Slice:
			c.Path[0]++
		}
		if base.Same(c) || sameState(base, c) {
			t.Errorf("field %s changed, still the same", v.Type().Field(i).Name)
		}
	}
	longer := base
	longer.Path = append(append([]int(nil), base.Path...), 16, 5)
	if !base.Same(longer) || base.Same(longer) != sameState(base, longer) {
		t.Error("a report's path is the start of the server's: the same")
	}
	if longer.Same(base) {
		t.Error("a longer predicted path than the server's: not the same")
	}
}
