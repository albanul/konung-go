# Dragomir's starting location and map data

## What is confirmed

The available [Konung walkthrough](https://shara-games.ru/knyaz/) identifies **Old Camp** as Dragomir's starting location. This supports the intended scene for the New Game menu action. The walkthrough does not connect Old Camp to a particular `KONUNG.N` filename.

`OBJECTS.RES` entry **15** decodes as a robed human-shaped figure (30 × 74 pixels, palette 152). The current scene pairs it with `GRAPH.RES` Type 5 sprite **10**, a standalone sword, as a temporary sword-bearing fighter. This is a visual approximation; the character's identity and warrior role are not established by the resource data. Entry 16 is a child-sized person, entry 4 is a plant-like monster, and entry 5 is the corpse-like humanoid that was mistakenly selected in an earlier build.

## Candidate map and uncertainty

The scene currently loads `assets/Konung/KONUNG.1` as a **provisional Old Camp map**. That filename-to-location mapping is unverified. A visual preview shows terrain and repeated diagonal structures, but no direct map label or executable reference has tied that file to Old Camp yet.

Observed facts for `KONUNG.1`:

* File size is 128,800 bytes.
* File size is 128,800 bytes (`0x1F6A0`).
* The executable loader at VA `0x0043C0FC` skips `0x200` bytes and reads sections of `0x6400`, `0xC800`, `0x3E80`, and `0x8CA0` bytes. These sections sum to the map file's full size. It processes 160 records of `0xA0` bytes from the first section and later initializes a 200-entry array of `0x20`-byte runtime records.
* The earlier seven-plane / 80-by-115 interpretation used by the Go preview is provisional and does not match the executable's read layout. Its tile placement and projection should not be treated as the game's map renderer.
* Scanning the file as little-endian words finds values 87 twice, 88 once, and no 89 in `KONUNG.1`; the 88 reference is at file offset `0x16D4C` (87 references at `0x17EBC` and `0x18414`). These are raw references only: their record and coordinate interpretation is not established, so they cannot yet be positioned faithfully in the Go scene.

Do not record `KONUNG.1 = Old Camp` as a confirmed game-data fact until the map is matched using executable references, a labeled in-game screenshot, or a known save.

## Village-chief screenshot clue

The supplied gameplay screenshot labels the selected speaker as a level-5 warrior and village elder (`староста`). In the image catalog, `OBJECTS.RES` entries 87 and 88 are wooden house images, while entry 89 is a dark shadow image; these are building art and cannot be the person. The executable's initializer at VA `0x00414078` confirms an in-game Type 0 model path based at `0x007145B4`, with `0x4D28` bytes per model block. Entity fields choose the model, variant, and direction/animation slot. The palette/dye operation remains untraced, and the screenshot has not been tied to a specific Type 0 block or confirmed `KONUNG.N` map.

## `GRAPH.RES` terrain tile block

The useful portions of `GRAPH.RES` for the scene are:

* `0x00000..0x1FFFF`: 256 × 256 little-endian RGB555 palette words.
* `0x227E8`: an embedded Type 1 block with an 8-byte `(type, size)` header. This nested block does not use the 12-byte top-level header documented for standalone Type 1 archives.
* Immediately after the nested header: 512 little-endian `(offset, paletteByteOffset)` entries. The image data starts after this table.
* `paletteByteOffset / 512` selects one of the 256-color palettes in the initial palette table.
* The indexed image uses the width/height, per-row byte sizes, and RLE commands documented by the external [CTPAX-X GRAPH extractor source](https://www.ctpax-x.org/index.php?goto=files&show=147&view=sources/kngrpres.c). A high-bit command skips transparent pixels; otherwise the command gives the count of following palette indices. Image byte length is derived from the next entry's image offset, or the enclosing block end for the last image.
* The first 304 entries (0–303) decode as terrain images. Entry 304 has a valid index record but its image header reports `603 × 657`; it is not a map tile. The provisional renderer uses byte-sized map references, so it only addresses entries 0–255.

The nested block's recorded payload size is 1,880,262 bytes. These layout details were checked against the local file bytes and the extractor's source. They are separate from the leading Type 5 cursor sprites listed in [the GRAPH sprite catalog](konung_graph_sprites.md).

## `OBJECTS.RES` image path and object lookup

The external [CTPAX-X object extractor source](https://www.ctpax-x.org/index.php?goto=files&show=147&view=sources/knobjres.c) describes the Type 2 object block layout used here:

* The object table occupies offsets `0x8..0x1007`; each 8-byte entry stores an object offset and size/boundary value. The first object offset is zero, so its first dword overlaps the generic header's reserved word. For object entry 15, the first dword of its table entry points to Type 2 block offset `0x00DD9BBE` after adding base `0x1008`.
* Type 2 blocks contain a 48 × 19 word header, 1,024 frame/shadow offsets at block offset `0x728`, and image bytes starting at block offset `0x3228`.
* Adjacent frame/shadow offsets are XOR-swapped in the stored table; the first image is decoded with the indexed RLE format. Palette assignments for the first 24 object entries are listed in the extractor source.
* Transparent object pixels use RGB555 magenta `0x7C1F`.

The current scene uses entry **15**, palette **152**, and sword sprite `GRAPH.T5:10`. Object entry **16** uses palette **159** and is a child-sized figure; object entry **5** is the corpse-like sprite. Entry IDs must use the table's first dword as the offset; shifting the table by four bytes changes which object is selected.

The specific palette assignment and offsets above describe the selected placeholder path, not a character identity. The live Ebitengine scene reads `GRAPH.RES`, `OBJECTS.RES`, and the candidate `KONUNG.1` directly; it does not depend on extracted image files.

## Remaining map and runtime work

The Go start scene still needs the executable's record parser, coordinate projection, object draw order, and Type 0 palette selection before it can reproduce a real location. The data loader and Type 0 model-selection path are documented in [the executable loader notes](konung_exe_resource_loaders.md). Keep `KONUNG.1 = Old Camp` provisional until the numbered map is tied to a labeled screenshot, save, or game location reference.
