# World map window (window 5)

The map window «Карта (M)»: a parchment picture of the land with the locations the party knows drawn in, the current one tinted red. With a «Шар магического прыжка» in the bag, a click on a known location travels there.

Status: **verified** against the code (`0x00428698`, `0x00439A28`, `0x00422AD8`) and the exe tables. The picture was checked by rendering it to PNG.

Code: `resfmt.WorldMap` (`internal/resfmt/worldmap.go`), `gamestate.Game.KnowMap`, `MapKnown`, `Travel` (`internal/gamestate/worldmap.go`), `worldMapWindow`, `startScene.toggleMap`, `mapClick` (`internal/game/worldmap.go`).

## Opening and closing

* **M** (`0x00436EB9`) and HUD button 2 (hotspot 3, `0x00422C01`) work only with no window or window 5 open (`0x00A3E740` = 0 or 5). They return a held item (`0x00429194(1)`), play the click (sound 4) and press button 2 with image 14 (`0x004290AC(2, 14)`). With window 5 open they close it (`0x00A3E740` = 0, `0x004292E4`). Otherwise they open it with `0x0042DBBC(4)`.
* **Escape** closes it (`0x00437447`).
* **Misc item type 20** «Шар магического прыжка» (case `0x004153B0` of `0x00414E64`) hides the bag strip (`0x00A3E730` = 0), opens the window in place of any other (`0x0042DBBC(4)`) and plays sound 10. The orb is not used up here. Port: `Game.UseMisc` sets `Game.OpenMap`, and the scene opens the window.
* While it is open, Q and I do nothing. The quest log `0x0042459C` and the character window `0x00424610` need window 0 or their own window.

## Draw

Dispatcher case 5 (`0x0042F491`) sets `0x00A3E740` = 5 and `0x00A3E74C |= 2`, then tiles the view with INTR slot 5 (`0x0043141C`, which does nothing unless background tiling is on, [settings](konung_settings.md#background-tiling-t)). With tiling off the world keeps being drawn around the parchment (the world draw, then the open window, `0x0042AE68`). The frame redraw (`0x0042B1C9`) sees flag 2, clears it and calls `0x00428698(surface)`:

1. Clip to the view (`0x0043FCE2`). Blit INTR slot 4 (the empty parchment; 651×478 in `Intr800r.res`) at the view's top-left with its palette.
2. Build a red-tinted copy of slot 179's palette (`0x0043FD18(24, 0, 0)`, `0x0043F763`; `resfmt.TintPalette`).
3. For each area row y (0 … height − 1) whose row offset is ≥ 0, walk the row's (region, length) pairs. For a pair with length ≠ 0 whose region is known (`0x00A3E0A8[region]` ≠ 0), clip to that run on line area y + y. Then blit slot 179 (the drawn map, 502×361, transparent background) at the area's top-left. The current map's region (`0x00A3E7FC − 1`) uses the tinted palette. The row's end pair (length 0) covers the rest of the row up to the view's right edge, if the run position is still left of the area's right edge.

Region = map − 1. The port composes the known runs once into an area-sized image (`WorldMap.Compose`), rebuilt when the known flags or the current map change.

### Exe tables (DGROUP)

| VA | Layout |
|---|---|
| `0x0046280C` | per video mode (8 bytes): int16 area x, y (relative to the view's top-left), width, height. 640×480: (13, 13, 502, 361); 800×600: (80, 60, 502, 361) |
| `0x00456A7C` | int32 per area row: byte offset of the row's pairs in the run data; −1 = no row (one −1 follows the 361 rows) |
| `0x00450F1C` | run data, `0x1528` bytes: per row (region byte, length byte) pairs; length 0 ends the row. Rows are stored back to back, so the draw reads them sequentially and the hit test uses the offsets; both agree |
| `0x0045AE90` | 56 bytes: 1 = region never picked by misc item type 0 (regions 41–54) |
| `0x0045B3F4` | location names, 40-byte CP866 strings, entry map − 1 ([map format](konung_map_format.md#location-names-verified)) |

The run data has 30 regions: maps 1, 3, 4, 5, 7, 10, 11, 12, 14, 15, 16, 17, 19, 20, 22, 23, 24, 25, 27, 28, 29, 30, 32, 33, 35, 36, 37, 38, 39, 40. These include the three start maps. Sub-maps such as dungeons and map 2 have no region.

Validation: `resfmt.TestWorldMap` checks the area, the 30 regions, a hit at (250, 180) → region 13 «Верхний лагерь», and that `Compose` draws only the asked region's pixels.

## Known maps `0x00A3E0A8`

56 bytes, one per map (index map − 1), saved with the game ([saves](konung_save_format.md), `Game.KnownMaps`).

* The map loader sets the loaded map's flag (`0x0043C12A`, `[0x00A3E0A7 + map] = 1`). Port: `Game.KnowMap` in `buildScene`.
* Misc item type 0 «Потерянная карта» (case `0x00414E95`), used from the bag, the in-hand box or a portrait, picks a random region, `rand() % 56`. From there it takes the first region, wrapping around, that is neither known nor excluded by `0x0045AE90`, and marks it known. If every candidate is known, nothing changes. The scroll is used up: item byte +0 = `0xFF`, and the common tail `0x00415BD3` removes it from the user's bag (`0x00421D24`) and plays sound 10 (`0x00415C6C`). Port: `Game.RevealMap`, `Game.UseMisc`. The exclusion list is the constant `neverRevealed`, which `TestRevealMap` checks against the exe bytes.

## Hit test and hover

With window 5 open, the hotspot routine (`0x0043946E`) first tests the map area. Let (x, y) be the mouse position relative to the area. If 0 < x < width and 0 < y < height, it calls `0x00439A28(x, y, rows, runs)`. That routine returns 0 for a row of −1. Otherwise it returns the region of the first pair whose cumulative length exceeds x, or the end pair's region. A known region gives hotspot `region << 8 + 0x103` (so `id >> 8` is the map number and the low byte routes to the map button's handler). An unknown region gives −1. Outside the area, the normal hotspots apply (HUD buttons, portraits). Port: `WorldMap.RegionAt`, `worldMapWindow.regionAt`.

* **Status line** (`0x004240D0`): over a known location its name (`0x0045B3F4`). Over the map button it shows «Карта (M)» (`0x0045DF7C`).
* **Left click** in the view outside a known location does nothing (`0x00422EDF`). **Right click** only returns a held item: the map-click part of `0x004238EE` needs window 0 (`0x0042393D`).

## Travel by orb (`0x00422AD8`)

A left click on hotspot ≥ `0x100` in window 5, target m = `id >> 8`:

1. If the party is already on m (group 0 `+0x1B`), nothing happens.
2. `0x00434C90(0x15)` finds the first bag item of category 11 with type 20. If there is none, nothing happens.
3. The orb is used up (`0x00421D24`: removed from the bag, the rest shifts left, `+0xB0` = 0).
4. It takes the first map exit in record order (`0x007AA2DC`, count `0x00A3E81C`) whose target `+4` is m. `0x00A3E740` = 0, the bag strip is redrawn if shown, `0x00A3E810` = −m, and the group gets the exit's arrival row, column and facing, as in `0x00421CCD` ([map exits](konung_game_format.md#map-exits-verified)). If no exit leads to m, the orb is still gone and the window stays open.

The travel click plays no sound of its own. Port: `Game.Travel` returns the exit, and `startScene.mapClick` closes the window and calls `changeMap`.

## Not ported / open

* The 640×480 layout is decoded (`WorldMap(0)`), but the port runs at 800×600 only.
