# Ebitengine Main Menu Notes

This document records the current Konung menu implementation and the asset findings that informed it. It is a rendering and input prototype; starting a new game, loading a save, and the other menu actions are not implemented yet.

## Source layout

The executable entry point is under `cmd/konung/`; the implementation is an importable package under `internal/konungmenu/`:

| File | Responsibility |
|---|---|
| `cmd/konung/main.go` | Resolve default asset paths, initialize Ebitengine, set fullscreen/window title/cursor mode, start the game loop |
| `internal/konungmenu/menu.go` | Load menu frames, map button frames, handle keyboard/mouse input, draw the background/buttons/custom pointer |
| `internal/konungmenu/graph.go` | Read Type 5 sprites from the mixed `GRAPH.RES` stream after its palette area |
| `internal/konungmenu/startscene.go` | New Game scene: viewport-culled `KONUNG.1` terrain, decorations, shadow layer, depth-sorted objects, camera scrolling, preview warrior |
| `internal/konungmap/` | Engine-independent map/terrain decoding used by the scene and `cmd/map-render` |
| `internal/konungmenu/resources.go` | Find assets by searching upward from the process working directory and source file location |

The executable uses `MENU800R.RES` and `GRAPH.RES` from `assets/Konung/`. It decodes the resources in memory; it does not need extracted PNG files.

## Menu resource mapping

`MENU800R.RES` is loaded as a sequence of Type 5 frames using `internal/resource.LoadType5File`.

* Frame 0 is drawn as the full-screen background.
* Frame 2 is a small blue UI marker. It is not the mouse pointer, and the current menu does not draw it separately.
* The six menu button pairs begin at frame 3. For menu item `i` from 0 through 5, frame `3 + 2*i` is the normal state and frame `4 + 2*i` is the selected state. Thus the pairs are 3/4, 5/6, 7/8, 9/10, 11/12, and 13/14.
* Button hitboxes use the normal frame's dimensions. Their left edge is `408 - width/2`; their top edge is `185 + 46*i`.

The menu code checks for at least 15 frames. Frame 1 is currently unused. These observations apply to the current 800×600 menu layout; no 640×480 layout adaptation has been implemented here.

## Cursor selection and alignment

`GRAPH.RES` begins with 128 KiB of palette data and then a mixed stream of resource blocks. The first eleven Type 5 images are `32 × 32`; their complete offsets, sizes, and visual descriptions are listed in the [sprite catalog](konung_graph_sprites.md).

The plain sword pointer matching the in-game screenshot is Type 5 sprite 4 at file offset `0x20EF6`, payload size 557 bytes. Sprite 1 at `0x20665` is a related cursor image with a crossed-swords marker beside the blade, so it is not suitable as the general mouse pointer. This visual distinction is why the menu selects sprite index 4.

Ebitengine's system cursor is hidden. The menu draws the game sprite at the current Ebitengine cursor position with a `(-1, -1)` pixel offset, placing the blade tip close to the native pointer location. The sprite is displayed at its source `32 × 32` dimensions.

The nearby cursor variants include a red barred-circle marker, parchment/speech marker, flame marker, and hand marker. Their appearances are catalogued, but their exact in-game meanings have not been confirmed by tracing the executable's cursor-selection logic.

Selecting menu item 4 («Новая игра», New game) opens the `KONUNG.1` scene. It draws the terrain, flat decorations, a screen-sized shadow layer (all shadows unioned, then drawn as 50% black once), and static objects sorted by bottom edge. Everything comes from map data through `internal/konungmap`, and only on-screen items are drawn. Arrow keys, WASD, or holding the mouse within 8 px of a screen edge scroll the camera, which is clamped to the map extents. A `GRAPH.RES` Type 0 block 0 warrior (palette 2, native size) is hand-placed beside the well and depth-sorted with the objects. Characters/NPCs are not decoded. `Escape` or `Backspace` returns to the menu, and the sword cursor still tracks the pointer.

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

The extracted implementation is in `internal/konungmenu`, so functions used by the entry point are imported instead of relying on sibling source files. From `cmd/konung`, either form is supported:

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
