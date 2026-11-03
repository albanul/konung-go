# GRAPH.RES Sprite Notes

This is a catalog of the confirmed leading Type 5 sprites in `assets/Konung/GRAPH.RES`. Indices are zero-based within the Type 5 blocks following the palette area; they are not global indices for every resource block in the file.

## File layout relevant to sprites

* Bytes `0x00000..0x1FFFF` (128 KiB) are a `256 × 256` table of little-endian `uint16` palette colors.
* The first resource block begins at `0x20000`.
* The first 11 blocks are Type 5 images, each `32 × 32`.
* A Type 1 block begins at `0x227E8`. Stop scanning sprites when the next block tag is not Type 5.

## Confirmed sprites

| Type 5 index | Block offset | Payload size | Dimensions | Identification |
|---:|---:|---:|---:|---|
| 0 | `0x20000` | 1,629 bytes | `32 × 32` | Crate / container |
| 1 | `0x20665` | 763 bytes | `32 × 32` | Sword mouse pointer; blade points upper-left, hilt lower-right |
| 2 | `0x20964` | Not recorded | `32 × 32` | Crossed swords |

Payload sizes exclude each block's 8-byte header. The first payload size (1,629 bytes) follows from the next block offset: `0x20665 - 0x20000 - 8`. The index 2 block offset is recorded, but its payload size has not yet been verified.

## Related menu resources

Frame 2 in `MENU640R.RES` and `MENU800R.RES` is a small blue UI marker, not the sword pointer. Menu button pairs begin at frame 3. The Ebitengine menu loads Type 5 sprite 1 directly from `GRAPH.RES` in memory; it does not need an extracted image file.

The observed layout is cross-checked against the [Konung GRAPH.RES extractor source](https://www.ctpax-x.org/index.php?goto=files&show=147&view=sources/kngrpres.c), which processes a 256×256 palette before the resource blocks.
