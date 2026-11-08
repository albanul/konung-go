# Not ported

What the Go port still leaves out, what it only approximates, and what is unknown about the formats. Each entry says what the original does (as far as it is traced), where it is in `konung.exe`, why it is missing, and what porting it would take. The topic docs remain the source of truth; this page collects their "not ported" and "open questions" lists in one place. Update both when an item is ported.

Reasons used below: **not traced** (the exe code has not been read), **presentation** (a deliberate port choice).

`SPEECHS.RES` (voices) and `FACES<width>.RES` (hired faces) are not on this list: the exe has code for them, but the game never shipped the files, so the original never uses that code ([sound](konung_sound.md#files), [interface](konung_interface_res.md#hired-faces-faceswidthres-never-shipped)).

## Features

### Misc items

Ported: every case of `0x00414E64` ([misc items](konung_misc_items.md)), including a jump to the map the party is on (only the party is placed again, `0x004377A6`).

### Movement

Ported ([movement](konung_movement.md)): the 60 ms tick, the game's path search (straight routes, 31-step paths), turning one octant per tick, the creature throttle `+0x11` and the `0x4D`/`0x4F` pauses. Left over (see [remaining differences](konung_movement.md#remaining-differences)): a blocked goal of the player's order still sends the walker to the nearest free cell; during a step both cells stay held; the party's re-search ignores the AI budget.

### Films

All four films play, with the original's triggers, timing and skip key ([films](konung_films.md)).

### Screens

* The 640×480 layouts (`INTR640R.RES`, `MENU640R.RES`, `NEWH640R.RES`, the 640 hotspot map): every screen runs at 800×600 (**presentation**).

## Approximations

Ported, but not bit-exact.

| What | Port | Original |
|---|---|---|
| Random numbers | Go's RNG | the C runtime's `rand()`; same distributions, different sequences |
| Spawn of non-creature groups without coordinates | nearest free fine cells around the group ([GAME.N](konung_game_format.md#placement-of-entities-without-coordinates)) | `0x00411B8C` formation offsets, not traced for villagers |
| Idle animation | one frame per tick, with a per-entity phase offset | one frame per tick from step 0 |
| Creature chase | re-plans the whole path when the target moves | patches the last step |
| Arrow points | nearest cell centre | `0x00439BDC` |
| Search budget | separate 3-search budgets for the combat AI and the calm village AI | one budget `0x00A3E720` |
| HUD press | held for one animation step | one main-loop pass |
| New game time | 0 (day) | not traced |
| A walk cut by a hit reaction | snaps back to the cell being left | the cell semantics of a step are not traced |

## Format unknowns

* `GAME.N`: item fields other than category and type; the gear table's remaining fields; which jewellery box is which; group `+0x1E` bits other than `0x20`/`0x40`/`0x80` and kind `0x53`; map exit bytes `+0`/`+1`; settlement bytes outside the traced ones ([GAME.N](konung_game_format.md#open-questions)).
* Saves: later use of the hero copy `0x00A3E584` after a load ([saves](konung_save_format.md#open-questions)).
