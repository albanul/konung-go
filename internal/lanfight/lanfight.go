// Package lanfight builds the LAN heroes' fighting entities and resolves
// fights between players on the dedicated server (netplay.Combat), with the
// game's own strike rules (gamestate.Strike, ArrowDamage, Kill). The client
// builds its own hero with the same Hero function, so both sides fight with
// the same values.
//
// In the original the LAN server runs the whole world, combat included
// (LAN_KONG.EXE 0x445988, docs/konung_lan.md#fights). Here only fights
// between heroes are decided on the server; each hero is the hero type's
// GAME.N hero (its items: weapons, armour, quiver) with the account's values.
package lanfight

import (
	"fmt"
	"math/rand/v2"
	"sync"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
)

// Hero is a LAN hero's entity in tmpl, the hero type's GAME.N: the file's
// hero (gear, quiver, bag, HP) started like the single-player game start
// (gamestate.StartHeroEntity with the type's selection) and, with an
// account, its values on top (ApplyHero). hero nil: the type's default.
func Hero(tmpl *gamestate.Game, model int, hero *netplay.Hero) gamestate.Entity {
	e := tmpl.Entities[tmpl.Groups[0].First]
	t := gamestate.NewHeroTemplate()
	gamestate.SelectHero(&t, model)
	gamestate.StartHeroEntity(&e, t)
	if hero != nil {
		ApplyHero(&e, hero)
	}
	return e
}

// ApplyHero puts an account's hero (LANUSERS.DAT record) on e: level, gold,
// free points, the class (entity +4, as SelectClass; record +0x6B), the
// skills and stats, the model and its palette (index × 512 = the palette's
// byte offset in GRAPH.RES), and full HP.
func ApplyHero(e *gamestate.Entity, h *netplay.Hero) {
	e.Level, e.Gold, e.FreeExp = h.Level, h.Gold, h.FreePoints
	if h.Class >= gamestate.ClassMerchant && h.Class <= gamestate.ClassWarrior {
		e.Flags4 = byte(h.Class)
	}
	e.Fencing, e.Marksmanship = h.Fencing, h.Archery
	e.Skills = [5]int{h.Herbalism, h.Smithing, h.Trade, h.Carpentry, h.Identification}
	e.BaseStats = [4]int{h.Charisma, h.Strength, h.Dexterity, h.Endurance}
	e.Stats = e.BaseStats
	e.Model = h.Model
	e.PaletteOffset = uint32(h.Palette) * 512
	e.HP = gamestate.HPFull // 0x0042105C: 1600 on entry
}

// LookOf is how e looks to other players: stance, weapon mode and the
// equipped items' kinds in g.
func LookOf(g *gamestate.Game, e gamestate.Entity) netplay.Look {
	l := netplay.Look{Stance: e.InStance(), Weapon: e.WeaponMode}
	for k, i := range e.Equipment {
		if c, t := g.ItemKind(i); c >= 0 {
			l.Equip[k] = netplay.EquipLook{Item: true, Category: c, Type: t}
		}
	}
	return l
}

// World is the server's side of the fights: every hero in the world as an
// entity in one Game (the templates' items copied in), so the strike rules
// see both heroes' items. It implements netplay.Combat.
type World struct {
	mu     sync.Mutex
	g      *gamestate.Game
	t      *resfmt.ExeTables
	tmpl   [netplay.Models]*gamestate.Game
	heroes map[string]int // player ID → entity index in g
	accts  map[string]*netplay.Hero
	rng    *rand.Rand
}

// New builds the World from konung.exe (the strike tables) and the three
// hero types' GAME.0, GAME.1, GAME.2.
func New(exe []byte, games [netplay.Models][]byte) (*World, error) {
	t, err := resfmt.NewExeTables(exe)
	if err != nil {
		return nil, err
	}
	w := &World{t: t, heroes: map[string]int{}, accts: map[string]*netplay.Hero{}, rng: rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))}
	for m, data := range games {
		if w.tmpl[m], err = gamestate.Parse(data); err != nil {
			return nil, fmt.Errorf("GAME.%d: %w", m, err)
		}
	}
	// The shared Game: GAME.0's records give the item table its size; the
	// heroes are appended.
	if w.g, err = gamestate.Parse(games[0]); err != nil {
		return nil, err
	}
	return w, nil
}

// Enter implements netplay.Combat: the hero's entity with its own copies of
// the template's items, at full HP, out of the stance.
func (w *World) Enter(id string, model int, hero *netplay.Hero) (netplay.Look, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.leaveLocked(id)
	model = min(max(model, 0), netplay.Models-1)
	src := w.tmpl[model]
	e := Hero(src, model, hero)
	for k, i := range e.Equipment {
		e.Equipment[k] = w.g.CopyItemFrom(src, i)
	}
	e.Quiver = w.g.CopyItemFrom(src, e.Quiver)
	e.Bag = [32]int{} // not used in fights
	gamestate.LeaveStance(&e)
	e.Flags4 &^= 0x80
	e.Group = len(w.g.Groups) - 1 // no group rules
	e.Index = len(w.g.Entities)
	w.g.Entities = append(w.g.Entities, e)
	w.heroes[id] = e.Index
	if hero != nil {
		h := *hero
		w.accts[id] = &h
	}
	return LookOf(w.g, e), e.HP
}

// Leave implements netplay.Combat: the hero's items are freed.
func (w *World) Leave(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.leaveLocked(id)
}

func (w *World) leaveLocked(id string) {
	i, ok := w.heroes[id]
	if !ok {
		return
	}
	e := &w.g.Entities[i]
	for _, it := range append(e.Equipment[:], e.Quiver) {
		w.g.SetItemKind(it, -1, 0)
	}
	e.Equipment, e.Quiver = [6]int{}, 0
	e.Flags4 |= 0x80
	delete(w.heroes, id)
	delete(w.accts, id)
}

// Stance implements netplay.Combat: into or out of the stance
// (gamestate.EnterStance, LeaveStance) and the weapon (SelectWeapon: only
// a slot that holds an item).
func (w *World) Stance(id string, stance bool, weapon int) netplay.Look {
	w.mu.Lock()
	defer w.mu.Unlock()
	i, ok := w.heroes[id]
	if !ok {
		return netplay.Look{}
	}
	e := &w.g.Entities[i]
	if !e.Dead() {
		if stance {
			gamestate.EnterStance(e)
		} else {
			gamestate.LeaveStance(e)
		}
		gamestate.SelectWeapon(e, weapon)
	}
	return LookOf(w.g, *e)
}

// Strike implements netplay.Combat: a blow (gamestate.Strike) or an arrow
// (Shoot, ArrowAttack, ArrowDamage: the bow's range, one arrow and bow
// wear) of the attacker at the target. A kill gives the attacker the kill
// experience (Kill, KillByArrow) and the target dies.
func (w *World) Strike(r netplay.StrikeRequest) netplay.Strike {
	w.mu.Lock()
	defer w.mu.Unlock()
	ai, ok1 := w.heroes[r.Attacker]
	ti, ok2 := w.heroes[r.Target]
	if !ok1 || !ok2 {
		return netplay.Strike{Refused: true}
	}
	a, t := &w.g.Entities[ai], &w.g.Entities[ti]
	if a.Dead() || t.Dead() || t.HP <= 0 || !a.InStance() {
		return netplay.Strike{Refused: true}
	}
	a.Variant, t.Variant = r.AttackerFacing&7, r.Facing&7
	var result, damage int
	if r.Ranged {
		bow, ok := w.g.Item(a.Equipment[gamestate.WeaponBow])
		if a.WeaponMode != gamestate.WeaponBow || !ok || r.Distance > w.t.GearRange(bow.Type) {
			return netplay.Strike{Refused: true}
		}
		from := mapdata.Cell{}
		to := mapdata.Cell{Row: r.Distance}
		arrow, ok := w.g.Shoot(a, from, 4, to, w.t, 0)
		if !ok {
			return netplay.Strike{Refused: true}
		}
		result, damage = w.g.ArrowDamage(t, arrow, w.g.ArrowAttack(arrow, *a, *t, w.t), w.t, w.rng.IntN)
	} else {
		if a.WeaponMode == gamestate.WeaponBow {
			return netplay.Strike{Refused: true} // the bow does not strike
		}
		result, damage = w.g.Strike(a, t, w.t, w.rng.IntN)
	}
	out := netplay.Strike{Result: hitResult(result), Damage: damage, HP: t.HP}
	if result == gamestate.StrikeKill {
		if r.Ranged {
			w.g.KillByArrow(a, t, w.t)
		} else {
			w.g.Kill(a, t, w.t)
		}
		t.Flags4 |= 0x80
		out.HP = 0
		if h := w.accts[r.Attacker]; h != nil {
			h.FreePoints, h.Fencing, h.Archery = a.FreeExp, a.Fencing, a.Marksmanship
			k := *h
			out.Killer = &k
		}
	}
	return out
}

func hitResult(r int) int {
	switch r {
	case gamestate.StrikeMiss:
		return netplay.HitMiss
	case gamestate.StrikeDodged:
		return netplay.HitDodged
	case gamestate.StrikeKill:
		return netplay.HitKill
	case gamestate.StrikeHeavy:
		return netplay.HitHeavy
	}
	return netplay.HitLight
}
