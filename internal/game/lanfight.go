package game

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

// Fights between players, the client's side (netplay/fight.go): a blow or
// an arrow of the own hero that reaches a remote hero is reported
// (onAttack) instead of resolved here; the server's verdict (MsgHit) plays
// for everyone: the attacker's swing or shot, the target's HP, flinch or
// death. The own hero's HP changes only by the server's verdicts, so its
// death (the game over) is the server's.

// strikeRemote reports the own hero's blow at remote hero target (the end
// of the swing, swing) and reports whether target is a remote hero.
func (s *startScene) strikeRemote(attacker *gamestate.Entity, target int, ranged bool) bool {
	r := s.remoteAt(target)
	if r == nil {
		return false
	}
	if len(s.Party) > 0 && attacker.Index == s.Party[0].Index && s.onAttack != nil {
		s.onAttack(r.info.ID, ranged)
	}
	return true
}

// canTarget reports whether the own hero may attack r's hero: always,
// unless the server protects teams and r is of the own team (hero type).
func (s *startScene) canTarget(r *remotePlayer) bool {
	return !s.teamSafe || len(s.Party) == 0 || r.info.Model != s.Party[0].Model
}

// hitWalker is the walker and entity of player id's hero on this map: the
// own hero when id is self.
func (s *startScene) hitWalker(id, self string) (*walker, *gamestate.Entity) {
	if id == self {
		if len(s.Party) == 0 {
			return nil, nil
		}
		return s.Combatant(s.Party[0].Index)
	}
	if r := s.remotes[id]; r != nil && r.Entity >= 0 {
		return s.Combatant(r.Entity)
	}
	return nil, nil
}

// remoteHit plays the server's verdict on an attack (MsgHit).
func (s *startScene) remoteHit(h netplay.HitPayload, self string) {
	tw, te := s.hitWalker(h.Target, self)
	// The attacker's swing or shot as seen here (its own client plays it
	// already).
	if h.Attacker != self {
		// Not while it walks on to a further cell or plays a flinch or
		// death; a swing in progress starts again (the next blow).
		if aw, _ := s.hitWalker(h.Attacker, self); aw != nil && len(aw.Path) == 0 &&
			aw.Act != gamestate.ActFlinch && aw.Act != gamestate.ActDeath {
			aw.Halt()
			if tw != nil {
				aw.Dir = world.Facing(aw.Cell, tw.Cell)
			}
			if h.Ranged {
				aw.StartAct(gamestate.ActShoot)
			} else {
				aw.StartAct(gamestate.ActAttack)
			}
		}
	}
	if r := s.remotes[h.Target]; r != nil {
		r.hp, r.dead = h.HP, h.Killed
	}
	if tw == nil || te == nil {
		return // off this map
	}
	switch {
	case h.Killed:
		te.HP = 0
		tw.Halt()
		tw.StartAct(gamestate.ActDeath)
	case h.Result == netplay.HitHeavy:
		te.HP = h.HP
		s.Flinch(tw)
	case h.Result == netplay.HitLight:
		te.HP = h.HP
	case h.Result == netplay.HitDodged && h.Attacker == self:
		s.playAt(gamestate.SoundDodge, tw.Cell)
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
