package netplay

import "time"

// Fights between players (PvP). In the original the server runs the whole
// world and its own combat (0x445988: melee 0x445D18, arrows 0x444CD0,
// hit 0x43E33C, damage 0x43E968); clients send orders and draw what the
// server reports (0x19: HP and the action). Here each client runs its own
// world, so a client reports a blow or an arrow that reached another hero
// (MsgAttack) and the server decides (Combat): it checks both heroes are in
// the world, alive, on one map, the attacker in the combat stance, close
// enough and not striking faster than a swing, and runs the game's own
// strike rules on the heroes as it builds them. Everyone in the world gets
// the verdict (MsgHit). A kill scores (OnKill); the dead hero stays dead
// until it enters again, as in the original (no respawn: 0x436A46).

// Equipment slots of a Look (gamestate: axe, bow, blade, armour, helmet,
// shield).
const EquipSlots = 6

// EquipLook is an equipped item as far as drawing needs it; without Item
// the slot is empty (the zero value: a look without equipment).
type EquipLook struct {
	Item     bool `json:"i,omitempty"`
	Category int  `json:"c,omitempty"`
	Type     int  `json:"t,omitempty"`
}

// Look is how a hero looks (the original's entity word: stance bit 15,
// weapon mode bits 12–13; 0x5A: the equipment).
type Look struct {
	Stance bool                  `json:"stance"`
	Weapon int                   `json:"weapon"` // 0 axe, 1 bow, 2 blade
	Equip  [EquipSlots]EquipLook `json:"equip"`
}

// LookPayload is a Look; ID names the hero from the server.
type LookPayload struct {
	ID string `json:"id,omitempty"`
	Look
}

// AttackPayload is a blow (melee) or an arrow (Ranged) of the own hero
// that reached Target's hero.
type AttackPayload struct {
	Target string `json:"target"`
	Ranged bool   `json:"ranged,omitempty"`
}

// Strike results (gamestate.Strike*).
const (
	HitMiss   = -2
	HitDodged = -1
	HitKill   = 1
	HitLight  = 2
	HitHeavy  = 3
)

// HitPayload is the server's verdict on an attack: Result (Hit*), the
// damage, the target's HP after it, and whether it died.
type HitPayload struct {
	Attacker string `json:"attacker"`
	Target   string `json:"target"`
	Ranged   bool   `json:"ranged,omitempty"`
	Result   int    `json:"result"`
	Damage   int    `json:"damage"`
	HP       int    `json:"hp"`
	Killed   bool   `json:"killed,omitempty"`
}

// StrikeRequest is an attack as the server checked it: the heroes'
// facings (the dodge depends on them) and the distance in cells.
type StrikeRequest struct {
	Attacker, Target       string
	Ranged                 bool
	AttackerFacing, Facing int
	Distance               int
}

// Strike is a Combat's verdict. Refused: the attack is not possible (out
// of range, no weapon); nothing happens. Killer is the attacker's hero
// after a kill (the experience), nil otherwise.
type Strike struct {
	Refused bool
	Result  int
	Damage  int
	HP      int
	Killer  *Hero
}

// Combat resolves fights on the server (lanfight.World). Enter builds the
// hero's fighting entity (hero nil: the type's default hero) and returns
// its look and HP; Stance applies a stance and weapon change and returns
// the look the server keeps (a weapon slot without an item is not taken).
type Combat interface {
	Enter(id string, model int, hero *Hero) (Look, int)
	Leave(id string)
	Stance(id string, stance bool, weapon int) Look
	Strike(r StrikeRequest) Strike
}

// strikeEvery is the shortest time between two strikes of one hero at
// normal speed: three game ticks, below any swing.
const strikeEvery = 3 * TickDuration

// meleeReach is how far (cells, Chebyshev) a blow reaches: the next cell,
// with one more for a step in flight between the reports.
const meleeReach = 2

// look applies a client's Look (MsgLook): with a Combat the stance and
// weapon go through it and the equipment is the server's; without one the
// client's look is relayed as it is.
func (s *Server) look(p *serverPlayer, l Look) {
	if s.cfg.Combat != nil {
		l = s.cfg.Combat.Stance(p.info.ID, l.Stance, l.Weapon)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !p.entered {
		return
	}
	p.look = l
	m := mustMessage(MsgLook, LookPayload{ID: p.info.ID, Look: l})
	for _, o := range s.players {
		if o.entered {
			o.conn.Send(m) // the sender too: its look as the server keeps it
		}
	}
}

// attack checks a client's attack (MsgAttack) and lets the Combat decide.
func (s *Server) attack(p *serverPlayer, a AttackPayload) {
	if s.cfg.Combat == nil {
		return
	}
	s.mu.Lock()
	t := s.players[a.Target]
	now := time.Now()
	ok := t != nil && t != p && p.entered && t.entered && !p.dead && !t.dead &&
		p.pos.Map == t.pos.Map && p.look.Stance &&
		now.Sub(p.lastStrike) >= strikeEvery/time.Duration(1+s.cfg.Speed)
	var req StrikeRequest
	if ok {
		d := chebyshev(p.pos, t.pos)
		ok = a.Ranged || d <= meleeReach
		req = StrikeRequest{Attacker: p.info.ID, Target: t.info.ID, Ranged: a.Ranged,
			AttackerFacing: p.pos.Facing, Facing: t.pos.Facing, Distance: d}
		p.lastStrike = now
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	r := s.cfg.Combat.Strike(req)
	if r.Refused {
		return
	}
	hit := HitPayload{Attacker: p.info.ID, Target: t.info.ID, Ranged: a.Ranged, Result: r.Result, Damage: r.Damage, HP: r.HP, Killed: r.Result == HitKill}
	s.mu.Lock()
	if hit.Killed {
		t.dead = true
	}
	t.hp = r.HP
	m := mustMessage(MsgHit, hit)
	for _, o := range s.players {
		if o.entered {
			o.conn.Send(m)
		}
	}
	if r.Killer != nil {
		p.hero = r.Killer
		p.conn.Send(mustMessage(MsgHero, r.Killer))
	}
	killer, victim, killerHero, victimHero := p.info, t.info, p.hero, t.hero
	s.mu.Unlock()
	if hit.Killed && s.cfg.OnKill != nil {
		kl, vl := 1, 1
		if killerHero != nil {
			kl = killerHero.Level
		}
		if victimHero != nil {
			vl = victimHero.Level
		}
		s.cfg.OnKill(killer, victim, killerHero, kl, vl)
	}
}

func chebyshev(a, b Position) int {
	abs := func(x int) int {
		if x < 0 {
			return -x
		}
		return x
	}
	return max(abs(a.Row-b.Row), abs(a.Col-b.Col))
}
