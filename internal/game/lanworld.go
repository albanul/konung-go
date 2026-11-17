package game

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
)

// lanWorld builds the LAN world for a hero the server let in (the client's
// 0x8C handler 0x0042105C and state 10, 0x0042F620): the map comes from
// MAPS.RES, the hero from the account (nil on a server without accounts:
// the model's single-player hero as it starts). The single-player GAME.<model>
// serves as the template for everything the scene needs from a GAME.N (items,
// sprites, the HUD's party); the LAN world's own characters (GAME.RES
// entities) and the other players are not placed yet.
func (a *sceneAssets) lanWorld(gameDir string, exe *gamestate.LANExe, at netplay.EnterPayload, hero *netplay.Hero, model int) (*startScene, error) {
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
	if model < 0 || model >= netplay.Models {
		return nil, fmt.Errorf("hero model %d", model)
	}
	la := *a // the same resources, maps from MAPS.RES
	la.lanMaps = maps
	// The LAN map window (M): INTO640R.RES's map picture and LAN_KONG.EXE's
	// tables; without them the window shows the bare parchment.
	if exe != nil {
		if into, err := readGameFile(gameDir, "INTO640R.RES"); err != nil {
			log.Printf("LAN map window: %v", err)
		} else if la.lanMap, err = newLANWorldMap(exe, into, a.graph); err != nil {
			log.Printf("LAN map window: %v", err)
		}
	}
	la.gamePath = filepath.Join(a.mapDir, fmt.Sprintf("GAME.%d", model))
	tmpl, err := os.ReadFile(la.gamePath)
	if err != nil {
		return nil, err
	}
	game, err := gamestate.Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", la.gamePath, err)
	}
	// As the single-player start (0x00438F3F): the chosen hero's model,
	// name, palette and gear (GAME.N holds a placeholder model), then the
	// account's values on top.
	tmplHero := gamestate.NewHeroTemplate()
	gamestate.SelectHero(&tmplHero, model)
	game.StartHero(tmplHero)
	if hero != nil {
		applyLANHero(&game.Entities[game.Groups[0].First], hero)
	}
	// GAME.RES's map exits: the teleports between the LAN maps. (The
	// original client skips them and the server moves players between maps;
	// here the client changes maps itself until the world is synchronised.)
	world, err := readGameFile(gameDir, "GAME.RES")
	if err != nil {
		return nil, err
	}
	exits, err := gamestate.LANExits(world)
	if err != nil {
		return nil, err
	}
	game.EnterLANWorld(at.Map, at.Row, at.Col, at.Facing, exits)
	return la.buildScene(game, at.Map, nil, nil)
}

// applyLANHero puts an account's hero (LANUSERS.DAT record) on the scene's
// hero entity: level, gold, experience, the skills and stats, the model and
// its palette (index × 512 = the palette's byte offset in GRAPH.RES).
func applyLANHero(e *gamestate.Entity, h *netplay.Hero) {
	e.Level, e.Gold, e.Experience = h.Level, h.Gold, h.Experience
	e.Fencing, e.Marksmanship = h.Fencing, h.Archery
	e.Skills = [5]int{h.Herbalism, h.Smithing, h.Trade, h.Carpentry, h.Identification}
	e.BaseStats = [4]int{h.Charisma, h.Strength, h.Dexterity, h.Endurance}
	e.Stats = e.BaseStats
	e.Model = h.Model
	e.PaletteOffset = uint32(h.Palette) * 512
	e.HP = gamestate.HPFull // 0x0042105C: 1600 on entry
}
