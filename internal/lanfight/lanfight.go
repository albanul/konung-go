// Package lanfight builds a LAN hero's fighting entity: the hero type's
// GAME.N hero (gear, quiver) with the account's values on top. The server's
// world (internal/lanserver) builds every player's hero with it, and the
// client its own hero for the HUD, so both have the same values. Fights
// themselves run in the server's world (internal/world).
package lanfight

import (
	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
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
// byte offset in GRAPH.RES), and the health and poison it left with (full
// when unknown).
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
	// 0x0042105C enters with 1600; the port keeps the health and poison
	// the hero left with (lanstore), so leaving does not heal.
	e.HP, e.Poison = gamestate.HPFull, h.Poison
	if h.HP > 0 && h.HP < gamestate.HPFull {
		e.HP = h.HP
	}
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
