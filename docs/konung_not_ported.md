# Not ported

What the Go port still leaves out, what it only approximates, and what is unknown about the formats. Each entry says what the original does (as far as it is traced), where it is in `konung.exe`, why it is missing, and what porting it would take. The topic docs remain the source of truth; this page collects their "not ported" and "open questions" lists in one place. Update both when an item is ported.

Reasons used below: **not traced** (the exe code has not been read), **missing file** (the installed game lacks the resource), **presentation** (a deliberate port choice).

## Features

### Other misc items

Status: dispatcher traced, most cases **not traced**. Misc items (category ≥ 10, table `0x0046053C`) are used by `0x00414E64(e, target, item)` from the bag (right click), the in-hand box (hotspot 19) and a portrait (hotspots 30–39, cursor mode 9). Its jump table `0x00415ADC` has one case per type; the common tail `0x00415BD3` plays sound 10.

| Types | Case | Status |
|---|---|---|
| 24 Лопата | `0x00415504` | ported: [dig mode](konung_ground_items.md#hover-and-pick-up) |
| 31 Факел | `0x004156E6` | ported: [loot highlight](konung_potions.md#loot-highlight-0x00a3e758) |
| 20 Шар магического прыжка | `0x004153B0` | ported: opens the [map window](konung_world_map.md#opening-and-closing) |
| 0 Потерянная карта | `0x00414E95` | ported: marks a random location known ([world map](konung_world_map.md#known-maps-0x00a3e0a8)) |
| 14–19, 21 Магический шар … (волхв Бережков … Подземного города) | `0x00415329` … `0x004153D9` | not traced. Type 21 calls `0x004106E0(0x2B, 0x12, 0x18, 2)` with constants; hypothesis: a jump to a fixed place |
| 6 Слова Воскрешения | `0x00414FFB` | ported: revives every dead character on the map ([party](konung_party.md#revival-0x00414b3c-verified)) |
| 1–5, 7–10, 12, 13, 22, 23, 25–28, 30, 33, 35–39 | own cases | not traced (books, quest items, amulet parts 25–27 share `0x0041552D`, 35/37 share `0x004157D2`) |
| 11, 29, 32, 34 | `0x00415ACD` (default) | no use (32 «Неразменный пятак» counts as money, [trade](konung_trade.md)) |

Names from `resfmt.ExeTables.ItemName(resfmt.ItemTableMisc, t)`. Porting means tracing each case; the dialogue scripts already handle quest items given or taken by NPCs.

### Party leftovers

Party play is ported ([party](konung_party.md)): every member walks, follows, fights, is selected and revived. Not ported:

* the portrait clicks in window 7 (switch the trading character).

### Movement

**Not traced.** The game's walk code (speed, path rules, direction choice) is reimplemented: A* on the fine grid (`mapdata/finegrid.go`), 10 animation steps/s. Also missing: the per-entity walk throttle `+0x11` (ticks where `time & m == m`) and the random pauses of creature kinds `0x4D`/`0x4F` ([combat: creature AI](konung_combat.md#creature-ai-verified)).

### Combat leftovers

See [combat: port](konung_combat.md#port).

* God mode `0x00A3E768`, toggled by `0x00436E93` (key not identified): damage to the hero −2.
* Kind `0x49` steps back after its blow (`0x004101E4`, not traced).
* Equipping the best bag gear on re-equip (`0x004104D4`, not traced; `ReEquip` takes the quiver only).

### Films and voices (missing files)

* `GAMEOVER.FLM` (hero death, `0x004376A8`), `DEFEAT<map>.FLM` (village surrender), `WIN.FLM` (the index-0 script pile, below), all through `0x0042B690`. No `.FLM` file is installed, and the format is unknown. The port returns to the menu 2 s after the hero's death and shows nothing else.
* `SPEECHS.RES` voices for dialogue lines and the deferred reply action while a voice plays ([dialogues](konung_dialogue.md#open-questions), [sound](konung_sound.md#not-ported)).

### Ground items

The pile record with index 0 runs `0x0042B690(0x00450004)` = `WIN.FLM` when picked up ([ground items](konung_ground_items.md)). Hypothesis: the end-of-game pile. Not ported (missing film).

### HUD

* The alternate portrait table (`0x0062F3E0` set → `0x0062F3D8`, images `0x0083E090`, index `+0xF6`) and the portrait variant `+0xF8` written by hire (script 291). See [interface](konung_interface_res.md#open-questions).

### Village AI

* The post index from entity `+4` bits `0x38` (checked before the settlement posts; 0 for every `GAME.0` villager, so nothing visible is missing in `GAME.0`).
* Building record `+4` ([buildings](konung_buildings.md#not-ported--open)).

### Map drawing

* Animated static objects and random variants: the loader picks `rand() % header[+0x0C]` and cycles frames when header `+0x18` > 0 ([map format](konung_map_format.md)); the port always draws variant 0 (`mapdata/objects.go`).

### Screens

* The 640×480 layouts (`INTR640R.RES`, `MENU640R.RES`, `NEWH640R.RES`, the 640 hotspot map): every screen runs at 800×600 (**presentation**).
* Sounds on the hero-creation screen were not looked for.

## Approximations

Ported, but not bit-exact.

| What | Port | Original |
|---|---|---|
| Random numbers | Go's RNG | the C runtime's `rand()`; same distributions, different sequences |
| Spawn of non-creature groups without coordinates | nearest free fine cells around the group ([GAME.N](konung_game_format.md#placement-of-entities-without-coordinates)) | `0x00411B8C` formation offsets, not traced for villagers |
| Animation rate | 10 steps/s, per-entity phase offset | tick rate not traced |
| Creature chase | re-plans the whole path when the target moves | patches the last step |
| Arrow points | nearest cell centre | `0x00439BDC` |
| Search budget | separate 3-search budgets for the combat AI and the calm village AI | one budget `0x00A3E720` |
| HUD press | held for one animation step | one main-loop pass |
| New game time | 0 (day) | not traced |
| A walk cut by a hit reaction | snaps back to the cell being left | the cell semantics of a step are not traced |

## Format unknowns

* `KONUNG.N` section 1: flag bits other than walls (`0xFFF`) and arrow obstacles (`0xC0`); grid bits `0xC000` ([map format](konung_map_format.md#open-questions)).
* `LIGHTS.N` fine-grid values beyond zero / non-zero ([day and night](konung_daylight.md#open-questions)).
* `GRAPH.RES`: the 64 KiB gap at `0x1ED8B6`; the Type 0 palette/dye choice for NPCs and the model ID → character mapping ([GRAPH](konung_graph_sprites.md)).
* `GAME.N`: item fields other than category and type; the gear table's remaining fields; which jewellery box is which; group `+0x1E` bits other than `0x20`/`0x40`/`0x80` and kind `0x53`; map exit bytes `+0`/`+1`; settlement bytes outside the traced ones ([GAME.N](konung_game_format.md#open-questions)).
* Saves: later use of the hero copy `0x00A3E584` after a load ([saves](konung_save_format.md#open-questions)).
* `SPEECHS.RES` and `.FLM` files: not installed.

## Tooling

* `cmd/sprite-catalog` has its own Type 0 walker that duplicates `resfmt.DecodeType0Frames`.
* `map-render` clips objects anchored above y = 0.
