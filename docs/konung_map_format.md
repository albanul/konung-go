# `KONUNG.N` map format

This records the verified layout of the `KONUNG.N` map files (terrain, flat decorations, static objects with shadows) and how the game draws them. Characters, NPCs and monsters come from `GAME.N`; see [the GAME.N notes](konung_game_format.md).

Code: `internal/mapdata` (`Parse`, `ParseDecorations`, `ParsePlacements`, `LoadPlacedObjects`, `DrawOrder`, `RenderScene`), `resfmt.LoadMapObject`, `resfmt.DecodeShadowMask`. Tool: `cmd/map-render`.

## File sections (observed)

Every installed `KONUNG.N` is 128,800 bytes (`0x1F720`). The map loader at VA `0x0043C0FC` reads four consecutive sections (the `0x200` argument to the open helper is an open flag, not a header skip):

| File offset | Size | Loaded to | Role |
|---|---:|---|---|
| `0x00000` | `0x6400` | `0x00627FD8` | **Terrain**: 160 rows × 80 cells × 2 layer bytes (verified, below) |
| `0x06400` | `0xC800` | `0x00512470` | 320 × 160 byte grid, expanded to 16-bit words at `0x00505C70` (below). Values in `KONUNG.1`: `0x00`, `0x0F`, `0x4F`, `0x40`. Hypothesis: walkability/obstacle grid at half-cell resolution. **Not terrain.** |
| `0x12C00` | `0x3E80` | `0x00828378` | **Flat decorations**: up to 999 × 16-byte records (verified, below) |
| `0x16A80` | `0x8CA0` | `0x0082CE78` | **Static object placements**: up to 999 × 36-byte records (verified, below) |

Expansion of section 1 (loop at `0x0043C498`): byte `0xFF` → word `0x0FFF`; if `(b & 0x0F) == 0x0F` → `(b << 8) | 0x0FFF`; otherwise → `b << 8`. Arrows stop at word bits `0xC000`, i.e. byte bits `0xC0` (`0x0041130C`, `0x00421170`; `WalkGrid.ArrowBlocked`), while plain walls (`0xFFF` only) let them pass ([bows](konung_combat.md#bows-verified)).

## Terrain cells (verified)

* Row `r` (0–159) starts at byte `r * 0xA0`; cell `c` (0–79) holds two bytes at `r * 0xA0 + c * 2`: layer 0 (base) and layer 1 (overlay).
* Each byte is a **1-based** index into the `GRAPH.RES` Type 1 tile table (`GRAPH.T1` slot = byte − 1); `0` means nothing drawn. The `GRAPH.RES` loader at `0x0043B6B1` reads the 512 × 8-byte table into `0x006314B8`, the same table the terrain renderer indexes.
* Map extents (loader, `0x0043C353`–`0x0043C43B`): the highest row with a non-zero layer-0 byte gives height `row * 0x20`; the highest such column gives width `(col − 1) * 0x74`. `KONUNG.1`: last row 114, last column 32.

## Projection and draw order (verified)

Terrain renderer at VA `0x0042562F` (simple path; a second path at `0x004257A9` also reads the `LIGHTS.N` buffer at `0x00671440`, not ported):

```
x = originX + col * 0x74 + (row odd ? 0x3A : 0)   // 116 px, odd rows shifted 58 px
y = originY + row * 0x20                          // 32 px
```

Rows are drawn top to bottom and columns left to right. Layer 0 is drawn, then layer 1, both at the same position. The blit routine at `0x0043E242` treats `(x, y)` as the image's top-left corner (no hotspot) and clips against the viewport.

Validation: `go run ./cmd/map-render assets/Konung/KONUNG.1 output/maps/konung1.png` produces a 3886 × 3712 image (use `-terrain` for the terrain layer alone); its terrain is seamless: coast and sea along the top, a branching river, and a road on the right. `go test ./internal/mapdata` checks the extents and that every referenced tile decodes.

`go run ./cmd/map-render -all output/site/maps` renders all 43 installed maps (terrain, decorations, shadows, objects) to `konung_NN.png` plus a browsable `index.html`. All of them parse and render without missing tiles or object decode errors. Extents range from 10 × 23 cells (`KONUNG.6`) to 80 × 67 (`KONUNG.36`, the full 80-column width).

## Flat decorations: section 2 (verified)

Loader `0x0043C537`; draw loop `0x00425A95`, which runs right after the terrain pass, in file order. Records end at 999 entries or when the first dword is ≤ −1. The count is stored at `0x00A3E7F8`.

| Offset | Type | Meaning |
|---|---|---|
| `+0x00` | `int32` | **0-based** `GRAPH.T1` slot. Terrain bytes are 1-based; these are not. The loader replaces it with an image pointer. |
| `+0x04` | `uint32` | Overwritten by the loader with the T1 palette byte offset (file value ignored) |
| `+0x08` | 4 bytes | zero in files; role unknown |
| `+0x0C` | `int16` | x, world pixels (image top-left) |
| `+0x0E` | `int16` | y, world pixels |

`KONUNG.1`: 373 records sorted by y; slots 16–31 and 169–197 (ground clutter, flowers, logs).

## Static object placements: section 3 (verified)

Loader `0x0043CB98`; shadow pass and depth bucketing `0x00426833`–`0x00426B78`. Records end at 999 entries or when the first dword is ≤ −1. The count is stored at `0x00A3E800`.

| Offset | Type | Meaning |
|---|---|---|
| `+0x00` | `int32` | Object ID. `OBJECTS.RES` 0-based slot = ID + `0x1E` (always a static entry). The loader replaces it with a pointer to the object's 0x100-byte header (`0x00780DDC + ID*0x100`). |
| `+0x04` | `uint32` | Palette byte offset override; `0` → header `+0x14` |
| `+0x08` | 8 bytes | zero in files |
| `+0x10` | 12 bytes | `0xFF` in files; role unknown |
| `+0x1C` | `uint16` | runtime (animation counter, set from header `+0x18`) |
| `+0x1E` | `uint16` | fine-grid column → replaced by world x |
| `+0x20` | `uint16` | fine-grid row → replaced by world y |
| `+0x22` | byte | flags: `0x08` raises the depth baseline by h/4; `0x80` runtime |
| `+0x23` | byte | variant/frame. The loader sets it to 0, or to `rand() % header[+0x0C]` when header `+0x18` > 0. |

Position (loader `0x0043CC0B`), on the same 160 × 320 fine grid as section 1:

```
x = col * 0x3A - (row odd ? 0x1D : 0)    // 58 px, odd rows shifted LEFT 29 px
y = row * 0x10                           // 16 px
```

At load time, the loader at `0x0043CB21` can substitute record IDs from a game-state table (`0x00458A62`), e.g. for quest-dependent objects. This is not ported; the files' IDs are drawn as-is.

### Static object header (0x100 bytes, `OBJECTS.RES` slot ≥ 30)

Per variant `f` (0–7), 8-byte entries:

| Offset | Meaning |
|---|---|
| `+0x04` | data size in file; replaced at runtime by a pointer to the data after the header |
| `+0x08` | if ≥ 2, the shadow is not drawn |
| `+0x0C` | variant count (random variant pick) |
| `+0x14` | default palette byte offset |
| `+0x18` | > 0 enables animation / random variant |
| `+0x80 + 8f` | shadow image offset (relative to the data after the header); −1 = none |
| `+0x84 + 8f` | image offset; variant 0 is typically 0, so the image sits right after the header |
| `+0xC0 + 8f`, `+0xC2 + 8f` | shadow dx, dy (`int16`) added to the placement position |
| `+0xC4 + 8f`, `+0xC6 + 8f` | image dx, dy (`int16`) |

Example: `KONUNG.1`'s first record is ID 117 at col 27, row 33, giving world (1537, 528). It maps to slot 147, a 454 × 378 log house with image offset (−225, −346) and shadow offset (−432, −180).

### Shadows and draw order

* Shadow images use the indexed row-RLE header and row sizes, but a positive command is a run of shadow pixels with **no pixel payload** (`0x0043D3D8` turns them into per-row span lists).
* After all shadows are collected, `0x0043E5DF` darkens every covered framebuffer pixel once: `(c & 0x7BDF) >> 1` (RGB555; `0xF7DF` mask in 565 mode). Overlapping shadows don't stack.
* Object images are bucketed by bottom edge: `y + dy + height`, minus `height/4` if flag `0x08` is set. They are drawn in that order, interleaved with characters.
* Full order: terrain → decorations → shadows (single 50% darkening) → depth-sorted objects and characters.
* **No occlusion effect (checked).** A frame is the terrain pass `0x0042562C` and the sprite pass `0x00426684` (called from `0x00428A34`/`0x00428A40`), nothing after. In the sprite pass every object and character (kinds 2 and 4, plus the second list `0x00A3E8A0`) goes through the opaque clipped RLE blit `0x0043E242`. There is no stencil test, no outline and no second, transparent draw of a character hidden behind a building. A character covered by an object stays covered. The only per-character palette choice is lighting: a character on a cell whose byte in `0x00664C40` (`row*0xA0 + col`) is set uses its raw palette, otherwise the light-tinted one (`0x0043ED09`). The only blending is the 50% shadow pass and the flames: `FLAMES.RES`'s 74 frames (`0x0083B990`) are alpha-blended through `0x0043D8EF` (table `0x0047FEF0`) when option `0x00A3E700` is on, otherwise copied with `0x0043E696`. Option `0x00A3E750` turns the shadow pass on (`0x004266C4`).

Validation: the `KONUNG.1` render shows a palisaded village (log houses, a smithy, a building site, a well), trees with ground shadows, and a stone dolmen. All 43 maps render with no object decode errors. `go test ./internal/mapdata ./internal/resfmt` checks counts, the first records, offsets and depth order.

## Location names (verified)

The exe names every map: a table of 40-byte CP866 strings at DGROUP `0x0045B3F4`, entry `map − 1` (`gamestate.LocationName`). It is indexed with the current map `0x00A3E7FC` when the default save name is built (`0x00437A34`, [saves](konung_save_format.md#save-0x00437a25--0x00438a2f--0x00424e34)). Maps marked with * have no `KONUNG.N` among the installed files.

| N | Name | N | Name | N | Name |
|---|---|---|---|---|---|
| 1 | Родники | 19 | Черный Бор | 37 | Нижний лагерь |
| 2 | Укромное место | 20 | Волхв у Черного Бора | 38 | Волхв у Нижнего лагеря |
| 3 | Волхв у Родников | 21 | Лагерь Волка | 39 | Лесной лагерь |
| 4 | Старый лагерь | 22 | Поречье | 40 | Волхв у Лесного лагеря |
| 5 | Волхв у Старого лагеря | 23 | Волхв у Поречья | 41* | Гнездо |
| 6 | Мыс | 24 | Куницын Бор | 42 | Пещера Воды |
| 7 | Византийский Лагерь | 25 | Волхв у Куницыного Бора | 43 | Пещера Зверя |
| 8 | Полуостров | 26* | Островок на болоте | 44 | Пещера Луны |
| 9* | Болото у Византийского Лагеря | 27 | Бережки | 45 | Пещера Огня |
| 10 | Волхв у Византийского Лагеря | 28 | Волхв у Бережков | 46 | Гнездо Дракона |
| 11 | Ловье | 29 | Камни | 47 | Старая шахта |
| 12 | Волхв у Ловье | 30 | Волхв у Камней | 48–50 | Гнездо Дракона |
| 13* | Поляны у Ловье | 31* | Пещера у Камней | 51–55* | Пещера №10 … №14 |
| 14 | Верхний лагерь | 32 | Волхв у Пропасти | 56* | Остров |
| 15 | Волхв у Верхнего лагеря | 33 | Борье | | |
| 16 | Лесовье | 34* | Рудник у Борья | | |
| 17 | Волхв у Лесовья | 35 | Средний лагерь | | |
| 18* | Пепелище у Лесовья | 36 | Волхв у среднего лагеря | | |

So `KONUNG.1` (Volk's start) is «Родники», and «Старый лагерь» (Old Camp), Dragomir's start in the walkthrough, is `KONUNG.4` (model 1 → map 4). Michail's start `KONUNG.7` is «Византийский Лагерь».

## Open questions

* Characters/NPCs/monsters come from `GAME.N`, not from the map; see [the GAME.N notes](konung_game_format.md).
* Animated static objects (header `+0x18` > 0, frame cycling) and random variants: the port always draws variant 0.
* ID substitution from the game-state table at `0x0043CB21`.
* Objects anchored above y = 0 are clipped in `map-render` PNGs; the game scrolls past them.
* Section 1 semantics beyond walls (`0xFFF`) and arrow obstacles (bits `0xC0`): the other bits.
* The `LIGHTS.N` shading path. The loader reads `0x3200` bytes to `0x00671440` (80 × 160, one byte per terrain cell, indexed `row*0x50 + col` by the lit draw path) and `0xC800` bytes to `0x00664C40`.
