package game

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/lanfight"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
)

// Other players in a LAN world. The server tells where each hero is
// (netplay.MsgAppear, MsgMoved, like the original's 0x5A and 0x19); a hero
// on this map is a walker that walks to each reported cell, drawn with the
// stance, weapon and equipment the server reports (MsgLook). Out of the
// stance a remote hero takes no orders or dialogue; the cursor over one
// shows its name and clan. In the stance it is a target like any other
// (lanfight.go): blows and arrows that reach it go to the server, which
// decides (MsgHit).

// remoteWalkLimit: a reported cell farther than this (in cells, Chebyshev)
// is a jump, not a walk; the hero is placed there at once.
const remoteWalkLimit = 24

type remotePlayer struct {
	info   netplay.PlayerInfo
	hero   *netplay.Hero
	pos    netplay.Position
	Look   netplay.Look
	hp     int
	dead   bool
	Entity int    // index in game.Entities, -1 until first placed
	items  [6]int // the equipment's item records in game (drawing only)
}

// remoteAt is the remote player whose hero is entity i, or nil.
func (s *startScene) remoteAt(i int) *remotePlayer {
	for _, r := range s.remotes {
		if r.Entity == i && i >= 0 {
			return r
		}
	}
	return nil
}

// remoteAppear adds or updates another player's hero (MsgAppear).
func (s *startScene) remoteAppear(a netplay.AppearPayload) {
	if s.remotes == nil {
		s.remotes = map[string]*remotePlayer{}
	}
	r := s.remotes[a.Player.ID]
	if r == nil {
		r = &remotePlayer{Entity: -1}
		s.remotes[a.Player.ID] = r
	}
	r.info, r.hero, r.Look, r.hp, r.dead = a.Player, a.Hero, a.Look, a.HP, a.Dead
	if n := s.ByEntity[r.Entity]; n != nil {
		s.removeNPC(n) // entered again: placed anew, alive
	}
	if r.Entity >= 0 {
		e := &s.Game.Entities[r.Entity]
		e.Flags4 &^= 0x80
		e.HP = r.hp
	}
	s.remoteMove(a.Player.ID, a.Pos)
}

// remoteMove follows another hero to its reported position (MsgMoved): on
// this map it walks there (or is placed there after a jump), off this map
// it is taken off.
func (s *startScene) remoteMove(id string, pos netplay.Position) {
	r := s.remotes[id]
	if r == nil {
		return
	}
	r.pos = pos
	n := s.ByEntity[r.Entity]
	if r.dead {
		return // the dead lie where they fell
	}
	if pos.Map != s.MapNumber {
		if n != nil {
			s.removeNPC(n)
		}
		return
	}
	cell := mapdata.Cell{Row: pos.Row, Col: pos.Col}
	if n == nil || chebyshev(n.Cell, cell) > remoteWalkLimit {
		if n != nil {
			s.removeNPC(n)
		}
		s.placeRemote(r)
		return
	}
	n.WalkTo(cell)
}

// remoteLeave removes a player who left the session (MsgPlayerLeft).
func (s *startScene) remoteLeave(id string) {
	r := s.remotes[id]
	if r == nil {
		return
	}
	if n := s.ByEntity[r.Entity]; n != nil {
		s.removeNPC(n)
	}
	delete(s.remotes, id)
}

// placeRemotes puts the remote heroes on this map after it was built (a map
// change keeps the remotes and their entities).
func (s *startScene) placeRemotes() {
	for _, r := range s.remotes {
		if r.pos.Map == s.MapNumber && !r.dead {
			s.placeRemote(r)
		}
	}
}

// placeRemote makes r's hero entity on first use (the own hero's template
// with r's model and palette, the account's values on top; in an
// emptied group, so no party or village rule applies to it) and puts its
// walker on r's cell, or the nearest free one.
func (s *startScene) placeRemote(r *remotePlayer) {
	if len(s.Game.Groups) == 0 {
		return
	}
	if r.Entity < 0 {
		e := s.Game.Entities[s.Game.Groups[0].First]
		model := min(max(r.info.Model, 0), netplay.Models-1)
		t := gamestate.NewHeroTemplate()
		gamestate.SelectHero(&t, model)
		// As the own hero's start (StartHero): model, name and palette from
		// the selection, the account's values on top.
		e.Model, e.NameIndex, e.PaletteOffset = t.Model, t.NameIndex, t.PaletteOffset
		if r.hero != nil {
			lanfight.ApplyHero(&e, r.hero)
		}
		// Out of the stance, idle (type 6); its own items come with the
		// look (applyLook), never the own hero's.
		e.Flags3 &^= 0x04
		e.Type, e.Step, e.Quiver, e.Bag, e.Equipment = 6, 0, 0, [32]int{}, [6]int{}
		e.Group = len(s.Game.Groups) - 1 // emptied by EnterLANWorld: no group rules
		e.Index = len(s.Game.Entities)
		s.Game.Entities = append(s.Game.Entities, e)
		r.Entity = e.Index
		s.applyLook(r)
		if s.Game.Names == nil {
			s.Game.Names = map[int][]byte{}
		}
		s.Game.Names[e.Index] = cp866(r.info.Name)
	}
	e := &s.Game.Entities[r.Entity]
	e.HP = r.hp
	cell := mapdata.NearestFree(mapdata.Cell{Row: r.pos.Row, Col: r.pos.Col}, func(c mapdata.Cell) bool {
		return s.Walk.Blocked(c.Row, c.Col) || s.Occ[c]
	}, 6)
	e.Row, e.Col, e.Variant = cell.Row, cell.Col, r.pos.Facing&7
	upload := newUploader()
	p := gamestate.Placed{Entity: *e, Row: cell.Row, Col: cell.Col}
	n := &npc{
		Walker: walker{
			Entity: r.Entity, Cell: cell, Dir: mapdata.Direction(r.pos.Facing & 7), Idle: -1,
			Look: lookOf(*e),
			Reserve: func(c mapdata.Cell) bool {
				if s.Occ[c] {
					return false
				}
				s.Occ[c] = true
				return true
			},
			Release: func(c mapdata.Cell) { delete(s.Occ, c) },
			Sound:   s.actionSound,
		},
		Mode: npcRemote, LiveMode: npcRemote, Target: -1, Rng: s.Rng.IntN,
	}
	withSprites(&n.Walker, entityLoader(s.Game, p, liveEntity(s.Game, r.Entity), s.assets.objects, s.assets.graph, s.Tables,
		func(sp *resfmt.EntitySprite) (*ebiten.Image, *ebiten.Image) { return upload(sp.Image, sp.Shadow) },
		func(err error) { log.Printf("LAN player %s: %v", r.info.Name, err) }))
	n.Blocked = s.Blocked
	s.Occ[cell] = true
	s.NPCs = append(s.NPCs, n)
	s.ByEntity[r.Entity] = n
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

// reportMove sends the own hero's position when its cell, facing or map
// changed (MsgMove).
func (s *startScene) reportMove() {
	if s.onMove == nil || s.Hero == nil {
		return
	}
	p := netplay.Position{Map: s.MapNumber, Row: s.Hero.Cell.Row, Col: s.Hero.Cell.Col, Facing: int(s.Hero.Dir) & 7}
	if p != s.lastMove {
		s.lastMove = p
		s.onMove(p)
	}
}

// faceRemotes turns idle remote heroes to their reported facing.
func (s *startScene) faceRemotes() {
	for _, r := range s.remotes {
		if n := s.ByEntity[r.Entity]; n != nil && n.IsIdle() && n.Cell == (mapdata.Cell{Row: r.pos.Row, Col: r.pos.Col}) {
			n.Dir = mapdata.Direction(r.pos.Facing & 7)
		}
	}
}

// remoteStatus is the status line over a remote hero: its name, «, клан»
// and the clan.
func remoteStatus(r *remotePlayer) []byte {
	s := r.info.Name
	if r.info.Clan != "" {
		s += ", клан " + r.info.Clan
	}
	return cp866(s)
}

func chebyshev(a, b mapdata.Cell) int {
	d := func(x int) int {
		if x < 0 {
			return -x
		}
		return x
	}
	return max(d(a.Row-b.Row), d(a.Col-b.Col))
}

// playerCounts is the number of heroes per map for the LAN map window: the
// own hero and every remote hero the server reported.
func (s *startScene) playerCounts() map[int]int {
	c := map[int]int{s.MapNumber: 1}
	for _, r := range s.remotes {
		c[r.pos.Map]++
	}
	return c
}

// applyLook puts r's look on its entity: the stance (type 0 or 6 with the
// stance bit, as ToggleStance), the weapon mode and the equipment as item
// records of this game that only drawing reads.
func (s *startScene) applyLook(r *remotePlayer) {
	if r.Entity < 0 {
		return
	}
	e := &s.Game.Entities[r.Entity]
	if !e.Dead() && e.HP > 0 {
		if r.Look.Stance {
			gamestate.EnterStance(e)
		} else {
			gamestate.LeaveStance(e)
		}
	}
	e.WeaponMode = r.Look.Weapon
	for k, it := range r.Look.Equip {
		if !it.Item {
			if r.items[k] != 0 {
				s.Game.SetItemKind(r.items[k], -1, 0)
				r.items[k] = 0
			}
			e.Equipment[k] = 0
			continue
		}
		r.items[k] = s.Game.SetItemKind(r.items[k], it.Category, it.Type)
		e.Equipment[k] = r.items[k]
	}
	if n := s.ByEntity[r.Entity]; n != nil {
		n.Look = lookOf(*e)
	}
}

// remoteLook is another hero's new look (MsgLook).
func (s *startScene) remoteLook(l netplay.LookPayload) {
	if r := s.remotes[l.ID]; r != nil {
		r.Look = l.Look
		s.applyLook(r)
	}
}

// reportLook sends the own hero's look when its stance, weapon or equipment
// changed (MsgLook).
func (s *startScene) reportLook() {
	if s.onLook == nil || len(s.Party) == 0 {
		return
	}
	l := lanfight.LookOf(s.Game, s.Party[0])
	if l != s.lastLook {
		s.lastLook = l
		s.onLook(l)
	}
}
