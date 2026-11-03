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

## Implication for the missing character animations

The CTPAX-X extractor author independently reported that prince and warrior animations were not extracted. The executable trace now confirms that the ordinary `OBJECTS.RES` table only exposes 30 animated entries. This makes a missing resource path or a separate character-animation format more likely than a bug in the current frame loop. The next trace should follow the code that maps map/game character IDs to renderable frames and inspect references to `GAME.*`, `KONUNG.*`, and the newly identified Type 0 blocks in `GRAPH.RES`.

## External reference

* [CTPAX-X / Extractor.ru Konung resource thread](https://www.extractor.ru/ipb/index.php?showtopic=202) — the extractor author describes the released unpacker as partial and excludes prince/warrior animations.
