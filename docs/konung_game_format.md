# `GAME.N` start state: characters and creatures

Characters, NPCs and monsters are **not** stored in `KONUNG.N`. They come from `GAME.N`, the start-of-game state loaded by VA `0x0043BB6C`. `N` is the byte at `0x00A3E657`, set by the new-game choice.

Code: `internal/konunggame` (`Parse`, `Game.OnMap`, `WorldPos`, `Placed.LoadSprite`), `resource.LoadEntitySprite`. `cmd/map-render -game assets/Konung/GAME.0` draws them; `-game ""` disables them.

## File layout (verified)

All three installed files are 709,822 bytes, exactly the sum of six reads:

| File offset | Size | Loaded to | Content |
|---|---:|---|---|
| `0x00000` | `0x20000` | `0x0067AC34` | not traced |
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
| `+0x00` | `int8` | type (animation set; animated models use `type − 6` when ≥ 6, `0x0041409A`) |
| `+0x01` | `int8` | variant |
| `+0x03` | byte | bit `0x80`: not drawn (shadow and draw passes skip it) |
| `+0x04` | byte | bit `0x40`: animated `OBJECTS.RES` model, otherwise `GRAPH.RES` Type 0 |
| `+0x08` | `uint16` | fine-grid row; `0` together with `+0x0A` = `0` means placed at runtime |
| `+0x0A` | `uint16` | fine-grid column |
| `+0x10` | byte | direction |
| `+0xD6` | `int8` | model: `OBJECTS.RES` slot 0–29, or `GRAPH.RES` Type 0 block. **Earlier notes said `+0xD3`**; the code reads `dword[+0xD3] >> 24`, i.e. byte `+0xD6`. |
| `+0xD8` | `uint32` | palette byte offset (per-entity dye), passed to `0x0043ED09` by the draw at `0x004272CB` |

World position (VA `0x00439BA0`, fine-cell centre): `x = col*58 + (row odd ? 29 : 58)`, `y = row*16 + 16`.

## Frame and anchor lookup (model initializer `0x00414078`)

| | Animated `OBJECTS.RES` (`0x00722D2C + slot*0x3228`) | `GRAPH.RES` Type 0 (`0x007145B4 + n*0x4D28`) |
|---|---|---|
| frame index | `int16[meta + 0x0A + t*0x130 + v*0x26 + d*2]` | `int16[meta + 0x0A + t*0xD0 + v*0x1A + d*2]` |
| frame pair (shadow, image offsets) | `meta + 0x728 + idx*8` | `meta + 0x1728 + idx*8` |
| anchor record (int16 shadow dx, dy, image dx, dy) | `meta + 0x1728 + t*0x480 + v*0x90 + d*8` | `meta + 0x2F28 + t*0x300 + v*0x60 + d*8` |
| image data base | `meta + 0x3228` | `meta + 0x4D28` |

`meta` is the block as stored in the file, including its 8-byte tag/size header (the runtime replaces `+4` with the data pointer). Image position = world position + image dx/dy (`0x00414277`). The shadow (pair dword 0, skipped when −1) goes into the same merged 50% shadow pass as static objects, at world position + shadow dx/dy (`0x004267BD`). Entities are depth-sorted with static objects.

## Placement of entities without coordinates

Entities with row = col = 0 are positioned at runtime around their group's position. The party code at `0x00411B8C` uses random formation offsets from tables near `0x0045FF79`. The port places them deterministically on the nearest free fine cells, using the `KONUNG.N` walk grid (`konungmap.WalkGrid.Blocked`). These positions are an **approximation**. The exact spawn routine for non-party groups is not traced.

## `KONUNG.1` (Volk's starting location) in `GAME.0`

The user identified `KONUNG.1` as Volk's (Волк) initial location.

| Group | Entities | Content |
|---|---|---|
| 0 | 1 | the player: Type 0 model 1, palette 33, at row 86 col 33 (inside the village) |
| 27 | 18 | villagers with fixed positions: Type 0 model 0 men; `OBJECTS` slot 14 women, slot 15 children (1-based index IDs 15/16) |
| 64 | 1 | a single Type 0 villager |
| 3 | 4 | giant spiders (slot 12, palette 148) at the nest around row 184 col 40 |
| 105–109 | 26 | giant ants (slot 0, palette 56), five groups along the stream |

`GAME.1` and `GAME.2` have the same groups but more monsters (spiders 4 → 8 → 16), and the player starts at (72, 34) and (100, 60) respectively. Whether `N` selects difficulty or hero is **not verified**.

Validation: `go test ./internal/konunggame` checks the player group, the 50 drawable entities on map 1, sprite decoding, and that spawned entities sit on distinct free cells. `go run ./cmd/map-render -all output/maps` renders 987 `GAME.0` entities across all maps with no decode errors.

## Open questions

* What `N` in `GAME.N` selects (difficulty vs hero), and the new-game screen that sets `0x00A3E657`.
* Exact runtime spawn positions for groups whose entities have no coordinates.
* Group fields `+0x0A..+0x0E`, `+0x12..+0x16`, `+0x1E`; the conditional spawn check at `0x00411DE4`.
* Animation: frames are static (the current type/variant/direction frame); frame sequencing is not ported.
* The other four `GAME.N` sections.
