# AGENT.MD: Konung Resource Reverse Engineering & Parsing

## 1. Project Overview & Role
You are working on **konung1.5**, an engine reimplementation / resource tooling project for the classic Slavic fantasy RPG **«Князь: Легенды лесной страны» (Konung: Legends of the North)** (released in 1999 by 1C / Game Factory Interactive).

The primary goal in this domain is reverse engineering, parsing, extracting, and re-implementing game assets and runtime systems from the original Windows x86 binary (`assets/Konung/konung.exe`) and game resource archives (`assets/Konung/*.RES`).

---

## 2. Environment & Tooling Guidelines

### Container Environment
The project runs inside a Fedora container managed via Podman:
* **Container engine**: Podman (`podman ps`, `podman exec`)
* **Workspace mount**: `/var/home/alba/Coding/konung1.5` -> `/workspaces/konung1.5`
* **Go toolchain**: Installed inside the devcontainer (Go 1.27+).
* **Command invocation pattern**:
  ```bash
  # Execute Go commands inside the dev container:
  podman exec -w /workspaces/konung1.5 <container_id> go run cmd/res-parser/main.go <args>
  podman exec -w /workspaces/konung1.5 <container_id> go test ./...
  ```
* **Python**: Available in the environment for binary inspection, hexdumps, prototyping, and PE analysis.

---

## 3. Reverse-Engineered Knowledge Base

### 3.1 Endianness Rule
> [!IMPORTANT]
> The game engine is a 32-bit Windows x86 application. **All multi-byte integers across all resource files (`.RES`) are Little-Endian (`binary.LittleEndian`)**, unless explicitly specified. Never read fields with `BigEndian`.

### 3.2 Classification of `.RES` Files

| Resource Type | Tag (First 4 Bytes) | Representative Files | Purpose & Inner Format |
|---|---|---|---|
| **Type 5 (RLE Graphics / Sprites)** | `0x00000005` (BE: 5, LE: `0x05000000`) | `FLAMES.RES`, `MENU640R.RES`, `MENU800R.RES`, `SAVE640R.RES`, `SAVE800R.RES`, `NEWH640R.RES`, `NEWH800R.RES` | Sequential animation frames or UI screens, compressed per row using custom RLE with 15-bit RGB 555 color values. |
| **Type 1 (File Containers / Archives)** | `0x00000001` (BE: 1, LE: `0x01000000`) | `SOUNDS.RES`, `MUSICS.RES`, `OBJECTS.RES`, `INTR640R.RES`, `Intr800r.res` | Archive containers containing raw PCM audio, music tracks, and game object definitions indexed by an offset/size table. |
| **Terrain Graphics** | `0x0000e003` (992/1000 tiles) | `GRAPH.RES` | 16-bit isometric map terrain and ground tiles. |

---

## 4. Binary Specification: Type 5 Graphics Resources

Type 5 files have **no global header**. Instead, they contain a contiguous stream of $N$ frames until EOF.

### 4.1 Frame Header (8 bytes)
```
Offset  Size  Type    Field Name
0x00    4     uint32  Tag           Constant 5 (0x00000005 Big-Endian / 0x05000000 Little-Endian)
0x04    4     uint32  FrameDataSize Compressed payload length in bytes (Little-Endian)
```

### 4.2 Frame Payload (`FrameDataSize` bytes)
```
Offset              Size        Type               Field Name
0x00                2           uint16             Width (pixels, LE)
0x02                2           uint16             Height (pixels, LE)
0x04                Height * 2  [Height]uint16     RowSizes (byte size of each compressed line, LE)
0x04 + Height * 2   Variable    []byte             Compressed RLE pixel stream for all rows
```

### 4.3 Row RLE Compression Protocol
Each row $y \in [0, \text{Height}-1]$ is parsed independently according to its corresponding entry in `RowSizes[y]`:

Read signed 8-bit command byte `cmd := int8(data[idx])`:
1. `cmd == 0`: **End of Row**. Stop processing current row and advance to row $y + 1$.
2. `cmd < 0`: **Transparent Run**. Skip transparent pixels.
   $$\text{skip\_count} = \text{cmd} \ \& \ \text{0x7F}$$
   Advance coordinate $x \mathrel{+}= \text{skip\_count}$. In RGBA output, transparent pixels remain $(0, 0, 0, 0)$.
3. `cmd > 0`: **Opaque Pixel Run**.
   $$\text{pixel\_count} = \text{cmd}$$
   Read the next $\text{pixel\_count} \times 2$ bytes as an array of `uint16` (Little-Endian) in **RGB 555** format:
   * **Bit 15**: Reserved / 0
   * **Bits 14..10** (5 bits): Red channel
   * **Bits 9..5** (5 bits): Green channel
   * **Bits 4..0** (5 bits): Blue channel

#### 15-bit RGB 555 to 32-bit RGBA Conversion Formula
```go
r := uint8(((val >> 10) & 0x1F) * 255 / 31)
g := uint8(((val >> 5) & 0x1F) * 255 / 31)
b := uint8((val & 0x1F) * 255 / 31)
a := uint8(255)
```

---

## 5. Catalog of Frames: `FLAMES.RES`

`FLAMES.RES` contains exactly **74 frames** ($\text{0x4A}$ in hex, matching `cmp dword ptr [ebp-4], 0x4a` in `konung.exe` at `0x0043babe`):

| Frame Range | Count | Dimensions | Description / In-Game Usage |
|---|---|---|---|
| `frame_000` – `frame_010` | 11 | $256 \times 195$ | Large campfire / settlement fire animation |
| `frame_011` – `frame_021` | 11 | $120 \times 90$ | Medium campfire flame |
| `frame_022` – `frame_032` | 11 | $100 \times 130$ | Standing torch flame (variant 1) |
| `frame_033` – `frame_043` | 11 | $93 \times 130$ | Standing torch flame (variant 2) |
| `frame_044` – `frame_054` | 11 | $100 \times 130$ | Wall torch flame (variant 3) |
| `frame_055` – `frame_059` | 5 | $38 \times 31$ | Candle / small flame |
| `frame_060` – `frame_073` | 14 | $96 \times 72$ | Horizontal fire ribbon / brazier |

---

## 6. Binary Specification: Type 1 Container Resources (`SOUNDS.RES`, `MUSICS.RES`, `OBJECTS.RES`)

Type 1 resources store multiple independent assets indexed by an offset/size table.

### 6.1 Header (12 bytes)
```
Offset  Size  Type    Field Name
0x00    4     uint32  Tag          Value 1 (0x00000001 BE / 0x01000000 LE)
0x04    4     uint32  TotalSize    Total payload size in bytes (LE)
0x08    4     uint32  Reserved     Always 0
```

### 6.2 Index Table
Following the 12-byte header up to `FirstEntryOffset`:
* Array of 8-byte entries:
  ```
  Size   uint32 (Little-Endian)
  Offset uint32 (Little-Endian)
  ```
* An entry with `0xFFFFFFFF` signifies an unused/tombstoned slot.
* Entry data is extracted by seeking to `Offset` and reading `Size` bytes.

---

## 7. How to Use & Extend the Resource Parser

The resource parser lives in [`cmd/res-parser/main.go`](file:///var/home/alba/Coding/konung1.5/cmd/res-parser/main.go).

### Common Tasks

#### Unpacking Type 5 Graphics (`FLAMES.RES`, `MENU640R.RES`, etc.)
```bash
# Inside the container:
go run cmd/res-parser/main.go assets/Konung/FLAMES.RES output/flames
go run cmd/res-parser/main.go assets/Konung/MENU640R.RES output/menu640
```
Outputs PNG images with alpha transparency for transparent pixels.

#### Reverse Engineering Checklist for Unparsed Game Files
1. **Never guess the format**: Extract strings and binary references from `konung.exe` first.
2. **Locate String References**: Search `konung.exe` for the file name in uppercase (e.g. `b"FLAMES.RES"`).
3. **Trace PE Call Sites**: Calculate Virtual Address (`VA = 0x400000 + RVA`) and find file offsets referencing that VA.
4. **Identify Read Calls**: Look for `fread` / `read` calls preceded by push instructions indicating buffer, sizes, and loop counts.
5. **Verify Palette / Video Mode**: Konung supported both 15-bit (RGB 555) and 16-bit (RGB 565) video modes. Pay attention to bit masks (`0xF800` vs `0x7C00`, `0x07E0` vs `0x03E0`, `0x001F`).
