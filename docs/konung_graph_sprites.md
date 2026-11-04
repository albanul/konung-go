# GRAPH.RES Layout and Sprite Notes

## Whole-file layout (single reference)

| File offset | Contents | Loaded to (VA) | Details |
|---|---|---|---|
| `0x000000` | 256 palettes × 256 RGB555 LE words (`0x20000` bytes); palette `p` is at `p*512` | — | below |
| `0x020000` | 11 Type 5 sprites (`32 × 32`): cursors and UI | — | [sprite table below](#leading-sprite-sequence) |
| `0x0227E8` | Type 1 terrain block: 8-byte `(BE tag, LE size)` header, 512 × `(offset, paletteByteOffset)` LE entries, then image data. Slots 0–303 are terrain tiles and slot 304 is a 603 × 657 image. Palette = `paletteByteOffset / 512`. | table → `0x006314B8`; image base pointer → `0x00A3E6BC` | [start location: terrain block](konung_start_location.md#graphres-terrain-tile-block); map bytes reference slot = byte − 1 per [map format](konung_map_format.md) |
| `0x1ED8B6` | 64 KiB, unidentified | — | [exe notes](konung_exe_resource_loaders.md) |
| `0x1EE8B6` | 27 Type 0 blocks (BE tag 0, LE size including header) through EOF `0xDD559B`: blocks 0–2 = character bodies, 3–26 = equipment overlays | bodies → `0x007145B4`, overlays → `0x006A09F4`, stride `0x4D28` | [exe notes](konung_exe_resource_loaders.md#map-entity-and-warrior-model-initialization) |

Indexed images (terrain, OBJECTS, Type 0) share one row-RLE format: `uint16 width, height`, `height × uint16` row byte sizes, then per row: a high-bit byte skips `n & 0x7F` transparent pixels, `0` ends the row, and otherwise `n` palette indices follow. Decoder: `resfmt.DecodeIndexedRLEFrame`. The executable's blit for these images is VA `0x0043E242`, which draws at the image's top-left corner.

The rest of this document catalogs the leading Type 5 sprites.

This catalog covers the first 11 Type 5 images in `assets/Konung/GRAPH.RES`. Indices are zero-based within the Type 5 sequence after the palette area; they are not global indices for every resource block in the file.

## File layout relevant to sprites

* Bytes `0x00000..0x1FFFF` (128 KiB) are a `256 × 256` table of little-endian `uint16` palette colors.
* The first resource block begins at `0x20000`.
* The first 11 blocks are Type 5 images, each `32 × 32`.
* The next block is Type 1 and begins at `0x227E8`. Stop scanning sprites when the next block tag is not Type 5.

To walk this sequence, seek to `0x20000`, read each 8-byte block header, interpret its 4-byte tag as big-endian (`5`) and its 4-byte payload length as little-endian, then read that many payload bytes. The next block begins at `current offset + 8 + payload length`. Decode a Type 5 payload with the Type 5 frame decoder documented in [the RES format notes](konung_res_format.md). The first non-Type-5 tag ends this sprite sequence; do not continue interpreting the mixed `GRAPH.RES` file as a flat Type 5 stream.

## Leading sprite sequence

Payload sizes exclude each block's 8-byte header.

| Index | Block offset | Payload bytes | Dimensions | Visual contents |
|---:|---:|---:|---:|---|
| 0 | `0x20000` | 1,629 | `32 × 32` | Crate / container |
| 1 | `0x20665` | 763 | `32 × 32` | Small sword cursor with a crossed-swords marker beside it |
| 2 | `0x20968` | 521 | `32 × 32` | Single brown/dark blade angled down-right |
| 3 | `0x20B79` | 885 | `32 × 32` | Small sword cursor with a red barred-circle marker |
| 4 | `0x20EF6` | 557 | `32 × 32` | Plain small sword cursor, blade upper-left and hilt lower-right |
| 5 | `0x2112B` | 1,127 | `32 × 32` | Small sword cursor with a parchment / speech marker |
| 6 | `0x2159A` | 883 | `32 × 32` | Small sword cursor with a flame marker |
| 7 | `0x21915` | 1,071 | `32 × 32` | Small sword cursor with a hand marker |
| 8 | `0x21D4C` | 1,374 | `32 × 32` | Globe with a hand marker |
| 9 | `0x222B2` | 854 | `32 × 32` | Blue starburst |
| 10 | `0x22610` | 464 | `32 × 32` | Larger standalone sword |

The screenshot's plain blade cursor visually matches sprite 4. Sprite 1 is a cursor variant with a crossed-swords marker. **The cursor is sprite number `0x00A3E790`** over the HUD and windows (`0x00428AC0`): 4 = empty hand; while an item is held in the character window 0 (armour), 2 (bow, arrows), 8 (herbs), 9 (jewellery, misc), 10 (melee weapons) ([character window](konung_character_window.md#moving-items)). The other markers' map semantics are not traced.

The sprite numbers, dimensions, payload lengths, offsets, and visible image contents above were checked against the resource bytes by decoding the leading Type 5 blocks. The runtime meaning of each pictured marker is still unknown. An external [Konung GRAPH.RES extractor source](https://www.ctpax-x.org/index.php?goto=files&show=147&view=sources/kngrpres.c) independently supports the 256×256 palette-before-blocks layout.

## Related menu resources

Frame 2 in `MENU640R.RES` and `MENU800R.RES` is a small blue UI marker, not the sword pointer. Menu button pairs begin at frame 3. The Ebitengine menu loads sprite 4 directly from `GRAPH.RES` in memory; it does not need an extracted image file.
