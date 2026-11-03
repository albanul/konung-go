# Konung sprite and map image index

This is the searchable index for image resources in the installed Konung data. The complete row-by-row inventory is [konung_sprite_index.tsv](data/konung_sprite_index.tsv). It is plain tab-separated text, so it can be searched with `rg`, `awk`, or a spreadsheet.

## Index namespaces

| Namespace | IDs | Contents |
|---|---|---|
| `GRAPH.T5` | 0-based | The 11 leading `GRAPH.RES` cursor and interface sprites. |
| `GRAPH.T1` | 0-based, 512 table slots | Indexed images in the embedded `GRAPH.RES` Type 1 block. IDs 0–303 decode as terrain images; ID 304 is a 603 × 657 image; IDs 305–511 are unused table slots. |
| `OBJECTS` | 1-based, 512 table slots | Objects in `OBJECTS.RES`: 493 used entries and 19 unused slots. Type 2 entries include animation frame counts; other supported block types are inventoried by type and first image dimensions. |
| `KONUNG.N` | 0–255 byte values | Map image references counted across the 43 installed `KONUNG.N` files. The packed-byte interpretation is still provisional. |
| `TYPE5` | 0-based per file | Every sequential Type 5 frame found in the installed frame resources. |

The index has 1,425 data rows: 11 GRAPH Type 5 sprites, 512 GRAPH table slots, 512 object table slots, 256 map byte values, and 134 sequential Type 5 frames.

## Useful lookups

Object entry 16 is the current small human-shaped placeholder. It is 27 × 48 pixels in its first frame, uses palette 159, and has 129 indexed animation frames. Its identity is unknown. Object entry 5 is the corpse-like humanoid that was mistakenly selected earlier; see [the Old Camp scene findings](konung_start_location.md).

```sh
# Find an object by one-based OBJECTS.RES ID
awk -F '\t' '$1 == "OBJECTS" && $3 == "16" { print }' docs/data/konung_sprite_index.tsv

# List map sprite references used by KONUNG.1
awk -F '\t' '$1 == "KONUNG.N" && $12 ~ /KONUNG.1/ { print $3, $11, $12 }' docs/data/konung_sprite_index.tsv

# List all FLAMES.RES frames and dimensions
awk -F '\t' '$1 == "TYPE5" && $2 == "FLAMES.RES" { print $3, $7 "x" $8 }' docs/data/konung_sprite_index.tsv
```

## Inventory notes

* The `OBJECTS.RES` visual notes for entries 1–30 are short descriptions of decoded first frames, not official character or creature names. The resource data does not provide those names. Entries 25–30 visually duplicate entry 1; the CTPAX-X extractor source also lists these entries as duplicates.
* `OBJECTS` width and height describe only the first decoded frame. Frame dimensions can change during an animation. `frame_count` comes from the even image offsets after correcting the stored image/shadow offset pairs.
* `GRAPH.T1` palette IDs are `paletteByteOffset / 512`, using the 256-color palettes at the start of `GRAPH.RES`. See [the GRAPH sprite and terrain notes](konung_graph_sprites.md) and the external [CTPAX-X GRAPH extractor source](https://www.ctpax-x.org/index.php?goto=files&show=147&view=sources/kngrpres.c).
* Map usage counts include all installed maps, not missing numbered files. The current directory has `KONUNG.1`–`.50` except `.9`, `.13`, `.18`, `.26`, `.31`, `.34`, and `.41`.
* Map values are counted as byte references in each 16-bit cell. This matches the visible byte range and 256-entry map addressing, but has not yet been confirmed from the game executable. `0x00` and `0xFF` are excluded from usage counts as empty markers in the provisional scene renderer.
* The per-frame Type 5 resources include `FLAMES.RES`, both menu sizes, both new-character screens, and both save screens. Their visible menu labels and flame animation groups are documented in [the menu frame catalog](konung_menu_res.md) and [the resource format notes](konung_res_format.md).

## External format references

* [CTPAX-X GRAPH.RES extractor source](https://www.ctpax-x.org/index.php?goto=files&show=147&view=sources/kngrpres.c)
* [CTPAX-X OBJECTS.RES extractor source](https://www.ctpax-x.org/index.php?goto=files&show=147&view=sources/knobjres.c)
