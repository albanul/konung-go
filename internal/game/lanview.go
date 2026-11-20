package game

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/lanfight"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
	"konung1.5/internal/world"
)

// The LAN world as a viewer (docs/konung_lan.md#server-world): the server
// runs the world (internal/lanserver, as LAN_KONG.EXE's server) and the
// scene shows the own hero's map as the server reports it. Nothing is
// simulated here (World.Step does not run): every character on the map is
// a walker whose cell, walk step, facing, action and HP are the server's
// (netplay.MsgView, the original's 0x19); the characters other than the
// players' heroes are GAME.RES's records, read here too; the players'
// heroes are built from what the server says of them (MsgPlayer, 0x5A).
// Orders go to the server (MsgOrder, 0x55): a left click walks, a right
// click attacks (in the stance) or walks with a pick-up, Space, 1/2/3 and
// A as in single player. The own hero is party member 0 (group 0 here; its
// entity is its server slot) and feeds the HUD.
//
// Items, piles and the pile window are the server's (lanitems.go), and so
// are dialogues and trade. The own hirelings (MsgParty) are party members
// here too: their portraits, the selection (portrait clicks, Shift, a right
// click on them) and «Все ко мне»; orders carry the selection. Not
// synchronised yet, so off in a LAN world: the shovel's dig mode, a
// hireling's character window.

// lanView is a viewer scene's link to the server.
type lanView struct {
	slot    int                          // the own hero's entity (its server slot)
	order   func(o netplay.OrderPayload) // sends an order
	players map[int]*viewPlayer          // the players' heroes by slot
	tmpl    [netplay.Models]*gamestate.Game
	attach  world.Attach // sprites for a new party member (the scene's)
	// party is the own hirelings as the server last said (MsgParty);
	// home is a hired character's GAME.RES group (to put back when it
	// leaves a party).
	party []int
	home  map[int]int
	// arrows are the arrows the server released that have not flown yet;
	// arrowTick the server tick the scene's arrows have flown to
	// (lanarrows.go).
	arrows    []viewArrow
	arrowTick int
	// list is the player list and own quest log the server last sent
	// (MsgPlayers, lanplayers.go).
	list *netplay.PlayersPayload
	// reported is each character's last report and its tick: between
	// reports the scene shows the report advanced by
	// netplay.CharState.Predict, as the server expects.
	reported map[int]reportedChar
}

type reportedChar struct {
	cs   netplay.CharState
	tick int
}

// viewPlayer is a player's hero as the server described it.
type viewPlayer struct {
	p     netplay.PlayerPayload
	items [netplay.EquipSlots]int // its equipment's item records here (drawing only)
}

// lanView builds the viewer scene for a hero the server let in (the
// client's 0x8C handler 0x0042105C and state 10, 0x0042F620): GAME.RES's
// world, the MAPS.RES map, and the own hero (the type's GAME.N hero with
// the account's values, lanfight.Hero) as the only party member.
func (a *sceneAssets) lanView(gameDir string, exe *gamestate.LANExe, at netplay.EnterPayload, hero *netplay.Hero, model int, name string) (*startScene, error) {
	data, err := readGameFile(gameDir, "MAPS.RES")
	if err != nil {
		return nil, err
	}
	maps, err := mapdata.SplitMapsRes(data)
	if err != nil {
		return nil, err
	}
	if hero != nil {
		model = hero.Model
	}
	if model < 0 || model >= netplay.Models || at.Slot < 0 || at.Slot >= world.LANPlayers {
		return nil, fmt.Errorf("hero model %d, slot %d", model, at.Slot)
	}
	res, err := readGameFile(gameDir, "GAME.RES")
	if err != nil {
		return nil, err
	}
	lw, err := gamestate.ParseLANWorld(res)
	if err != nil {
		return nil, err
	}
	la := *a // the same resources, maps from MAPS.RES
	la.lanMaps = maps
	la.view = true
	if exe != nil {
		if into, err := readGameFile(gameDir, "INTO640R.RES"); err != nil {
			log.Printf("LAN map window: %v", err)
		} else if la.lanMap, err = newLANWorldMap(exe, into, a.graph); err != nil {
			log.Printf("LAN map window: %v", err)
		}
	}
	v := &lanView{slot: at.Slot, players: map[int]*viewPlayer{}, home: map[int]int{}}
	for m := range v.tmpl {
		la.gamePath = filepath.Join(a.mapDir, fmt.Sprintf("GAME.%d", m))
		data, err := os.ReadFile(la.gamePath)
		if err != nil {
			return nil, err
		}
		if v.tmpl[m], err = gamestate.Parse(data); err != nil {
			return nil, fmt.Errorf("%s: %w", la.gamePath, err)
		}
	}
	game := lw.Game
	e := v.heroEntity(game, model, hero, at.Slot)
	e.Group = 0 // the own party is group 0 here, as single player's HUD rules expect
	e.Row, e.Col, e.Variant = at.Row, at.Col, at.Facing&7
	game.Entities[at.Slot] = e
	game.Groups[0].First, game.Groups[0].Count, game.Groups[0].Map = at.Slot, 1, at.Map
	if game.Names == nil {
		game.Names = map[int][]byte{}
	}
	if name != "" {
		game.Names[at.Slot] = cp866(name) // the player's name, not the template's (Волк…)
	}
	la.viewState = v
	return la.buildScene(game, at.Map, nil, nil)
}

// heroEntity is a player's hero in game: the type's GAME.N hero with the
// account's values (lanfight.Hero), its gear copied into game's items.
func (v *lanView) heroEntity(game *gamestate.Game, model int, hero *netplay.Hero, slot int) gamestate.Entity {
	model = min(max(model, 0), netplay.Models-1)
	src := v.tmpl[model]
	e := lanfight.Hero(src, model, hero)
	for k, i := range e.Equipment {
		e.Equipment[k] = game.CopyItemFrom(src, i)
	}
	e.Quiver = game.CopyItemFrom(src, e.Quiver)
	for k, i := range e.Bag {
		e.Bag[k] = game.CopyItemFrom(src, i)
	}
	gamestate.LeaveStance(&e)
	e.Flags4 &^= 0x80
	e.Flags3 &^= 0x80
	e.Index = slot
	return e
}

// placeOwn puts the own hero on the scene as party member 0, at its
// record's cell (lanView, a map change).
func (s *startScene) placeOwn(attach world.Attach) {
	e := s.Game.Entities[s.view.slot]
	s.AddMember(s.view.slot, mapdata.Cell{Row: e.Row, Col: e.Col}, e.Variant, attach)
}

// viewGroup is the group a remote player's hero is drawn in here: its
// slot, except slot 0 (the own party's group here), which takes the own
// slot's group (unused here).
func (v *lanView) viewGroup(slot int) int {
	if slot == 0 {
		return v.slot
	}
	return slot
}

// applyPlayer takes a player's hero's description (MsgPlayer): its record
// (model, palette, values, name) and its look.
func (s *startScene) applyPlayer(p netplay.PlayerPayload) {
	v := s.view
	if p.Slot == v.slot || p.Slot < 0 || p.Slot >= world.LANPlayers {
		return
	}
	vp := v.players[p.Slot]
	fresh := vp == nil || vp.p.Player.Model != p.Player.Model || vp.p.Player.ID != p.Player.ID
	if vp == nil {
		vp = &viewPlayer{}
		v.players[p.Slot] = vp
	}
	vp.p = p
	if fresh {
		e := v.heroEntity(s.Game, p.Player.Model, p.Hero, p.Slot)
		e.Equipment, e.Quiver, e.Bag = [6]int{}, 0, [32]int{} // its own items come with the look
		e.Group = v.viewGroup(p.Slot)
		old := s.Game.Entities[p.Slot]
		e.Row, e.Col, e.Variant, e.HP = old.Row, old.Col, old.Variant, old.HP
		s.Game.Entities[p.Slot] = e
		s.Game.Names[p.Slot] = cp866(p.Player.Name)
		if n := s.ByEntity[p.Slot]; n != nil {
			s.removeNPC(n) // drawn anew with the new record
		}
	}
	s.applyPlayerLook(p.Slot, vp)
}

// applyPlayerLook puts a player's look on its record: stance, weapon and
// the equipment as item records of this game that only drawing reads.
func (s *startScene) applyPlayerLook(slot int, vp *viewPlayer) {
	e := &s.Game.Entities[slot]
	l := vp.p.Look
	for k, it := range l.Equip {
		if !it.Item {
			if vp.items[k] != 0 {
				s.Game.SetItemKind(vp.items[k], -1, 0)
				vp.items[k] = 0
			}
			e.Equipment[k] = 0
			continue
		}
		// From the table's top: the low indices mirror the server's items.
		vp.items[k] = s.Game.SetItemKindTop(vp.items[k], it.Category, it.Type)
		e.Equipment[k] = vp.items[k]
	}
	setStance(e, l.Stance)
	e.WeaponMode = l.Weapon
	if n := s.ByEntity[slot]; n != nil {
		n.Look = lookOf(*e)
	}
}

// setStance puts e into the combat stance or out of it (the action type's
// set and the stance bit, as ToggleStance).
func setStance(e *gamestate.Entity, on bool) {
	if e.Dead() || e.InStance() == on {
		return
	}
	if on {
		gamestate.EnterStance(e)
	} else {
		gamestate.LeaveStance(e)
	}
}

// applyView takes the server's view of the map (MsgView): every
// character's state; characters new to the scene become walkers, gone
// ones leave.
func (s *startScene) applyView(v netplay.ViewPayload) {
	if v.Map != s.MapNumber {
		return // the map change (MsgMapChange) comes first
	}
	s.Tick = v.Tick         // the server's clock: time of day
	s.flyViewArrows(v.Tick) // on the cells of the last view, as the server's flight
	if v.Full || s.view.reported == nil {
		s.view.reported = map[int]reportedChar{}
	}
	if v.Full {
		keep := map[int]bool{}
		for _, c := range v.Chars {
			keep[c.Entity] = true
		}
		for _, n := range append([]*npc(nil), s.NPCs...) {
			if !keep[n.Entity] {
				s.removeNPC(n)
			}
		}
	}
	// The characters not reported: their last report advanced to this
	// tick (walk and action steps, the next cells of a walk).
	got := map[int]bool{}
	for _, c := range v.Chars {
		got[c.Entity] = true
	}
	for e, r := range s.view.reported {
		if !got[e] && (r.cs.Walking || len(r.cs.Path) > 0 || r.cs.Act != 0) {
			s.applyChar(r.cs.Predict(r.tick, v.Tick))
		}
	}
	for _, c := range v.Chars {
		s.view.reported[c.Entity] = reportedChar{c, v.Tick}
		s.applyChar(c)
	}
	for _, e := range v.Gone {
		delete(s.view.reported, e)
		if n := s.ByEntity[e]; n != nil {
			s.removeNPC(n)
		}
	}
}

// applyChar sets a character's walker and record to the server's state.
func (s *startScene) applyChar(c netplay.CharState) {
	if c.Entity < 0 || c.Entity >= len(s.Game.Entities) {
		return
	}
	var w *walker
	var e *gamestate.Entity
	if m, pe := s.PartyMember(c.Entity); m != nil {
		w, e = &m.Walker, pe // the own hero, an own hireling
	} else if c.Entity == s.view.slot {
		return
	} else {
		s.view.ownerGroup(s.Game, c)
		n := s.ByEntity[c.Entity]
		if n == nil {
			n = s.viewNPC(c)
		}
		w, e = &n.Walker, &s.Game.Entities[c.Entity]
	}
	cell := mapdata.Cell{Row: c.Row, Col: c.Col}
	from := cell
	if c.Walking {
		from = mapdata.Cell{Row: c.FromRow, Col: c.FromCol}
	}
	if !e.Dead() {
		delete(s.Occ, w.Cell)
		delete(s.Occ, w.From)
	}
	w.Cell, w.From, w.Goal, w.Walking, w.StepT = cell, from, cell, c.Walking, c.Step
	w.Path = nil
	w.Dir = mapdata.Direction(c.Facing & 7)
	if c.Act != w.Act || c.ActStep != w.ActStep {
		w.Act, w.ActStep = c.Act, c.ActStep
	}
	if w.Walking || w.Act != 0 {
		w.Idle = -1
	}
	e.Row, e.Col, e.Variant = c.Row, c.Col, c.Facing&7
	e.HP = c.HP
	if c.Dead {
		e.Flags4 |= 0x80
	} else {
		e.Flags4 &^= 0x80
		s.Occ[cell] = true
		if c.Walking {
			s.Occ[from] = true
		}
	}
	setStance(e, c.Stance)
	e.WeaponMode = c.Weapon
	w.Look = lookOf(*e)
}

// viewNPC makes the walker of a character the server reports for the first
// time: a GAME.RES record, or a player's hero (applyPlayer). It runs no AI.
func (s *startScene) viewNPC(c netplay.CharState) *npc {
	e := &s.Game.Entities[c.Entity]
	e.Row, e.Col, e.Variant = c.Row, c.Col, c.Facing&7
	if c.Player && e.Group < 0 {
		e.Group = s.view.viewGroup(c.Entity)
	}
	p := gamestate.Placed{Entity: *e, Row: c.Row, Col: c.Col}
	upload := newUploader()
	n := &npc{
		Walker: walker{
			Entity: c.Entity, Cell: mapdata.Cell{Row: c.Row, Col: c.Col}, Dir: mapdata.Direction(c.Facing & 7), Idle: -1,
			Look:    lookOf(*e),
			Sound:   s.actionSound,
			Blocked: s.Blocked,
		},
		Mode: npcRemote, LiveMode: npcRemote, Target: -1, Rng: s.Rng.IntN,
	}
	withSprites(&n.Walker, entityLoader(s.Game, p, liveEntity(s.Game, c.Entity), s.assets.objects, s.assets.graph, s.Tables,
		func(sp *resfmt.EntitySprite) (*ebiten.Image, *ebiten.Image) { return upload(sp.Image, sp.Shadow) },
		func(err error) { log.Printf("LAN character %d: %v", c.Entity, err) }))
	s.NPCs = append(s.NPCs, n)
	s.ByEntity[c.Entity] = n
	return n
}

// removeNPC takes a walker off the map and frees its cells.
func (s *startScene) removeNPC(n *npc) {
	delete(s.Occ, n.Cell)
	delete(s.Occ, n.From)
	delete(s.ByEntity, n.Entity)
	for i, m := range s.NPCs {
		if m == n {
			s.NPCs = append(s.NPCs[:i], s.NPCs[i+1:]...)
			break
		}
	}
}

// applyEvents plays the server's sounds (MsgEvents): a sound at a cell, or
// an action's sound by the walker that started it; arrows fly, messages
// show, and village buildings take the server's states (their sprites and
// flames follow, updateBuildings).
func (s *startScene) applyEvents(ev []netplay.Event) {
	buildings := false
	defer func() {
		if buildings && s.Tables != nil {
			s.updateBuildings()
		}
	}()
	for _, e := range ev {
		switch e.Kind {
		case netplay.EventBuilding:
			if e.Building != nil && s.Game.SetBuildingState(s.MapNumber, e.Building.Placement, e.Building.State) {
				buildings = true
			}
		case netplay.EventSound:
			s.playAt(e.Sound, mapdata.Cell{Row: e.Row, Col: e.Col})
		case netplay.EventAction:
			if w, _ := s.Combatant(e.Entity); w != nil {
				s.actionSound(w, e.Act)
			}
		case netplay.EventMsg:
			s.showMessage(e.Text) // a pick-up's or an item click's message
		case netplay.EventArrow:
			s.addViewArrow(e)
		}
	}
}

// ownHero takes the server's values after a change of the own hero (the
// kill experience, MsgHero; the original's 0x5C).
func (s *startScene) ownHero(h netplay.Hero) {
	if len(s.Party) == 0 {
		return
	}
	e := &s.Party[0]
	e.FreeExp, e.Fencing, e.Marksmanship = h.FreePoints, h.Fencing, h.Archery
}

// viewChangeMap is the own hero's map change (MsgMapChange): the scene is
// built for the new map with the hero at its arrival cell; the
// characters there come with the next view.
func (s *startScene) viewChangeMap(at netplay.EnterPayload) {
	if at.Map == s.MapNumber {
		return
	}
	e := s.Party[0]
	e.Row, e.Col, e.Variant = at.Row, at.Col, at.Facing&7
	s.Game.Entities[s.view.slot] = e
	s.Game.Groups[0].Map = at.Map
	ns, err := s.assets.buildScene(s.Game, at.Map, s, nil)
	if err != nil {
		log.Printf("LAN map %d: %v", at.Map, err)
		return
	}
	s.view.arrows, s.view.arrowTick = nil, 0 // the arrows stay on the map left
	hideHelp, amulets := s.hideHelp, s.amulets
	*s = *ns
	s.hideHelp = hideHelp // F1 stays as the player left it
	s.amulets = amulets   // the amulet state is the world's, not the map's
	s.installHooks()
}

// viewFrame runs the viewer's own animation for one 60 ms frame: idle
// loops of the characters standing (the server sends no idle frames).
func (s *startScene) viewFrame() {
	idle := func(w *walker) {
		if w.Walking || w.Act != 0 {
			return
		}
		if n := w.FrameCount(0); n > 0 {
			w.Idle = (w.Idle + 1) % n
		}
	}
	for _, m := range s.Members {
		idle(&m.Walker)
	}
	for _, n := range s.NPCs {
		idle(&n.Walker)
	}
}

// viewOrder sends an order for the own hero; walks, right clicks and the
// stance go to the selection (its hero and hirelings).
func (s *startScene) viewOrder(o netplay.OrderPayload) {
	if len(s.Party) > 0 && s.Party[0].Dead() {
		return // the dead give no orders; the server ignores them too
	}
	switch o.Kind {
	case netplay.OrderWalk, netplay.OrderRight, netplay.OrderStance:
		if o.Sel == nil {
			o.Sel = append([]int(nil), s.Sel...)
		}
	}
	if s.view != nil && s.view.order != nil {
		s.view.order(o)
	}
}

// viewClick is a click on the map in a viewer scene: right on a character
// with a dialogue out of the stance talks to it, right on a character
// (not the own hero) attacks it in the stance (the server decides), right
// elsewhere walks there with a pick-up, left walks.
func (s *startScene) viewClick(mx, my int, right bool) {
	c := mapdata.CellAt(mx+s.camX, my+s.camY)
	switch {
	case right && s.lanTalkHireling(s.hovered):
		// Ctrl over an own hireling: its dialogue (the port's dismissal,
		// gamestate.LANMemberDialogue), as single player's 0x004239EE.
		s.viewOrder(netplay.OrderPayload{Kind: netplay.OrderTalk, Target: s.hovered})
	case right && s.hovered >= 0 && s.PartyIndex(s.hovered) >= 0:
		s.selectInWorld(s.hovered) // 0x004239EE: an own party member: the selection
	case right && s.lanCanTalk():
		// The original's 0x43: the server opens the dialogue (talk.go).
		s.viewOrder(netplay.OrderPayload{Kind: netplay.OrderTalk, Target: s.hovered})
	case right && s.hovered >= 0 && s.hovered != s.view.slot:
		s.viewOrder(netplay.OrderPayload{Kind: netplay.OrderRight, Target: s.hovered, Row: c.Row, Col: c.Col})
	case right:
		s.viewOrder(netplay.OrderPayload{Kind: netplay.OrderRight, Target: -1, Row: c.Row, Col: c.Col})
	default:
		s.viewOrder(netplay.OrderPayload{Kind: netplay.OrderWalk, Row: c.Row, Col: c.Col})
	}
}

// viewStatus is the status line over a player's hero in a viewer scene:
// its name and «, клан» clan; ok false for other characters.
func (s *startScene) viewStatus(i int) ([]byte, bool) {
	if s.view == nil {
		return nil, false
	}
	vp := s.view.players[i]
	if vp == nil {
		return nil, false
	}
	t := vp.p.Player.Name
	if vp.p.Player.Clan != "" {
		t += ", клан " + vp.p.Player.Clan
	}
	return cp866(t), true
}

// playerCounts is the number of heroes per map for the LAN map window:
// from the server's player list (MsgPlayers), or before it arrives the own
// hero and the other players' heroes on its map.
func (s *startScene) playerCounts() map[int]int {
	if s.view != nil && s.view.list != nil {
		// Every player's map from the server's list (the original counts
		// the 16 player groups' +0x1B).
		c := map[int]int{}
		for _, p := range s.view.list.Players {
			c[p.Map]++
		}
		return c
	}
	c := map[int]int{s.MapNumber: 1}
	if s.view != nil {
		for slot := range s.view.players {
			if n := s.ByEntity[slot]; n != nil {
				c[s.MapNumber]++
			}
		}
	}
	return c
}

// ownerGroup puts a character's group as the view says: another player's
// hireling (Owner) is drawn in that player's group, a character back from
// a party in its GAME.RES group.
func (v *lanView) ownerGroup(game *gamestate.Game, c netplay.CharState) {
	if c.Entity < world.LANPlayers || c.Entity >= len(game.Entities) {
		return
	}
	e := &game.Entities[c.Entity]
	switch {
	case c.Owner > 0:
		if _, ok := v.home[c.Entity]; !ok {
			v.home[c.Entity] = e.Group
		}
		e.Group = v.viewGroup(c.Owner - 1)
	default:
		if g, ok := v.home[c.Entity]; ok {
			e.Group = g
			delete(v.home, c.Entity)
		}
	}
}

// applyParty takes the own hirelings (MsgParty): characters hired become
// party members (group 0 here, with portraits and the selection), ones
// gone from the party walkers of the world again.
func (s *startScene) applyParty(p netplay.PartyPayload) {
	v := s.view
	cur := -1
	if len(s.Party) > 0 {
		cur = s.Current().Index
	}
	v.party = append([]int(nil), p.Hirelings...)
	want := map[int]bool{}
	for _, i := range p.Hirelings {
		want[i] = true
	}
	for _, e := range append([]gamestate.Entity(nil), s.Party...) {
		i := e.Index
		if i == v.slot || want[i] {
			continue
		}
		m, pe := s.PartyMember(i)
		if m == nil {
			continue
		}
		cs := netplay.CharState{Entity: i, Row: m.Cell.Row, Col: m.Cell.Col, Facing: int(m.Dir),
			HP: pe.HP, Dead: pe.Dead(), Stance: pe.InStance(), Weapon: pe.WeaponMode}
		s.RemoveMember(i)
		s.applyChar(cs) // a walker of the world again (ownerGroup puts its group back)
	}
	for _, i := range p.Hirelings {
		if m, _ := s.PartyMember(i); m != nil || i < world.LANPlayers || i >= len(s.Game.Entities) {
			continue
		}
		e := &s.Game.Entities[i]
		if _, ok := v.home[i]; !ok {
			v.home[i] = e.Group
		}
		cell, dir := mapdata.Cell{Row: e.Row, Col: e.Col}, e.Variant
		if n := s.ByEntity[i]; n != nil {
			cell, dir = n.Cell, int(n.Dir)
			s.removeNPC(n)
		}
		e.Group = 0 // the own party here
		s.AddMember(i, cell, dir, v.attach)
	}
	// The window's member stays when it is still in the party; else the
	// hero.
	s.Cur = 0
	for k, e := range s.Party {
		if e.Index == cur {
			s.Cur = k
		}
	}
}
