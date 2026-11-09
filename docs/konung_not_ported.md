# Not ported

What the Go port still leaves out, what it only approximates, and what is unknown about the formats. Each entry says what the original does (as far as it is traced), where it is in `konung.exe`, why it is missing, and what porting it would take. The topic docs remain the source of truth; this page collects their "not ported" and "open questions" lists in one place. Update both when an item is ported.

Reasons used below: **not traced** (the exe code has not been read), **presentation** (a deliberate port choice). [Port choices](#port-choices) lists the differences that are deliberate and not planned for porting.

`SPEECHS.RES` (voices) and `FACES<width>.RES` (hired faces) are not on this list: the exe has code for them, but the game never shipped the files, so the original never uses that code ([sound](konung_sound.md#files), [interface](konung_interface_res.md#hired-faces-faceswidthres-never-shipped)).

## Features

### Misc items

Ported: every case of `0x00414E64` ([misc items](konung_misc_items.md)), including a jump to the map the party is on (only the party is placed again, `0x004377A6`).

### Movement

Ported ([movement](konung_movement.md)): the 60 ms tick, the game's path search (straight routes, 31-step paths), turning one octant per tick, the creature throttle `+0x11` and the `0x4D`/`0x4F` pauses, and entering the next cell at the step's start. Nothing is left; the remaining differences are [port choices](#port-choices).

### Films

All four films play, with the original's triggers, timing and skip key ([films](konung_films.md)).

## Port choices

Deliberate differences, not planned for porting:

| What | Port | Original | Why |
|---|---|---|---|
| Screen size | 800×600 only | 640×480 too (`INTR640R.RES`, `MENU640R.RES`, `NEWH640R.RES`, the 640 hotspot map) | presentation |
| A walk order to a blocked cell | walks to the nearest free cell | does not walk | convenience |
| Path search budget | 3 searches per tick for the combat AI and 3 for the calm village AI; none for the party | one budget of 3 for everyone, in entity order (`0x004116CC`) | hardware limit of 1999 |
| Creature chase | a new path whenever the target moves | bends the path's last step (`0x00411FD0`, partly traced) | hardware limit of 1999 |

Details: [movement: port choices](konung_movement.md#port-choices).

## Approximations

Ported, but not bit-exact.

| What | Port | Original |
|---|---|---|
| Random numbers | Go's RNG | `rand()` `0x0043FDE5`: state · `0x41C64E6D` + `0x3039`, result (state >> 16) & `0x7FFF`, seeded from the clock at start-up (`0x0043D270`); its sequences differ per run, so only the 15-bit rounding could be copied |
| Spawn of non-creature groups without coordinates | nearest free fine cells around the group | does not occur: no living entity of such a group lacks coordinates in `GAME.0`–`GAME.2` |

## Format unknowns

* `GAME.N`: settlement bytes outside the traced fields; item record byte `+0x0F` and `+0x0E` bits 1–7 ([GAME.N](konung_game_format.md#open-questions)).
