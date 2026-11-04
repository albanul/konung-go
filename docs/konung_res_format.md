# Konung `.RES` formats

Specification of the `.RES` container and image formats used by «Князь: Легенды лесной страны». Per-file catalogs live elsewhere: [menu frames](konung_menu_res.md), [GRAPH.RES layout](konung_graph_sprites.md), [map files](konung_map_format.md), [sprite index](konung_sprite_index.md).

## Common block header

Every block starts with 8 bytes:

| Field | Type | Notes |
|---|---|---|
| `Tag` | `uint32` **big-endian** | Block type: 0, 1, 2, 5, 8 observed |
| `Size` | `uint32` little-endian | Payload length. For Type 5 it excludes the header; for `GRAPH.RES` Type 0 blocks it includes the header. Check per format. |

All other multi-byte values are little-endian (32-bit x86).

**Pitfall:** `FLAMES.RES` starts with `00 00 00 05`. Read as a big-endian count, this looks like "5 files", and the next bytes then give a >4 GB offset. These files are not archives with a file table: Type 5 files are flat frame sequences.

## File families

| Tag | Files | Contents |
|---|---|---|
| 5 | `FLAMES.RES`, `MENU640R.RES`, `MENU800R.RES`, `SAVE640R.RES`, `SAVE800R.RES`, `NEWH640R.RES`, `NEWH800R.RES` | Sequential Type 5 RGB555 frames, no file header |
| 1 | `SOUNDS.RES`, `MUSICS.RES`, `OBJECTS.RES`, `INTR640R.RES`, `Intr800r.res` | Containers with an offset table (layout differs per file; see below; `INTR*`: [interface resources](konung_interface_res.md)) |
| — | `GRAPH.RES` (14.5 MB) | Starts with palettes (`00 00 E0 03`…), then mixed blocks. See [GRAPH.RES layout](konung_graph_sprites.md). |

## Type 5 frame (RGB555 row-RLE)

Loader evidence: `FLAMES.RES` loader at VA `0x0043BA50` reads an 8-byte header to `0x00A53994`, then `[0x00A53998]` bytes of frame data, for 74 frames (`cmp [ebp-4], 0x4A` at `0x0043BABE`). 74 header+payload pairs sum exactly to the 1,533,070-byte file. The decoder logic comes from the draw routine at VA `0x0043D8E7`.

Payload:

| Field | Type | Notes |
|---|---|---|
| `Width` | `uint16` | pixels |
| `Height` | `uint16` | pixels |
| `RowSizes` | `[Height]uint16` | packed byte length of each row |
| `PixelStream` | bytes | rows `0..Height-1`, decoded independently |

Row commands (signed byte `cmd`):

* `cmd == 0`: end of row.
* `cmd < 0`: skip `cmd & 0x7F` transparent pixels.
* `cmd > 0`: `cmd` opaque pixels follow as `uint16` RGB555 values.

RGB555 → RGBA: `R = ((v>>10)&31)*255/31`, `G = ((v>>5)&31)*255/31`, `B = (v&31)*255/31`, `A = 255`. Evidence: the palette-correction routine at `0x00427D9C` handles 15/16-bit modes, and the brightest flame value `0x7EC4` decodes to R 31, G 22, B 4 (orange) only as 555. The game also has an RGB565 path, so verify masks before assuming a pixel format for new data.

Decoder: `resfmt.DecodeRLEFrame`, `resfmt.LoadType5File`.

## Indexed row-RLE (GRAPH terrain, OBJECTS, Type 0)

Same header (`Width`, `Height`, `RowSizes`). Each row: a byte with the high bit set skips `n & 0x7F` transparent pixels, `0` ends the row, and otherwise `n` 8-bit palette indices follow. Palettes are 256 RGB555 words taken from the start of `GRAPH.RES` (palette `p` at byte `p*512`). The colour `0x7C1F` (magenta) is transparent. Decoders: `resfmt.DecodeIndexedRLEFrame` (plus a legacy fallback, `DecodeIndexedRLELegacyFrame`), `resfmt.RGB555Palette`.

## `OBJECTS.RES`

8-byte header, then 512 × `(offset, size)` entries at `0x8..0x1007`; offsets are relative to `0x1008`. 0-based slots 0–29 are animated Type 2 blocks; later slots are static images. See [start-location notes](konung_start_location.md#objectsres-image-path-and-object-lookup) and [exe loader](konung_exe_resource_loaders.md#objectsres-loader). Decoder for static entries: `resfmt.DecodeStaticObject`.

## `SOUNDS.RES` / `Sounds_.res`

8-byte header (tag 1 BE, data-section size LE), then exactly 1000 `(offset, size)` LE pairs. Offsets are relative to the data start at byte 8008. An unused entry is `FFFFFFFF FFFFFFFF`. Payloads are PCM: mono, 16-bit, 22,050 Hz, matching CTPAX-X `knsfxres.c`. Decoder: `resfmt.LoadSoundEntries`, `SoundEntry.WriteWAV`.

## `FLAMES.RES` frame groups

| Frames | Size | Content |
|---|---|---|
| 0–10 | 256 × 195 | Large bonfire |
| 11–21 | 120 × 90 | Medium flame |
| 22–32 | 100 × 130 | Torch, type 1 |
| 33–43 | 93 × 130 | Torch, type 2 |
| 44–54 | 100 × 130 | Torch, type 3 |
| 55–59 | 38 × 31 | Candle / small flame |
| 60–73 | 96 × 72 | Fire strip / hearth |

Content names are visual descriptions.

## Code coverage

| Data | Code |
|---|---|
| Type 5 frames and files | `internal/resfmt/rle.go` |
| Indexed RLE, palettes | `internal/resfmt/indexed.go` |
| Static OBJECTS entries, `GRAPH.RES` Type 0 frames | `internal/resfmt/objects.go` |
| Sounds | `internal/resfmt/sound.go` |
| `GRAPH.RES` leading Type 5 sprites | `internal/game/cursor.go` |
| `GRAPH.RES` terrain tiles, `KONUNG.N` terrain | `internal/mapdata/` |
| Bulk extraction (all OBJECTS frames, Type 0 blocks) | `cmd/sprite-catalog` (has its own Type 0 walker) |

There is no single CLI that extracts every mixed `GRAPH.RES` block yet.

## Tools

```bash
go run cmd/res-parser/main.go assets/Konung/FLAMES.RES output/flames     # Type 5 → frame_000.png …
go run cmd/res-parser/main.go assets/Konung/MENU640R.RES output/menu640
```

## External references

* [CTPAX-X Konung tools](https://www.ctpax-x.org/index.php?goto=files&lang=en&show=147): menu/Type 5, `GRAPH.RES`, `OBJECTS.RES`, and sound extractors. Use them as an independent cross-check, and validate any structure against the installed files.
