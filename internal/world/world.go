package world

import (
	"math/rand/v2"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

// World is one map's simulation: the party and every other character on
// the map as walkers, the occupancy, the active ground items and the arrows
// in flight, over the game state (gamestate.Game, all maps' records). Step
// advances it one game tick (60 ms, 0x00A3E770).
type World struct {
	Game      *gamestate.Game
	Tables    *resfmt.ExeTables
	MapNumber int
	Tick      int // game ticks (0x00A3E770, one per step, 60 ms each)
	Rng       *rand.Rand

	// WorldW, WorldH are the map's size in world pixels (Map.WorldSize).
	WorldW, WorldH int

	Walk *mapdata.WalkGrid
	Occ  map[mapdata.Cell]bool // cells held or being entered by a character

	// Party is group 0's entities (copies; [0] = hero) shown in the
	// portrait grid; Members are their walkers, one per party entry; Sel is
	// the selection list 0x0083B93C (entity indices, at most 10) and Cur
	// the character window's character 0x00A3E688 (party index). Hero is
	// the hero's walker (Members[0]); nil when the party is not on this map.
	Party   []gamestate.Entity
	Members []*Member
	Sel     []int
	Cur     int
	Hero    *Walker

	NPCs     []*NPC       // every character outside the party
	ByEntity map[int]*NPC // non-party characters on this map by entity index
	AINext   int          // round-robin start of the village AI's path budget
	// AIBudget is the combat AI's path searches left this game tick;
	// FightNext the round-robin start of its budget.
	AIBudget  int
	FightNext int

	PileAt map[mapdata.Cell]int // active piles by cell (index into Game.Piles)
	Arrows []gamestate.Arrow
	Jump   *gamestate.MapExit // a map change asked by a misc item (World.Jump)

	// Film is a film the world asks the game to play (0x0042B690): the
	// village surrender's DEFEAT<map>.FLM, then back to the game, or
	// WIN.FLM, then the menu (FilmToMenu).
	Film       string
	FilmToMenu bool

	Hooks Hooks

	// LAN marks a map of the LAN server's world (lan.go): the party list
	// holds the heroes of the player slots on this map (groups 0–15), each
	// its own leader (no following, no shared selection), and a hero that
	// walks onto an exit leaves alone (Hooks.MemberExit) instead of the
	// whole party changing maps. Single-player never sets it.
	LAN bool
}
