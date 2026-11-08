# Ebitengine Main Menu Notes

This document records the current Konung menu implementation and the asset findings that informed it. New game, Return to game, Save game, Load game, Settings and Exit work ([saves](konung_save_format.md), [settings](konung_settings.md)).

## Source layout

The executable entry point is under `cmd/konung/`; the implementation is an importable package under `internal/game/`:

| File | Responsibility |
|---|---|
| `cmd/konung/main.go` | Resolve default asset paths, initialize Ebitengine, set fullscreen/window title/cursor mode, start the game loop |
| `internal/game/menu.go` | Load menu frames, map button frames, handle keyboard/mouse input, draw the background/buttons/custom pointer |
| `internal/game/saves.go` | Load and save screens, writing and reading `KONUNG.SAV` / `KONUNG.SA<n>` |
| `internal/game/newhero.go` | Character-creation screen «Выбор героя» ([new game](konung_new_game.md)) |
| `internal/game/cursor.go` | Read Type 5 sprites from the mixed `GRAPH.RES` stream after its palette area |
| `internal/game/hero.go` | Controllable hero: idle/walk animations per facing, click-to-move path following |
| `internal/game/scene.go` | New Game scene: viewport-culled `KONUNG.N` terrain, decorations, shadow layer, depth-sorted objects, camera scrolling, `GAME.N` characters/creatures; `loadSceneAssets`, `sceneAssets.newGame` |
| `internal/mapdata/` | Engine-independent map/terrain decoding used by the scene and `cmd/map-render` |
| `internal/game/assets.go` | Find assets by searching upward from the process working directory and source file location |

The executable uses `MENU800R.RES` and `GRAPH.RES` from `assets/Konung/`. It decodes the resources in memory; it does not need extracted PNG files.

## Menu resource mapping

`MENU800R.RES` is loaded as a sequence of Type 5 frames using `internal/resfmt.LoadType5File`.

* Frame 0 is drawn as the full-screen background.
* Frame 2 is a small blue UI marker. It is not the mouse pointer, and the current menu does not draw it separately.
* The six menu button pairs begin at frame 3. For menu item `i` from 0 through 5, frame `3 + 2*i` is the normal state and frame `4 + 2*i` is the selected state. Thus the pairs are 3/4, 5/6, 7/8, 9/10, 11/12, and 13/14.
* Button positions come from the executable's layout table at DGROUP VA `0x0045B314`, verified by tracing the draw routine at VA `0x0042F8D8`. Each button has 32 bytes: the 640×480 rectangle `(x1, y1, x2, y2)`, then the 800×600 rectangle. The routine blits frame `3+2i` (normal) or `4+2i` (selected) with its top-left at `(x1, y1)`. The 800×600 rectangles' sizes match the button frames, except "Return to game", which is 300 px wide against a 245 px frame. Both menu loops hit-test the mouse against the same table (`0x004398C4(x, y, 0x0045B314)`), and the Go menu uses each rectangle as the hitbox.

| Button | 640×480 rect | 800×600 rect |
|---|---|---|
| 0 Вернуться в игру | 233,130–425,161 | 290,161–590,210 |
| 1 Сохранить игру | 231,164–404,199 | 288,205–504,250 |
| 2 Загрузить игру | 212,200–390,240 | 265,250–488,303 |
| 3 Новая игра | 224,241–352,281 | 280,303–440,354 |
| 4 Установки | 215,282–333,321 | 268,353–416,402 |
| 5 Выход | 222,327–300,371 | 276,409–366,464 |

* The original draws buttons 0 and 1 (Return to game, Save game) only while a game is running (`[0x00A3E728] != 0`); otherwise the draw loop starts at button 2. Both menu loops (in-game `0x00437982`, start menu `0x00438D64`) also discard hits below 2 in that state. «Загрузить игру» has no such gate: its click case opens the load screen whether or not saves exist. The start menu therefore shows Load, New game, Settings and Exit. The Go menu does the same (`Menu.firstItem`); after a game starts, Escape opens the menu with all six.
* Click switch (jump table `0x00437E50`, index = button): 0 `0x004379F8` return to game, 1 `0x00437A25` save screen, 2 `0x00437B54` load screen, 3 `0x00437B91` new game, 4 `0x00437DDD` settings, 5 `0x00437E3D` exit.
* `MENU<width>R.RES` is loaded by `0x0043BEF8`: frame 0 is drawn as the background, and frames 1–14 go to the frame-pointer table `0x006304B8`, slots `0x1F2`–`0x1FF`. These slots are reused by other screens.

The menu code checks for at least 15 frames. Frame 1 is currently unused. These observations apply to the current 800×600 menu layout; no 640×480 layout adaptation has been implemented here.

## Cursor selection and alignment

`GRAPH.RES` begins with 128 KiB of palette data and then a mixed stream of resource blocks. The first eleven Type 5 images are `32 × 32`; their complete offsets, sizes, and visual descriptions are listed in the [sprite catalog](konung_graph_sprites.md).

The plain sword pointer matching the in-game screenshot is Type 5 sprite 4 at file offset `0x20EF6`, payload size 557 bytes. Sprite 1 at `0x20665` is a related cursor image with a crossed-swords marker beside the blade, so it is not suitable as the general mouse pointer. This visual distinction is why the menu selects sprite index 4.

Ebitengine's system cursor is hidden. The menu draws the game sprite at the current Ebitengine cursor position with a `(-1, -1)` pixel offset, placing the blade tip close to the native pointer location. The sprite is displayed at its source `32 × 32` dimensions.

The nearby cursor variants include a red barred-circle marker, parchment/speech marker, flame marker, and hand marker. Their appearances are catalogued, but their exact in-game meanings have not been confirmed by tracing the executable's cursor-selection logic.

Selecting menu item 3 («Новая игра», New game) opens the [character-creation screen](konung_new_game.md); its «Ok» opens the chosen hero's start map (Волк `KONUNG.1`, Драгомир `KONUNG.4`, Михаил `KONUNG.7`). The scene draws terrain, flat decorations, a screen-sized shadow layer (all shadows unioned, then drawn as 50% black once), and static objects plus `GAME.N` characters and creatures, sorted by bottom edge. Only on-screen items are drawn. NPCs and creatures play their idle loop at 10 steps/s, each phase-offset. The original draws one frame, with one game tick per speed level, every 60 ms (`0x00A3E6F4` = `0x3C`, set at `0x0042B9C4`). The message loop `0x0042BD19` measures this with `timeGetTime` and posts `WM_USER` (`0x400`). That is 16.7 ticks/s; the port's game time runs at 15/s (one unit per 4 updates at 60 updates/s). When the hero dies, `GAMEOVER.FLM` plays as the death animation ends, then the menu returns, and the next «Новая игра» starts over at the creation screen ([combat: hero death](konung_combat.md#melee-flow-verified)).

The hero (Type 0 model and palette from the [creation screen](konung_new_game.md)) is controllable (`internal/game/hero.go`):

* **Left click** walks to the clicked spot, or to the nearest free cell when the spot is blocked. `mapdata.FindPath` runs A* on the fine grid's 8 neighbours, with costs in world pixels. It avoids walk-grid obstacles, NPC/creature cells and cells off the map. Clicking again while walking finishes the current step, then follows the new path.
* Like the original, the hero moves **one fine cell per walk cycle**. During a step he stays anchored on the cell he is leaving, and the walk frames' per-step anchors carry the body toward the next cell; at the end of the 12-step cycle he snaps to it. An earlier version also interpolated the position, which doubled the motion and made him jump back every cell. He faces each step via the grid's neighbour table, plays the walk loop (body type 7, with equipment) while moving and the idle loop (type 6) when standing, and is depth-sorted live among objects and NPCs. The camera follows a smoothed position.
* **C** toggles the camera following the hero (port key, the game has none); it is on by default. **F** / HUD button 4 is the game's «Все ко мне» (`0x004226C0`): return a held item, press the button, and set every living party member except the hero to `+2` = `0x60` (follow the leader). With Volk alone only the button feedback shows (`startScene.gatherOrder`). With follow off, the arrow keys or the mouse within 8 px of a screen edge scroll the camera. Letter keys are not used for scrolling because the game binds A, F, I, L, M, Q, S, T (key switch `0x00437272`).
* **T** toggles background tiling: an open window hides the world behind a tiled background ([settings](konung_settings.md#background-tiling-t)).
* A key help box (Go Regular, 11 px, `internal/game/help.go`) sits in the top-right corner of the game view, in Russian; **F1** hides it.
* **I** (or the HUD bag button) toggles the [character window](konung_character_window.md) with the bag strip; its arrows scroll the bag. Click an item to pick it up (the cursor changes), click a box or the bag to put it down.
* The hero has the game's selection ellipse under the feet: green, yellow at ≤ 800 HP, red at ≤ 400 ([interface](konung_interface_res.md)).
* **Left click** walks; **right click** walks and picks up what lies on that cell ([ground items](konung_ground_items.md)). Hovering an item on the ground shows «Что-то лежит на земле».
* Hovering an item (bag strip, character window) shows its description, and hovering a HUD button its tooltip, in the status line ([hover texts](konung_character_window.md#hover-texts-verified)).
* **~** shows the bag strip alone. In the bag, a right click uses or equips the item (the shovel enters dig mode), and a held item dropped on the map lands at the hero's feet ([character window](konung_character_window.md#right-click-and-the-quick-bag)).
* **Numpad + / −** (port: also `=` / `-`) set the game speed `0x00A3E764` to 0–2. Each drawn frame runs 1 + speed game ticks (`0x00437849`–`0x004378CC`); the extra ticks stop while the hero is dead. Numpad +/− (VK 0x6B/0x6D) are the game's keys (`0x00436FD2`, `0x00436FF0`). The port runs `startScene.step` 1 + speed times per update; messages and button presses count drawn frames.
* **Space** (or the HUD stance button) toggles the hero's combat stance; **1/2/3** (or the HUD weapon button, which cycles) select blade, axe/club and bow when that equipment slot is filled. Rules and drawn-weapon overlays: [GAME.N: weapons and combat stance](konung_game_format.md#weapons-and-combat-stance-verified).
* Walking speed comes from the walk tick rate: `walkTicksPerStep` = 4 gives 0.8 s per cell (about 72 px/s east–west, 40 px/s north–south). The original runs a tick every 60 ms (see above), so its 12-tick walk cycle would take 0.72 s; the port is about 10 % slower. NPCs and creatures wander inside their group's rectangle (the village group stays in rows 36–136, cols 24–50) using the game's target rule; see [GAME.N: NPC wandering](konung_game_format.md#npc-wandering-partly-traced). All characters reserve the cell they step into.

## Input behavior currently implemented

* **Escape** (VK 0x1B, `0x00437430`) is switched on the screen state `[0x00A3E6D8]` (jump table `0x0043756C`, states 0–6):
  * State 0, in the game (`0x00437447`): return a held item (`0x00429194(1)`). Then, with window `[0x00A3E740]` = 1, do nothing; with another window open, close it (`0x00A3E740` = 0, `0x00A3E7B4` = 0, redraw `0x004292E4`); with no window open, open the menu (state 7, `[0x00A3E728]` = 1, `[0x00A3E7F0]` = 0). Window 1 is the [dialogue](konung_dialogue.md), window 2 the quest log, window 3 the character window; the port does the same (`startScene.escape`).
  * State 1, the menu (`0x004374A5`): if a game is running, resume it (as «Вернуться в игру»); otherwise do nothing. Escape never quits; only «Выход» does.
  * State 2 (new game, `0x004374DB`): back to the menu with `[0x00A3E728]` = 0. States 4/5 (load/save screens, `0x004374FE`) and 6 (settings, `0x00437517`) return to the menu. The load, save, settings and new-game screens are ported (Escape returns to the menu; on the settings screen it restores the toggles; from the new-game screen the running game ends).
* Up/down arrows move the selection among the visible items and wrap around. Moving the mouse over a visible button selects it. `Enter` or a left click activates it.
* «Новая игра» opens the [character-creation screen](konung_new_game.md); «Ok» there starts a fresh game, «Отмена» or Escape returns to the menu without a running game. «Вернуться в игру» resumes the scene. «Выход» exits.
* «Сохранить игру» and «Загрузить игру» open the save and load screens ([saves](konung_save_format.md#save-and-load-screen)): hover selects a slot, a left click on a slot saves or loads it and goes to the game, a click elsewhere returns to the menu.
* «Установки» opens the [settings screen](konung_settings.md): clicks toggle the settings, «Ok» writes `KONUNG.CFG`, «Отмена» restores them.
* After the hero dies, the game counts as not running, so the menu goes back to its four start items.

## Asset path behavior

By default the menu looks for `assets/Konung/MENU800R.RES` and `assets/Konung/GRAPH.RES`. `findGameResource` walks upward from both the process working directory and the directory containing the source file. This accommodates IDEs such as Zed that may launch with a working directory below or outside the repository, as long as the source/repository path remains available.

The first command-line argument overrides the menu resource path only. `GRAPH.RES` is still resolved using `findGameResource`. If the resources cannot be found, startup exits with an error naming the missing path.

Typical devcontainer invocation:

```bash
podman exec -w /workspaces/konung1.5 <container_id> go run ./cmd/konung
```

The extracted implementation is in `internal/game`, so functions used by the entry point are imported instead of relying on sibling source files. From `cmd/konung`, either form is supported:

```bash
go build
go build main.go
```

The package form is the normal Go build. The single-file form also works because `main.go` imports the implementation package.

To use a different menu resource file:

```bash
podman exec -w /workspaces/konung1.5 <container_id> go run ./cmd/konung /path/to/MENU640R.RES
```

The second example only changes the source file; the current fixed 800×600 layout does not scale to 640×480.
