# Not ported

What the Go port still leaves out, what it only approximates, and what is unknown about the formats. Each entry says what the original does (as far as it is traced), where it is in `konung.exe`, why it is missing, and what porting it would take. The topic docs remain the source of truth; this page collects their "not ported" and "open questions" lists in one place. Update both when an item is ported.

Reasons used below: **not traced** (the exe code has not been read), **missing file** (the installed game lacks the resource), **single hero** (the port's party is the hero alone, so the feature has nothing to act on), **presentation** (a deliberate port choice).

## Features

### Map window and travel by item

Status: entry points traced, the window itself **not traced**.

* **Opening.** HUD button 2 «Карта (M)» (hotspot 3, handler `0x00422C01`) and the M key (`0x00436EB9`) work when no window or window 5 is open (`0x00A3E740` = 0 or 5). They return a held item (`0x00429194(1)`), play the click (sound 4, `0x0042A664`) and press button 2 with image 14 (`0x004290AC(2, 14)`). With window 5 open they close it (`0x00A3E740` = 0, `0x004292E4`); otherwise they call the window draw dispatcher `0x0042DBBC(4)`.
* **Draw.** Dispatcher case 5 (`0x0042F491`; jump table `0x0042F4C8`, index = argument + 1): `0x00A3E740` = 5, `0x00A3E74C |= 2`, then `0x0043141C` with the view rectangle (`0x00A3E874`, `0x00A3E890`, `0x00A3E880`, `0x00A3E89C`). `0x0043141C` (the map picture) is not traced.
* **Misc item type 20** «Шар магического прыжка» (case 20 of `0x00414E64`, at `0x004153B0`): `0x00A3E730` = 0, opens the same window (`0x0042DBBC(4)`), sound 10.
* **Travel by item** (`0x00422AD8`): in window 5 a click with hotspot id ≥ `0x100` (`id >> 8` selects the target) uses up the item (`0x00421D24`, sound 10 at `0x00415C6C`) and takes the first [map exit](konung_game_format.md#map-exits-verified) whose target is the chosen map.
* **To port:** trace `0x0043141C` (image source, hotspot ids, the location list) and the rest of `0x00422AD8`; the exit taking itself is already `Game.TakeExit`.

### Other misc items

Status: dispatcher traced, most cases **not traced**. Misc items (category ≥ 10, table `0x0046053C`) are used by `0x00414E64(e, target, item)` from the bag (right click), the in-hand box (hotspot 19) and a portrait (hotspots 30–39, cursor mode 9). Its jump table `0x00415ADC` has one case per type; the common tail `0x00415BD3` plays sound 10.

| Types | Case | Status |
|---|---|---|
| 24 Лопата | `0x00415504` | ported: [dig mode](konung_ground_items.md#hover-and-pick-up) |
| 31 Факел | `0x004156E6` | ported: [loot highlight](konung_potions.md#loot-highlight-0x00a3e758) |
| 20 Шар магического прыжка | `0x004153B0` | opens the map window ([above](#map-window-and-travel-by-item)) |
| 14–19, 21 Магический шар … (волхв Бережков … Подземного города) | `0x00415329` … `0x004153D9` | not traced. Type 21 calls `0x004106E0(0x2B, 0x12, 0x18, 2)` with constants; hypothesis: a jump to a fixed place |
| 0–10, 12, 13, 22, 23, 25–28, 30, 33, 35–39 | own cases | not traced (books, quest items, amulet parts 25–27 share `0x0041552D`, 35/37 share `0x004157D2`) |
| 11, 29, 32, 34 | `0x00415ACD` (default) | no use (32 «Неразменный пятак» counts as money, [trade](konung_trade.md)) |

Names from `resfmt.ExeTables.ItemName(resfmt.ItemTableMisc, t)`. Porting means tracing each case; the dialogue scripts already handle quest items given or taken by NPCs.

### Revival by Живая Вода

`0x00414B3C` revives a dead member when Живая Вода (herb 13, concentration ≥ 10) is used on it ([potions](konung_potions.md)). **Single hero**: the hero's death ends the game, so there is no dead member to target. Port together with a multi-member party.

### Party play

**Single hero.** The port's selection and party are the hero. Not ported:

* multi-selection (list `0x0083B93C`, up to 10) and Shift (`0x00A3E754`, apparently "whole selection", not traced);
* the party AI (`0x0041F11C`, follow the leader with `+2` = `0x60`, formation `0x0041FA5D` with offsets `0x0045FF79`), so «Все ко мне» (F) only presses the button;
* party placement around the hero on a map change (`0x004118DC`, the other members);
* giving an item to another member by dropping it on its portrait (`0x004246F8`).

Hiring (dialogue script 291) adds members to group 0 in `gamestate`, but the scene does not draw or move them. Porting needs the party AI traced and the selection list in the scene.

### Movement

**Not traced.** The game's walk code (speed, path rules, direction choice) is reimplemented: A* on the fine grid (`mapdata/finegrid.go`), 10 animation steps/s. Also missing: the per-entity walk throttle `+0x11` (ticks where `time & m == m`) and the random pauses of creature kinds `0x4D`/`0x4F` ([combat: creature AI](konung_combat.md#creature-ai-verified)).

### Combat leftovers

See [combat: port](konung_combat.md#port).

* God mode `0x00A3E768`, toggled by `0x00436E93` (key not identified): damage to the hero −2.
* Kind `0x49` steps back after its blow (`0x004101E4`, not traced).
* The portrait feedback when a party member is hit (`0x0042D574`).
* Equipping the best bag gear on re-equip (`0x004104D4`, not traced; `ReEquip` takes the quiver only).

### Films and voices (missing files)

* `GAMEOVER.FLM` (hero death, `0x004376A8`), `DEFEAT<map>.FLM` (village surrender), `WIN.FLM` (the index-0 script pile, below), all through `0x0042B690`. No `.FLM` file is installed, and the format is unknown. The port returns to the menu 2 s after the hero's death and shows nothing else.
* `SPEECHS.RES` voices for dialogue lines and the deferred reply action while a voice plays ([dialogues](konung_dialogue.md#open-questions), [sound](konung_sound.md#not-ported)).

### Ground items

The pile record with index 0 runs `0x0042B690(0x00450004)` = `WIN.FLM` when picked up ([ground items](konung_ground_items.md)). Hypothesis: the end-of-game pile. Not ported (missing film).

### HUD

* The alternate portrait table (`0x0062F3E0` set → `0x0062F3D8`, images `0x0083E090`, index `+0xF6`) and the portrait variant `+0xF8` written by hire (script 291).
* The modified palette for entity types 2 and 8 (`0x0043FD18`, `0x0043F763`). See [interface](konung_interface_res.md#open-questions).

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
| A walking target | does not flinch | — |

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
