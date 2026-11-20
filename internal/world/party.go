package world

import (
	"sort"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// partyMember returns the member walker of entity i and its party record,
// or nil.
func (s *World) PartyMember(i int) (*Member, *gamestate.Entity) {
	for _, m := range s.Members {
		if m != nil && s.Party[m.K].Index == i {
			return m, &s.Party[m.K]
		}
	}
	return nil, nil
}

// lead is the hero's member (group 0's first entity, 0x00A3E694), nil when
// the hero is not on this map.
func (s *World) Lead() *Member {
	if len(s.Members) == 0 {
		return nil
	}
	return s.Members[0]
}

// livingMembers returns the members that can act (not dead, not dying).
func (s *World) LivingMembers() []*Member {
	var out []*Member
	for _, m := range s.Members {
		if m != nil && Alive(&m.Walker, &s.Party[m.K]) {
			out = append(out, m)
		}
	}
	return out
}

// selected returns the living selected members in selection order.
func (s *World) Selected() []*Member {
	var out []*Member
	for _, i := range s.Sel {
		if m, e := s.PartyMember(i); m != nil && Alive(&m.Walker, e) {
			out = append(out, m)
		}
	}
	return out
}

// buttonEntity is whose state the HUD buttons show (0x004290D8): the
// window's character in window 3, else the first selected, else the hero.
func (s *World) ButtonEntity() *gamestate.Entity {
	if s.windowOpen() {
		return s.Current()
	}
	if len(s.Sel) > 0 {
		if _, e := s.PartyMember(s.Sel[0]); e != nil {
			return e
		}
	}
	return &s.Party[0]
}

// current is the character of the character window and the bag strip
// (0x00A3E688).
func (s *World) Current() *gamestate.Entity {
	if s.Cur < 0 || s.Cur >= len(s.Party) {
		s.Cur = 0
	}
	return &s.Party[s.Cur]
}

// partyIndex returns entity i's index in s.Party, or -1.
func (s *World) PartyIndex(i int) int {
	for k, e := range s.Party {
		if e.Index == i {
			return k
		}
	}
	return -1
}

// walkOrder is the left click on the map (0x004229F7): every selected
// member walks to cell c (+2 = 0x48, members other than the hero also get
// bit 0x02, so they stay there instead of following), without picking up.
// With pick (the right click, 0x004239D5) the walk ends with a pick-up.
func (s *World) WalkOrder(m *Member, c mapdata.Cell, pick bool) {
	s.EndFight(m)
	m.Intent = gamestate.IntentWalk | gamestate.IntentIdle
	if !s.Leads(m) && !pick {
		m.Intent |= gamestate.IntentStrike
	}
	m.ResumeSet = false
	m.WalkTo(c)
	m.PickIntent = pick
}

// rightOrder is the right click's loop over the selection (0x00423B89) for
// member m, with the character under the cursor (hovered, -1 = none):
//   - bow ready (bow active and equipped, arrows, the stance): shoot the
//     character; without one, with incendiary arrows, the hovered building
//     (0x00423C2A);
//   - a character, in the stance: attack it in melee (0x00423C8B); out of
//     the stance nothing happens;
//   - no character: walk to the cell and pick up there (0x00423CCF).
//
// building is placement + 1 of the settlement building under the cursor
// (0x00A3E6A4), 0 = none.
func (s *World) RightOrder(m *Member, hovered int, c mapdata.Cell, building int) {
	e := &s.Party[m.K]
	switch {
	case gamestate.CanShoot(*e):
		if hovered >= 0 {
			s.EndFight(m)
			m.Target, m.Intent = hovered, gamestate.IntentShoot|gamestate.IntentIdle // 0x00423BEF: +2 = 0x44
			return
		}
		if s.Game.Incendiary(*e) && building != 0 && s.Game.SettlementOnMap(s.MapNumber) != nil {
			s.EndFight(m)
			m.BurnTarget, m.Intent = building, gamestate.IntentShoot|gamestate.IntentIdle
			m.TargetCell = c
		}
	case hovered >= 0:
		if e.InStance() {
			s.StartFight(m, hovered)
		}
	default:
		s.WalkOrder(m, c, true)
	}
}

// GatherOrder is the rule of the F key / HUD button 4 «Все ко мне»
// (0x004226C0; the game first returns a held item and presses the button):
// every living member other than the hero gets +2 = 0x60 (idle, following
// the leader) with +0x14 = 0xFF (the walk ends after the current step).
func (s *World) GatherOrder() {
	for _, m := range s.LivingMembers() {
		if !s.Leads(m) {
			s.Gather(m)
		}
	}
}

// AttackOrder is the rule of the A key / HUD button 5 (0x00421698; the
// game first returns a held item and presses the button): every living
// member gets +6 = 0, +2 = 0x44 with the bow active (else 0x41) and +0x14 =
// 0xFF, entering the stance. The planner then picks the nearest enemy
// (NearestEnemy) or shoots (MemberShoot).
func (s *World) AttackOrder() {
	for _, m := range s.LivingMembers() {
		e := &s.Party[m.K]
		if !e.InStance() {
			gamestate.ToggleStance([]*gamestate.Entity{e})
			m.Look = LookOf(*e)
		}
		s.EndFight(m)
		m.PickIntent, m.ResumeSet = false, false
		if e.WeaponMode == gamestate.WeaponBow {
			m.Intent = gamestate.IntentShoot | gamestate.IntentIdle
			continue
		}
		m.Path = m.Path[:min(len(m.Path), 1)] // finish the step in progress
		m.Intent = gamestate.IntentMelee | gamestate.IntentIdle
	}
}

// startFight makes member m attack entity target (the right click in the
// stance, 0x00423C8B: +6, +2 = 0x41).
func (s *World) StartFight(m *Member, target int) {
	m.Intent = gamestate.IntentMelee | gamestate.IntentIdle
	m.Target = target
	m.PickIntent, m.ResumeSet = false, false
	if tw, _ := s.Combatant(target); tw != nil {
		m.TargetCell = tw.Cell
	}
	s.SetMemberStopAt(m)
}

// setMemberStopAt makes m's walks end before its target's cell.
func (s *World) SetMemberStopAt(m *Member) {
	m.StopAt = func(c mapdata.Cell) bool {
		tw, _ := s.Combatant(m.Target)
		return m.Target >= 0 && tw != nil && c == tw.Cell
	}
}

// endFight drops m's target and fight intents (a new order, the target
// dead or gone).
func (s *World) EndFight(m *Member) {
	m.Target, m.BurnTarget = -1, 0
	m.Intent &^= gamestate.IntentMelee | gamestate.IntentShoot
	m.StopAt = nil
	if m.Act == gamestate.ActAttack {
		m.Act = 0
	}
}

// endAllFights ends every member's fight (a village surrenders: «the
// party's targets are cleared»).
func (s *World) EndAllFights() {
	for _, m := range s.Members {
		if m != nil {
			s.EndFight(m)
		}
	}
}

// partyTick is the party branch of 0x00421DA4: every member's tick.
func (s *World) PartyTick() {
	for _, m := range s.Members {
		if m != nil {
			s.MemberTick(m)
		}
	}
}

// memberTick is one game tick of member m: its order (melee, shooting) or,
// idle without one, 0x0041F11C (retaliation, following the leader).
func (s *World) MemberTick(m *Member) {
	e := &s.Party[m.K]
	if !Alive(&m.Walker, e) || m.Act == gamestate.ActFlinch {
		return
	}
	if m.Shoot() {
		s.MemberShoot(m)
		return
	}
	if m.Target >= 0 {
		if tw, t := s.Combatant(m.Target); !Alive(tw, t) {
			m.Target = -1 // 0x0041E384: look for another enemy
			if m.Act == gamestate.ActAttack {
				m.Act = 0
			}
		}
	}
	if m.Target < 0 {
		if !m.Melee() {
			s.IdleTick(m)
			return
		}
		if m.Walking {
			return // the step in progress ends first
		}
		t := s.NearestEnemy(m.Cell)
		if t < 0 {
			s.EndFight(m) // no enemy left: the intent ends (0x0041E453)
			return
		}
		s.StartFight(m, t)
	}
	tw, t := s.Combatant(m.Target)
	switch {
	case m.Act == gamestate.ActAttack:
		if s.Swing(&m.Walker, e, m.Target, m.TargetCell) {
			m.Act = 0 // killed or moved: next tick finds a new target or approaches again (0x0041F258)
			if !Alive(tw, t) || !m.Melee() {
				if !Alive(tw, t) {
					m.Intent &^= gamestate.IntentStrike // 0x004220FC
				}
				m.Target = -1
				m.StopAt = nil
			}
		}
	case !m.Melee():
		m.Target = -1 // a retaliation ends when the attacker leaves
	case m.Walking || len(m.Path) > 0:
		if tw.Cell != m.TargetCell {
			m.TargetCell = tw.Cell
			m.Chase(tw.Cell)
		}
	default: // idle with the intent: the planner 0x0041ED8C
		m.TargetCell = tw.Cell
		if Adjacent(m.Cell, tw.Cell) {
			gamestate.EnterStance(e) // the swing enters the stance (0x004149F5)
			m.Look = LookOf(*e)
			m.StartAct(gamestate.ActAttack)
			return
		}
		m.Chase(tw.Cell)
		if len(m.Path) == 0 {
			s.EndFight(m) // unreachable
		}
	}
}

// idleTick is the idle case of the party AI for a member without a fight
// order (0x004223AD → 0x0041F11C): a walk interrupted by a hit reaction
// goes on; then, unless a bow target is in sight (0x00410E84, which only
// stores +6), retaliation (0x0041EC0C); then, with +2 bit 0x20, following
// the leader (0x0041F5D8 → 0x0041FA5D). 0x00414AB4 gives every member but
// the hero bit 0x20 when it goes idle without an intent (+2 & 0x0F = 0).
func (s *World) IdleTick(m *Member) {
	if m.Walking || len(m.Path) > 0 || m.Act != 0 {
		return
	}
	if m.ResumeSet {
		m.ResumeSet = false
		m.WalkTo(m.Resume)
		if len(m.Path) > 0 {
			return
		}
	}
	m.Intent &^= gamestate.IntentWalk
	m.Intent |= gamestate.IntentIdle
	if !s.Leads(m) && m.Intent&0x0F == 0 { // LAN heroes lead themselves, their hirelings follow
		m.Intent |= gamestate.IntentFollow
	}
	if s.BowTargetInSight(m) {
		s.Retaliate(m)
		return
	}
	if s.Retaliate(m) || m.Intent&gamestate.IntentFollow == 0 {
		return
	}
	s.FollowLeader(m)
}

// bowTargetInSight is 0x00410E84's test for an idle member: with the bow
// active, a bow and arrows, a living member of the enemy group with a
// clear line.
func (s *World) BowTargetInSight(m *Member) bool {
	e := &s.Party[m.K]
	if e.WeaponMode != gamestate.WeaponBow || e.Equipment[gamestate.WeaponBow] == 0 || e.Quiver == 0 {
		return false
	}
	return s.FirstShootable(&m.Walker, e) >= 0
}

// followLeader is 0x0041FA5D for member m: at FollowDistance or more from the
// leader (group 0's first entity; a LAN hireling's player's hero) it walks
// to a free formation cell near it (+2 |= 0x48; the follow bit stays).
func (s *World) FollowLeader(m *Member) {
	l := s.leaderOf(m)
	if l == nil || l == m || s.Tables == nil {
		return
	}
	free := func(r, c int) bool { return !s.Blocked(mapdata.Cell{Row: r, Col: c}) }
	r, c, ok := gamestate.FollowTarget(s.Tables, l.Cell.Row, l.Cell.Col, m.Cell.Row, m.Cell.Col, s.Rng.IntN, free)
	if !ok {
		return
	}
	m.Intent |= gamestate.IntentWalk | gamestate.IntentIdle
	m.WalkTo(mapdata.Cell{Row: r, Col: c})
}

// retaliate is 0x0041EC0C for idle member m: a neighbour of another group
// going for m (melee intent, target = m) is struck back (+6 = it, +2 |= 2,
// the stance, the swing). It reports whether m strikes.
func (s *World) Retaliate(m *Member) bool {
	if m.Walking || len(m.Path) > 0 || m.Act != 0 {
		return false
	}
	e := &s.Party[m.K]
	for _, n := range s.NPCs {
		if n.Intent&IntentMelee == 0 || n.Target != e.Index || !Adjacent(m.Cell, n.Cell) ||
			!Alive(&n.Walker, &s.Game.Entities[n.Entity]) {
			continue
		}
		m.Target, m.TargetCell = n.Entity, n.Cell
		m.Intent |= gamestate.IntentStrike
		gamestate.EnterStance(e)
		m.Look = LookOf(*e)
		m.Dir = Facing(m.Cell, n.Cell)
		m.StartAct(gamestate.ActAttack)
		return true
	}
	return false
}

// toggleSelectionStance is Space / the HUD stance button (0x0042179C) for
// the selection: the first selected member decides (enter or leave) and
// everyone follows; leaving it ends a melee swing.
func (s *World) ToggleSelectionStance() {
	sel := s.Selected()
	var es []*gamestate.Entity
	for _, m := range sel {
		es = append(es, &s.Party[m.K])
	}
	gamestate.ToggleStance(es)
	for _, m := range sel {
		e := &s.Party[m.K]
		if !e.InStance() {
			s.EndFight(m) // the swing needs the stance's animation set
		}
		m.Look = LookOf(*e)
	}
}

// selectWeapon is 1/2/3 and the HUD weapon button (0x0042192C) for the
// first selected member, or the whole selection with Shift (0x00A3E754).
//
// all is Shift (0x00A3E754): the whole selection, else its first member.
func (s *World) SelectWeapon(mode int, all bool) {
	sel := s.Selected()
	if len(sel) == 0 {
		return
	}
	if !all {
		sel = sel[:1]
	}
	for _, m := range sel {
		e := &s.Party[m.K]
		gamestate.SelectWeapon(e, mode)
		m.Look = LookOf(*e)
	}
}

// partyPositions returns the cells of the living members (the enemy group
// as the creature and village alarm checks see it).
func (s *World) PartyPositions() [][2]int {
	var out [][2]int
	for _, m := range s.LivingMembers() {
		out = append(out, [2]int{m.Cell.Row, m.Cell.Col})
	}
	return out
}

// partyOnCells returns copies of the living members with their current
// cells (the village alarm and surrender).
func (s *World) PartyOnCells() []gamestate.Entity {
	var out []gamestate.Entity
	for _, m := range s.LivingMembers() {
		e := s.Party[m.K]
		e.Row, e.Col = m.Cell.Row, m.Cell.Col
		out = append(out, e)
	}
	return out
}

// nearestMember is 0x004202A8 with the party as the enemy group: the living
// member closest to cell c (gamestate.Dist), -1 = none. With skip, members
// skip reports true for are left out (0x004203A0).
func (s *World) NearestMember(c mapdata.Cell, inside func(mapdata.Cell) bool, skip func(i int) bool) int {
	best, bestD := -1, 1<<30
	for _, m := range s.LivingMembers() {
		i := s.Party[m.K].Index
		if (inside != nil && !inside(m.Cell)) || (skip != nil && skip(i)) {
			continue
		}
		if d := gamestate.Dist(c.Row, c.Col, m.Cell.Row, m.Cell.Col); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// revive is Game.Reviver for this scene: 0x00414B3C with the scene's
// occupancy and the hero's cell for character e (a party member's party
// copy is revived whatever record e points to, and e gets the result),
// then its walker takes the cell and idles; a revived NPC goes back to its
// AI (npc.LiveMode).
func (s *World) Revive(e *gamestate.Entity) bool {
	var w *Walker
	target := e
	m, pe := s.PartyMember(e.Index)
	n := s.ByEntity[e.Index]
	switch {
	case m != nil:
		w, target = &m.Walker, pe
	case n != nil:
		w, target = &n.Walker, &s.Game.Entities[e.Index]
	default:
		return false
	}
	if !target.Dead() {
		return false
	}
	hr, hc := w.Cell.Row, w.Cell.Col
	switch {
	case s.LAN:
		// A LAN world has a hero per player: a member comes back by its
		// own player's hero (itself for a hero), else where it lies.
		if m != nil {
			if l := s.leaderOf(m); l != nil {
				hr, hc = l.Cell.Row, l.Cell.Col
			}
		}
	case s.Lead() != nil:
		l := s.Lead()
		hr, hc = l.Cell.Row, l.Cell.Col
	}
	target.Row, target.Col = w.Cell.Row, w.Cell.Col
	free := func(r, c int) bool { return !s.Blocked(mapdata.Cell{Row: r, Col: c}) }
	if !s.Game.Revive(target, hr, hc, s.Rng.IntN, free) {
		return false
	}
	if e != target {
		*e = *target
	}
	w.Halt()
	w.Cell = mapdata.Cell{Row: target.Row, Col: target.Col}
	s.Occ[w.Cell] = true
	w.Act, w.ActStep = 0, 0
	w.Look = LookOf(*target)
	if m != nil {
		m.Intent = gamestate.IntentIdle
		m.Target = -1
	} else {
		n.Mode, n.Target, n.Intent = n.LiveMode, -1, 0
	}
	return true
}

// placeAgain is the main loop's map change to the map the party is on
// (0x004377A6): a teleport exit within the map, or an orb or key whose map
// is the current one. There is no leave cleanup and no load, so creatures,
// piles and objects stay as they are. The party group gets the arrival cell
// and facing, then 0x004118DC frees the cells of the members that can act
// (0x00411906) and places the party around the arrival cell. A placed
// member stops and idles with the arrival facing; its +0x0C/+0x0E become
// its cell (0x00411A1D–0x00411AE8). The port also stops members that are
// left unplaced (no free formation cell), whose walk the original lets run.
func (s *World) PlaceAgain(x gamestate.MapExit) {
	if s.Tables == nil {
		return
	}
	s.StoreWorld()
	for _, m := range s.Members {
		if m != nil && Alive(&m.Walker, &s.Party[m.K]) {
			m.Halt()
			delete(s.Occ, m.Cell)
		}
	}
	s.Game.SetArrival(x)
	free := func(r, c int) bool { return !s.Blocked(mapdata.Cell{Row: r, Col: c}) }
	take := func(r, c int) { s.Occ[mapdata.Cell{Row: r, Col: c}] = true }
	for _, i := range s.Game.PlaceParty(s.Tables, s.Rng.IntN, free, take) {
		m, e := s.PartyMember(i)
		if m == nil {
			continue
		}
		*e = s.Game.Entities[i]
		m.Cell = mapdata.Cell{Row: e.Row, Col: e.Col}
		m.TargetCell = m.Cell
		m.Dir = mapdata.Direction(e.Variant & 7)
		m.Intent = e.Flags2
		m.Act, m.ActStep = 0, 0
		m.PickIntent, m.ResumeSet = false, false // +2 &= 0xF0 drops the walk bit 0x08
	}
}

// reviveAll is «Слова Воскрешения» (0x00414FFB): every dead character of
// every group on this map is revived (0x00414B3C), the party first, then
// the others in group order.
func (s *World) ReviveAll() {
	for _, m := range s.Members {
		if m != nil && s.Party[m.K].Dead() {
			s.Revive(&s.Party[m.K])
		}
	}
	order := append([]*NPC(nil), s.NPCs...)
	sort.SliceStable(order, func(i, j int) bool {
		a, b := s.Game.Entities[order[i].Entity], s.Game.Entities[order[j].Entity]
		return a.Group < b.Group || a.Group == b.Group && a.Index < b.Index
	})
	for _, n := range order {
		if e := &s.Game.Entities[n.Entity]; e.Dead() {
			s.Revive(e)
		}
	}
}

// syncParty reads the party copies back after the trade rules changed the
// records.
func (s *World) SyncParty() {
	for k := range s.Party {
		s.Party[k] = s.Game.Entities[s.Party[k].Index]
	}
}
