# Sound effects and music

`SOUNDS.RES` effects, `MUSICS.RES` music, what triggers which sound, and the «Музыка» / «Звук» settings. Status: **verified** against `konung.exe` code and the files, and fully ported. Port: `gamestate/sound.go` (selection rules, `Game.Sounds` queue), `resfmt/audio.go` (`LoadMusicTracks`, `RenderSound`, `DSGain`, `DSPanGains`), `game/audio.go` (`soundSystem`, `startScene.playAt`, `actionSound`, `soundTick`). Tests: `gamestate.TestSoundRules`, `resfmt.TestLoadMusicTracks`, `resfmt.TestRenderSound`.

## Files

* **`SOUNDS.RES`**: Type 1 header, 1000 × (offset, size) at `0x00632500` (`0x1F40` bytes), raw mono 16-bit PCM ([res format](konung_res_format.md#soundsres--sounds_res)). 391 entries are used. The resource loader `0x0043A3DC` (from `0x0043A61F`) keeps ids 0–32 and the hero voice groups 32–55 resident. Others (creature voices, the map's ambient sounds) are queued per map and loaded one per main-loop pass (`0x0043D1D0`). An id whose data isn't loaded yet doesn't play. The port loads all entries at start.
* **`MUSICS.RES`**: Type 1 header (tag 1 BE, data size LE), 9 × (offset, size) at `0x006324B8`, then the data, read whole into `[0x00A3E6C4]` (`0x00438E04`). Each track is an Ogg Vorbis stream (2.5 minutes each), decoded through `vorbisfile.dll` (`0x0042C558`, `0x0042C7AC`). `resfmt.LoadMusicTracks`.
* **`SPEECHS.RES`** (dialogue voices, 8192-entry table `0x00634440`, played by `0x0042A9C4`) was never shipped with the game (per the project owner). The loader (`0x0043A6D7`) skips the read when the open fails, so every entry's size (`0x00634444`) stays 0 and `0x0042A9C4` returns at once (size < 2). 22 of the 3644 dialogue nodes with text still name a voice: leftovers. Nothing to port.

## Playing a sound (`0x0042A664(id, volume, pan, loop)`)

Volume and pan are DirectSound units: hundredths of a dB (0 … −10000) and −10000 (left) … 10000 (right).

1. Nothing happens when:
   * there is no DirectSound, or «Звук» `0x00A3E75C` is off;
   * volume < −4000;
   * the entry is smaller than 2 bytes, or not loaded.
2. Finished buffers are released. At 45 playing buffers (`0x2D`), nothing more plays.
3. **Format**: mono 16-bit at 22050 Hz (`0x00A4E368`). Voices (id ≥ 0x20) and ids 2, 3, 6, 7, 8 pick one of three formats at random: 22050, 19050 or 25050 Hz. Playing a sample at another rate shifts its pitch.
4. Create the buffer, copy the sample, SetVolume, SetPan, Play (looping if asked). The return value is a handle; `0x00A3E7A8` tracks one looped sound.

**Position** (callers that pass a cell): fine-cell centres (`0x00439BA0`).
* Volume `0x00439F4C` = −10000·d/2048 with d = trunc(√(dx² + dy²)) from the hero, −10000 beyond 2048 px. With the −4000 cut-off, sounds are heard within about 820 px.
* Pan `0x00439EEC` = trunc((col − hero col) · 10000 · 0.00625), i.e. 62.5 per fine column.

Port: `soundSystem.play` renders the sample to 44.1 kHz stereo with the gains (`resfmt.RenderSound`, linear interpolation). The positional part is `startScene.playAt`.

## Which sound when

| Id | Trigger | Code | Port |
|---|---|---|---|
| 0 | lift or put on jewellery | `0x0041E930` | `ClickJewellery` |
| 1 / 11 | lift or put on equipment: 1 for category > 2, else 11 | `0x0041E50C`, `0x0041E5B1`, `0x0041E6E3` | `ClickEquipment` |
| 2 / 8 | swing (action 5) with axe/club / blade (weapon mode 2) | `0x004146D3` | `startAct` |
| 3 | bow shot (action 4, humans) | `0x004146C7` | `startAct` |
| 4 | click: HUD buttons (attack all `0x00421698`, stance `0x0042179C`, weapon `0x0042192C`, gather `0x004226C0`, bag `0x00424610`), the in-hand box when something moves (`0x0041EB84`), a mouse click on an active item of the main menu, with or without a running game (`0x004379E2`; without a game items 0–1 are inactive and silent; Enter makes no sound), the world map toggle (M / HUD 2, `0x00436EB9`, `0x00422C01`) and the quest log toggle (Q / HUD 5, `0x0042459C`); the settings screen's clicks are silent | | `hud.press`, `boxSwap`, `Menu.Update` |
| 6 | the party's blow is dodged (`0x0041C480` = −2), at the target | `0x0042204B` | `startScene.swing` |
| 7 | the hero starts walking (action 1) | `0x004146F6` | `walkTo` |
| 9 | a herb is used: mixed or drunk | `0x0041E195` | `UseHerb` |
| 10 | every misc item use: the common tail of `0x00414E64` ([misc items](konung_misc_items.md)); the shovel (`0x00415520`) and the jump orb (`0x004153CC`) play it once more in their cases | `0x00415C6C`, `0x00415520`, `0x004153CC` | `UseMisc` |
| 8m + 0x22 … 0x24 | human voice (model m = `+0xD6`): death (action 3) +0, idle +1 (`rand() % 100 > 95`, 4 in 100; also on the hero-creation screen, [new game](konung_new_game.md#sound-verified)), hit reaction (2) +2 | table `0x00414768` | `startAct`, idle, `newHeroScreen.idle` |
| 8·`+0x04` − 0x1D0 + k | animated models (creatures, women, children; `+0x04` with bit `0x40`): k = 0 swing (5), 1 wake-up cry (step 1 of action 4, `0x00412A60`), 2 death (3), 3 idle (`> 95`, 4 in 100) or walk (`> 75`, 24 in 100), 4 hit reaction (2) | table `0x00414500` | `startAct`, `creatureTick`, idle |
| 0x100 + (map − 1)·8 + 0…7 | ambient: once per main-loop pass with `rand() % 100 > 98` | `0x004376F4` | `soundTick` |

* Action sounds are played by `0x004143DC`, which starts an action animation (port `walker.startAct`); actions 6–9 (outside the stance) use 0–3. Character sounds are positional.
* Walk: `0x004142D4` (set a destination) starts action 1 once per walk order, so the hero's footstep plays once per click (port `walker.walkTo`). Idle: `0x00410450` and the AI's idle decisions start action 0 (port: a walker that has just stopped walking or acting, checked once per game frame).

## Music

* **Start** (`0x00438E80`): a random track, rand() % 9.
* **Next track**: the main loop checks the music buffer (`0x00429A98`). When it has stopped, it picks rand() % 9; if that equals the current track `0x00A3E7D4`, it takes the next one (mod 9), then plays it (`0x0042A474`). The track plays once and is not looped.
* **«Музыка»** `0x00A3E73C` only sets the music buffer volume (`0x0042A420`): on −1000 (−10 dB), off −10000. The music keeps playing silently. Calls:
  * when a track starts (`0x0042A617`);
  * on the settings screen's music toggle and «Отмена» (`0x00438BD0`, `0x00438CDB`);
  * not on Escape, so a change there keeps its volume ([settings](konung_settings.md)).
* MCI CD-audio code (`0x00429982`–`0x00429A93`, device `0x00A4E3C8`, flag `0x00A3E708`) is unused with the installed files.

Port: `startAudio` (from `game.New`) and `soundSystem.update` (each `Menu.Update`). `setMusic` mirrors `0x0042A420`. The sound setting is read at every play.

## Port notes

* The game draws a frame every 60 ms ([menu](konung_menu.md)). `soundTick` runs once per such frame (updates · 5/18). It plays queued rule sounds, idle sounds and the ambient roll.
* Sound rolls use the audio system's own RNG, so the AI's random sequence is unaffected.
* All sounds are loaded up front, so the per-map loading delay is not reproduced.

## Dialogue greeting

The dialogue greeting (ids 12–15 at random without repeating, 16–19 for women and children, `0x004365EF`) is ported ([dialogues](konung_dialogue.md#opening-0x004364f0)); it is skipped, as in the game, when a shown line has speech.
