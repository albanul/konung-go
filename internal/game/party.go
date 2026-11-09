package game

import (
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
)

// Party play (docs/konung_party.md): every member of group 0 on the map is
// a member with its own walker and orders; the selection list (0x0083B93C)
// says who takes the player's orders. The party branch of the AI dispatcher
// 0x00421DA4 runs the same code for every member: the hero is only special
// in that it never follows (0x00414AB4 sets +2 bit 0x20 for the others).

// member is a party member on this map: its walker and the orders the
// party AI works on (entity +2, +6, +0x0C/+0x0E, +3 bit 0x40).
type member struct {
	walker
	k int // index in startScene.party

	// intent mirrors entity +2 (gamestate.Intent* bits): melee, strike
	// (retaliating; set on members other than the hero by a walk order,
	// which keeps them from following), shoot, walk, follow, idle.
	intent byte
	// target is +6 (entity index, -1 = none) and targetCell its cell when
	// last checked (+0x0C/+0x0E); burnTarget is placement + 1 of a building
	// shot at with incendiary arrows (+6 < 0), 0 = none.
	target     int
	targetCell mapdata.Cell
	burnTarget int
	pickIntent bool // +3 bit 0x40: pick up where the walk ends
	// resume is the walk order's goal kept through a hit reaction: the
	// planner (0x0041ED8C, +2 bit 0x08) walks on afterwards.
	resume    mapdata.Cell
	resumeSet bool
}

func (m *member) melee() bool { return m.intent&gamestate.IntentMelee != 0 }
func (m *member) shoot() bool { return m.intent&gamestate.IntentShoot != 0 }

// partyMember returns the member walker of entity i and its party record,
// or nil.
func (s *startScene) partyMember(i int) (*member, *gamestate.Entity) {
	for _, m := range s.members {
		if m != nil && s.party[m.k].Index == i {
			return m, &s.party[m.k]
		}
	}
	return nil, nil
}

// lead is the hero's member (group 0's first entity, 0x00A3E694), nil when
// the hero is not on this map.
func (s *startScene) lead() *member {
	if len(s.members) == 0 {
		return nil
	}
	return s.members[0]
}

// livingMembers returns the members that can act (not dead, not dying).
func (s *startScene) livingMembers() []*member {
	var out []*member
	for _, m := range s.members {
		if m != nil && alive(&m.walker, &s.party[m.k]) {
			out = append(out, m)
		}
	}
	return out
}

// selected returns the living selected members in selection order.
func (s *startScene) selected() []*member {
	var out []*member
	for _, i := range s.sel {
		if m, e := s.partyMember(i); m != nil && alive(&m.walker, e) {
			out = append(out, m)
		}
	}
	return out
}

// buttonEntity is whose state the HUD buttons show (0x004290D8): the
// window's character in window 3, else the first selected, else the hero.
func (s *startScene) buttonEntity() *gamestate.Entity {
	if s.inv.open {
		return s.current()
	}
	if len(s.sel) > 0 {
		if _, e := s.partyMember(s.sel[0]); e != nil {
			return e
		}
	}
	return &s.party[0]
}

// current is the character of the character window and the bag strip
// (0x00A3E688).
func (s *startScene) current() *gamestate.Entity {
	if s.cur < 0 || s.cur >= len(s.party) {
		s.cur = 0
	}
	return &s.party[s.cur]
}

// partyIndex returns entity i's index in s.party, or -1.
func (s *startScene) partyIndex(i int) int {
	for k, e := range s.party {
		if e.Index == i {
			return k
		}
	}
	return -1
}

// shiftHeld is 0x00A3E754 (set and cleared around VK_SHIFT, 0x00437374).
func shiftHeld() bool { return ebiten.IsKeyPressed(ebiten.KeyShift) }

// portraitClick is a left click with an empty hand on portrait k
// (0x00422EA8): a dead member is ignored, as in windows 1, 2 and 5. In the
// character window (window 3) the member becomes its character and is
// selected; otherwise an unselected member is selected (0x00425084, added
// with Shift) and becomes the strip's character when the strip shows, and
// a selected one toggles its bag strip.
func (s *startScene) portraitClick(k int) {
	m, e := s.partyMember(s.party[k].Index)
	if e == nil {
		e = &s.party[k]
	}
	if e.Dead() || (m != nil && m.act == gamestate.ActDeath) {
		return
	}
	i := e.Index
	switch {
	case s.inv.open:
		s.cur = k
		s.inv.scroll = 0
		if !gamestate.Selected(s.sel, i) {
			s.sel = gamestate.Select(s.sel, i, shiftHeld())
		}
	case !gamestate.Selected(s.sel, i):
		s.sel = gamestate.Select(s.sel, i, shiftHeld())
		if s.inv.quick {
			s.cur = k
			s.inv.scroll = 0
		}
	case s.inv.quick && s.cur == k:
		s.inv.quick = false // 0x0042308C
	default:
		s.cur = k
		s.inv.scroll = 0
		s.inv.quick = true
	}
}

// portraitRightClick is a right click on portrait k (0x00423D02): with Ctrl
// on a member with a dialogue, the dialogue; otherwise the member leaves
// the selection (0x00439230), and when it was the strip's character the
// first selected member takes its place.
func (s *startScene) portraitRightClick(k int) {
	e := s.party[k]
	if e.Dead() {
		return
	}
	if ebiten.IsKeyPressed(ebiten.KeyControl) && e.Dialogue != gamestate.NoDialogue && s.assets != nil && s.assets.quests != nil {
		s.inv.quick = false
		s.talkTo(e.Index)
		return
	}
	s.sel = gamestate.Deselect(s.sel, e.Index)
	if s.cur == k && len(s.sel) > 0 {
		if j := s.partyIndex(s.sel[0]); j >= 0 {
			s.cur = j
			s.inv.scroll = 0
		}
	}
}

// selectInWorld is a right click on a party member on the map
// (0x00423A5C): with Shift it toggles the member in the selection,
// otherwise the member becomes the selection.
func (s *startScene) selectInWorld(i int) {
	if shiftHeld() && gamestate.Selected(s.sel, i) {
		s.sel = gamestate.Deselect(s.sel, i)
		return
	}
	s.sel = gamestate.Select(s.sel, i, shiftHeld())
}

// walkOrder is the left click on the map (0x004229F7): every selected
// member walks to cell c (+2 = 0x48, members other than the hero also get
// bit 0x02, so they stay there instead of following), without picking up.
// With pick (the right click, 0x004239D5) the walk ends with a pick-up.
func (s *startScene) walkOrder(m *member, c mapdata.Cell, pick bool) {
	s.endFight(m)
	m.intent = gamestate.IntentWalk | gamestate.IntentIdle
	if m.k != 0 && !pick {
		m.intent |= gamestate.IntentStrike
	}
	m.resumeSet = false
	m.walkTo(c)
	m.pickIntent = pick
}

// rightOrder is the right click's loop over the selection (0x00423B89) for
// member m, with the character under the cursor (hovered, -1 = none):
//   - bow ready (bow active and equipped, arrows, the stance): shoot the
//     character; without one, with incendiary arrows, the hovered building
//     (0x00423C2A);
//   - a character, in the stance: attack it in melee (0x00423C8B); out of
//     the stance nothing happens;
//   - no character: walk to the cell and pick up there (0x00423CCF).
func (s *startScene) rightOrder(m *member, hovered int, c mapdata.Cell) {
	e := &s.party[m.k]
	switch {
	case gamestate.CanShoot(*e):
		if hovered >= 0 {
			s.endFight(m)
			m.target, m.intent = hovered, gamestate.IntentShoot|gamestate.IntentIdle // 0x00423BEF: +2 = 0x44
			return
		}
		if s.game.Incendiary(*e) && s.hoveredBuilding != 0 && s.game.SettlementOnMap(s.mapNumber) != nil {
			s.endFight(m)
			m.burnTarget, m.intent = s.hoveredBuilding, gamestate.IntentShoot|gamestate.IntentIdle
			m.targetCell = c
		}
	case hovered >= 0:
		if e.InStance() {
			s.startFight(m, hovered)
		}
	default:
		s.walkOrder(m, c, true)
	}
}

// gatherOrder is the F key / HUD button 4 «Все ко мне» (0x004226C0): return
// a held item, press the button, and set every living member other than
// the hero to +2 = 0x60 (idle, following the leader) with +0x14 = 0xFF
// (the walk ends after the current step).
func (s *startScene) gatherOrder() {
	s.hand.Return(s.current())
	s.hud.press(hudGatherButton, hudGatherImage)
	for _, m := range s.livingMembers() {
		if m.k == 0 {
			continue
		}
		s.endFight(m)
		m.intent = gamestate.IntentGather
		m.pickIntent, m.resumeSet = false, false
		m.stopWalk()
	}
}

// attackOrder is the A key / HUD button 5 (0x00421698): return a held item,
// press the button, and give every living member +6 = 0, +2 = 0x44 with
// the bow active (else 0x41) and +0x14 = 0xFF, entering the stance. The
// planner then picks the nearest enemy (nearestEnemy) or shoots
// (memberShoot).
func (s *startScene) attackOrder() {
	s.hand.Return(s.current())
	s.hud.press(hudAttackButton, hudAttackImage)
	for _, m := range s.livingMembers() {
		e := &s.party[m.k]
		if !e.InStance() {
			gamestate.ToggleStance([]*gamestate.Entity{e})
			m.look = lookOf(*e)
		}
		s.endFight(m)
		m.pickIntent, m.resumeSet = false, false
		if e.WeaponMode == gamestate.WeaponBow {
			m.intent = gamestate.IntentShoot | gamestate.IntentIdle
			continue
		}
		m.path = m.path[:min(len(m.path), 1)] // finish the step in progress
		m.intent = gamestate.IntentMelee | gamestate.IntentIdle
	}
}

// startFight makes member m attack entity target (the right click in the
// stance, 0x00423C8B: +6, +2 = 0x41).
func (s *startScene) startFight(m *member, target int) {
	m.intent = gamestate.IntentMelee | gamestate.IntentIdle
	m.target = target
	m.pickIntent, m.resumeSet = false, false
	if tw, _ := s.combatant(target); tw != nil {
		m.targetCell = tw.cell
	}
	s.setMemberStopAt(m)
}

// setMemberStopAt makes m's walks end before its target's cell.
func (s *startScene) setMemberStopAt(m *member) {
	m.stopAt = func(c mapdata.Cell) bool {
		tw, _ := s.combatant(m.target)
		return m.target >= 0 && tw != nil && c == tw.cell
	}
}

// endFight drops m's target and fight intents (a new order, the target
// dead or gone).
func (s *startScene) endFight(m *member) {
	m.target, m.burnTarget = -1, 0
	m.intent &^= gamestate.IntentMelee | gamestate.IntentShoot
	m.stopAt = nil
	if m.act == gamestate.ActAttack {
		m.act = 0
	}
}

// endAllFights ends every member's fight (a village surrenders: «the
// party's targets are cleared»).
func (s *startScene) endAllFights() {
	for _, m := range s.members {
		if m != nil {
			s.endFight(m)
		}
	}
}

// partyTick is the party branch of 0x00421DA4: every member's tick.
func (s *startScene) partyTick() {
	for _, m := range s.members {
		if m != nil {
			s.memberTick(m)
		}
	}
}

// memberTick is one game tick of member m: its order (melee, shooting) or,
// idle without one, 0x0041F11C (retaliation, following the leader).
func (s *startScene) memberTick(m *member) {
	e := &s.party[m.k]
	if !alive(&m.walker, e) || m.act == gamestate.ActFlinch {
		return
	}
	if m.shoot() {
		s.memberShoot(m)
		return
	}
	if m.target >= 0 {
		if tw, t := s.combatant(m.target); !alive(tw, t) {
			m.target = -1 // 0x0041E384: look for another enemy
			if m.act == gamestate.ActAttack {
				m.act = 0
			}
		}
	}
	if m.target < 0 {
		if !m.melee() {
			s.idleTick(m)
			return
		}
		if m.walking {
			return // the step in progress ends first
		}
		t := s.nearestEnemy(m.cell)
		if t < 0 {
			s.endFight(m) // no enemy left: the intent ends (0x0041E453)
			return
		}
		s.startFight(m, t)
	}
	tw, t := s.combatant(m.target)
	switch {
	case m.act == gamestate.ActAttack:
		if s.swing(&m.walker, e, m.target, m.targetCell) {
			m.act = 0 // killed or moved: next tick finds a new target or approaches again (0x0041F258)
			if !alive(tw, t) || !m.melee() {
				if !alive(tw, t) {
					m.intent &^= gamestate.IntentStrike // 0x004220FC
				}
				m.target = -1
				m.stopAt = nil
			}
		}
	case !m.melee():
		m.target = -1 // a retaliation ends when the attacker leaves
	case m.walking || len(m.path) > 0:
		if tw.cell != m.targetCell {
			m.targetCell = tw.cell
			m.chase(tw.cell)
		}
	default: // idle with the intent: the planner 0x0041ED8C
		m.targetCell = tw.cell
		if adjacent(m.cell, tw.cell) {
			gamestate.EnterStance(e) // the swing enters the stance (0x004149F5)
			m.look = lookOf(*e)
			m.startAct(gamestate.ActAttack)
			return
		}
		m.chase(tw.cell)
		if len(m.path) == 0 {
			s.endFight(m) // unreachable
		}
	}
}

// idleTick is the idle case of the party AI for a member without a fight
// order (0x004223AD → 0x0041F11C): a walk interrupted by a hit reaction
// goes on; then, unless a bow target is in sight (0x00410E84, which only
// stores +6), retaliation (0x0041EC0C); then, with +2 bit 0x20, following
// the leader (0x0041F5D8 → 0x0041FA5D). 0x00414AB4 gives every member but
// the hero bit 0x20 when it goes idle without an intent (+2 & 0x0F = 0).
func (s *startScene) idleTick(m *member) {
	if m.walking || len(m.path) > 0 || m.act != 0 {
		return
	}
	if m.resumeSet {
		m.resumeSet = false
		m.walkTo(m.resume)
		if len(m.path) > 0 {
			return
		}
	}
	m.intent &^= gamestate.IntentWalk
	m.intent |= gamestate.IntentIdle
	if m.k != 0 && m.intent&0x0F == 0 {
		m.intent |= gamestate.IntentFollow
	}
	if s.bowTargetInSight(m) {
		s.retaliate(m)
		return
	}
	if s.retaliate(m) || m.intent&gamestate.IntentFollow == 0 {
		return
	}
	s.followLeader(m)
}

// bowTargetInSight is 0x00410E84's test for an idle member: with the bow
// active, a bow and arrows, a living member of the enemy group with a
// clear line.
func (s *startScene) bowTargetInSight(m *member) bool {
	e := &s.party[m.k]
	if e.WeaponMode != gamestate.WeaponBow || e.Equipment[gamestate.WeaponBow] == 0 || e.Quiver == 0 {
		return false
	}
	return s.firstShootable(&m.walker, e) >= 0
}

// followLeader is 0x0041FA5D for member m: at FollowDistance or more from the
// leader (group 0's first entity) it walks to a free formation cell near
// it (+2 |= 0x48; the follow bit stays).
func (s *startScene) followLeader(m *member) {
	l := s.lead()
	if l == nil || l == m || s.tables == nil {
		return
	}
	free := func(r, c int) bool { return !s.blocked(mapdata.Cell{Row: r, Col: c}) }
	r, c, ok := gamestate.FollowTarget(s.tables, l.cell.Row, l.cell.Col, m.cell.Row, m.cell.Col, s.rng.IntN, free)
	if !ok {
		return
	}
	m.intent |= gamestate.IntentWalk | gamestate.IntentIdle
	m.walkTo(mapdata.Cell{Row: r, Col: c})
}

// retaliate is 0x0041EC0C for idle member m: a neighbour of another group
// going for m (melee intent, target = m) is struck back (+6 = it, +2 |= 2,
// the stance, the swing). It reports whether m strikes.
func (s *startScene) retaliate(m *member) bool {
	if m.walking || len(m.path) > 0 || m.act != 0 {
		return false
	}
	e := &s.party[m.k]
	for _, n := range s.npcs {
		if n.intent&intentMelee == 0 || n.target != e.Index || !adjacent(m.cell, n.cell) ||
			!alive(&n.walker, &s.game.Entities[n.entity]) {
			continue
		}
		m.target, m.targetCell = n.entity, n.cell
		m.intent |= gamestate.IntentStrike
		gamestate.EnterStance(e)
		m.look = lookOf(*e)
		m.dir = facing(m.cell, n.cell)
		m.startAct(gamestate.ActAttack)
		return true
	}
	return false
}

// toggleSelectionStance is Space / the HUD stance button (0x0042179C) for
// the selection: the first selected member decides (enter or leave) and
// everyone follows; leaving it ends a melee swing.
func (s *startScene) toggleSelectionStance() {
	sel := s.selected()
	var es []*gamestate.Entity
	for _, m := range sel {
		es = append(es, &s.party[m.k])
	}
	gamestate.ToggleStance(es)
	for _, m := range sel {
		e := &s.party[m.k]
		if !e.InStance() {
			s.endFight(m) // the swing needs the stance's animation set
		}
		m.look = lookOf(*e)
	}
}

// selectWeapon is 1/2/3 and the HUD weapon button (0x0042192C) for the
// first selected member, or the whole selection with Shift (0x00A3E754).
func (s *startScene) selectWeapon(mode int) {
	sel := s.selected()
	if len(sel) == 0 {
		return
	}
	if !shiftHeld() {
		sel = sel[:1]
	}
	for _, m := range sel {
		e := &s.party[m.k]
		gamestate.SelectWeapon(e, mode)
		m.look = lookOf(*e)
	}
}

// partyPositions returns the cells of the living members (the enemy group
// as the creature and village alarm checks see it).
func (s *startScene) partyPositions() [][2]int {
	var out [][2]int
	for _, m := range s.livingMembers() {
		out = append(out, [2]int{m.cell.Row, m.cell.Col})
	}
	return out
}

// partyOnCells returns copies of the living members with their current
// cells (the village alarm and surrender).
func (s *startScene) partyOnCells() []gamestate.Entity {
	var out []gamestate.Entity
	for _, m := range s.livingMembers() {
		e := s.party[m.k]
		e.Row, e.Col = m.cell.Row, m.cell.Col
		out = append(out, e)
	}
	return out
}

// nearestMember is 0x004202A8 with the party as the enemy group: the living
// member closest to cell c (gamestate.Dist), -1 = none. With skip, members
// skip reports true for are left out (0x004203A0).
func (s *startScene) nearestMember(c mapdata.Cell, inside func(mapdata.Cell) bool, skip func(i int) bool) int {
	best, bestD := -1, 1<<30
	for _, m := range s.livingMembers() {
		i := s.party[m.k].Index
		if (inside != nil && !inside(m.cell)) || (skip != nil && skip(i)) {
			continue
		}
		if d := gamestate.Dist(c.Row, c.Col, m.cell.Row, m.cell.Col); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// revive is Game.Reviver for this scene: 0x00414B3C with the scene's
// occupancy and the hero's cell for character e (a party member's party
// copy is revived whatever record e points to, and e gets the result),
// then its walker takes the cell and idles; a revived NPC goes back to its
// AI (npc.liveMode).
func (s *startScene) revive(e *gamestate.Entity) bool {
	var w *walker
	target := e
	m, pe := s.partyMember(e.Index)
	n := s.byEntity[e.Index]
	switch {
	case m != nil:
		w, target = &m.walker, pe
	case n != nil:
		w, target = &n.walker, &s.game.Entities[e.Index]
	default:
		return false
	}
	if !target.Dead() {
		return false
	}
	hr, hc := w.cell.Row, w.cell.Col
	if l := s.lead(); l != nil {
		hr, hc = l.cell.Row, l.cell.Col
	}
	target.Row, target.Col = w.cell.Row, w.cell.Col
	free := func(r, c int) bool { return !s.blocked(mapdata.Cell{Row: r, Col: c}) }
	if !s.game.Revive(target, hr, hc, s.rng.IntN, free) {
		return false
	}
	if e != target {
		*e = *target
	}
	w.halt()
	w.cell = mapdata.Cell{Row: target.Row, Col: target.Col}
	s.occ[w.cell] = true
	w.act, w.actStep = 0, 0
	w.look = lookOf(*target)
	if m != nil {
		m.intent = gamestate.IntentIdle
		m.target = -1
	} else {
		n.mode, n.target, n.intent = n.liveMode, -1, 0
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
func (s *startScene) placeAgain(x gamestate.MapExit) {
	if s.tables == nil {
		return
	}
	s.storeWorld()
	for _, m := range s.members {
		if m != nil && alive(&m.walker, &s.party[m.k]) {
			m.halt()
			delete(s.occ, m.cell)
		}
	}
	s.game.SetArrival(x)
	free := func(r, c int) bool { return !s.blocked(mapdata.Cell{Row: r, Col: c}) }
	take := func(r, c int) { s.occ[mapdata.Cell{Row: r, Col: c}] = true }
	for _, i := range s.game.PlaceParty(s.tables, s.rng.IntN, free, take) {
		m, e := s.partyMember(i)
		if m == nil {
			continue
		}
		*e = s.game.Entities[i]
		m.cell = mapdata.Cell{Row: e.Row, Col: e.Col}
		m.targetCell = m.cell
		m.dir = mapdata.Direction(e.Variant & 7)
		m.intent = e.Flags2
		m.act, m.actStep = 0, 0
		m.pickIntent, m.resumeSet = false, false // +2 &= 0xF0 drops the walk bit 0x08
	}
	s.centerHero() // 0x00437832 → 0x00428F90
}

// reviveAll is «Слова Воскрешения» (0x00414FFB): every dead character of
// every group on this map is revived (0x00414B3C), the party first, then
// the others in group order.
func (s *startScene) reviveAll() {
	for _, m := range s.members {
		if m != nil && s.party[m.k].Dead() {
			s.revive(&s.party[m.k])
		}
	}
	order := append([]*npc(nil), s.npcs...)
	sort.SliceStable(order, func(i, j int) bool {
		a, b := s.game.Entities[order[i].entity], s.game.Entities[order[j].entity]
		return a.Group < b.Group || a.Group == b.Group && a.Index < b.Index
	})
	for _, n := range order {
		if e := &s.game.Entities[n.entity]; e.Dead() {
			s.revive(e)
		}
	}
}

// initSelection keeps the selection of the scene left (prev) for the
// members on this map that can act; an empty selection becomes the hero
// (the game starts with the hero selected).
func (s *startScene) initSelection(prev *startScene) {
	var sel []int
	for _, i := range s.sel {
		if m, e := s.partyMember(i); m != nil && !e.Dead() && !gamestate.Selected(sel, i) {
			sel = append(sel, i)
		}
	}
	if len(sel) == 0 && s.lead() != nil && !s.party[0].Dead() {
		sel = []int{s.party[0].Index}
	}
	s.sel = sel
	if prev != nil && prev.cur < len(s.party) {
		s.cur = prev.cur
	}
}
