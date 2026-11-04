# `GAME.N` start state: characters and creatures

Characters, NPCs and monsters are **not** stored in `KONUNG.N`. They come from `GAME.N`, the start-of-game state loaded by VA `0x0043BB6C`. **`N` is the hero's model number.** The loader reads `dword[0x00A3E657] >> 24`, i.e. byte `0x00A3E65A`, which is `+0xD6` of the 0x100-byte hero template at `0x00A3E584`.

## Heroes and start maps

The new-game code (`0x00438F3F`–`0x004390D5`) copies the hero template into entity 0, including model `+0xD6` from `0x00A3E65A` and palette `+0xD8` from `0x00A3E65C`. It then sets the party group's map byte to `model*3 + 1` (`0x004390B4`).

| Hero model (Type 0) | `GAME.N` | Start map | Hero |
|---|---|---|---|
| 0 | `GAME.0` | `KONUNG.1` | Volk (Волк), per the user's identification of `KONUNG.1` |
| 1 | `GAME.1` | `KONUNG.4` | Dragomir (the user recognized model 1 as Dragomir) |
| 2 | `GAME.2` | `KONUNG.7` | Michail (Михаил), per the user |

Entity 0's model and palette in the files (model 1, palette 33 in all three) are placeholders that this code overwrites. The template's values come from the character-creation screen, which is not decoded. The port therefore uses model N (`Game.HeroModelFromGame`) with **placeholder palette 72**. Palettes 70–74 and 113–114 render model 0 cleanly; 33 does not.

Code: `internal/gamestate` (`Parse`, `Game.OnMap`, `Game.HeroModelFromGame`, `WorldPos`, `Placed.LoadSprite`, `Placed.LoadAnimation`), `resfmt.LoadEntitySprite`, `resfmt.LoadEntityAnimation`. `cmd/map-render -game assets/Konung/GAME.0` draws them (Volk's game). By default `map-render` draws map content only, which keeps the site's map pages clean.

## File layout (verified)

All three installed files are 709,822 bytes, exactly the sum of six reads:

| File offset | Size | Loaded to | Content |
|---|---:|---|---|
| `0x00000` | `0x20000` | `0x0067AC34` | 8192 × 16-byte **items**; byte `+3` = item type |
| `0x20000` | `0x1900` | `0x0069D1B4` | 200 × 32-byte **groups** |
| `0x21900` | `0x7D00` | `0x0079EFDC` | 500 × `0x40` records (loader post-processes entries with word `+0x12` ≠ 0 through `0x00439DF0`); not traced |
| `0x29600` | `0x109A` | `0x007AA2DC` | 250 × `0x11` records; not traced |
| `0x2A69A` | `0x7D000` | `0x007AB378` | 2000 × 256-byte **entities** |
| `0xA769A` | `0x5E24` | `0x00835B18` | 20 × `0x4B5` records; not traced |

## Groups (32 bytes)

Group 0 is the player's party: `0x0043A895` sets the party pointer `0x00A3E684` to `0x0069D1B4`, and the hero pointer `0x00A3E694` to entity `word[group0+0]`. The map loader iterates groups 1–199 (loop at `0x0043C612`).

| Offset | Type | Meaning |
|---|---|---|
| `+0x00` | `int16` | first entity index |
| `+0x08` | `uint16` | group fine-grid **row** (the party is placed around it, `0x00411B15`) |
| `+0x0A`, `+0x0C`, `+0x0E` | `uint16` | not traced; row-like values (e.g. spider nest 152, 228, 228) |
| `+0x10` | `uint16` | group fine-grid **column** |
| `+0x12`, `+0x14`, `+0x16` | `uint16` | not traced; column-like values (34, 62, 62) |
| `+0x1B` | byte | map number (`KONUNG.N`) |
| `+0x1C` | byte | entity count (consecutive from `+0x00`) |
| `+0x1E` | byte | 0 = inactive. Bits `0x40`/`0x80` and the value range `0x41..0x50` select loader paths; meaning not decoded. |

## Entities (256 bytes, fields used by the port)

| Offset | Type | Meaning |
|---|---|---|
| `+0x00` | `int8` | type = action / animation set (animated models use `type − 6` when ≥ 6, `0x0041409A`) |
| `+0x01` | `int8` | variant = **facing direction** (8 per type) |
| `+0x03` | byte | bit `0x80`: not drawn (shadow and draw passes skip it) |
| `+0x04` | byte | bit `0x40`: animated `OBJECTS.RES` model, otherwise `GRAPH.RES` Type 0 |
| `+0x08` | `uint16` | fine-grid row; `0` together with `+0x0A` = `0` means placed at runtime |
| `+0x0A` | `uint16` | fine-grid column |
| `+0x10` | byte | **animation step**, advanced by `0x00413DE4` until it reaches the count in `+0xD7` |
| `+0xD6` | `int8` | model: `OBJECTS.RES` slot 0–29, or `GRAPH.RES` Type 0 block. **Earlier notes said `+0xD3`**; the code reads `dword[+0xD3] >> 24`, i.e. byte `+0xD6`. |
| `+0xD8` | `uint32` | palette byte offset (per-entity dye), passed to `0x0043ED09` by the draw at `0x004272CB` |

World position (VA `0x00439BA0`, fine-cell centre): `x = col*58 + (row odd ? 29 : 58)`, `y = row*16 + 16`.

## Animation types and facings (Type 0 bodies)

Body records come in two sets of six types: 0–5, and 6–11 with the same layout. The peaceful set (6–11) is used by the villagers and Volk; animated objects fold types ≥ 6 back onto 0–5. Seen in body model 0's records and frames:

| Type | Steps | Meaning |
|---|---|---|
| 6 (0) | 12, 6 frames held 2 steps each | idle |
| 7 (1) | 12 distinct frames | **walk** (verified visually; the game scene uses it) |
| 8 (2) | 7 | unknown (repeats frames; possibly hit/death) |
| 9 (3), 10 (4), 11 (5) | 12 | unknown (possibly actions/attacks) |

**Facing (variant):** 0 W, 1 NW, 2 N, 3 NE, 4 E, 5 SE, 6 S, 7 SW, i.e. clockwise from west. Identified visually from the walk frames, not from movement code. On the fine grid, the matching neighbour cells are: same row ±1 col (E/W, 58 px), row ±2 (N/S, 32 px), and row ±1 for the diagonals (±29, ±16 px; the column depends on row parity). See `mapdata.Neighbors`.

## Frame and anchor lookup (model initializer `0x00414078`)

Each (type, variant) has a record of `int16` words: `[count, frame index for step 0, step 1, …]`. Animated objects have 19 words (up to 18 steps), Type 0 has 13 (up to 12). The idle sets seen so far have 12 steps that cycle 6 frames, each held for 2 steps, e.g. Type 0 block 1 type 6 variant 5: `12, 24,24, 25,25, … 29,29`. In the table below, `d` is the **step**. Earlier notes called it direction.

| | Animated `OBJECTS.RES` (`0x00722D2C + slot*0x3228`) | `GRAPH.RES` Type 0 (`0x007145B4 + n*0x4D28`) |
|---|---|---|
| frame index | `int16[meta + 0x0A + t*0x130 + v*0x26 + d*2]` | `int16[meta + 0x0A + t*0xD0 + v*0x1A + d*2]` |
| frame pair (shadow, image offsets) | `meta + 0x728 + idx*8` | `meta + 0x1728 + idx*8` |
| anchor record (int16 shadow dx, dy, image dx, dy) | `meta + 0x1728 + t*0x480 + v*0x90 + d*8` | `meta + 0x2F28 + t*0x300 + v*0x60 + d*8` |
| image data base | `meta + 0x3228` | `meta + 0x4D28` |

`meta` is the block as stored in the file, including its 8-byte tag/size header (the runtime replaces `+4` with the data pointer). Image position = world position + image dx/dy (`0x00414277`). The shadow (pair dword 0, skipped when −1) goes into the same merged 50% shadow pass as static objects, at world position + shadow dx/dy (`0x004267BD`). Entities are depth-sorted with static objects.

## Equipment overlays (verified)

Type 0 bodies wear equipment drawn as overlay layers by `0x004281A4`, called right after the body blit (`0x0042739B` / `0x004273DD`).

* **Blocks.** `GRAPH.RES` Type 0 blocks 0–2 are the three **bodies** (loaded to `0x007145B4`); blocks 3–26 are 24 **equipment overlays** (loaded to `0x006A09F4`). Each overlay gets a fixed palette from the exe table `0x00465DCC[k]` (loader `0x0043B797`): `2,2,2,2,4,4,3,3,3,4,4,4,4,4,4,4,4,4,2,2,5,5,5,5`.
* **Slots.** Entity words `+0x66 + 2*slot` (slots 0–7) hold item indices into the items section; 0 = empty. Only slots 0–5 are drawn.
* **Item → overlay.** `type = items[i*16 + 3]`; overlay `k = int16[0x00459BEE + type*64]` (−1 = invisible).
* **Order.** With entity `+3` bit `0x04` clear, the slots are drawn in the order of `0x00462E1C + variant*0x18` (6 `int32`, e.g. facing 5: `3,2,5,4,1,0`). With the bit set (`0x004281B3`), action-dependent tables `0x0046281C`/`0x00462B1C` are used and the slot equal to entity byte `+0x12` (or 5) uses overlay `k+1`, apparently a drawn weapon. That path is not ported.
* **Frame and position.** The overlay frame comes from the overlay block's own `(type, variant, step)` sequence. The position is world position + the body's per-step attachment point (`int16 x, y` at `body + 0x828 + type*0x180 + variant*0x30 + step*4`) + the overlay anchor's image dx/dy (`overlay + 0x2F28 + type*0x300 + variant*0x60 + step*8`, words `+4`/`+6`).

Code: `resfmt.ExeTables` (reads the three exe tables from `konung.exe`), `resfmt.ComposeEquipment`, `Game.Overlays`. The port composites body and overlays into one image per step.

`KONUNG.1` in `GAME.0`: villagers wear helmets, armour, axes and shields (e.g. villager 355: overlays 17, 9, 20, 0 by facing order). Volk's starting entity has items in slots 2, 6, 7. Only slot 2 is drawn (overlay 4); slots 6–7 are carried items. The new-game code does not overwrite the slots, so this is Volk's starting kit.

## Placement of entities without coordinates

Entities with row = col = 0 are positioned at runtime around their group's position. The party code at `0x00411B8C` uses random formation offsets from tables near `0x0045FF79`. The port places them deterministically on the nearest free fine cells, using the `KONUNG.N` walk grid (`mapdata.WalkGrid.Blocked`). These positions are an **approximation**. The exact spawn routine for non-party groups is not traced.

## `KONUNG.1` (Volk's starting location) in `GAME.0`

The user identified `KONUNG.1` as Volk's (Волк) initial location.

| Group | Entities | Content |
|---|---|---|
| 0 | 1 | the player (Volk): drawn as Type 0 model 0 with placeholder palette 72 (the file's model 1 / palette 33 are overwritten on new game), at row 86 col 33 inside the village |
| 27 | 18 | villagers with fixed positions: Type 0 model 0 men; `OBJECTS` slot 14 women, slot 15 children (1-based index IDs 15/16) |
| 64 | 1 | a single Type 0 villager |
| 3 | 4 | giant spiders (slot 12, palette 148) at the nest around row 184 col 40 |
| 105–109 | 26 | giant ants (slot 0, palette 56), five groups along the stream |

`GAME.1` and `GAME.2` are the other heroes' games. Their map-1 groups are similar, with more monsters (spiders 4 → 8 → 16), but those heroes start on `KONUNG.4` / `KONUNG.7`.

Validation: `go test ./internal/gamestate` checks the player group, the 50 drawable entities on map 1, sprite decoding, and that spawned entities sit on distinct free cells. `go run ./cmd/map-render -game assets/Konung/GAME.0 -all output/maps` renders 987 `GAME.0` entities across all maps with no decode errors.

## Open questions

* The character-creation screen: the hero template's palette (`0x00A3E65C`) and other fields.
* The game's animation tick rate. The scene uses 10 steps/s and offsets each entity's phase so neighbours don't move in lockstep; both are presentation choices.
* Exact runtime spawn positions for groups whose entities have no coordinates.
* Group fields `+0x0A..+0x0E`, `+0x12..+0x16`, `+0x1E`; the conditional spawn check at `0x00411DE4`.
* The game's own movement code (speed, path rules, direction computation) is not traced; the port's click-to-move is a reimplementation. Actions other than idle/walk, and the combat overlay path (`+3` bit `0x04`, drawn weapons), are not ported.
* Item record fields other than the type byte.
* The other four `GAME.N` sections.
