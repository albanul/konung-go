# Saved games: `KONUNG.SAV`, `KONUNG.SA0`–`KONUNG.SA9`

Status: the file layout, the save/load screens and the load path are verified against code (writer `0x00424E34`, reader `0x004249AC`). The port writes and reads the same layout (`gamestate.Game.MarshalSave`, `gamestate.ParseSave`). No original save files were available, so byte compatibility with saves the original game wrote is checked only through the traced layout. `Game.Marshal` reproduces `GAME.0`–`GAME.2` byte for byte (`gamestate.TestMarshalRoundTrip`). A save → load → save cycle gives identical bytes (`TestSaveRoundTrip`), and a full scene round trip works (`game.TestSaveAndLoadScene`).

The files sit in the game directory (`0x0083DE58`, the directory of `KONUNG.N`).

## `KONUNG.SAV`: slot names

500 bytes, loaded to `0x00674640` at startup (`0x0043A969`): 10 × 50-byte CP866 names, zero-terminated. Without the file, every slot is named «Пустая строка» (`0x0045070F`) and the selection `0x00A3E7AC` is −1. The writer rewrites the file before each slot file. Code: `gamestate.SlotNames`, `MarshalSlotNames`.

## `KONUNG.SA<n>`: one slot

The name is `"KONUNG.SA0"` with the slot added to the last character (`0x00424EE3`). The file is a raw dump of globals, written and read in this order (`gamestate.SaveSize` = `0xB07A4` = 722,852 bytes):

| File offset | Size | Global | Contents |
|---|---|---|---|
| `0x00` | 4 | `0x00A3E71C` | setting «Смена дня и ночи» ([settings](konung_settings.md)); 1 without a config file (`0x0042BB58`). `Game.Options[0]` |
| `0x04` | 4 | `0x00A3E734` | setting «Световые эффекты» (lit terrain path); default 1. `Game.Options[1]` |
| `0x08` | 4 | `0x00A3E770` | game time, ticks |
| `0x0C` | 4 | — | hero entity index, `(0x00A3E694 − 0x007AB378) / 256` |
| `0x10` | 4 | `0x00A3E7D8` | projectiles in flight |
| `0x14` | 4 | `0x00A3E7E0` | active piles on the current map |
| `0x18` | `0x4B0` | `0x00625068` | quest flags: 300 dwords, bit `0x80` set and tested by the dialogue code (`0x00435E38`), `uint16 +2` = the quest-log text. Initially the last `0x4B0` bytes of `QUESTR.RES` ([dialogues](konung_dialogue.md#questrres-layout-verified)). `Game.Quests` |
| `0x4C8` | `0xC80` | `0x0082C1F8` | projectiles, 100 × 32 bytes ([combat](konung_combat.md#bows-verified)). `gamestate.Arrow` |
| `0x1148` | `0x20000` | `0x0067AC34` | items (`GAME.N` section 1) |
| `0x21148` | `0x3200` | `0x007A6CDC` | active pile list, 200 × `0x40` (below) |
| `0x24348` | `0x1900` | `0x0069D1B4` | groups (section 2) |
| `0x25C48` | `0x7D00` | `0x0079EFDC` | pile records (section 3) |
| `0x2D948` | `0x7D000` | `0x007AB378` | entities (section 5) |
| `0xAA948` | `0x5E24` | `0x00835B18` | settlements (section 6) |
| `0xB076C` | `0x38` | `0x00A3E0A8` | 56 known-map flags (index map − 1) of the [map window](konung_world_map.md#known-maps-0x00a3e0a8): set by the map loader and by misc item type 0. `Game.KnownMaps` |

A save does **not** hold the map exits (section 4); the loader takes them from `GAME.<hero model>` (below).

**Active pile entry** (`0x40`): a copy of the section 3 record, except `+0`/`+4` hold the image's screen position (cell centre minus half the image size), `+0x0C` the INTR image slot and `+0x0E` the record index. Dropped items and loot that went into a new pile have index `0xFFFF`; they exist only here ([ground items](konung_ground_items.md#hover-and-pick-up)). `gamestate.ActivePile`.

## Save (`0x00437A25` → `0x00438A2F` → `0x00424E34`)

* Menu «Сохранить игру» (only while a game runs) builds the new save's name in `0x0083DF5C`. The name is the current map's location name (`0x0045B3F4 + (map − 1)·0x28`), a space, and C `ctime()` with the weekday, seconds and year cut off: «Родники Jun  5 21:49». `gamestate.SaveName`. There is no name editing.
* The save screen opens (state 5): `SAVE<width>R.RES` frame 1, «Сохранение игры». See [screen](#save-and-load-screen) below.
* A left click on a slot copies the name into the slot, writes both files and returns straight to the game (state 0). A click outside the slots returns to the menu.

## Load (`0x00437B54` → `0x00438966` → `0x004249AC`)

* Menu «Загрузить игру» opens the load screen (state 4, frame 0 «Загрузка игры»). It is offered with or without a running game, and whether or not saves exist.
* A left click on a slot runs the loader. On success: state 0 (game). If the slot file can't be opened (an empty slot), or the click was outside the slots: back to the menu.
* The loader:
  1. Closes windows and resets the UI state.
  2. Reads the header and points the hero at the stored index.
  3. Seeks to the hero's entity record (file offset `0x10 + 0x2D938 + hero·256`), copies it to `0x00A3E584`, then seeks back.
  4. Runs the `GAME.N` loader `0x0043BB6C` for `GAME.<hero +0xD6>`. That loads map exits, settlement flags and `QUESTR.RES`.
  5. Reads the rest of the save over them.
  6. Sets the party pointer and the current map from group 0's `+0x1B`.
  7. Runs the map loader in **restore mode**, `0x0043C0FC(1)`. Restore mode keeps the groups' alarm `+0x1D`, skips the respawn check, keeps the entities' `+2`, skips the group/settlement setup (`0x0043C878`), and does not rebuild the active pile list (`0x0043CF1A`).
  8. Marks the party's and the active piles' cells in the fine grid.
* Port: `sceneAssets.loadSaved`, then `buildScene(…, restore)`, which skips `Game.EnterMap` and the alarm reset and applies the clock, the arrows and the active piles (`startScene.restoreSaved`).

## Save and load screen

`SAVE<width>R.RES` (loader `0x0042D3DC`) has two 800×600 Type 5 frames, in frame slots `0x1FE` (load) and `0x1FF` (save). The draw routine `0x004314E0(save)` blits the background, then the 10 slot names. Each name sits at its slot's top-left. The table `0x0046203C` has 32 bytes per slot: 640×480 x1,y1,x2,y2, then 800×600. At 800×600, slot *i* is (175, y1)–(638, y2), with y1 = 86, 125, 164, 203, 241, 280, 319, 358, 396, 435.

The slot under the mouse (hit test `0x004398C4` on the same table) is selected. It is drawn in the large font `0x00592CFC`, the others in `0x00593040` (`resfmt.FontSlotVA`). In save mode the selected slot shows the new name instead of its own. Escape returns to the menu (`0x004374FE`). Port: `game.saveScreen`, `Menu.updateSaves`.

## Port notes

* Saving first writes the scene back into the game, as a map change does (`startScene.storeWorld`), plus the hero's live cell and facing.
* The port's runtime piles on maps other than the current one (it keeps them across map changes; the game does not) are not in the active list, so a save drops them.
* NPC runtime state that the port keeps outside the entity records is rebuilt from the records on load: walk paths, combat targets and intents.
* `Game.Quests` is read and written by the [dialogues](konung_dialogue.md) and listed by the quest log. `Game.KnownMaps` is set on every map load and drawn by the [map window](konung_world_map.md).

## Open questions

* The 64 dwords at `0x00A3E584` that the loader fills with the hero record: the new-game start uses the same buffer for the [hero template](konung_new_game.md#game-start-0x00438f3f) and `GAME.N` picks the model from it; any later use after a load was not traced.
