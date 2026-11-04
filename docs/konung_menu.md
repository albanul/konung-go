# Ebitengine Main Menu Notes

This document records the current Konung menu implementation and the asset findings that informed it. It is a rendering and input prototype; starting a new game, loading a save, and the other menu actions are not implemented yet.

## Source layout

The executable entry point is under `cmd/konung/`; the implementation is an importable package under `internal/game/`:

| File | Responsibility |
|---|---|
| `cmd/konung/main.go` | Resolve default asset paths, initialize Ebitengine, set fullscreen/window title/cursor mode, start the game loop |
| `internal/game/menu.go` | Load menu frames, map button frames, handle keyboard/mouse input, draw the background/buttons/custom pointer |
| `internal/game/cursor.go` | Read Type 5 sprites from the mixed `GRAPH.RES` stream after its palette area |
| `internal/game/hero.go` | Controllable hero: idle/walk animations per facing, click-to-move path following |
| `internal/game/scene.go` | New Game scene: viewport-culled `KONUNG.1` terrain, decorations, shadow layer, depth-sorted objects, camera scrolling, GAME.0 characters/creatures |
| `internal/mapdata/` | Engine-independent map/terrain decoding used by the scene and `cmd/map-render` |
| `internal/game/assets.go` | Find assets by searching upward from the process working directory and source file location |

The executable uses `MENU800R.RES` and `GRAPH.RES` from `assets/Konung/`. It decodes the resources in memory; it does not need extracted PNG files.

## Menu resource mapping

`MENU800R.RES` is loaded as a sequence of Type 5 frames using `internal/resfmt.LoadType5File`.

* Frame 0 is drawn as the full-screen background.
* Frame 2 is a small blue UI marker. It is not the mouse pointer, and the current menu does not draw it separately.
* The six menu button pairs begin at frame 3. For menu item `i` from 0 through 5, frame `3 + 2*i` is the normal state and frame `4 + 2*i` is the selected state. Thus the pairs are 3/4, 5/6, 7/8, 9/10, 11/12, and 13/14.
* Button positions come from the executable's layout table at DGROUP VA `0x0045B314`, verified by tracing the draw routine at VA `0x0042F8D8`. Each button has 32 bytes: the 640×480 rectangle `(x1, y1, x2, y2)`, then the 800×600 rectangle. The routine blits frame `3+2i` (normal) or `4+2i` (selected) with its top-left at `(x1, y1)`. The 800×600 rectangles' sizes match the button frames, except "Return to game", which is 300 px wide against a 245 px frame. The Go menu uses each rectangle as the hitbox; that the game hit-tests with the same rectangle is inferred, not traced.

| Button | 640×480 rect | 800×600 rect |
|---|---|---|
| 0 Вернуться в игру | 233,130–425,161 | 290,161–590,210 |
| 1 Сохранить игру | 231,164–404,199 | 288,205–504,250 |
| 2 Загрузить игру | 212,200–390,240 | 265,250–488,303 |
| 3 Новая игра | 224,241–352,281 | 280,303–440,354 |
| 4 Установки | 215,282–333,321 | 268,353–416,402 |
| 5 Выход | 222,327–300,371 | 276,409–366,464 |

* The original draws buttons 0 and 1 (Return to game, Save game) only while a game is running (`[0x00A3E728] != 0`); otherwise the loop starts at button 2. The Go menu currently still shows all six.
* `MENU<width>R.RES` is loaded by `0x0043BEF8`: frame 0 is drawn as the background, and frames 1–14 go to the frame-pointer table `0x006304B8`, slots `0x1F2`–`0x1FF`. These slots are reused by other screens.

The menu code checks for at least 15 frames. Frame 1 is currently unused. These observations apply to the current 800×600 menu layout; no 640×480 layout adaptation has been implemented here.

## Cursor selection and alignment

`GRAPH.RES` begins with 128 KiB of palette data and then a mixed stream of resource blocks. The first eleven Type 5 images are `32 × 32`; their complete offsets, sizes, and visual descriptions are listed in the [sprite catalog](konung_graph_sprites.md).

The plain sword pointer matching the in-game screenshot is Type 5 sprite 4 at file offset `0x20EF6`, payload size 557 bytes. Sprite 1 at `0x20665` is a related cursor image with a crossed-swords marker beside the blade, so it is not suitable as the general mouse pointer. This visual distinction is why the menu selects sprite index 4.

Ebitengine's system cursor is hidden. The menu draws the game sprite at the current Ebitengine cursor position with a `(-1, -1)` pixel offset, placing the blade tip close to the native pointer location. The sprite is displayed at its source `32 × 32` dimensions.

The nearby cursor variants include a red barred-circle marker, parchment/speech marker, flame marker, and hand marker. Their appearances are catalogued, but their exact in-game meanings have not been confirmed by tracing the executable's cursor-selection logic.

Selecting menu item 4 («Новая игра», New game) opens the `KONUNG.1` scene. It draws terrain, flat decorations, a screen-sized shadow layer (all shadows unioned, then drawn as 50% black once), and static objects plus `GAME.0` characters and creatures, sorted by bottom edge. Only on-screen items are drawn. NPCs and creatures play their idle loop at 10 steps/s, each phase-offset; the original tick rate is not traced.

The player, Volk (Type 0 model 0, placeholder palette), is controllable (`internal/game/hero.go`):

* **Left click** walks to the clicked spot, or to the nearest free cell when the spot is blocked. `mapdata.FindPath` runs A* on the fine grid's 8 neighbours, with costs in world pixels. It avoids walk-grid obstacles, NPC/creature cells and cells off the map. Clicking again while walking finishes the current step, then follows the new path.
* Like the original, the hero moves **one fine cell per walk cycle**. During a step he stays anchored on the cell he is leaving, and the walk frames' per-step anchors carry the body toward the next cell; at the end of the 12-step cycle he snaps to it. An earlier version also interpolated the position, which doubled the motion and made him jump back every cell. He faces each step via the grid's neighbour table, plays the walk loop (body type 7, with equipment) while moving and the idle loop (type 6) when standing, and is depth-sorted live among objects and NPCs. The camera follows a smoothed position.
* **F** toggles the camera following the hero; it is on by default. With follow off, the arrow keys, WASD or the mouse within 8 px of a screen edge scroll the camera. A hint line at the bottom shows the controls.
* Walking speed comes from the walk tick rate: `walkTicksPerStep` = 4 gives 0.8 s per cell (about 72 px/s east–west, 40 px/s north–south). The original tick rate is not traced. NPCs and creatures wander inside their group's rectangle (the village group stays in rows 36–136, cols 24–50) using the game's target rule; see [GAME.N: NPC wandering](konung_game_format.md#npc-wandering-partly-traced). All characters reserve the cell they step into.

## Input behavior currently implemented

* `Escape` exits the prototype.
* Up/down arrows move the selected menu item and wrap around at either end.
* Moving the mouse over a button selects it.
* `Enter` exits only when the last item is selected. Clicking exits only when the last item is under the pointer.
* Menu item 4 opens the scrollable `KONUNG.1` scene. While it is open, the arrow keys scroll the map instead of changing the menu selection. The other non-exit items still have visual states only.

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
