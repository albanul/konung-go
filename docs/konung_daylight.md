# Day and night

The world's light tint by time of day. Status: **verified** against `konung.exe` code and the exe data, and ported. Port: `gamestate.DayTint`, `resfmt.ExeTables.DayThresholds` / `MapLight`, `resfmt.TintTable`, `mapdata.ParseLitCells`, and `startScene.updateLight` / `drawTinted` (`internal/game/daylight.go`). Tests: `gamestate.TestDayTint`, `mapdata.TestParseLitCells`, `game.TestTintShaderCompiles`.

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
* Night lights `0x00A4E2A0` (8100 ≤ d < 18900) is the second condition, after «Световые эффекты» `0x00A3E734`, for the lit terrain path `0x004257A9` (`0x0042565D`). Not ported.
* After `0x00429330`, the scene draw also calls `0x0043FD18(16 + rand()%8, 8 + rand()%4, 0)`, a flickering warm tint in the tables `0x0051EC78`/`0x0051EC88`/`0x0051ECB8`. Its consumer is probably the lit path; not traced.

## Tinted palettes `0x0043ED09`

`0x0043ED09(palette byte offset)` returns the palette that a blit uses. With tint 0 it is the raw palette (`[0x0051ECB4]` + offset). Otherwise it is a converted copy at `0x004E5C70` + offset, cached per palette by tint value (`0x004A30F0 + offset>>7`). The conversion (`0x0043F796`, RGB555 branch) maps each 5-bit channel through a table for the channel's percentage:

* blue: `0x00475E2C` (negative) / `0x0047772C` (positive)
* green: `0x0047992C` / `0x0047CB2C`
* red: `0x0049FEF0` / `0x004A17F0`

The tables are built by `0x0043B27C` for 0–99 %, with the same formula as `0x0043FD18` (`resfmt.TintTable`, `resfmt.TintPalette`).

Who draws through it (tinted) and who doesn't (raw):

| Draw | Palette |
|---|---|
| Terrain (`0x0042571D`), decorations (`0x00425A0E`…) | tinted |
| Piles (`0x00426645`) | tinted; raw while the loot highlight `0x00A3E758` is on |
| Map objects (sprite pass kind 2, `0x0042754E`) | raw if header `+0x08` ≥ 2 (`resfmt.MapObject.SelfLit`, also no shadow), else tinted |
| Characters (kind 4, `0x00427285`), corpses | raw on a lit cell (`LIGHTS.N`, below), else tinted |
| Arrows and other list-2 sprites (kind 1), shadows, HUD | raw |

## `LIGHTS.N` lit cells

`LIGHTS.<map>` is 64000 bytes. The map loader reads `0x3200` bytes to `0x00671440` (80 × 160, terrain cells, for the lit terrain path) and `0xC800` bytes to `0x00664C40`. The second part is 320 × 160 bytes on the fine grid, indexed `row·0xA0 + col`. Any non-zero byte marks a lit cell; `KONUNG.1` has about 450, around the fires. A missing file means zeros. The values (1–9 and more) and the terrain part are only used by the lit path; they are not decoded.

## Port

* `startScene.updateLight` runs at the start of each `Draw` with the port's game time (`gameTime`, the value saved as `0x00A3E770`). It rolls the moon at time-of-day 0 and takes the fixed map light or `gamestate.DayTint`. The menu sets `dayNight` from the setting.
* A Kage shader (`tintShaderSrc`) applies the three 32-entry tables to the decoded RGBA sprites. Because the tint is a per-channel lookup on the 5-bit values, this gives the same colours as tinting the palette.
* The day passes at the port's game time rate: one time unit per 4 ticks (`walkTicksPerStep`), so a day takes 21600 units. The original tick rate is not traced, so how long a day lasts in real time may differ.
* Not ported: the lit terrain path (`0x00A4E2A0` and «Световые эффекты»), the flicker tint, flames.

## Open questions

* The lit terrain path `0x004257A9`: how it uses both `LIGHTS.N` parts and the flicker tint.
* The in-game start time of a new game (the port starts at 0, i.e. day).
