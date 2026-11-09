# Day and night

The world's light tint by time of day. Status: **verified** against `konung.exe` code and the exe data, and ported. Port: `gamestate.DayTint`, `resfmt.ExeTables.DayThresholds` / `MapLight`, `resfmt.TintTable`, `resfmt.LightMasks` / `ComposeTerrainCell` / `NewLitTables` / `LightTerrainCell`, `mapdata.ParseLitCells` / `ParseTerrainLights`, and `startScene.updateLight` / `drawTinted` / `drawLitCell` (`internal/game/daylight.go`). Tests: `gamestate.TestDayTint`, `mapdata.TestParseLitCells`, `resfmt.TestLightMasks`, `resfmt.TestLightTerrainCell`, `game.TestTintShaderCompiles`.

## Light tint `0x0051EC7C`

The scene draw `0x0042894C` calls `0x00429330` once per drawn frame and stores the result in `0x0051EC7C`. The value is packed `R<<16 | G<<8 | B`, each a signed byte: a percentage for the palette tables (`resfmt.UnpackTint`). Negative darkens, positive brightens; 0 means the raw palette.

`0x00429330`:

1. Clear `0x00A4E2A0` (night lights). Let `d = [0x00A3E770] % 0x5460`: game time mod 21600 (`gamestate.DayLength`).
2. If `d == 0`: roll the moon, `0x00A3E738 = rand() % 100 < 90`. The flag starts at 0 (`.bss`) and is not saved.
3. If the map has a fixed light (`0x00A3E70C`, set by the map loader `0x0043C108` from byte `0x0045AE8F + map`): return dword `0x0045AD40 + 4·map`. Maps 42–51 have the flag. `KONUNG.42` = (−70, 0, 0), 44 = (−20, −1, −70), 45 = (0, −50, −50), the others (−1, −1, −1).
4. If day/night `0x00A3E71C` is off: return 0.
5. Otherwise pick a segment by `d`. The thresholds are 12 `int16` at `0x0045AE70`: 1800, 1830, 3570, 3600, 5400, 8100, 11700, 11880, 11910, 16155, 16200, 18900. The code loads each as `dword[0x0045AE6E + 2k] >> 16`. In a ramp, `v = (d − start) / 2`, and past the ramp length the segment holds its end value. Float products are truncated toward zero (`0x0043FE4C` sets the FPU to chop).

| `d` | Tint (R, G, B) | Night lights |
|---|---|---|
| < 1800 | 0 | 0 |
| < 1830 | ramp 24: (v, v/2, 0), then (12, 6, 0) | 0 |
| < 3570 | (12, 6, 0) | 0 |
| < 3600 | ramp 24 down: v = (24 − (d − 3570)) / 2, (v, v/2, 0); then 0 | 0 |
| < 5400 | 0 | 0 |
| < 8100 | evening, ramp 100: (trunc(0.2v), trunc(−0.4v), −v), then (10, −20, −50) | 0 |
| < 11700 | dusk, ramp 60: (trunc(10 − v/3.0), −20 − v, trunc(−50 − 2v/3.0)), then (0, −50, −70) | 1 |
| < 11880 | ramp 120: (−v, −50 − v/3, −70), then (−60, −70, −70) | 1 |
| < 11910 | no moon: (−60, −70, −70). Moonrise, ramp 40: (−60, v − 70, −70), then (−60, −50, −70) | 1 |
| < 16155 | moon: (−60, −50, −70), else (−60, −70, −70) | 1 |
| < 16200 | no moon: (−60, −70, −70). Moonset, ramp 40: (−60, −50 − v, −70), then (−60, −70, −70) | 1 |
| < 18900 | dawn, ramp 100: (trunc(0.5988·v − 60), v/5 − 70, v − 70), then (−30, −60, −20) | 1 |
| else | ramp 120: (v/2 − 30, v − 60, v/3 − 20), then 0 | 0 |

* The moonrise segment is 30 ticks but its ramp is written for 40, so on a moonlit night green jumps from −56 to −50 at 11910. Every other step is at most 1.
* Night lights `0x00A4E2A0` (8100 ≤ d < 18900) is the second condition, after «Световые эффекты» `0x00A3E734`, for the [lit terrain path](#lit-terrain-path-0x004257a9) (`0x0042565D`). Caves (fixed light) and day/night off leave it 0.
* After `0x00429330`, the scene draw rolls the flicker `0x00A4E2A4` = 8 + rand()%4 (green), then `0x00A4E2A8` = 16 + rand()%8 (red), every drawn frame. It also calls `0x0043FD18(red, green, 0)`, but the lit path overwrites those tables (`0x0051EC78`/`88`/`B8`) before any terrain blit uses them, so that call has no visible effect.

## Tinted palettes `0x0043ED09`

`0x0043ED09(palette byte offset)` returns the palette that a blit uses. With tint 0 it is the raw palette (`[0x0051ECB4]` + offset). Otherwise it is a converted copy at `0x004E5C70` + offset, cached per palette by tint value (`0x004A30F0 + offset>>7`). The conversion (`0x0043F796`, RGB555 branch) maps each 5-bit channel through a table for the channel's percentage:

* blue: `0x00475E2C` (negative) / `0x0047772C` (positive)
* green: `0x0047992C` / `0x0047CB2C`
* red: `0x0049FEF0` / `0x004A17F0`

The tables are built by `0x0043B27C` for 0–99 %, with the same formula as `0x0043FD18` (`resfmt.TintTable`, `resfmt.TintPalette`).

Who draws through it (tinted) and who doesn't (raw):

| Draw | Palette |
|---|---|
| Terrain (`0x0042571D`, `0x00425772`; on the lit path the unlit cells `0x00425A0E`, `0x00425A63`), decorations (`0x00425AE3`) | tinted |
| Lit terrain cells (night, «Световые эффекты») | [lit path](#lit-terrain-path-0x004257a9) |
| Piles (`0x00426645`) | tinted; raw while the loot highlight `0x00A3E758` is on |
| Map objects (sprite pass kind 2, `0x0042754E`) | raw if header `+0x08` (the flame count, [flames](konung_flames.md)) ≥ 2 (`resfmt.MapObject.SelfLit`, also no shadow), else tinted |
| Flames | raw |
| Characters (kind 4, `0x00427285`), corpses | raw on a lit cell (`LIGHTS.N`, below), else tinted |
| Arrows and other list-2 sprites (kind 1), shadows, HUD | raw |

## `LIGHTS.N` lit cells

`LIGHTS.<map>` is 64000 bytes, two parts; a missing file means zeros.

* `0x0000`, `0x3200` bytes → `0x00671440`: one byte per terrain cell, 160 rows × 80 columns, indexed `row·0x50 + col` (`mapdata.TerrainLights`). 0 = unlit; otherwise 1 + the `LIGHTS.RES` light mask the lit path uses (`KONUNG.1`: 163 cells, values up to 18).
* `0x3200`, `0xC800` bytes → `0x00664C40`: one byte per fine cell, 320 × 160, indexed `row·0xA0 + col` (`mapdata.LitCells`). Any non-zero byte marks a lit cell (`KONUNG.1`: about 450, around the fires); characters and corpses there are drawn untinted. **Only zero / non-zero matters** (verified): the exe references the buffer 10 times, the loader's clear and read (`0x0043C275`, `0x0043C2A1`) and eight `cmp byte, 0` that pick the raw or the tinted palette: corpses in the terrain pass (`0x00425DFC`, `0x00425EBD`, `0x00425FD6`, `0x00426097`) and characters in the sprite pass (`0x00427285`, `0x0042735C`, `0x004277CC`, `0x004278A3`), each for an animated body (pool `0x00722D30`) or a Type 0 body (`0x007145B8`). No code takes the buffer's address. The values are 1–19 (`0x13`) in the 40 installed files, 18 344 lit cells: the same range as the terrain part's 1 + light mask. Hypothesis: the editor's light number, unused by the game (a cell and its terrain cell at row/2, col/2 hold the same value in 12 526 cases, a different one in 2 878 and an unlit one in 2 940; the two grids are staggered differently, so borders disagree).

## Lit terrain path (`0x004257A9`)

Taken instead of the simple terrain pass when night lights `0x00A4E2A0` and «Световые эффекты» `0x00A3E734` are both set (`0x0042565D`).

1. **Flicker** (`0x004257A9`): if the map has a settlement (`0x00A3E6A8`) whose village group is alarmed (group `+0x1D` = 1), call `0x0043FB99(red, green)` with the flicker; otherwise with both halved.
2. **`0x0043FB99`** sets the blue/green/red tables from the day tint `0x0051EC7C`. It then builds, for light L = 0…31, the green pointer `0x004798AC[L]` and red pointer `0x0047FE70[L]`: the darken table for day + L, or the identity (darken 0) once day + L ≥ 0, so light never brightens past the flicker. It also builds the flickered channels: `0x0047982C[g]` = brighten(green flicker)[g] and `0x0047FDF0[r]` = brighten(red flicker)[r].
3. **Per terrain cell** in draw order, with L = terrain byte: if 0, draw both layers with the tinted palette as usual (`0x00425A0E`, `0x00425A63`). Otherwise compose both layers into the 114 × 64 buffer `0x004DEA70` (`0x0043FD85`, 4 bytes per pixel: b, g, r, 0, from the per-palette tables `0x00625FD8` built for terrain palettes 11–18 by `0x0043B524`). Then blit it at the cell origin with mask L − 1 (`0x0043DFE1`).
4. **`0x0043DFE1`** (RGB555 path), per buffer pixel that is not 0 (uncovered and black pixels are skipped), with mask byte m:
   * blue = day(B)[b];
   * if m ≠ 0: green = `0x004798AC[m]`[`0x0047982C[g]`], red = `0x0047FE70[m]`[`0x0047FDF0[r]`];
   * if m = 0: green = day(G)[g], red = day(R)[r].
   Only 114 of the tile's 116 columns and 64 rows are drawn.

**Light masks** (`LIGHTS.RES` from `0x1AC0`: 19 × `0x1C80` bytes → `0x005B1010`, `resfmt.LightMasks`): 114 × 64 bytes, light 0–31 per pixel. Mask 0 lights the whole cell; the others are soft gradients from the edges, corners, top or bottom, for cells at the rim of a light.

Pitfalls:

* The buffer is never cleared, so pixels a cell's layers don't cover keep the previous lit cell's colours. The port starts each cell empty (terrain tiles are diamonds that cover the same area, so this should rarely differ).
* The lit cells' sharp diamond edges next to unlit cells come from the data (mask 0 cells beside unlit ones), not from a decoding error.

## Port

* `startScene.updateLight` runs at the start of each `Draw` with the port's game time (`gameTime`, the value saved as `0x00A3E770`). It rolls the moon at time-of-day 0 and takes the fixed map light or `gamestate.DayTint`. The menu sets `dayNight` from the setting.
* A Kage shader (`tintShaderSrc`) applies the three 32-entry tables to the decoded RGBA sprites. Because the tint is a per-channel lookup on the 5-bit values, this gives the same colours as tinting the palette.
* The day passes at the game time rate, one unit per 60 ms tick, so a day of 21600 units lasts 21.6 minutes at normal speed, as in the original.
* The lit path is computed on the CPU (`resfmt.LightTerrainCell`) per visible lit cell and frame, because the flicker changes every frame. The cell's composed layers are cached (`litCell`), and the frame's tables are built once (`resfmt.NewLitTables`). This costs about 34 µs per cell.
* Flames: [flames and transparency](konung_flames.md).

## Open questions

* None. A new game starts at game time 0 (`0x00438F05`–`0x00438F15`), as in the port.
