# Konung sprite and map image index

This is the searchable index for image resources in the installed Konung data. The complete row-by-row inventory is [konung_sprite_index.tsv](data/konung_sprite_index.tsv). It is plain tab-separated text, so it can be searched with `rg`, `awk`, or a spreadsheet.

For visual review, run `go run ./cmd/sprite-catalog` in the devcontainer. It extracts the indexed image resources and all frames from animated objects into `output/sprite-catalog/`, then creates a searchable HTML gallery and frame-level `catalog.tsv`. The gallery opens in the grouped `OBJECTS` view. Select `GRAPH.T0` to see one card per Type 0 block with animation frames under “Explore frames.” The installed resources currently produce 24,082 images. Type 0 previews use palette 2 provisionally; the game's palette assignment has not been confirmed. Each catalog row includes its resource name, entry ID, entry offset, exact compressed-frame offset, indexed byte count, decoded payload byte count, dimensions, and palette ID when applicable. Offsets are hexadecimal byte offsets from the start of the named resource file; `entry_offset` points to the index entry's containing block/header and `frame_offset` points to the compressed image data. Add semantic names and review notes to [konung_sprite_labels.tsv](data/konung_sprite_labels.tsv); use `*` in the `frame` column for an object-wide label or a zero-based frame number for one frame. Rerunning the command joins these durable labels into the gallery and generated TSV.

For this hand-drawn, low-resolution art, human visual review is the dependable way to assign meanings such as “wolf,” “guard,” or “door.” Automated image matching can help find duplicate frames and related animation poses, but should suggest candidates rather than create names. The gallery exposes stable resource IDs and frame numbers to make that review traceable.

## Index namespaces

| Namespace | IDs | Contents |
|---|---|---|
| `GRAPH.T5` | 0-based | The 11 leading `GRAPH.RES` cursor and interface sprites. |
| `GRAPH.T1` | 0-based, 512 table slots | Indexed images in the embedded `GRAPH.RES` Type 1 block. IDs 0–303 decode as terrain images; ID 304 is a 603 × 657 image; IDs 305–511 are unused table slots. |
| `OBJECTS` | 1-based, 512 table slots | Objects in `OBJECTS.RES`: 493 used entries and 19 unused slots. Type 2 entries include animation frame counts; other supported block types are inventoried by type and first image dimensions. |
| `KONUNG.N` | 0–255 byte values | Map image references counted across the 43 installed `KONUNG.N` files. The packed-byte interpretation is still provisional. |
| `TYPE5` | 0-based per file | Every sequential Type 5 frame found in the installed frame resources. |

## Coverage gap: prince and warrior animations

The 24,082 extracted images are not the complete set of game art. There are only 30 Type 2 animated entries in the `OBJECTS.RES` table. The newly decoded Type 0 blocks include human-like animation sets; blocks 0–2 are leading candidates for the missing warrior graphics, though their palette mapping and identities remain unverified. This fits the CTPAX-X extractor author's [statement that prince and warrior animations were not extracted](https://www.extractor.ru/ipb/index.php?showtopic=202).

The executable loader trace confirms `OBJECTS.RES` has a 512-slot table, with animated-object handling only for indices 0–29 and static-image handling thereafter. Our extracted split matches that behavior, so there is no evidence that the current frame parser skipped additional warrior entries in that table. After the first `GRAPH.RES` Type 1 block, a 64 KiB region remains unidentified, followed by 27 structurally validated Type 0 blocks. Their 14,067 animation frames now appear under `GRAPH.T0` in the gallery; raw blocks, offsets, and decode counts remain in `output/sprite-catalog/type0.tsv`. Palette 2 is a provisional preview, and the village-chief identity is not yet confirmed. See the [executable loader notes](konung_exe_resource_loaders.md) for current findings.

The index has 1,425 data rows: 11 GRAPH Type 5 sprites, 512 GRAPH table slots, 512 object table slots, 256 map byte values, and 134 sequential Type 5 frames.

## Useful lookups

The current scene combines object entry 15 (a robed human-shaped figure) with `GRAPH.T5` sprite 10 (a standalone sword) as a temporary sword-bearing fighter. Entry 15 is 30 × 74 pixels in its first frame and uses palette 152. Its identity and warrior role are not confirmed. Entry 16 is a child-sized figure; entry 5 is the corpse-like humanoid that was mistakenly selected earlier. See [the Old Camp scene findings](konung_start_location.md).

```sh
# Find an object by one-based OBJECTS.RES ID
awk -F '\t' '$1 == "OBJECTS" && $3 == "16" { print }' docs/data/konung_sprite_index.tsv

# List map sprite references used by KONUNG.1
awk -F '\t' '$1 == "KONUNG.N" && $12 ~ /KONUNG.1/ { print $3, $11, $12 }' docs/data/konung_sprite_index.tsv

# List all FLAMES.RES frames and dimensions
awk -F '\t' '$1 == "TYPE5" && $2 == "FLAMES.RES" { print $3, $7 "x" $8 }' docs/data/konung_sprite_index.tsv
```

## Inventory notes

* The `OBJECTS.RES` visual notes for entries 1–30 are short descriptions of decoded first frames, not official character or creature names. The resource data does not provide those names. Entries 25–30 are not just rendered duplicates: each table entry has a valid Type 2 tag and a declared block size matching its table size, and its entire block (metadata and frame data) is byte-for-byte identical to entry 1. Their 316 extracted frame payloads also match entry 1 exactly.
* Entry 24 contains 240 frames. Its first 48 frames do not match entry 9; frames 49–240 (one-based) match entry 9 frames 97–288 exactly. I verified the raw indexed frame payload bytes and dimensions as well as decoded pixels. Its frame offsets are monotonic and each frame range ends at the next image offset. These are genuine repeated resource contents, not duplicates introduced by palette conversion or image decoding. The ranges are recorded in [konung_sprite_relations.tsv](data/konung_sprite_relations.tsv) and appear on the corresponding gallery frames.
* `OBJECTS` width and height describe only the first decoded frame. Frame dimensions can change during an animation. `frame_count` comes from the even image offsets after correcting the stored image/shadow offset pairs.
* `GRAPH.T1` palette IDs are `paletteByteOffset / 512`, using the 256-color palettes at the start of `GRAPH.RES`. See [the GRAPH sprite and terrain notes](konung_graph_sprites.md) and the external [CTPAX-X GRAPH extractor source](https://www.ctpax-x.org/index.php?goto=files&show=147&view=sources/kngrpres.c).
* Map usage counts include all installed maps, not missing numbered files. The current directory has `KONUNG.1`–`.50` except `.9`, `.13`, `.18`, `.26`, `.31`, `.34`, and `.41`.
* Map values are counted as byte references in each 16-bit cell. This matches the visible byte range and 256-entry map addressing, but has not yet been confirmed from the game executable. `0x00` and `0xFF` are excluded from usage counts as empty markers in the provisional scene renderer.
* The per-frame Type 5 resources include `FLAMES.RES`, both menu sizes, both new-character screens, and both save screens. Their visible menu labels and flame animation groups are documented in [the menu frame catalog](konung_menu_res.md) and [the resource format notes](konung_res_format.md).

## External format references

* [CTPAX-X GRAPH.RES extractor source](https://www.ctpax-x.org/index.php?goto=files&show=147&view=sources/kngrpres.c)
* [CTPAX-X OBJECTS.RES extractor source](https://www.ctpax-x.org/index.php?goto=files&show=147&view=sources/knobjres.c)
