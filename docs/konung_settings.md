# Settings screen and `KONUNG.CFG`

The settings screen «Настройки» (menu item 4, «Установки») and its file `KONUNG.CFG`. Status: **verified** against `konung.exe` code and the installed file. Port: `game.settingsScreen` (`internal/game/settings.go`), `Menu.updateSettings`, `gamestate.Config` (`internal/gamestate/config.go`).

## `KONUNG.CFG`

Path: game directory + `"KONUNG.CFG"` (strings `0x0045028D`, `0x00450811`; directory buffer `0x0083DE58`). Eight little-endian dwords, no header:

| Offset | Global | Setting (screen row) | Consumer |
|---|---|---|---|
| `0x00` | `0x00A3E774` | Video mode: 0 = 640×480, 1 = 800×600 («Видеорежим») | `0x00427B1C` sets the mode; indexes every 640/800 layout table |
| `0x04` | `0x00A3E700` | «Прозрачность»: alpha-blended flames ([flames](konung_flames.md)) | `0x0042713A`, `0x00427667` (blit `0x0043D8EF` vs `0x0043E696`), `0x0043BAFD` (FLAMES.RES conversion at load) |
| `0x08` | `0x00A3E71C` | «Смена дня и ночи»: day/night light tint | `0x004293B3`: off → no tint ([day and night](konung_daylight.md)) |
| `0x0C` | `0x00A3E734` | «Световые эффекты»: lit terrain path | `0x0042565D`: at night (`[0x00A4E2A0]`, [day and night](konung_daylight.md)), draw terrain via `0x004257A9` (`LIGHTS.N`) |
| `0x10` | `0x00A3E73C` | «Музыка» | `0x00429995`; toggle calls `0x0042A420(on)`: music buffer volume −1000 (on) / −10000 (off) via the DirectSound vtable `+0x3C` |
| `0x14` | `0x00A3E744` | background tiling: toggled by **T** in the game, not on the screen ([below](#background-tiling-t)) | `0x0042AE68` skips the world under an open window; `0x0043141C` tiles INTR slot 5 |
| `0x18` | `0x00A3E750` | «Тени»: the shadow pass | `0x004266C4` |
| `0x1C` | `0x00A3E75C` | «Звук»: sound effects | `0x0042A679` (play sound), `0x0042A9F0` |

* **Read** (`0x0042BA60`, at startup): open with `0x00440047`, then eight separate 4-byte reads `0x0044023E`. The globals are in `.bss` (zero at start), so a short file leaves the remaining settings at 0. A video mode outside 0–2 becomes 0 (`0x0042BB3A`).
* **No file** (`0x0042BB58`): every setting 1, video mode 1 (800×600).
* **Write** (`0x0043D2CC`, on «Ok» only): creates the file (`0x0044050F`, mode `0x180`) and writes the same eight dwords. The first is the *chosen* mode `0x00A539A0`, not the running one.
* The `KONUNG.CFG` that came with the install was 28 bytes (seven dwords: `1 1 1 1 0 1 1`): music off, and sound stays 0 because its dword is missing. Probably written by another build; the exe writes 32 bytes. «Ok» in the port's settings screen replaces it with the 32-byte form. Test: `gamestate.TestParseConfigInstalled` (accepts both sizes).
* A save stores `0x00A3E71C` and `0x00A3E734` (offsets `0x00`/`0x04` of `KONUNG.SA<n>`, `Game.Options`), and loading one overwrites them. Loading does not rewrite `KONUNG.CFG` ([saves](konung_save_format.md)).

## Background tiling (T)

* **T** (VK `0x54`, key switch `0x004372C7` → `0x00436F58`) flips `0x00A3E744`. It redraws the open window (`0x0042DBBC`, plus the bag strip `0x0042F50C` for window 3) and writes `KONUNG.CFG` at once (`0x0043D2CC`). The written video mode is `0x00A539A0`, which is still 0 (640×480) if the settings screen hasn't been opened since start.
* While it is on and a window is open (`0x00A3E740` ≠ 0), the frame redraw `0x0042AE5F` skips the world draw `0x0042894C`, so nothing moves behind the window.
* The windows tile their background first: `0x0043141C(x1, y1, x2, y2)` blits INTR slot 5 at (x1 + 100j, y1 + 100i) for (x2 − x1 + 100)/100 columns and (y2 − y1 + 100)/100 rows, clipped to the rectangle. Rectangles:
  * the character window and other windows: the whole view (140, 0)–(799, 555) (`0x0042DCA4`, `0x0042DEFD`, `0x0042E094`, `0x0042F4BA`);
  * the bag strip: from the strip top 486 to the bottom of the view (`0x0042F5E3`), also when the strip is shown alone.
* Port: `Menu` handles T and writes the file (`Menu.writeConfig`). `startScene.tiling` skips `drawWorld` while the character window is open. `tileBackground` tiles behind the window and the strip.

## Screen

State `0x00A3E6D8` = 6. Opening it (`0x00437DDD`) copies the current values to a backup: `0x00A5399C` transparency, `0x00A539A0` video (this is also the *chosen* mode while the screen is open), `0x00A539A4` day/night, `0x00A539A8` lighting, `0x00A539AC` music, `0x00A539B0` shadows, `0x00A539B4` sound. Then it draws.

**Draw** `0x004315F8(chosen mode)`: blit `MENU<width>R.RES` frame 1 (slot pointer `0x00631448`) at (0, 0). Then blit frame 2 (`0x00631450`, the 13×13 blue marker) centred in a hotspot box, at `x1 + (x2 − x1 − w)/2`, `y1 + (y2 − y1 − h)/2`. One marker goes in box 0 or 1 for the chosen mode, and one in each of boxes 2–7 whose setting is non-zero. Boxes come from the running mode's half of the table.

**Hotspots** `0x0046219C`: 32 bytes per entry (640×480 x1,y1,x2,y2, then 800×600), terminated by `-1`. Hit test: `0x004398C4`, on left click only (`0x00438A94`). Click switch: jump table `0x00438CE8`.

| # | 800×600 box | 640×480 box | Case | Action |
|---|---|---|---|---|
| 0 | 435,144–475,184 | 351,112–381,142 | `0x00438AC6` | chosen mode = 0 |
| 1 | 588,144–628,184 | 474,112–504,142 | `0x00438AD5` | chosen mode = 1 |
| 2 | 513,206–553,246 | 413,166–443,196 | `0x00438AE4` | flip transparency |
| 3 | 513,250–553,290 | 413,202–443,232 | `0x00438B13` | flip shadows |
| 4 | 513,294–553,334 | 413,236–443,266 | `0x00438B42` | flip day/night |
| 5 | 513,338–553,378 | 413,272–443,302 | `0x00438B71` | flip lighting |
| 6 | 513,382–553,422 | 413,306–443,336 | `0x00438BA0` | flip music, then `0x0042A420(music)` |
| 7 | 513,426–553,466 | 413,342–443,372 | `0x00438BDD` | flip sound |
| 8 | 300,510–399,600 | 250,420–319,480 | `0x00438C0C` | «Ok» |
| 9 | 400,510–499,600 | 320,420–390,480 | `0x00438C85` | «Отмена» |

* **Ok**: go to the menu (state 7, `0x00A3E7F0` = 6). If transparency changed, reload `FLAMES.RES` (`0x0043BA50`). If the chosen mode differs from the running one, switch modes (`0x00427B1C`); on failure the state becomes 3. Unless the state is 3, write `KONUNG.CFG`.
* **Отмена**: go to the menu and restore the six toggles from the backup, then `0x0042A420(music)`.
* **Escape** (`0x00437517`): same as «Отмена» without the `0x0042A420` call, so a music change keeps its volume until the next toggle.
* After every click (except Ok without a menu redraw), the screen is redrawn with the chosen mode.

## Port

* All screens run at 800×600. The video choice is stored and written to `KONUNG.CFG`, but the port does not switch modes.
* **Shadows** and **day/night** are honoured: `startScene.shadows` gates the shadow pass, `startScene.dayNight` the [day/night tint](konung_daylight.md) (the menu sets both from the config each frame).
* **Световые эффекты** is honoured: at night it switches the terrain to the [lit path](konung_daylight.md#lit-terrain-path-0x004257a9) (`startScene.lighting`).
* **Прозрачность** is honoured: flames are blended or copied ([flames](konung_flames.md), `startScene.transparency`).
* **Background tiling** is honoured (T, [above](#background-tiling-t)).
* **Музыка** and **Звук** are honoured ([sound](konung_sound.md)): music on is −10 dB, off silent (the track keeps playing); with sound off no effect plays.
* Saving puts day/night and lighting into `Game.Options`; loading copies them back into the config (`Menu.updateSaves`).
* `KONUNG.CFG` is read from and written to the game directory (next to `KONUNG.1`), like the saves.

## Open questions

* None specific to the screen; see [day and night](konung_daylight.md#open-questions) for the lit terrain path.
