package lanserver

import (
	"testing"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/netplay"
	"konung1.5/internal/world"
)

// useBag puts a new misc item of type typ into slot's hero's bag at the
// front and uses it (a right click on the bag strip).
func useBag(t *testing.T, w *World, slot, typ int) int {
	t.Helper()
	item := w.Game.SetItemKind(0, gamestate.CatQuest, typ)
	e := hero(w, slot)
	e.Bag = [32]int{item}
	w.Item(slot, netplay.OrderPayload{Kind: netplay.OrderItem, Item: netplay.ItemBag, Index: 0, On: true})
	return item
}

// The network game's misc items that reach beyond their user
// (LAN_KONG.EXE 0x004390F4: 3 the seer's scroll, 7 the doll, 9 the pipe)
// work on the server, seen from the player's map.
func TestLANMiscItems(t *testing.T) {
	w := load(t)
	const ivar, olaf, volk = 0, 1, 2
	for slot, model := range map[int]int{ivar: 1, olaf: 1, volk: 1} {
		if _, _, err := w.AddPlayer(slot, model, nil); err != nil {
			t.Fatal(err)
		}
	}
	m := w.PlayerMap(ivar)
	w.MoveTo(volk, 12, mapdata.Cell{Row: 60, Col: 30}) // another map

	// The seer's scroll: the user's unidentified items are identified, each
	// training its identification; the scroll is used up.
	sword := w.Game.SetItemKind(0, gamestate.CatBlade, 18)
	r := w.Game.ItemRecord(sword)
	r[0xE] |= 1
	w.Game.SetItemRecord(sword, r)
	h := hero(w, ivar)
	h.Bag = [32]int{0, sword}
	before := h.Skills[4]
	h.Bag[0] = w.Game.SetItemKind(0, gamestate.CatQuest, gamestate.MiscSeerScroll)
	w.Item(ivar, netplay.OrderPayload{Kind: netplay.OrderItem, Item: netplay.ItemBag, Index: 0, On: true})
	if it, _ := w.Game.Item(sword); it.Flagged || hero(w, ivar).Skills[4] != before+1 {
		t.Errorf("scroll: sword unidentified %v, identification %d → %d", it.Flagged, before, hero(w, ivar).Skills[4])
	}
	if c, _ := w.Game.ItemKind(hero(w, ivar).Bag[0]); c == gamestate.CatQuest {
		t.Error("the scroll was not used up")
	}

	// The doll: every living character of the other groups on the map —
	// Olaf and the villagers — is cursed; Ivar is not, nor Volk on another
	// map, nor a villager with «Заячий хвост».
	s := w.Maps[m]
	var vill, tailed int = -1, -1
	for _, n := range s.NPCs {
		if n.Mode == world.NPCVillager && !w.Game.Entities[n.Entity].Dead() {
			if vill < 0 {
				vill = n.Entity
			} else if tailed < 0 {
				tailed = n.Entity
			}
		}
	}
	if tailed < 0 {
		t.Fatalf("map %d has fewer than two villagers", m)
	}
	w.Game.Entities[tailed].Bag[0] = w.Game.SetItemKind(0, gamestate.CatQuest, 34) // «Заячий хвост»
	useBag(t, w, ivar, gamestate.MiscDoll)
	const curse = 19
	if e := hero(w, olaf); e.Enchants[0] != curse {
		t.Errorf("Olaf not cursed: %d", e.Enchants[0])
	}
	if w.Game.Entities[vill].Enchants[0] != curse {
		t.Errorf("villager %d not cursed", vill)
	}
	if w.Game.Entities[tailed].Enchants[0] == curse {
		t.Error("«Заячий хвост» did not protect")
	}
	if hero(w, ivar).Enchants[0] == curse || hero(w, volk).Enchants[0] == curse {
		t.Error("the user or a player on another map was cursed")
	}
	if c, _ := w.Game.ItemKind(hero(w, ivar).Bag[0]); c == gamestate.CatQuest {
		t.Error("the doll was not used up")
	}
}

// The pipe: the animals of the enemy group nearest to the user run off;
// the pipe is kept.
func TestLANPipe(t *testing.T) {
	w := load(t)
	const ivar = 0
	if _, _, err := w.AddPlayer(ivar, 1, nil); err != nil {
		t.Fatal(err)
	}
	scared := func(e gamestate.Entity) bool {
		k := e.Flags4
		return k&0x40 != 0 && !e.Dead() && k != 0x44 && k != 0x53 && k != 0x54 && k != 0x57
	}
	// A spot next to an animal whose group is the nearest enemy there.
	for m := 1; m <= Maps; m++ {
		s := w.Maps[m]
		for _, n := range s.NPCs {
			if !scared(w.Game.Entities[n.Entity]) || w.Game.Groups[n.Group].Kind&0x80 != 0 {
				continue
			}
			w.MoveTo(ivar, m, mapdata.Cell{Row: n.Cell.Row + 3, Col: n.Cell.Col})
			mem, _ := s.Member(ivar)
			near := s.NearestEnemy(mem.Cell, ivar)
			if near < 0 || w.Game.Entities[near].Group != n.Group {
				continue
			}
			pipe := useBag(t, w, ivar, gamestate.MiscPipe)
			ran := 0
			for _, o := range s.NPCs {
				if o.Group == n.Group && scared(w.Game.Entities[o.Entity]) && o.Intent == world.IntentWalk && (o.Walking || len(o.Path) > 0) {
					ran++
				}
			}
			if ran == 0 {
				t.Errorf("no animal of group %d on map %d ran", n.Group, m)
			}
			if hero(w, ivar).Bag[0] != pipe {
				t.Error("the pipe was used up")
			}
			return
		}
	}
	t.Skip("no animal group in the LAN world")
}
