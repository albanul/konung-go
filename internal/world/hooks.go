package world

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// Hooks are what the simulation asks of whoever runs it: the game plays
// sounds, shows messages and windows, redraws buildings and rebuilds the
// map on a change; the server relays. Every hook may be nil.
type Hooks struct {
	// PlaySound plays sound id at cell c (0x00439F4C); ActionSound the
	// sound of an action a walker starts (0x004143DC).
	PlaySound   func(id int, c mapdata.Cell)
	ActionSound func(w *Walker, act int)
	// Message shows a status-line message (0x0042F9B8).
	Message func(text []byte)
	// WindowOpen reports the character window (window 3: no pick-ups,
	// 0x004110F8); TradeOpen the trade window (window 7: the healer and
	// smith wait); OpenPileWindow opens the pile window for pile i.
	WindowOpen     func() bool
	TradeOpen      func() bool
	OpenPileWindow func(pile int)
	// BuildingsChanged: a settlement pass changed building states.
	// BuildingHit is the hit test of placement p's building image in
	// world pixels (0x0043F89C), nil when the map has no such building.
	BuildingsChanged func()
	BuildingHit      func(placement int) func(x, y int) bool
	// ChangeMap takes map exit x (the map is rebuilt; Step returns at once).
	ChangeMap func(x gamestate.MapExit)
	// Animate advances the animated map objects, once per tick after the AI.
	Animate func()
	// AfterStep runs at the end of every tick (the LAN reports).
	AfterStep func()
	// IsRemote reports another player's hero (a LAN world); CanTarget
	// whether the own hero may attack it; StrikeRemote reports a blow or an
	// arrow of attacker that reached it (the server decides).
	IsRemote     func(entity int) bool
	CanTarget    func(entity int) bool
	StrikeRemote func(attacker *gamestate.Entity, target int, ranged bool)
}

func (s *World) playAt(id int, c mapdata.Cell) {
	if s.Hooks.PlaySound != nil {
		s.Hooks.PlaySound(id, c)
	}
}

func (s *World) actionSound(w *Walker, act int) {
	if s.Hooks.ActionSound != nil {
		s.Hooks.ActionSound(w, act)
	}
}

func (s *World) message(text []byte) {
	if s.Hooks.Message != nil {
		s.Hooks.Message(text)
	}
}

func (s *World) windowOpen() bool { return s.Hooks.WindowOpen != nil && s.Hooks.WindowOpen() }

func (s *World) tradeOpen() bool { return s.Hooks.TradeOpen != nil && s.Hooks.TradeOpen() }

func (s *World) openPileWindow(i int) {
	if s.Hooks.OpenPileWindow != nil {
		s.Hooks.OpenPileWindow(i)
	}
}

func (s *World) buildingsChanged() {
	if s.Hooks.BuildingsChanged != nil {
		s.Hooks.BuildingsChanged()
	}
}

func (s *World) buildingHit(p int) func(x, y int) bool {
	if s.Hooks.BuildingHit == nil {
		return nil
	}
	return s.Hooks.BuildingHit(p)
}

func (s *World) changeMap(x gamestate.MapExit) {
	if s.Hooks.ChangeMap != nil {
		s.Hooks.ChangeMap(x)
	}
}

func (s *World) animate() {
	if s.Hooks.Animate != nil {
		s.Hooks.Animate()
	}
}

func (s *World) afterStep() {
	if s.Hooks.AfterStep != nil {
		s.Hooks.AfterStep()
	}
}

func (s *World) isRemote(i int) bool { return s.Hooks.IsRemote != nil && s.Hooks.IsRemote(i) }

func (s *World) canTarget(i int) bool { return s.Hooks.CanTarget == nil || s.Hooks.CanTarget(i) }

func (s *World) strikeRemote(a *gamestate.Entity, target int, ranged bool) {
	if s.Hooks.StrikeRemote != nil {
		s.Hooks.StrikeRemote(a, target, ranged)
	}
}
