# Konung Reverse Engineering Project Guide

## Project

This repository reimplements parts of the 1999 game «Князь: Легенды лесной страны» (Konung: Legends of the North) in Go/Ebitengine and provides tools for analyzing its Windows x86 executable (`assets/Konung/konung.exe`) and resource files (`assets/Konung/*.RES`, `KONUNG.N`, `LIGHTS.N`).

Use the existing findings as a starting point. Before reverse engineering anything, check the task map below and the referenced document; afterwards, record verified findings there so the work is not repeated.

## Working loop (fast path)

1. **Orient (≤ 5 min).** Find your row in the task map. Read only that doc plus the open questions. Don't re-derive anything a doc marks as verified.
2. **Clarify scope** when a request could mean "faithful port" or "visual preview". Those differ by hours of RE work, so ask.
3. **Trace, don't guess.** Find the loader (file-read buffer) → consumer (code that reads the buffer) → draw/parse routine. Record VAs as you go.
4. **Prototype engine-free.** Decode in `internal/resource` or `internal/konungmap`, render to PNG with a small `cmd/` tool or a temporary test, and look at the image. Ebitengine can't run in the container (no display).
5. **Integrate** into the Ebitengine package as a thin consumer.
6. **Done means:**
   * `go build ./...`, `go vet` and `go test ./...` pass in the container.
   * A new decoder has an asset-backed test that skips when assets are missing.
   * Docs are updated: the topic doc, plus the task map, resource overview, code map, open questions, and the [exe lookup table](docs/konung_exe_resource_loaders.md#known-routines-and-buffers) for new VAs.
   * Superseded claims are removed.
   * Temporary files are deleted.
   * Nothing is committed unless the user asks.

## Task map: where to start

| If you are working on… | Read first | Code |
|---|---|---|
| A resource file you haven't seen before | [Resource file overview](#resource-file-overview) below, then [RES formats](docs/konung_res_format.md) | `internal/resource/` |
| Map terrain, decorations, static objects, shadows, camera/projection | [Map format](docs/konung_map_format.md) | `internal/konungmap/`, `cmd/map-render/` |
| The New Game scene (map terrain/objects + GAME.0 characters) | [Menu & scene implementation](docs/konung_menu.md), [Start location](docs/konung_start_location.md) | `internal/konungmenu/startscene.go` |
| Main menu, buttons, cursor | [Menu implementation](docs/konung_menu.md), [Menu RES catalog](docs/konung_menu_res.md) | `internal/konungmenu/menu.go`, `graph.go` |
| `GRAPH.RES` (palettes, cursors, terrain tiles, Type 0 character models) | [GRAPH.RES layout and sprites](docs/konung_graph_sprites.md) | `konungmap.LoadTerrainTiles`, `resource.DecodeType0Frames`, `konungmenu/graph.go` |
| `OBJECTS.RES` (buildings, animated objects) | [RES formats: OBJECTS](docs/konung_res_format.md#objectsres), [Start location: OBJECTS path](docs/konung_start_location.md#objectsres-image-path-and-object-lookup), [Sprite index](docs/konung_sprite_index.md) | `resource.DecodeStaticObject` |
| Characters, NPCs, monsters, player start (`GAME.N`) | [GAME.N format](docs/konung_game_format.md) | `internal/konunggame/`, `resource.LoadEntitySprite` |
| Finding a sprite by appearance or ID | [Sprite index](docs/konung_sprite_index.md) + `docs/data/*.tsv` | `cmd/sprite-catalog/` |
| Tracing code in `konung.exe` | [Executable loader notes](docs/konung_exe_resource_loaders.md), [RE workflow](#reverse-engineering-workflow) below | — |

## Resource file overview

One line per file family. Details live in the linked documents; "verified" means confirmed against both executable code and file bytes.

| File | Layout summary | Status | Doc |
|---|---|---|---|
| `FLAMES.RES`, `MENU*R.RES`, `NEWH*R.RES`, `SAVE*R.RES` | Sequential Type 5 frames (BE tag 5, LE size), RGB555 row-RLE | verified | [res format](docs/konung_res_format.md), [menu RES](docs/konung_menu_res.md) |
| `GRAPH.RES` | 256 palettes → 11 Type 5 cursors → Type 1 terrain tile block (512-slot table) → 64 KiB gap → 27 Type 0 blocks (3 bodies + 24 equipment overlays) | verified except gap and Type 0 palette/dye | [GRAPH layout](docs/konung_graph_sprites.md) |
| `OBJECTS.RES` | 512-slot table; slots 0–29 animated (Type 2), others static | verified | [start location](docs/konung_start_location.md), [exe loaders](docs/konung_exe_resource_loaders.md) |
| `KONUNG.N` | 4 sections: terrain (160×80×2 tile bytes), 320×160 flag grid, flat decorations (16-byte records), static object placements (36-byte records) | verified except the flag grid | [map format](docs/konung_map_format.md) |
| `GAME.N` | Start state: 6 sections incl. 200 entity groups (group 0 = player) and 2000 × 256-byte entities | groups/entities/sprites verified; spawn placement approximate; 4 sections open | [GAME.N](docs/konung_game_format.md) |
| `LIGHTS.N` | Read by the map loader; used by an alternate terrain draw path | open | [map format](docs/konung_map_format.md) |
| `SOUNDS.RES` | Type 1 header + 1000 `(offset,size)` entries, PCM | verified via CTPAX-X + bytes | [res format](docs/konung_res_format.md#soundsres--sounds_res) |

## Code map

| Package / command | Responsibility |
|---|---|
| `internal/resource` | Engine-independent decoders: Type 5 RGB555 RLE, indexed RLE, RGB555 palettes, static OBJECTS entries and map objects with shadows (`LoadMapObject`), entity frames (`LoadEntitySprite`), equipment overlays (`ComposeEquipment`, exe tables via `ExeTables`), `GRAPH.RES` Type 0 frames, sound entries |
| `internal/konungmap` | Engine-independent `KONUNG.N` parsing (terrain, decorations, object placements), `GRAPH.RES` terrain tiles, projection, depth order, full-scene render |
| `internal/konunggame` | Engine-independent `GAME.N` parsing: groups, entities, world positions, spawn placement, entity sprites |
| `internal/konungmenu` | Ebitengine menu and New Game scene (camera, drawing); converts decoded `image.RGBA` to `ebiten.Image` |
| `cmd/konung` | Game entry point |
| `cmd/map-render` | `KONUNG.N` → PNG (terrain, decorations, shadows, objects, optional `GAME.N` characters via `-game assets/Konung/GAME.0`, `-terrain` for terrain only, `-all` for every map + `index.html`) |
| `cmd/sprite-catalog` | Extract all indexed images into an HTML gallery under `output/site/sprites/` |
| `internal/sitenav` | Shared nav bar and landing page (`output/site/index.html`) that join the tools' HTML outputs into one site |
| `cmd/res-parser` | Extract Type 5 frames from a RES file to PNG |

Put new format decoders in an engine-independent package (`internal/resource` or a sibling such as `internal/konungmap`) with a test against the installed assets, and keep Ebitengine code a thin consumer. This keeps tools and tests runnable without a display.

## Environment and commands

The project is developed in a Fedora Podman devcontainer. The workspace is mounted at `/workspaces/konung1.5`. Go, Python 3 (without Pillow), and GNU `objdump` are available only inside the container; the host has no `objdump`.

```bash
podman ps                                   # find <container>
C="podman exec -w /workspaces/konung1.5 <container>"
$C go build ./... && $C go test ./...      # asset-backed tests skip if assets are missing
$C go run ./cmd/map-render assets/Konung/KONUNG.1 output/maps/konung1.png
$C go run ./cmd/map-render -all output/site/maps   # every KONUNG.N + index.html
$C go run ./cmd/sprite-catalog              # -> output/site/sprites; open output/site/index.html for both
$C go run cmd/res-parser/main.go <resource.RES> <output_directory>
$C go run ./cmd/konung                      # needs a display; run on the host if the container has none
```

Files that the container writes under `output/` belong to the container user and may not be deletable from the host. Remove them with `$C rm`. Viewing a rendered PNG is the fastest way to validate a graphics hypothesis.

## Reverse-engineering workflow

```bash
$C sh -c 'objdump -D -Mintel -j BEGTEXT assets/Konung/konung.exe > /tmp/kn/text.asm'   # mkdir -p /tmp/kn first
$C grep -n '^  43c0fc:' /tmp/kn/text.asm          # jump to a known VA (lowercase, two-space indent)
$C grep -n '0x627fd8' /tmp/kn/text.asm            # find every reference to a global buffer
$C sed -n '53935,54260p' /tmp/kn/text.asm | awk -F'\t' '{print $1, $3}'   # address + mnemonic only
```

* Code is in `BEGTEXT` (VA `0x410000`), data in `DGROUP` (VA `0x450000`). See [the exe notes](docs/konung_exe_resource_loaders.md) for PE offsets.
* The file-read helper is `call 0x44023e(handle, buffer, size)`. Following its buffer argument from a loader to the code that reads that buffer is the fastest way to find a format's consumer. Known global buffers and routines are listed in [the exe notes](docs/konung_exe_resource_loaders.md#known-routines-and-buffers).
* Functions start with `push ebx/esi/edi/ebp; mov ebp,esp`; locals are `[ebp-N]`. Compiler identity not verified.

## Reverse-engineering rules

* The game is a 32-bit Windows x86 application. Treat multi-byte resource values as little-endian unless the field is specifically documented otherwise. Some block tags are stored big-endian; follow the per-format specification.
* Do not guess an unknown format. Inspect references in `konung.exe`, trace the read/parse call sites, and validate the interpretation against the resource bytes.
* When recording a finding, distinguish observed facts from hypotheses. Include file name, offset, size, indexing convention (0- vs 1-based), and a reproducible validation method. Remove or mark superseded statements; don't leave a stale claim next to the new one.
* Keep the `KONUNG.N` filename-to-location mapping provisional unless verified (`KONUNG.1 = Old Camp` is unconfirmed). `KONUNG.1` is Volk's initial location (identified by the user). In the New Game scene everything is data-derived: map content from `KONUNG.1`, characters from `GAME.0`. Approximations: spawn-group member positions, Volk's palette (placeholder 72), animation speed. `GAME.N` N = hero model; start map = model*3+1 (Volk 0 → `KONUNG.1`, Dragomir 1 → `KONUNG.4`, Michail 2 → `KONUNG.7`). The New Game scene intentionally starts as Volk on `KONUNG.1`.
* Konung supports both 15-bit RGB 555 and 16-bit RGB 565 paths. Verify masks and conversion behavior before assigning a pixel format. All decoded assets so far use RGB555 with magenta `0x7C1F` as the transparent key.
* The cursor is `GRAPH.RES` Type 5 sprite 4 (plain blade); sprite 1 includes a crossed-swords marker.

## Documentation conventions

* Keep this file focused on project-wide instructions and the indexes above. Detailed specs, offsets, inventories, and investigation notes belong in `docs/`.
* One topic per document. When a new resource or subsystem is understood, add a doc, a row to the task map and resource overview, and a line in [open questions](#open-questions).
* Write docs in English. Lead with the spec (tables, offsets, decoder names) and keep investigation history to a short evidence line or a pitfall note.
* Each doc's code references must name the current function or package; update them when code moves.

## Open questions

Highest-value unknowns, roughly in priority order:

1. Character-creation screen: hero palette (`0x00A3E65C`) and template fields; walking/fighting animations and the combat overlay path; exact runtime spawn positions. See [GAME.N](docs/konung_game_format.md#open-questions).
2. Type 0 model palette/dye selection and the model ID → character mapping. See [exe notes](docs/konung_exe_resource_loaders.md).
3. `KONUNG.N` section 1 (320×160 flag grid): is it walkability?
4. `LIGHTS.N` format and the lit terrain draw path at VA `0x004257A9`.
5. The 64 KiB gap in `GRAPH.RES` at `0x1ED8B6`, and the second 8-byte table at VA `0x006324B8`.
6. `KONUNG.N` ↔ in-game location names (`KONUNG.1` = Volk's start is known). Render all maps with `map-render -all` and compare them against screenshots or the walkthrough.
7. `cmd/sprite-catalog` has its own Type 0 walker that duplicates `resource.DecodeType0Frames`; consolidate when touching it.
