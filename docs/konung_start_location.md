# Dragomir's starting location and map data

## What is confirmed

The available [Konung walkthrough](https://shara-games.ru/knyaz/) identifies **Old Camp** as Dragomir's starting location. This supports the intended scene for the New Game menu action. The walkthrough does not connect Old Camp to a particular `KONUNG.N` filename.

`OBJECTS.RES` contains indexed, animated game objects. The first image of object entry **4** decodes as a red-clothed, human-shaped figure. It is used as a temporary person in the New Game scene. Its identity has not been established, so it must not be called Dragomir in asset catalogs. Object entry 5, initially considered as a person, decodes as a leafy figure and is not used for the human placeholder.

## Candidate map and uncertainty

The scene currently loads `assets/Konung/KONUNG.1` as a **provisional Old Camp map**. That filename-to-location mapping is unverified. A visual preview shows terrain and repeated diagonal structures, but no direct map label or executable reference has tied that file to Old Camp yet.

Observed facts for `KONUNG.1`:

* File size is 128,800 bytes.
* It divides evenly into seven 18,400-byte planes.
* Each plane contains 9,200 little-endian `uint16` values.
* `9,200 = 80 × 115`, so an 80-by-115 cell grid is a useful rendering hypothesis; the grid dimensions and each plane's meaning still need confirmation.
* Values include `0xFFFF`; the file also has high-byte values that are not valid tile indices if a whole word is treated as one index.
* The provisional renderer treats each word as two byte-sized tile references. This fits the observed byte range and the engine's 256-entry map addressing, but the packed-cell interpretation and ordering of the two references still need confirmation from executable code.
* Byte `0x00` is treated as empty. `0xFF` occurs only in planes 4–6 in this file; `0xFFFF` fills plane 6, so the renderer treats `0xFF` as empty there as well.

Do not record `KONUNG.1 = Old Camp` as a confirmed game-data fact until the map is matched using executable references, a labeled in-game screenshot, or a known save.

## `GRAPH.RES` terrain tile block

The useful portions of `GRAPH.RES` for the scene are:

* `0x00000..0x1FFFF`: 256 × 256 little-endian RGB555 palette words.
* `0x227E8`: an embedded Type 1 block with an 8-byte `(type, size)` header. This nested block does not use the 12-byte top-level header documented for standalone Type 1 archives.
* Immediately after the nested header: 512 little-endian `(offset, paletteByteOffset)` entries. The image data starts after this table.
* `paletteByteOffset / 512` selects one of the 256-color palettes in the initial palette table.
* The indexed image uses the width/height, per-row byte sizes, and RLE commands documented by the external [CTPAX-X GRAPH extractor source](https://www.ctpax-x.org/index.php?goto=files&show=147&view=sources/kngrpres.c). A high-bit command skips transparent pixels; otherwise the command gives the count of following palette indices. Image byte length is derived from the next entry's image offset, or the enclosing block end for the last image.
* The first 304 entries (0–303) decode as terrain images. Entry 304 has a valid index record but its image header reports `603 × 657`; it is not a map tile. The provisional renderer uses byte-sized map references, so it only addresses entries 0–255.

The nested block's recorded payload size is 1,880,262 bytes. These layout details were checked against the local file bytes and the extractor's source. They are separate from the leading Type 5 cursor sprites listed in [the GRAPH sprite catalog](konung_graph_sprites.md).

## `OBJECTS.RES` image path used by the placeholder

The external [CTPAX-X object extractor source](https://www.ctpax-x.org/index.php?goto=files&show=147&view=sources/knobjres.c) describes the Type 2 object block layout used here:

* `OBJECTS.RES` has a 12-byte outer header followed by a 512-entry table. For an entry, the second `uint32` is the offset used with base `0x1008`.
* Object entry 4 points to a Type 2 animation block. The block contains a 48 × 19 word header, 1,024 frame/shadow offsets at block offset `0x728`, and image bytes starting at block offset `0x3228`.
* Adjacent frame/shadow offsets are XOR-swapped in the stored table; the first image is then decoded with the indexed RLE format and palette 59 (palette assignment for the first 24 object entries is listed in the extractor source).
* Transparent object pixels use RGB555 magenta `0x7C1F`.

The specific palette assignment and offsets above describe the selected placeholder path, not a character identity. The live Ebitengine scene reads `GRAPH.RES`, `OBJECTS.RES`, and the candidate `KONUNG.1` directly; it does not depend on extracted image files.

## Follow-up to confirm the map

The next useful evidence is a direct file-to-location reference: find how `konung.exe` chooses the `KONUNG.N` filename for Dragomir's start, or compare a labeled Old Camp screenshot/save against renders of the numbered map files. Until then, keep the map assignment provisional.
