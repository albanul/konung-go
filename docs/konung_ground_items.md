# Items on the ground

Loose items (weapons, armour, herbs, arrows, gold, buried caches) lie on the maps as **piles**: `GAME.N` section 3. Nuts and roots grow back after being picked. Status: layout, drawing, pick-up and respawn verified against code; checked against `GAME.0` (`gamestate.TestPilesGame0`).

## Records (`GAME.N` section 3, file offset `0x21900`, 500 × `0x40` → `0x0079EFDC`)

| Offset | Type | Meaning |
|---|---|---|
| `+0x00` | `int32` | −(game time) of the last pick-up of a respawning herb, else 0 |
| `+0x04` | `int32` | same value; `+0 < 0 && +0 == +4` marks a running respawn timer |
| `+0x08`, `+0x0A` | `uint16` | fine-grid row, column |
| `+0x0C` | `uint16` | INTR image slot, computed at load (`0x00439DF0`); runtime only |
| `+0x10` | `int16` | **gold**, added to the hero's gold (`+0x34`) on pick-up. Negative = **buried**: not drawn, not hovered, taken only with «Лопата» in the hero's bag |
| `+0x12` | `uint16` | map (`KONUNG.N`); 0 ends the list (`0x0043BCA2`, count `0x00A3E804`) |
| `+0x14` | 22 × `uint16` | item indices, packed from the front |

`GAME.0` has 378 records over 38 maps. `KONUNG.1` has 13: «Элексир Мудрости» and arrows (both buried, 100 gold each), «Лопата», arrows, «Доспех кожаный», «Дубина», «Кольцо» with 30 gold, «Деревянный щит», four «Белый корень» and one «Земляной орех» (next to Volk's start).

Code: `gamestate.Pile`, `Game.Piles`.

## Map load (`0x0043CF1A`)

For each record on the current map (`0x00A3E7FC`): skip it while a respawn timer runs, i.e. `+0 < 0`, `+0 == +4` and `time + (+0) < 5400` (game time `0x00A3E770`, incremented once per main-loop tick next to the AI dispatch, `0x00437849`). Otherwise set the fine-grid flag `0x20` at `0x00505C71 + row·0x140 + col·2` and copy the record to the active list `0x007A6CDC` (count `0x00A3E7E0`). The copy keeps the record index at `+0x0E` and replaces `+0`/`+4` with the screen position: world position of the cell (`0x00439BA0`) minus half the image's width and height. `gamestate.Pile.Active`.

**Draw order.** The terrain pass draws the dead flat (`0x00425C23` draws only entities with `+4` bit `0x80`; the depth-sorted character pass `0x00426C07` skips them), then the visible piles (`0x0042647E`). A kill's loot pouch therefore lies on top of the body. Port: `startScene.drawCorpses` before `drawPiles`.

**Image (`0x00439DF0`, `Game.PileImage`).** INTR slot 0xA3 (a pouch) when the pile has a second item or non-zero gold. Otherwise the single item's ground image, which is the word after its inventory icon: gear table `+0x3C`, jewellery/herb/misc `+0x7E` (`resfmt.ItemGroundImage`). Arrows (category 12) use 0x99; categories 10 and above 12 use the pouch. Examples: «Белый корень» 0xA8 (a red-berried bush), «Земляной орех» 0xA0, «Деревянный щит» 0xA5.

## Drawing (`0x0042647E`)

The pile draw is part of the terrain pass, after the terrain and decorations and before shadows and objects. It walks the visible cells row by row; on cells with flag `0x20` it looks the pile up (`0x00410E10`) and skips it if the gold is negative. It draws the INTR image at the stored position, so the image is centred on the cell. With `0x00A3E758` set (the torch or Чистая слеза), a yellow glow from `LIGHTS.RES` is blended under it first ([loot highlight](konung_potions.md#loot-highlight-0x00a3e758), ported).

## Hover and pick-up

* **Hover.** A visible pile under the mouse shows «Что-то лежит на земле» (`0x0045C37C`) in the status line, before the hovered-character line (`0x00424009`). The cursor becomes sprite 7 when the cell has no occupant (`0x00428D25`).
* **Mouse buttons.** `WM_LBUTTONDOWN` sets `0x00A3E6DC` bit 0x01, `WM_RBUTTONDOWN` bit 0x02 (`0x0042BF3D`, `0x0042C009`). The click handler `0x00422864` has a left branch (`0x00422933`) and a right branch (`0x004238EE`).
* **Intent.** A **right** click on the map (hotspot 0, no window, no hovered character) gives the selected members a walk target and sets `+3` bit `0x40` (`0x004239E3`). A **left** click walks without picking up: it clears the bit (`0x00422A5A`).
* **Dropping.** A left click on the map while holding an item puts it on the ground at the holder's cell (`0x004229BC` → `0x004138C8(row, col, item)`). It goes into the pile there if one of its 22 slots is free; otherwise a new runtime pile is added to the active list (index `0xFFFF`, never written back to section 3, so no respawn), unless 200 piles are already active (`0x00413945`). The caller ignores failure, so the item is lost. A right click while holding returns the item (`0x00423919`).
* **Arrival** (`0x004222BC`): when the walk ends, the bit is cleared. If the member's cell has a pile that is visible, or the hero carries a shovel (`0x00434C90(0x19)`: misc item type 0x18 in the bag), `0x004110EC` picks it up.
* **The shovel.** Right-clicking «Лопата» in the bag (misc use `0x00414E64`, case 24 at `0x00415504`) closes the window, plays sound 10 and sets cursor mode `0x00A3E790` = 7 (dig cursor, sprite 7). The next left or right click resets the mode (`0x0042296C`, `0x004238FB`, `0x00423412`) and proceeds as usual, so digging is a right click on the spot. The arrival check above only requires the shovel to be **in the bag**: the code doesn't check the dig mode. A right click on a buried pile's cell digs it up even without activating the shovel. Otherwise a map exit on that cell is taken (`0x00421BB8`, see [GAME.N](konung_game_format.md#file-layout-verified)).
* **Pick-up** (`0x004110EC`), skipped while the member's character window is open:
  * The pile's gold goes to the hero, and the pile's gold becomes 0.
  * Then each item goes to the front of the picker's bag (`0x00424918` logic), until the bag is full («Мешок у тебя не резиновый!») or the weight would exceed the maximum («Превышен вес»). In those cases the game opens the pile window (`0x004170A0`, [trade](konung_trade.md#entry-points), ported), where bag items and the pile's items can be swapped.
  * A record with index 0 is special: before anything is taken, the game ends with `WIN.FLM` (`0x0042B690(0x00450004)`, next state the menu; [films](konung_films.md#triggers)). Record 0 is on map 46. Port: `startScene.arrive`.
* **Removal and respawn** (`0x0041093C`): the cell flag is cleared and the copy dropped from the active list.
  * If the record held exactly one item of category 9, type 1 «Земляной орех» or type 2 «Белый корень», the game allocates a free item record (`0x004399AC`: first index ≥ 1 with a negative category), copies the item into it, stores it as the record's item and sets `+0 = +4 = −time`. The herb is back on the next map load at least 5400 ticks later.
  * Any other record gets item 0.

**Port** (`internal/game/piles.go`, `startScene.click`): the scene activates the map's piles at load, draws them, and shows the hover line and cursor. A right click walks and picks up where the walk ends; a left click with a held item drops it (`gamestate.Game.DropItem`). The shovel's dig mode is `gamestate.CursorDig`. The game re-checks respawn timers only on map load. The port also re-runs the activation rule every game tick on the current map (port addition); piles persist in `gamestate.Game` across [map changes](konung_game_format.md#map-exits-verified). Kills drop their loot into runtime piles the same way (`Game.DropLoot`, [loot](konung_combat.md#loot-verified)). One game tick = one AI tick (`walkTicksPerStep` updates), so 5400 ticks ≈ 6 minutes at 60 updates/s.

## Open questions

* None. The index-0 pile is the end of the game (map 46, «Браслет»; a pick-up there plays `WIN.FLM`, [films](konung_films.md)). Only herb types 1 «Земляной орех» and 2 «Белый корень» respawn, and the data has no other berries: the single-herb piles of `GAME.0`–`GAME.2` are 42 nuts, 53 white roots and the potions «Элексир Мудрости» (8–12) and «Живая Вода» (8–10), which do not respawn.
