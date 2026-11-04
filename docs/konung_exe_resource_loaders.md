# Konung executable resource-loader notes

These are initial, observed findings from disassembling `assets/Konung/konung.exe` with GNU `objdump` inside the Go devcontainer. This is targeted x86 analysis of resource-loading routines; a full decompilation is not needed for this investigation.

## PE layout

* Format: PE32, i386; image base `0x00400000`.
* `BEGTEXT`: VA `0x00410000`, raw file offset `0x00000400`, size `0x37E00`.
* `DGROUP`: VA `0x00450000`, raw file offset `0x00038200`, size `0x15F000`.

To reproduce the initial inspection:

```sh
objdump -h assets/Konung/konung.exe
objdump -D -Mintel -j BEGTEXT assets/Konung/konung.exe
strings -t x assets/Konung/konung.exe
```

## `OBJECTS.RES` loader

The resource-name string is at executable VA `0x0045075F`; loader code is around VA `0x0043B8BB`.

Observed behavior:

1. Open `OBJECTS.RES`.
2. Read its 8-byte header, then the 512-entry (`0x1000` byte) table.
3. Treat each object offset as relative to the data section at file offset `0x1008`.
4. For table indices `0..29`, load the extended `0x3228`-byte animated-object metadata structure. The branch is explicit at `0x0043B9AA` (`index < 0x1E`).
5. For later indices, load a `0x100`-byte static-image metadata block.

The current extractor's 30 animated `OBJECTS.RES` entries and static entries match this loader split. The missing prince/warrior animations are not extra Type 2 frames hidden beyond our frame-offset scan of these 30 table entries. Their storage or construction path is still unknown.

## `GRAPH.RES` loader

The resource-name string begins at executable VA `0x00450723`; initialization code is around `0x0043B4AB`.

Observed behavior includes reading the leading `0x20000` bytes as palette data and then reading 11 Type 5 frame records. The next documented table starts at file offset `0x227E8`. The 512-slot table has 305 populated entries in the installed file. The first Type 1 block ends at `0x001ED8B6`.

At `0x1ED8B6`, the bytes do not form a valid block header. At `0x1EE8B6`, there is a sequence of 27 Type 0 blocks. Each begins with a big-endian tag `0` and a little-endian size; in this file, the size includes the 8-byte header. Advancing by each size lands exactly on the next Type 0 header, and block 27 ends exactly at EOF (`0x00DD559B`). This validates the block family and its boundaries byte-for-byte.

The 64 KiB gap between `0x1ED8B6` and `0x1EE8B6` remains unexplained. CTPAX-X's source describes Type 0 metadata with 2,960 16-bit values, 1,536 32-bit offsets, and 3,840 more 16-bit values. The corresponding 19,744-byte metadata layout fits all 27 blocks, and each block has populated entries in the offset table. Swapping adjacent image/shadow offsets yields monotonic image-frame starts; frame payloads begin after the metadata and decode with the existing indexed-RLE helper. All 14,067 frames across these blocks decode successfully.

The catalog extractor exports each complete Type 0 block, including its header, to `output/sprite-catalog/raw/GRAPH.T0/block_NNN.bin` and its decoded frames to the grouped `GRAPH.T0` gallery. The `type0.tsv` inventory records source offset, total and payload sizes, metadata size, remaining image-data bytes, populated offset count, block end, decoded frame count, and decode failures. Palette 2 is used only as a visual preview because the game's palette assignment for these blocks is not verified. Blocks 0–2 show adult human-like animated figures and are candidates for the warrior art, but no particular block has yet been matched to the village chief in the provided screenshot.

## Map entity and warrior model initialization

The map scene loader at VA `0x0043C0FC` opens `LIGHTS.N`, then `KONUNG.N`. For `KONUNG.N`, the `0x200` argument passed to the file helper is an open flag; it is not a skipped header. The loader reads `0x6400` bytes to `0x627FD8`, then reads sections of `0xC800`, `0x3E80`, and `0x8CA0` bytes to their respective buffers. These sizes total `0x1F720`, matching the map files' 128,800-byte size. The first section is processed as 160 records of `0xA0` bytes; later code initializes a 200-entry array of `0x20`-byte runtime records. This disproves the earlier seven-equal-plane interpretation as the executable's read layout.

The first section is the terrain layer: 160 rows × 80 cells × 2 layer bytes, each a 1-based index into the `GRAPH.T1` table that the `GRAPH.RES` loader reads to `0x006314B8` (at `0x0043B6B1`). The terrain renderer at `0x0042562F` draws cells at `x = col*0x74 + (odd row ? 0x3A : 0)`, `y = row*0x20`. The `0xC800`-byte second section is expanded into a 320 × 160 word grid at `0x00505C70` and is not terrain. Full details are in [the map format notes](konung_map_format.md).

At VA `0x004143DC`, an entity initializer assigns its map coordinates from the entity's words at `+0x08` and `+0x0A`, initializes state, and links it into the runtime grid. The loader calls this initializer from `0x0043C858` for one entity path and calls the model initializer at `0x00414078` from `0x0043C866` for the other.

The model initializer at `0x00414078` selects either a Type 2 animated object or a Type 0 model. The Type 0 path uses the signed entity byte at `+0xD3` as the model-block index into a table based at `0x007145B4`, with a `0x4D28` stride per model. Entity fields at `+0`, `+1`, and `+0x10` select the actor subtype, variant, and direction/animation slot. It stores pointers to selected records at entity offsets `+0xDC` and `+0xE0`; a Type0 metadata value is copied into entity state at `+0xF4`. The selected record's fixed-point offsets are combined with entity offsets `+0xE8/+0xF0` and global screen/map offsets to produce render coordinates at `+0xE4/+0xEC`.

This confirms that Type 0 is an in-game animated-model path and that model choice and frame/direction lookup are entity-driven. The palette/dye operation has not yet been traced, so gallery palette 2 must not be treated as a runtime assignment. The inspected code also does not establish that `OBJECTS.RES` object IDs 87–89 identify the same kind of map entity as the Type 0 characters.

## Known routines and buffers

Quick lookup table. Each row is described in the section or document named in the "Source" column.

| VA | Kind | Meaning | Source |
|---|---|---|---|
| `0x0044023E` | routine | File read `(handle, buffer, size)` | observed at every loader |
| `0x00440047` | routine | File open `(path, flags)`; returns `-1` on failure | map loader |
| `0x004404C1` | routine | File close | map loader |
| `0x0043B4AB` | routine | `GRAPH.RES` loader; reads the T1 table at `0x0043B6B1` | this doc |
| `0x0043B8BB` | routine | `OBJECTS.RES` loader | this doc |
| `0x0043BA50` | routine | `FLAMES.RES` loader (74 frames) | [res format](konung_res_format.md) |
| `0x0043C0FC` | routine | Map loader (`LIGHTS.N`, `KONUNG.N`) | [map format](konung_map_format.md) |
| `0x0042562F` | routine | Terrain draw pass | [map format](konung_map_format.md) |
| `0x0043E242` | routine | Indexed-RLE blit `(surface, image, palette, x, y)`, clipped, top-left origin | [map format](konung_map_format.md) |
| `0x0043ED09` | routine | Takes a palette byte offset and returns the palette used by the blit (conversion details not traced) | terrain draw pass |
| `0x0043BEF8` | routine | `MENU<width>R.RES` loader; frames 1–14 → slots `0x1F2`–`0x1FF` of `0x006304B8` | [menu](konung_menu.md) |
| `0x0042F8D8` | routine | Main-menu button draw (layout table `0x0045B314`) | [menu](konung_menu.md) |
| `0x0045B314` | data | Main-menu button rectangles, 32 bytes per button (640 then 800 x1,y1,x2,y2) | [menu](konung_menu.md) |
| `0x0043D8E7` | routine | Type 5 RGB555 frame draw | [res format](konung_res_format.md) |
| `0x00414078` | routine | Entity model initializer (Type 2 / Type 0) | this doc |
| `0x004143DC` | routine | Entity position initializer | this doc |
| `0x006314B8` | buffer | `GRAPH.T1` table, 512 × 8 bytes | [GRAPH layout](konung_graph_sprites.md) |
| `0x006324B8` | buffer | Another 8-byte-entry table directly after T1 (referenced at `0x0042A4E0`, `0x00438E4A`); role unknown | open |
| `0x00A3E6BC` | global | Pointer to the T1 image data | GRAPH loader |
| `0x00627FD8` | buffer | `KONUNG.N` terrain section (`0x6400`) | [map format](konung_map_format.md) |
| `0x00505C70` | buffer | Expanded 320 × 160 word grid from `KONUNG.N` section 1 | [map format](konung_map_format.md) |
| `0x00828378` | buffer | `KONUNG.N` section 2: flat decorations (16-byte records); count at `0x00A3E7F8` | [map format](konung_map_format.md) |
| `0x0082CE78` | buffer | `KONUNG.N` section 3: static object placements (36-byte records); count at `0x00A3E800` | [map format](konung_map_format.md) |
| `0x00780DDC` | buffer | Static `OBJECTS.RES` headers, `0x100` bytes per map object ID (slot − 30) | [map format](konung_map_format.md) |
| `0x00425A95` | routine | Decoration draw loop (after terrain) | [map format](konung_map_format.md) |
| `0x00426833` | routine | Object shadow pass + depth bucketing | [map format](konung_map_format.md) |
| `0x0043D3D8` | routine | Shadow RLE → per-row span list | [map format](konung_map_format.md) |
| `0x0043E5DF` | routine | Apply shadow spans: halve RGB once per pixel | [map format](konung_map_format.md) |
| `0x0069D1D4`, `0x007AB378` | buffers | Entity table (200 × `0x20`) and 0x100-byte entity records (characters; source file not traced) | [map format](konung_map_format.md) |
| `0x00671440`, `0x00664C40` | buffers | `LIGHTS.N` data: `0x3200` bytes then `0xC800` bytes; both zero-filled if the file is missing | map loader `0x0043C23C`–`0x0043C2BC` |
| `0x00A3E6F8`, `0x00A3E6FC` | globals | Screen origin X/Y used by the terrain pass | terrain draw pass |
| `0x007145B4` | buffer | Type 0 model table, stride `0x4D28` | this doc |

## Implication for the missing character animations

The CTPAX-X extractor author independently reported that prince and warrior animations were not extracted. The executable trace now identifies a separate Type 0 animated-model path in `GRAPH.RES`, alongside the 30 animated `OBJECTS.RES` entries. The missing warriors therefore were not a bad frame loop in the Type 2 object decoder; they use another resource family and entity initialization path. The remaining work is to map Type 0 model IDs to visible character variants and trace the palette/dye and equipment overlay logic.

## External reference

* [CTPAX-X / Extractor.ru Konung resource thread](https://www.extractor.ru/ipb/index.php?showtopic=202) — the extractor author describes the released unpacker as partial and excludes prince/warrior animations.
