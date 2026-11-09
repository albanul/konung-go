# Not ported

What the Go port still leaves out, what it only approximates, and what is unknown about the formats. Each entry says what the original does (as far as it is traced), where it is in `konung.exe`, why it is missing, and what porting it would take. The topic docs remain the source of truth; this page collects their "not ported" and "open questions" lists in one place. Update both when an item is ported.

Reasons used below: **not traced** (the exe code has not been read), **presentation** (a deliberate port choice).

`SPEECHS.RES` (voices) and `FACES<width>.RES` (hired faces) are not on this list: the exe has code for them, but the game never shipped the files, so the original never uses that code ([sound](konung_sound.md#files), [interface](konung_interface_res.md#hired-faces-faceswidthres-never-shipped)).

## Features

### Misc items

Ported: every case of `0x00414E64` ([misc items](konung_misc_items.md)), including a jump to the map the party is on (only the party is placed again, `0x004377A6`).

### Movement

Ported ([movement](konung_movement.md)): the 60 ms tick, the game's path search (straight routes, 31-step paths), turning one octant per tick, the creature throttle `+0x11` and the `0x4D`/`0x4F` pauses, and entering the next cell at the step's start. Left over (see [remaining differences](konung_movement.md#remaining-differences)): a blocked goal of the player's order still sends the walker to the nearest free cell; the shared search budget; the pixel-to-cell diamond test; the chase patch.

### Films

All four films play, with the original's triggers, timing and skip key ([films](konung_films.md)).

### Screens

* The 640×480 layouts (`INTR640R.RES`, `MENU640R.RES`, `NEWH640R.RES`, the 640 hotspot map): every screen runs at 800×600 (**presentation**).

## Approximations

Ported, but not bit-exact.

| What | Port | Original |
|---|---|---|
| Random numbers | Go's RNG | `rand()` `0x0043FDE5`: state · `0x41C64E6D` + `0x3039`, result (state >> 16) & `0x7FFF`, seeded from the clock at start-up (`0x0043D270`); its sequences differ per run, so only the 15-bit rounding could be copied |
| Pixel to cell (clicks, hover, arrows) | nearest cell centre (`mapdata.CellAt`) | diamond test `0x00439BDC` ([movement](konung_movement.md#pixel-to-cell-0x00439bdc-traced-not-ported)) |
| Creature chase | a new path whenever the target moves | patches the last step ([movement](konung_movement.md#chase-patch-0x00411fd0-partly-traced)) |
| Search budget | separate 3-search budgets for the combat AI and the calm village AI; none for the party | one budget for all, in entity order ([movement](konung_movement.md#remaining-differences)) |
| Idle animation | one frame per tick, with a per-entity phase offset | one frame per tick from step 0 |
| HUD press | held for 6 updates (100 ms) | one main-loop pass (one 60 ms frame) |
| Spawn of non-creature groups without coordinates | nearest free fine cells around the group | does not occur: no living entity of such a group lacks coordinates in `GAME.0`–`GAME.2` |

## Format unknowns

* `GAME.N`: item fields other than category and type; the gear table's remaining fields; which jewellery box is which; group `+0x1E` bits other than `0x20`/`0x40`/`0x80` and kind `0x53`; map exit bytes `+0`/`+1`; settlement bytes outside the traced ones ([GAME.N](konung_game_format.md#open-questions)).
* Saves: later use of the hero copy `0x00A3E584` after a load ([saves](konung_save_format.md#open-questions)).
