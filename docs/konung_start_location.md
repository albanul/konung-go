# Dragomir's starting location and map data

## What is confirmed

The available [Konung walkthrough](https://shara-games.ru/knyaz/) identifies **Old Camp** as Dragomir's starting location. This supports the intended scene for the New Game menu action. The walkthrough does not connect Old Camp to a `KONUNG.N` filename; the exe's location table does (`KONUNG.4`).

Human-like `OBJECTS.RES` entries: 15 (robed figure, 30 × 74, palette 152), 16 (child-sized), and 5 (corpse-like); entry 4 is a plant-like monster. None is used by the current scene, which uses a `GRAPH.RES` Type 0 warrior.

## Candidate map and uncertainty

**Identified by the user: `KONUNG.1` is Volk's (Волк) initial location.** Dragomir is hero model 1, whose start map is `KONUNG.4` (model*3+1). The exe's location table names `KONUNG.1` «Родники» and `KONUNG.4` «Старый лагерь» (Old Camp), so the walkthrough's Old Camp is `KONUNG.4` ([location names](konung_map_format.md#location-names-verified)). See [GAME.N notes](konung_game_format.md#heroes-and-start-maps). Its terrain, decorations and static objects are now decoded and rendered from the map data; see [the map format notes](konung_map_format.md). The render shows a palisaded village (log houses, a smithy, a building site, a well) on a coast, which fits a starting camp but is not proof. Only the warrior is still a hand-placed preview.

Observed facts for `KONUNG.1`:

* File size is 128,800 bytes (`0x1F720`).
* The executable loader at VA `0x0043C0FC` reads sections of `0x6400`, `0xC800`, `0x3E80`, and `0x8CA0` bytes, which sum to the file size. Its `0x200` argument is an open flag, not a skipped header.
* The terrain is the **first** `0x6400`-byte section: 160 rows × 80 two-byte cells (base and overlay layers), each byte a 1-based `GRAPH.T1` tile index, drawn on a staggered 116 × 32 px grid. This is verified against the executable's renderer at VA `0x0042562F`. An earlier preview drew the `0xC800`-byte section at `0x6400` as terrain. That section is a 320 × 160 grid expanded into flag words (probably walkability), not terrain.
* Scanning the file as little-endian words finds values 87 twice, 88 once, and no 89 in `KONUNG.1`; the 88 reference is at file offset `0x16D4C` (87 references at `0x17EBC` and `0x18414`). These are raw references only: their record and coordinate interpretation is not established, so they cannot yet be positioned faithfully in the Go scene.

`KONUNG.1` is not Old Camp: the exe's location table names it «Родники».

## Village-chief screenshot clue

The supplied gameplay screenshot labels the selected speaker as a level-5 warrior and village elder (`староста`). In the image catalog, `OBJECTS.RES` entries 87 and 88 are wooden house images, while entry 89 is a dark shadow image; these are building art and cannot be the person. The executable's initializer at VA `0x00414078` confirms an in-game Type 0 model path based at `0x007145B4`, with `0x4D28` bytes per model block. Entity fields choose the model, variant, and direction/animation slot. The body is dyed by the entity's palette `+0xD8` ([exe notes](konung_exe_resource_loaders.md#map-entity-and-warrior-model-initialization)). The screenshot has not been tied to a specific entity or confirmed `KONUNG.N` map.

## `GRAPH.RES` terrain tile block

For the whole-file overview, see [GRAPH.RES layout](konung_graph_sprites.md#whole-file-layout-single-reference). Terrain-block details:

* `0x00000..0x1FFFF`: 256 × 256 little-endian RGB555 palette words.
* `0x227E8`: an embedded Type 1 block with an 8-byte `(type, size)` header. This nested block does not use the 12-byte top-level header documented for standalone Type 1 archives.
* Immediately after the nested header: 512 little-endian `(offset, paletteByteOffset)` entries. The image data starts after this table.
* `paletteByteOffset / 512` selects one of the 256-color palettes in the initial palette table.
* The indexed image uses the width/height, per-row byte sizes, and RLE commands documented by the external [CTPAX-X GRAPH extractor source](https://www.ctpax-x.org/index.php?goto=files&show=147&view=sources/kngrpres.c). A high-bit command skips transparent pixels; otherwise the command gives the count of following palette indices. Image byte length is derived from the next entry's image offset, or the enclosing block end for the last image.
* The first 304 entries (0–303) decode as terrain images. Entry 304 has a valid index record but its image header reports `603 × 657`; it is not a map tile. Map terrain bytes are 1-based, so they address slots 0–254 (verified, see [map format](konung_map_format.md)). The decoder is `mapdata.LoadTerrainTiles`.

The nested block's recorded payload size is 1,880,262 bytes. These layout details were checked against the local file bytes and the extractor's source. They are separate from the leading Type 5 cursor sprites listed in [the GRAPH sprite catalog](konung_graph_sprites.md).

## `OBJECTS.RES` image path and object lookup

The external [CTPAX-X object extractor source](https://www.ctpax-x.org/index.php?goto=files&show=147&view=sources/knobjres.c) describes the Type 2 object block layout used here:

* The object table occupies offsets `0x8..0x1007`; each 8-byte entry stores an object offset and size/boundary value. The first object offset is zero, so its first dword overlaps the generic header's reserved word. For object entry 15, the first dword of its table entry points to Type 2 block offset `0x00DD9BBE` after adding base `0x1008`.
* Type 2 blocks contain a 48 × 19 word header, 1,024 frame/shadow offsets at block offset `0x728`, and image bytes starting at block offset `0x3228`.
* Adjacent frame/shadow offsets are XOR-swapped in the stored table; the first image is decoded with the indexed RLE format. Palette assignments for the first 24 object entries are listed in the extractor source.
* Transparent object pixels use RGB555 magenta `0x7C1F`.

Entry 15 uses palette 152 and entry 16 uses palette 159. Entry IDs must use the table's first dword as the offset; shifting the table by four bytes changes which object is selected. Static entries (≥ 30) have a `0x100`-byte header whose dword at `+20` is the palette byte offset; image data follows the header (`resfmt.DecodeStaticObject`). The scene reads `GRAPH.RES` and `OBJECTS.RES` directly, without extracted image files.

## Remaining map and runtime work

The New Game scene renders `KONUNG.1` from map data with a scrollable camera: terrain, decorations, merged 50% shadows, and depth-sorted static objects. Characters and creatures come from `GAME.0` when Волк is chosen ([GAME.N notes](konung_game_format.md), [new game](konung_new_game.md)), and the camera starts on the player inside the village. The earlier hand-placed warrior and house previews are gone.
