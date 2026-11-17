package game

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
)

// Other players in a LAN world. The server tells where each hero is
// (netplay.MsgAppear, MsgMoved, like the original's 0x5A and 0x19); a hero
// on this map is a walker that walks to each reported cell. Remote heroes
// cannot be clicked: no orders, dialogue or selection reach them, and the
// cursor over one shows its name and clan in the status line.

// remoteWalkLimit: a reported cell farther than this (in cells, Chebyshev)
// is a jump, not a walk; the hero is placed there at once.
const remoteWalkLimit = 24

type remotePlayer struct {
	info   netplay.PlayerInfo
	hero   *netplay.Hero
	pos    netplay.Position
	entity int // index in game.Entities, -1 until first placed
}

// remoteAt is the remote player whose hero is entity i, or nil.
func (s *startScene) remoteAt(i int) *remotePlayer {
	for _, r := range s.remotes {
		if r.entity == i && i >= 0 {
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
		r = &remotePlayer{entity: -1}
		s.remotes[a.Player.ID] = r
	}
	r.info, r.hero = a.Player, a.Hero
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
	n := s.byEntity[r.entity]
	if pos.Map != s.mapNumber {
		if n != nil {
			s.removeNPC(n)
		}
		return
	}
	cell := mapdata.Cell{Row: pos.Row, Col: pos.Col}
	if n == nil || chebyshev(n.cell, cell) > remoteWalkLimit {
		if n != nil {
			s.removeNPC(n)
		}
		s.placeRemote(r)
		return
	}
	n.walkTo(cell)
}

// remoteLeave removes a player who left the session (MsgPlayerLeft).
func (s *startScene) remoteLeave(id string) {
	r := s.remotes[id]
	if r == nil {
		return
	}
	if n := s.byEntity[r.entity]; n != nil {
		s.removeNPC(n)
	}
	delete(s.remotes, id)
}

// placeRemotes puts the remote heroes on this map after it was built (a map
// change keeps the remotes and their entities).
func (s *startScene) placeRemotes() {
	for _, r := range s.remotes {
		if r.pos.Map == s.mapNumber {
			s.placeRemote(r)
		}
	}
}

// placeRemote makes r's hero entity on first use (the own hero's template
// with r's model and palette, the account's values on top; in an
// emptied group, so no party or village rule applies to it) and puts its
// walker on r's cell, or the nearest free one.
func (s *startScene) placeRemote(r *remotePlayer) {
	if len(s.game.Groups) == 0 {
		return
	}
	if r.entity < 0 {
		e := s.game.Entities[s.game.Groups[0].First]
		model := min(max(r.info.Model, 0), netplay.Models-1)
		t := gamestate.NewHeroTemplate()
		gamestate.SelectHero(&t, model)
		// As the own hero's start (StartHero): model, name and palette from
		// the selection; the equipment stays the template's (SelectHero's
		// gear list is the creation screen's preview, not world items).
		e.Model, e.NameIndex, e.PaletteOffset = t.Model, t.NameIndex, t.PaletteOffset
		e.WeaponMode, e.Type, e.Step = t.WeaponMode, 0, 0
		if r.hero != nil {
			applyLANHero(&e, r.hero)
		}
		e.Group = len(s.game.Groups) - 1 // emptied by EnterLANWorld: no group rules
		e.Index = len(s.game.Entities)
		s.game.Entities = append(s.game.Entities, e)
		r.entity = e.Index
	}
	e := &s.game.Entities[r.entity]
	cell := mapdata.NearestFree(mapdata.Cell{Row: r.pos.Row, Col: r.pos.Col}, func(c mapdata.Cell) bool {
		return s.walk.Blocked(c.Row, c.Col) || s.occ[c]
	}, 6)
	e.Row, e.Col, e.Variant = cell.Row, cell.Col, r.pos.Facing&7
	upload := newUploader()
	p := gamestate.Placed{Entity: *e, Row: cell.Row, Col: cell.Col}
	n := &npc{
		walker: walker{
			entity: r.entity, cell: cell, dir: mapdata.Direction(r.pos.Facing & 7), idle: -1,
			look: lookOf(*e),
			load: entityLoader(s.game, p, liveEntity(s.game, r.entity), s.assets.objects, s.assets.graph, s.tables,
				func(sp *resfmt.EntitySprite) (*ebiten.Image, *ebiten.Image) { return upload(sp.Image, sp.Shadow) },
				func(err error) { log.Printf("LAN player %s: %v", r.info.Name, err) }),
			reserve: func(c mapdata.Cell) bool {
				if s.occ[c] {
					return false
				}
				s.occ[c] = true
				return true
			},
			release: func(c mapdata.Cell) { delete(s.occ, c) },
			sound:   s.actionSound,
		},
		mode: npcRemote, liveMode: npcRemote, target: -1, rng: s.rng.IntN,
	}
	n.blocked = s.blocked
	s.occ[cell] = true
	s.npcs = append(s.npcs, n)
	s.byEntity[r.entity] = n
}

// removeNPC takes a walker off the map and frees its cells.
func (s *startScene) removeNPC(n *npc) {
	delete(s.occ, n.cell)
	delete(s.occ, n.from)
	delete(s.byEntity, n.entity)
	for i, m := range s.npcs {
		if m == n {
			s.npcs = append(s.npcs[:i], s.npcs[i+1:]...)
			break
		}
	}
}

// reportMove sends the own hero's position when its cell, facing or map
// changed (MsgMove).
func (s *startScene) reportMove() {
	if s.onMove == nil || s.hero == nil {
		return
	}
	p := netplay.Position{Map: s.mapNumber, Row: s.hero.cell.Row, Col: s.hero.cell.Col, Facing: int(s.hero.dir) & 7}
	if p != s.lastMove {
		s.lastMove = p
		s.onMove(p)
	}
}

// faceRemotes turns idle remote heroes to their reported facing.
func (s *startScene) faceRemotes() {
	for _, r := range s.remotes {
		if n := s.byEntity[r.entity]; n != nil && n.idle() && n.cell == (mapdata.Cell{Row: r.pos.Row, Col: r.pos.Col}) {
			n.dir = mapdata.Direction(r.pos.Facing & 7)
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
	c := map[int]int{s.mapNumber: 1}
	for _, r := range s.remotes {
		c[r.pos.Map]++
	}
	return c
}
