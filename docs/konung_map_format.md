# `KONUNG.N` map format: terrain layer

This records the verified terrain layout of the `KONUNG.N` map files and how the game draws it. Object/entity sections are not decoded yet; see "Open questions".

## File sections (observed)

Every installed `KONUNG.N` is 128,800 bytes (`0x1F720`). The map loader at VA `0x0043C0FC` reads four consecutive sections (the `0x200` argument to the open helper is an open flag, not a header skip):

| File offset | Size | Loaded to | Role |
|---|---:|---|---|
| `0x00000` | `0x6400` | `0x00627FD8` | **Terrain**: 160 rows × 80 cells × 2 layer bytes (verified, below) |
| `0x06400` | `0xC800` | `0x00512470` | 320 × 160 byte grid, expanded to 16-bit words at `0x00505C70` (below). Values in `KONUNG.1`: `0x00`, `0x0F`, `0x4F`, `0x40`. Hypothesis: walkability/obstacle grid at half-cell resolution. **Not terrain.** |
| `0x12C00` | `0x3E80` | `0x00828378` | Records with a 16-byte stride (the loop at `0x0043C537` walks up to 999 entries and stops at a `-1` first dword). Role not traced. |
| `0x16A80` | `0x8CA0` | `0x0082CE78` | Not traced. Raw words 87/88 found here earlier (`0x16D4C`, `0x17EBC`, `0x18414`) may be object references. |

Expansion of section 1 (loop at `0x0043C498`): byte `0xFF` → word `0x0FFF`; if `(b & 0x0F) == 0x0F` → `(b << 8) | 0x0FFF`; otherwise → `b << 8`.

## Terrain cells (verified)

* Row `r` (0–159) starts at byte `r * 0xA0`; cell `c` (0–79) holds two bytes at `r * 0xA0 + c * 2`: layer 0 (base) and layer 1 (overlay).
* Each byte is a **1-based** index into the `GRAPH.RES` Type 1 tile table (`GRAPH.T1` slot = byte − 1); `0` means nothing drawn. The `GRAPH.RES` loader at `0x0043B6B1` reads the 512 × 8-byte table into `0x006314B8`, the same table the terrain renderer indexes.
* Map extents (loader, `0x0043C353`–`0x0043C43B`): the highest row with a non-zero layer-0 byte gives height `row * 0x20`; the highest such column gives width `(col − 1) * 0x74`. `KONUNG.1`: last row 114, last column 32.

## Projection and draw order (verified)

Terrain renderer at VA `0x0042562F` (simple path; a second path at `0x004257A9` also reads the `LIGHTS.N` buffer at `0x00671440`, not ported):

```
x = originX + col * 0x74 + (row odd ? 0x3A : 0)   // 116 px, odd rows shifted 58 px
y = originY + row * 0x20                          // 32 px
```

Rows are drawn top to bottom and columns left to right. Layer 0 is drawn, then layer 1, both at the same position. The blit routine at `0x0043E242` treats `(x, y)` as the image's top-left corner (no hotspot) and clips against the viewport.

Validation: `go run ./cmd/map-render assets/Konung/KONUNG.1 output/maps/konung1.png` produces a seamless 3886 × 3712 terrain image: coast and sea along the top, a branching river, and a road on the right. `go test ./internal/konungmap` checks the extents and that every referenced tile decodes.

## Open questions

* Object/building/NPC placement (sections at `0x12C00` and `0x16A80`) and their draw order relative to terrain.
* Section 1 semantics (walkability is a hypothesis).
* The `LIGHTS.N` shading path. The loader reads `0x3200` bytes to `0x00671440` (80 × 160, one byte per terrain cell, indexed `row*0x50 + col` by the lit draw path) and `0xC800` bytes to `0x00664C40`.
* `KONUNG.1 = Old Camp` remains provisional.
