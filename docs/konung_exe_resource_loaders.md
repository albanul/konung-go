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

The catalog extractor exports each complete Type 0 block, including its header, to `output/site/sprites/raw/GRAPH.T0/block_NNN.bin` and its decoded frames to the grouped `GRAPH.T0` gallery. The `type0.tsv` inventory records source offset, total and payload sizes, metadata size, remaining image-data bytes, populated offset count, block end, decoded frame count, and decode failures. Palette 2 is used only as a visual preview because the game's palette assignment for these blocks is not verified. The loader puts blocks 0–2 (the three character **bodies**, models 0–2) at `0x007145B4` and blocks 3–26 (24 **equipment overlays**) at `0x006A09F4`; see [the equipment notes](konung_game_format.md#equipment-overlays-verified).

## Map entity and warrior model initialization

The map scene loader at VA `0x0043C0FC` opens `LIGHTS.N`, then `KONUNG.N`. For `KONUNG.N`, the `0x200` argument passed to the file helper is an open flag; it is not a skipped header. The loader reads `0x6400` bytes to `0x627FD8`, then reads sections of `0xC800`, `0x3E80`, and `0x8CA0` bytes to their respective buffers. These sizes total `0x1F720`, matching the map files' 128,800-byte size. The first section is processed as 160 records of `0xA0` bytes; later code initializes a 200-entry array of `0x20`-byte runtime records. This disproves the earlier seven-equal-plane interpretation as the executable's read layout.

The first section is the terrain layer: 160 rows × 80 cells × 2 layer bytes, each a 1-based index into the `GRAPH.T1` table that the `GRAPH.RES` loader reads to `0x006314B8` (at `0x0043B6B1`). The terrain renderer at `0x0042562F` draws cells at `x = col*0x74 + (odd row ? 0x3A : 0)`, `y = row*0x20`. The `0xC800`-byte second section is expanded into a 320 × 160 word grid at `0x00505C70` and is not terrain. Full details are in [the map format notes](konung_map_format.md).

At VA `0x004143DC`, an entity initializer assigns its map coordinates from the entity's words at `+0x08` and `+0x0A`, initializes state, and links it into the runtime grid. The loader calls this initializer from `0x0043C858` for one entity path and calls the model initializer at `0x00414078` from `0x0043C866` for the other.

The model initializer at `0x00414078` selects an animated `OBJECTS.RES` model (entity `+4` bit `0x40`, metadata `0x00722D2C + slot*0x3228`) or a `GRAPH.RES` Type 0 model (`0x007145B4 + n*0x4D28`). In both cases the model index is the signed byte at `+0xD6`; the code reads `dword[+0xD3] >> 24`, which an earlier note misread as `+0xD3`. Type, variant and direction (`+0`, `+1`, `+0x10`) select the frame index and anchor record, stored at `+0xDC`/`+0xE0`. The render position is the world position plus the anchor record's image dx/dy. Full tables are in [the GAME.N notes](konung_game_format.md).

This confirms that Type 0 is an in-game animated-model path and that model choice and frame/direction lookup are entity-driven. The palette is per entity: dword `+0xD8` is a palette byte offset (draw at `0x004272CB`). Gallery palette 2 is not a runtime assignment. The inspected code also does not establish that `OBJECTS.RES` object IDs 87–89 identify the same kind of map entity as the Type 0 characters.

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
| `0x0043C0FC` | routine | Map loader (`LIGHTS.N`, `KONUNG.N`); argument 1 = restore mode after a load | [map format](konung_map_format.md), [saves](konung_save_format.md#load-0x00437b54--0x00438966--0x004249ac) |
| `0x0042562F` | routine | Terrain draw pass | [map format](konung_map_format.md) |
| `0x00426684`, `0x0043E242`, `0x0043D8EF` | routines | Sprite pass (shadows, then depth-sorted objects and characters); opaque clipped RLE blit; alpha-blended blit for flames | [map format](konung_map_format.md#shadows-and-draw-order) |
| `0x00A3E700`, `0x00A3E750` | globals | Options: transparent flames; shadows | [map format](konung_map_format.md#shadows-and-draw-order) |
| `0x0043E242` | routine | Indexed-RLE blit `(surface, image, palette, x, y)`, clipped, top-left origin | [map format](konung_map_format.md) |
| `0x0043ED09` | routine | Takes a palette byte offset and returns the palette used by the blit (conversion details not traced) | terrain draw pass |
| `0x0043BEF8` | routine | `MENU<width>R.RES` loader; frames 1–14 → slots `0x1F2`–`0x1FF` of `0x006304B8` | [menu](konung_menu.md) |
| `0x0042F8D8` | routine | Main-menu button draw (layout table `0x0045B314`); starts at button 2 unless `[0x00A3E728]` (game running) | [menu](konung_menu.md) |
| `0x004398C4` | routine | Hit-test `(x, y, rect table)` → index; used by the menu with `0x0045B314` | [menu](konung_menu.md) |
| `0x00424E34`, `0x004249AC`, `0x004314E0`, `0x0042D3DC` | routines | Save writer, save reader, load/save screen draw, `SAVE<width>R.RES` loader | [saves](konung_save_format.md) |
| `0x0046203C`, `0x00674640`, `0x0083DF5C`, `0x00A3E7AC` | data, buffers, global | Save slot rectangles; slot names (`KONUNG.SAV`); the new save's name; selected slot | [saves](konung_save_format.md#save-and-load-screen) |
| `0x0045B3F4` | data | Location names, 0x28 bytes, index map − 1 | [map format](konung_map_format.md#location-names-verified) |
| `0x0043BDD5`, `0x00625068`, `0x00A3E0A8` | routine, buffers | `QUESTR.RES` loader; quest flags (its last `0x4B0` bytes); misc-item draw flags (56 bytes) | [saves](konung_save_format.md) |
| `0x0043756C` | table | Escape switch on screen state `0x00A3E6D8` (0 game, 1 menu, 2 new game, 4/5 load/save, 6 settings) | [menu](konung_menu.md#input-behavior-currently-implemented) |
| `0x00437E50` | table | Main-menu click switch (6 cases: return, save, load, new, settings, exit) | [menu](konung_menu.md) |
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
| `0x0043BB6C` | routine | `GAME.N` loader (6 sections) | [GAME.N](konung_game_format.md) |
| `0x0069D1B4` | buffer | Entity groups, 200 × `0x20`; group 0 = player party | [GAME.N](konung_game_format.md) |
| `0x007AB378` | buffer | Entities, 2000 × `0x100` | [GAME.N](konung_game_format.md) |
| `0x00A3E684`, `0x00A3E694` | globals | Player group pointer, hero entity pointer | [GAME.N](konung_game_format.md) |
| `0x00A3E657` | global | Selected `GAME.N` number | [GAME.N](konung_game_format.md) |
| `0x00439BA0` | routine | Fine-grid row/col → world pixel (cell centre) | [GAME.N](konung_game_format.md) |
| `0x004281A4` | routine | Equipment overlay draw for Type 0 bodies | [GAME.N](konung_game_format.md#equipment-overlays-verified) |
| `0x006A09F4` | buffer | 24 equipment overlay blocks (`GRAPH.RES` Type 0 blocks 3–26), `0x4D28` each | [GAME.N](konung_game_format.md#equipment-overlays-verified) |
| `0x0067AC34` | buffer | `GAME.N` items, 8192 × 16 bytes | [GAME.N](konung_game_format.md) |
| `0x00459BEC`, `0x00462E1C`, `0x00465DCC` | data | Item type → overlay (64 B/type); slot draw order per facing; overlay palettes | [GAME.N](konung_game_format.md#equipment-overlays-verified) |
| `0x00459BB0`, `0x00458110`, `0x004594B0`, `0x0046053C` | data | Item type tables (gear 64 B; jewellery, herbs, misc 128 B): name, icon, gear stats/overlay | [GAME.N](konung_game_format.md#inventory-and-items-verified) |
| `0x0042C870` | routine | Item → icon INTR slot by category | [GAME.N](konung_game_format.md#inventory-and-items-verified) |
| `0x00424610`, `0x0042E04C`, `0x0042F50C` | routines | Character window toggle (I, HUD bag); window draw (case 3 of `0x0042DBBC`); bag strip draw | [character window](konung_character_window.md) |
| `0x0041C750`, `0x0041B244`, `0x0041B898`, `0x0041AC1C`, `0x0041AF48`, `0x0041ADE4`, `0x00418364` | routines | Recompute stats from enchantments; stat getter; carried weight; armour; attack; bow damage; can-use check | [character window](konung_character_window.md#derived-values) |
| `0x00458590`, `0x0046238C` | data | Enchantment table (16 B); special-property table (64 B: name, value `+0x2C`, kind `+0x2E`) | [character window](konung_character_window.md#derived-values) |
| `0x0046573C`–`0x00465874`, `0x0045AE20` | data | Character window layout rows (640/800 pairs); titles | [character window](konung_character_window.md) |
| `0x0041E210`, `0x00429194`, `0x00424918`, `0x0041E500`, `0x0041E778`, `0x004246F8`, `0x00425104`, `0x0041B164` | routines | Character window item moves: lift; return; bag insert; equipment box; jewellery box; bag drop; cursor by category; item weight | [character window](konung_character_window.md#moving-items) |
| `0x00A3E520`–`0x00A3E578`, `0x00A3E790` | globals | Held item, source and backups; cursor mode / sprite | [character window](konung_character_window.md#moving-items) |
| `0x0042F9B8`, `0x00A3E84C` | routine, global | Status-line message; its 12-pass counter | [character window](konung_character_window.md#moving-items) |
| `0x0079EFDC`, `0x007A6CDC`, `0x00A3E804`, `0x00A3E7E0` | buffers | Ground piles (`GAME.N` section 3) and their active list on the current map; counts | [ground items](konung_ground_items.md) |
| `0x00439DF0`, `0x00410E10`, `0x004110EC`, `0x0041093C`, `0x004399AC`, `0x00434C90` | routines | Pile image; pile at cell; pick-up; remove/respawn; allocate item record; bag has misc item | [ground items](konung_ground_items.md) |
| `0x00A3E770` | global | Game time, +1 per main-loop tick | [ground items](konung_ground_items.md#map-load-0x0043cf1a) |
| `0x00505C70` | buffer | Fine-grid word map, 0x140 bytes per row: occupant bits `0xFFF`, `0x1000` exit, `0x2000` pile | [ground items](konung_ground_items.md) |
| `0x00421E8C`, `0x0041ED8C`, `0x0041E384`, `0x00411758`, `0x0041F258`, `0x004143DC`, `0x00413DE4`, `0x004391BC` | routines | Melee swing case; intent planner; keep target cell; target in cell; keep attacking; start action; advance step; turn one octant | [combat](konung_combat.md) |
| `0x0041B100`, `0x0041B4CC`, `0x0041C480`, `0x0041B70C`, `0x0041079C`, `0x00450E14`, `0x00A3E828` | routines, data, global | Hit roll; hit chance; damage and wear; kill experience; gain free experience; facing dodge table; attacker's poison | [combat](konung_combat.md#formulas-verified) |
| `0x0041ED8C`, `0x0041130C`, `0x0041BFF8`, `0x00421170`, `0x0041ACE0`, `0x0041C29C`, `0x00410E84`, `0x004105A0`, `0x0082C1F8`, `0x0045AD30`, `0x00458100` | routines, buffer, data | Bow planner; line of fire; shoot (spawn arrow); arrow flight; arrow attack; arrow damage; bow target pick; re-equip; projectile list; launch offsets; arrow sprite offsets | [combat: bows](konung_combat.md#bows-verified) |
| `0x00418510`, `0x004399AC`, `0x004171D4`, `0x0041079C`, `0x00410B08`, `0x0045B010`, `0x0045B10C`, `0x0045AE88` | routines, data | Loot on kill; allocate item; loot level band; gain experience (auto-level); raise cost / level check; cost table; requirement/cap table; favoured stat | [combat: loot](konung_combat.md#loot-verified) |
| `0x0041D39F`, `0x00416700`, `0x0043BD5C`, `0x0045D37C` | routines, string | Settlement tick (surrender, tribute); village alarm check; settlement flag init; surrender message | [combat: village anger](konung_combat.md#village-anger-verified) |
| `0x0041BDF0`, `0x00411DE4`, `0x00420CCC`, `0x0045E278`, `0x00422778` | routines, data | Creature group upkeep (poison, respawn counters); place in territory; new member; respawn table; free a corpse's items | [combat: upkeep](konung_combat.md#upkeep-poison-and-respawn-verified) |
| `0x0041D0FC`, `0x00412C30`, `0x00412E00`, `0x00412BE0`, `0x00412F62`, `0x0041309D`, `0x004130EE`, `0x004132FE` | routines, data | Group upkeep (alarm every 16 ticks); alarm check; creature decision by alarm state; creature action jump table; stationary, dormant, other kinds; return home | [combat](konung_combat.md#creature-ai-verified) |
| `0x00420178`, `0x004203A0`, `0x004202A8`, `0x0041FD74`, `0x00439944`, `0x004142D4`, `0x00411FD0`, `0x0041EC0C`, `0x004376A8` | routines | Target selectors (assist, unengaged, nearest); enemy group in leash; grid distance; set destination and walk; chase check; party retaliation; hero death → game over | [combat](konung_combat.md) |
| `0x007AA2DC`, `0x00A3E81C`, `0x00421BB8`, `0x00422AD8` | buffer, global, routines | Map exits (`GAME.N` section 4) and their count; exit taken at walk end; travel by item | [GAME.N](konung_game_format.md#map-exits-verified) |
| `0x00A3E810`, `0x00A3E7FC`, `0x004118DC`, `0x00436AD0`, `0x0043C0FC` | globals, routines | Pending map change (−map); current map; place the party at group 0's cell; leave cleanup (remove the dead, refill post 0); load a map | [GAME.N](konung_game_format.md#map-exits-verified) |
| `0x00A3E764`, `0x00A3E6DC` | globals | Game speed 0–2 (extra ticks per frame); mouse buttons (bit 0x01 left, 0x02 right) | [menu](konung_menu.md) |
| `0x00423E26`, `0x00414E64`, `0x00415504`, `0x004371D2`, `0x004138C8` | routines | Right click on a bag cell; misc item use (shovel case 24); quick bag toggle; drop an item on the ground | [character window](konung_character_window.md#right-click-and-the-quick-bag), [ground items](konung_ground_items.md) |
| `0x0041DB64`, `0x0041BC24`, `0x00450E54`, `0x00422D50`, `0x0041E96C`, `0x0042054C`, `0x00A3E7A0`, `0x00A3E758` | routines, data, globals | Herb/potion use; herb mixing; recipe table (33 × 6 B); in-hand box click; box put/take; use held item on a character (portrait); in-hand box item; loot highlight flag (Чистая слеза, torch) | [potions](konung_potions.md) |
| `0x0043B7FC`, `0x00625518`, `0x0043DE4D`, `0x0043B27C`, `0x0047CB2C`, `0x004A17F0`, `0x00415ADC` | routine, buffer, routine, code, data, data, data | `LIGHTS.RES` loader; its 64×43 glow map; glow blend; brighten-table builder (RGB555); green / red brighten tables; misc item use jump table (case 31 torch) | [potions: loot highlight](konung_potions.md#loot-highlight-0x00a3e758) |
| `0x00426146`, `0x00426420` | code, data | Selection ellipses under selected members (INTR 172/126/178 by HP); walk offset jump table | [interface](konung_interface_res.md) |
| `0x00423FBA`, `0x004305E4`, `0x00465A24` | routines, data | Hover handler (status line, switch `0x004244B3`); item description; stat short names | [character window](konung_character_window.md#hover-texts-verified) |
| `0x0042C914`, `0x0043FD18` | routines | Bag cell tint; palette tint | [character window](konung_character_window.md#bag-strip-0x0042f50c) |
| `0x004290AC`, `0x0042DBBC` | routines | Press a HUD button for one loop pass; window draw dispatcher (table `0x0042F4C8`, index = arg + 1) | [interface](konung_interface_res.md) |
| `0x0046281C`, `0x00462B1C` | data | Combat-stance equipment draw order: 4 actions × 8 facings × 6 `int32` (bow-drawn variant) | [GAME.N](konung_game_format.md#equipment-overlays-verified) |
| `0x0042179C` | routine | Toggle combat stance of the selection `0x0083B93C` (Space, HUD hotspot 2) | [GAME.N](konung_game_format.md#weapons-and-combat-stance-verified) |
| `0x00421698` | routine | Whole party into combat stance (key A, HUD hotspot 5) | [GAME.N](konung_game_format.md#weapons-and-combat-stance-verified) |
| `0x0042192C` | routine | Select weapon mode `+0x12` (keys 1/2/3 → 2/0/1; HUD hotspot 1 → cycle) | [GAME.N](konung_game_format.md#weapons-and-combat-stance-verified) |
| `0x004227A0` | routine | Release an entity's items (equipment `+0x66`, bag `+0x72`, `+0xB2`) | [GAME.N](konung_game_format.md#inventory-and-items-verified) |
| `0x00437301` | code | Keyboard switch of the in-game key handler (keys: Space, 1/2/3, A, F, I, L, M, Q, S, T, Tab, numpad +/− speed, ` quick bag) | [GAME.N](konung_game_format.md#weapons-and-combat-stance-verified) |
| `0x004392F4`, `0x00439A28` | routines | HUD hotspot id under the cursor; RLE hotspot-map lookup (maps `0x004577A0`/`0x004543DC` for 800×600) | [interface](konung_interface_res.md#hud-hotspots-verified) |
| `0x00422864` | routine | HUD click handler: switch on the hotspot id | [interface](konung_interface_res.md#hud-hotspots-verified) |
| `0x0042D21A` (in loader) | routine | `INTR<width>R.RES` loader: table → `0x006304B8`, data → `[0x00A3E6B8]` | [interface](konung_interface_res.md) |
| `0x0042D574` | routine | Party portrait + HP bar draw for one entity (cells `0x0045AF70`) | [interface](konung_interface_res.md#party-hud-portrait-routine-0x0042d574-verified-against-code-and-data) |
| `0x0042DA30`, `0x004290D8` | routines | HUD panel buttons: draw (positions `0x0045AF00`) / set slot bytes `0x00A3E090..96` | [interface](konung_interface_res.md) |
| `0x0042DBD3` | code | HUD redraw: frame slot 3, buttons, portraits | [interface](konung_interface_res.md) |
| `0x0042CA90` | routine | Entity name → buffer (creature / first-name / nickname tables `0x0045E3BC`, `0x0045E77C`, `0x0045FCDC`) | [interface](konung_interface_res.md#hover-status-line-verified) |
| `0x0043195C` | routine | Hover status line in the bottom bar | [interface](konung_interface_res.md#hover-status-line-verified) |
| `0x0043F89C` | routine | Cursor-over-opaque-pixel test for an RLE image at (x, y); sets the hover entity `0x00A3E6A0` | [interface](konung_interface_res.md#hover-status-line-verified) |
| `0x0043F821`, `0x0042CBBC`, `0x00427A80` | routines | Text width; wrapped text draw (15 px lines); queue a text draw | [interface](konung_interface_res.md#hover-status-line-verified) |
| `0x00592CFC`, `0x005A31F4`, `0x00593040` | data | Embedded fonts (large, small, save-slot names) | [interface](konung_interface_res.md#hover-status-line-verified) |
| `0x00437EC3`, `0x00437F88`, `0x00438042` | code | Hero selection: writes name `+0xD4` and model `+0xD6` | [GAME.N](konung_game_format.md) |
| `0x00427A0C` | routine | Queue a clipped indexed blit `(image, palette, x, y, xmin, ymin, xmax, ymax)` (32-byte records at `0x0069B8B4`) | [interface](konung_interface_res.md) |
| `0x0051ECB4` | global | Pointer to the `GRAPH.RES` palette block; images add a palette byte offset to it | [interface](konung_interface_res.md) |
| `0x00421DA4` | routine | Per-tick NPC AI dispatcher: kind `0x40` → `0x004123BC` (creatures, not traced), `0x80` → `0x00416058` (village), else party | [GAME.N](konung_game_format.md#npc-ai-village-ai-traced) |
| `0x00416058`, `0x00416934` | routines | Village AI: per-group loop; per-villager decision (post, or random walk at 25 % / 50 %) | [GAME.N](konung_game_format.md#village-ai-verified-against-code-and-game0) |
| `0x00413A84` | routine | Village random step target (quadrant, ranges `0x00A3E818`/`0x00A3E7DC`) | [GAME.N](konung_game_format.md#village-ai-verified-against-code-and-game0) |
| `0x0041182C` | routine | Entity → post slot (1-based, 0 = none) in its map's settlement | [GAME.N](konung_game_format.md#village-ai-verified-against-code-and-game0) |
| `0x004142D4` | routine | Set an entity's move target `(e, row, col, state)` | [GAME.N](konung_game_format.md#village-ai-verified-against-code-and-game0) |
| `0x004116CC` | routine | AI budget: < 3 path searches this tick (`0x00A3E720`), round-robin after `0x00A3E690` | [GAME.N](konung_game_format.md#npc-ai-village-ai-traced) |
| `0x00835B18`, `0x00A3E6A8` | buffer, global | Settlements (`GAME.N` section 6, 20 × `0x4B5`); current map's settlement | [GAME.N](konung_game_format.md#village-ai-verified-against-code-and-game0) |
| `0x00433AF8` | routine | Dismiss party member into the settlement's village group | [GAME.N](konung_game_format.md#village-ai-verified-against-code-and-game0) |
| `0x0041F5D8` | routine | Party/return-to-home-area target `(group, entity)` | [GAME.N](konung_game_format.md#partyreturn-routine-0x0041f5d8group-entity-partly-traced) |
| `0x0041F11C` | routine | Party AI step; calls `0x0041F5D8` only when entity `+2` bit `0x20` is set (call site `0x00422415`) | [GAME.N](konung_game_format.md#partyreturn-routine-0x0041f5d8group-entity-partly-traced) |
| `0x00414AB4` | routine | Entity goes idle: sets `+2` bit `0x40`; also `0x20` when the entity's group (`+5`) is the hero's | [GAME.N](konung_game_format.md#partyreturn-routine-0x0041f5d8group-entity-partly-traced) |
| `0x004364F0` | routine | Opens the clicked entity's dialogue (entity `+0xD0`, `0xFF` = none; callers `0x00423A41`, `0x00423B6E`, `0x00423D93`) | [GAME.N](konung_game_format.md) |
| `0x00722D2C`, `0x007145B4` | buffers | Animated `OBJECTS.RES` metadata (`0x3228` per slot); Type 0 model metadata (`0x4D28` per block) | [GAME.N](konung_game_format.md) |
| `0x00671440`, `0x00664C40` | buffers | `LIGHTS.N` data: `0x3200` bytes then `0xC800` bytes; both zero-filled if the file is missing | map loader `0x0043C23C`–`0x0043C2BC` |
| `0x00A3E6F8`, `0x00A3E6FC` | globals | Screen origin X/Y used by the terrain pass | terrain draw pass |
| `0x007145B4` | buffer | Type 0 model table, stride `0x4D28` | this doc |

## Implication for the missing character animations

The CTPAX-X extractor author independently reported that prince and warrior animations were not extracted. The executable trace now identifies a separate Type 0 animated-model path in `GRAPH.RES`, alongside the 30 animated `OBJECTS.RES` entries. The missing warriors therefore were not a bad frame loop in the Type 2 object decoder; they use another resource family and entity initialization path. The remaining work is to map Type 0 model IDs to visible character variants and trace the palette/dye and equipment overlay logic.

## External reference

* [CTPAX-X / Extractor.ru Konung resource thread](https://www.extractor.ru/ipb/index.php?showtopic=202) — the extractor author describes the released unpacker as partial and excludes prince/warrior animations.
