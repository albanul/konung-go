# Films (`Video/*.flm`)

The game plays full-screen-window films at start-up, on the hero's death, on a village surrender and on the win pile. Status: triggers and the player interface **verified** against `konung.exe`. The file format was checked against the bytes, and the conversion against ffmpeg's decode of the originals. Port: all four triggers are wired up.

## Files

`assets/Konung/Video/`, 19 files. All are RIFF **AVI** with the extension `.flm`:

| Stream | Format |
|---|---|
| `00dc` video | `WMV3` (VC-1 Main profile), 640×360, 12 fps (`avih` 83333 µs/frame). Sequence header `4E 41 1A 01`: no B-frames, no loop filter, no variable-size transform, no `dquant`; overlap smoothing, range reduction and extended MV on |
| `01wb` audio | PCM, mono, 16-bit, 22050 Hz (`WAVEFORMATEX` tag 1) |

Empty `00dc` chunks are AVI drop frames, which repeat the previous frame: `credits` has 50, `poster` 86 and `win` 99, 8 of them trailing. The `avih` total-frames field of `win` (4261) is one more than its 4260 video chunks.

| File | Frames | Played by |
|---|---|---|
| `poster.flm` | 2793 | start-up |
| `gameover.flm` | 173 | the hero's death |
| `defeat<m>.flm`, m ∈ {1, 4, 7, 11, 14, 16, 19, 22, 24, 27, 29, 33, 35, 37, 39} | 101 | the surrender of the village on map m |
| `win.flm` | 4260 | the index-0 pile |
| `credits.flm` | 617 | nothing: the exe has no `CREDITS` string |

The exe asks for upper-case names (`POSTER.FLM`…). Windows doesn't care about case. The port uses the installed lower-case names.

## Player (`0x0042B690`)

The game has no decoder of its own. `MP.DLL` (Delphi, imports `quartz.dll`, uses `IMediaControl`/`IMediaEvent`) is a DirectShow wrapper, so Windows' installed codecs decode the films. `0x0042BC4D` loads it with `LoadLibrary("MP.DLL")` and gets `CreateAviWnd` `0x00A4E418`, `FreeAviWnd` `0x00A4E41C`, `isPlayingAviWnd` `0x00A4E420` and `StopAviWnd` `0x00A4E424`.

`0x0042B690(name)`:
1. `0x00A3E724` = 0; music volume off, `0x0042A420(0)`.
2. Builds `<game dir 0x0083DE58>` + `VIDEO\` (`0x0045026C`) + name + `,80,120,640,360` (`0x00450273`), so the film window sits at x 80, y 120, 640×360, on the 800×600 screen.
3. `CreateAviWnd(hwnd 0x00A4E3A4, string, 0x00A4E428)`. On success `0x00A3E724` = 1 (playing). On failure it goes straight on as in `0x0042B774` below: state `0x00A3E6D8` = `0x00A3E7F0`, the music volume is restored, and on state 0 the game redraws.

While `0x00A3E724` is set the message loop (`0x0042BD26`) waits in `GetMessage` and sends no game ticks (`0x400`), so the game is frozen and the screen is not redrawn. The last frame stays around the film window.

**End** `0x0042B774`: `FreeAviWnd`, state = `0x00A3E7F0`, `0x00A3E724` = 0, music volume from the setting (`0x0042A420([0x00A3E73C])`). On state 0, redraw (`0x0042DBBC(-1)`, `0x004292E4`). It is called from the window procedure `0x0042BDE8`:
* message `0x501` (sent by `MP.DLL`) with lParam 2: the film has ended (`0x0042BE6D`);
* `WM_KEYDOWN` (`0x0042BE7D`): the key goes into the game's key queue (`0x0083D27C`, 16 entries, `0x00A3E79C`) first, then a playing film stops. The main loop handles the key afterwards, in the state that follows the film: Enter after the poster activates the selected menu item, Escape after `GAMEOVER.FLM` reaches the menu's Escape handler, a key after `DEFEAT<map>.FLM` reaches the game. Mouse buttons do not stop it.

## Triggers

| Film | Where | Before | Next state `0x00A3E7F0` |
|---|---|---|---|
| `POSTER.FLM` | state 9, `0x0043914B` (set by `0x0042B9A9` at start-up) | `0x00A3E728` = 0 | 7: the menu, no game running |
| `GAMEOVER.FLM` | state 0, `0x004376A3`: hero `+4` bit `0x80` | `0x00A3E728` = 0 | 7: the menu, no game running |
| `DEFEAT<map>.FLM` | village surrender, `0x0041D781` ([combat](konung_combat.md)); `<map>` = decimal `0x00A3E7FC` | message `0x0045D37C` | 0: back to the game |
| `WIN.FLM` | pick-up `0x004110EC` (`0x00411154`): the pile on the cell has record index (`+0x0E`) 0; checked before the gold and items are taken, and nothing is taken. Record 0 is the same in `GAME.0`–`GAME.2`: map 46, cell (52, 21), one «Браслет» (category 7, type 4). Piles dropped at runtime have `+0x0E` = `0xFFFF` (`0x004139A5`) | — | 7: the menu. `0x00A3E728` is **not** cleared, so «Вернуться в игру» stays |

State 7 (`0x00438D3C`) sets state 1 and draws the main menu.

## Port

No pure-Go WMV3 decoder exists, and macOS and browsers have no system WMV decoder. So the films are converted once, and the port decodes the result in pure Go on every platform (desktop, macOS, WASM):

* `cmd/video-convert <Video dir>` runs ffmpeg (needs the WMV3 decoder, `libsvtav1` and `libvorbis`; Fedora's `ffmpeg-free` lacks WMV3, use RPM Fusion's `ffmpeg` or Homebrew's). For each `X.flm` it writes `x.ivf` (AV1, SVT-AV1 preset 6, CRF 35, key frame every 120 frames) and `x.ogg` (Vorbis q3). All 19 films: 62 MB → 23 MB. ffmpeg turns interior drop frames into duplicates, so the frame counts match the source chunk counts less trailing empty chunks.
* `resfmt.ParseIVF`, `resfmt.FilmDecoder` (AV1 via `github.com/gen2brain/gav1d`, BSD-2, pure Go), `resfmt.YUVToRGBA` (BT.601 limited range unless the stream signals BT.709 or full range), `resfmt.FilmPaths`, `resfmt.DefeatFilm`, `resfmt.FilmRect`.
* `game.filmPlayer` (`internal/game/film.go`) follows the audio clock, decodes up to 3 frames per update and ends when the frames and the sound are done, or on any key. `Menu.Update` then goes on with the same frame's input in the new state, so the key that stopped the film is handled as in the original. `Menu.playFilm`/`Menu.endFilm` mirror `0x0042B690`/`0x0042B774`: music muted, the game loop frozen, the frame it started on kept around the film (black for the poster), then the next state.
* The scene asks for the in-game films through `startScene.film` / `filmToMenu`, which the menu plays after the scene's update: `DEFEAT<map>` from `settlementTick` after the surrender message (the message stays on the frozen frame around the film; then back to the game), `WIN` from `startScene.arrive` on pile record 0 (`Pile.Index` 0; then the menu with the game still running). `GAMEOVER` comes from `startScene.gameOver`.
* Missing converted files behave like a failed `CreateAviWnd`: the next state follows at once.

Validation: `game.TestSurrenderFilm` and `game.TestWinPile` (the triggers), `resfmt.TestFilmsMatchSources` (frame counts per film; `FILM_ALL=1` decodes all of them, ~1 min) and `resfmt.TestFilmDefeat1`. Frame 50 of `defeat1` matches ffmpeg's WMV3 decode of the original at 36 dB PSNR. Decode speed of `defeat1` (101 frames) on the container: 0.7 s with SIMD and 2.9 s with `-tags noasm`, against 8.4 s of film. WASM speed has not been measured.

## Not ported / open

* What the 1999 CD films were (codec, size). Only the GOG files are known.
* `credits.flm` has no caller in the exe.
