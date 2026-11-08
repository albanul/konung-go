# Not ported

What the Go port still leaves out, what it only approximates, and what is unknown about the formats. Each entry says what the original does (as far as it is traced), where it is in `konung.exe`, why it is missing, and what porting it would take. The topic docs remain the source of truth; this page collects their "not ported" and "open questions" lists in one place. Update both when an item is ported.

Reasons used below: **not traced** (the exe code has not been read), **missing file** (the installed game lacks the resource), **presentation** (a deliberate port choice).

## Features

### Misc items

Ported: every case of `0x00414E64` ([misc items](konung_misc_items.md)). Left over:

* An orb jump to the map the party is already on rebuilds the scene like a map exit; the original only re-places the party (`0x004118DC`).

### Movement

**Not traced.** The game's walk code (speed, path rules, direction choice) is reimplemented: A* on the fine grid (`mapdata/finegrid.go`), 10 animation steps/s. Also missing: the per-entity walk throttle `+0x11` (ticks where `time & m == m`) and the random pauses of creature kinds `0x4D`/`0x4F` ([combat: creature AI](konung_combat.md#creature-ai-verified)).

### Films

All four films play, with the original's triggers, timing and skip key ([films](konung_films.md)).

### Voices and faces (missing files)

* `FACES800.RES` (hired companions' faces, chosen by `+0xF8`; [interface](konung_interface_res.md#hired-faces-faceswidthres-missing-file)): with no file the game draws the INTR faces, as the port does.
* `SPEECHS.RES` voices for dialogue lines and the deferred reply action while a voice plays ([dialogues](konung_dialogue.md#open-questions), [sound](konung_sound.md#not-ported)).

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

* `GRAPH.RES`: the 64 KiB gap at `0x1ED8B6`; the Type 0 palette/dye choice for NPCs and the model ID → character mapping ([GRAPH](konung_graph_sprites.md)).
* `GAME.N`: item fields other than category and type; the gear table's remaining fields; which jewellery box is which; group `+0x1E` bits other than `0x20`/`0x40`/`0x80` and kind `0x53`; map exit bytes `+0`/`+1`; settlement bytes outside the traced ones ([GAME.N](konung_game_format.md#open-questions)).
* Saves: later use of the hero copy `0x00A3E584` after a load ([saves](konung_save_format.md#open-questions)).
* `SPEECHS.RES` and `FACES<width>.RES`: not installed.

## Tooling

* `cmd/sprite-catalog` has its own Type 0 walker that duplicates `resfmt.DecodeType0Frames`.
