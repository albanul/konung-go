# `GAME.N` start state: characters and creatures

Characters, NPCs and monsters are **not** stored in `KONUNG.N`. They come from `GAME.N`, the start-of-game state loaded by VA `0x0043BB6C`. **`N` is the hero's model number.** The loader reads `dword[0x00A3E657] >> 24`, i.e. byte `0x00A3E65A`, which is `+0xD6` of the 0x100-byte hero template at `0x00A3E584`.

## Heroes and start maps

The new-game code (`0x00438F3F`–`0x004390D5`) copies the hero template into entity 0, including model `+0xD6` from `0x00A3E65A` and palette `+0xD8` from `0x00A3E65C`. It then sets the party group's map byte to `model*3 + 1` (`0x004390B4`).

| Hero model (Type 0) | `GAME.N` | Start map | Hero |
|---|---|---|---|
| 0 | `GAME.0` | `KONUNG.1` | Volk (Волк), per the user's identification of `KONUNG.1` |
| 1 | `GAME.1` | `KONUNG.4` | Dragomir (the user recognized model 1 as Dragomir) |
| 2 | `GAME.2` | `KONUNG.7` | Michail (Михаил), per the user |

Entity 0's model, name and palette in the files (model 1, name 124 «Драгомир», palette 33 in all three) are placeholders that this code overwrites. **Hero selection (verified)** writes the name index `+0xD4` and the body model `+0xD6` together: Волк 10 / model 0 (`0x00438042`), Драгомир 124 / model 1 (`0x00437EC3`), Михаил 151 / model 2 (`0x00437F88`). The palette is set with them: Волк 74, Драгомир 33, Михаил 39 (`gamestate.HeroPalette`). The game start writes the rest of the [creation template](konung_new_game.md#game-start-0x00438f3f) (class, stats, skills, level 1) over the file's hero (`Game.StartHero`). `Game.HeroModelFromGame` sets only model, name and palette, for `cmd/map-render`.

Code: `internal/gamestate` (`Parse`, `Game.OnMap`, `Game.HeroModelFromGame`, `WorldPos`, `Placed.LoadSprite`, `Placed.LoadAnimation`), `resfmt.LoadEntitySprite`, `resfmt.LoadEntityAnimation`. `cmd/map-render -game assets/Konung/GAME.0` draws them (Volk's game). By default `map-render` draws map content only, which keeps the site's map pages clean.

## File layout (verified)

All three installed files are 709,822 bytes, exactly the sum of six reads:

| File offset | Size | Loaded to | Content |
|---|---:|---|---|
| `0x00000` | `0x20000` | `0x0067AC34` | 8192 × 16-byte **items**; byte `+3` = item type |
| `0x20000` | `0x1900` | `0x0069D1B4` | 200 × 32-byte **groups** |
| `0x21900` | `0x7D00` | `0x0079EFDC` | 500 × `0x40` **items on the ground** (piles) — [ground items](konung_ground_items.md) |
| `0x29600` | `0x109A` | `0x007AA2DC` | 250 × `0x11` **map exits** (teleports) — [map exits](#map-exits-verified) |
| `0x2A69A` | `0x7D000` | `0x007AB378` | 2000 × 256-byte **entities** |
| `0xA769A` | `0x5E24` | `0x00835B18` | 20 × `0x4B5` **settlements** with named-character posts ([village AI](#village-ai-verified-against-code-and-game0)) |

A saved game stores every section except the map exits ([saves](konung_save_format.md)). `gamestate.Game.Marshal` writes the decoded records back in this layout; each record keeps its original bytes for the fields the port does not decode (`Entity.Raw`, `Group.Raw`). It reproduces `GAME.0`–`GAME.2` exactly.

## Groups (32 bytes)

Group 0 is the player's party: `0x0043A895` sets the party pointer `0x00A3E684` to `0x0069D1B4`, and the hero pointer `0x00A3E694` to entity `word[group0+0]`. The map loader iterates groups 1–199 (loop at `0x0043C612`).

| Offset | Type | Meaning |
|---|---|---|
| `+0x00` | `int16` | first entity index |
| `+0x02` | int32 | creature respawn debt (`Group.Debt`; [respawn](konung_combat.md#upkeep-poison-and-respawn-verified)) |
| `+0x06` | int16 | creature respawn countdown (`Group.Countdown`) |
| `+0x08` | `uint16` | group fine-grid **row** (the party is placed around it, `0x00411B15`) |
| `+0x0A`, `+0x0C`, `+0x0E` | `uint16` | `+0x0C` = territory (inner rectangle) max row, with `+0x08` as min (`0x0041F5D8`, `0x00412C30`); `+0x0A`..`+0x0E` = leash (outer rectangle) rows (`0x0041FD74`, `0x00411FD0`) |
| `+0x10` | `uint16` | group fine-grid **column** |
| `+0x12`, `+0x14`, `+0x16` | `uint16` | `+0x14` = territory max col, with `+0x10` as min; `+0x12`..`+0x16` = leash cols. `gamestate.Group.WanderArea` / `LeashArea` |
| `+0x18`, `+0x19` | int8 | creature population target and maximum (`Group.Target`, `MaxPop`; the maximum = reserved entity slots) |
| `+0x1A` | int8 | enemy group of an alarmed creature group (`Group.Enemy`) |
| `+0x1D` | byte | creature alarm state: 0 calm, 1 attack, `0x20` return (`Group.Alarm`); `0x80` in the files, cleared on map load |
| `+0x1B` | byte | map number (`KONUNG.N`) |
| `+0x1C` | byte | entity count (consecutive from `+0x00`) |
| `+0x1E` | byte | **kind**, 0 = inactive. `0x20` the party; `0x80` \| tribe (low 2 bits) a village (village AI); `0x40` \| creature kind a creature group (creature AI): `0x41`–`0x50` equal the members' entity `+4`; `0x53` (45 groups in `GAME.0`) a lone character with a dialogue (wizards, hermits, guards…; members' `+4` = `0x53`, a human kind 2/3/5, or `0x43`/`0x44`/`0x55`/`0x56`/`0x57`). The code tests only bits `0x20`/`0x40`/`0x80`, zero, and the value `0x53` (excluded from the party's enemy search `0x00420024` and from the lamp of the skeleton king `0x004155EF`). Values from the data of all three files. |

## Entities (256 bytes, fields used by the port)

| Offset | Type | Meaning |
|---|---|---|
| `+0x00` | `int8` | type = action / animation set (animated models use `type − 6` when ≥ 6, `0x0041409A`) |
| `+0x01` | `int8` | variant = **facing direction** (8 per type) |
| `+0x02` | byte | action state bits: `0x40` idle, `0x48` walking to `+0x0C`/`+0x0E` (village AI); intents `0x01` melee, `0x04` shoot, `0x02` retaliating ([combat](konung_combat.md)); bit `0x04` cleared when leaving the bow ([weapons](#weapons-and-combat-stance-verified)) |
| `+0x03` | byte | bit `0x80`: not drawn (shadow and draw passes skip it); bit `0x04`: **combat stance** |
| `+0x04` | byte | bit `0x40`: animated `OBJECTS.RES` model, otherwise `GRAPH.RES` Type 0; bit `0x80`: dead (HUD); bits `0x07`: portrait face; `0x51`/`0x52` = child/woman (village AI) |
| `+0x08` | `uint16` | fine-grid row; `0` together with `+0x0A` = `0` means placed at runtime |
| `+0x0A` | `uint16` | fine-grid column |
| `+0x10` | byte | **animation step**, advanced by `0x00413DE4` until it reaches the count in `+0xD7` |
| `+0x33` | byte | level (status line «Уров:») |
| `+0x42`, `+0x46`, `+0x48`, `+0x4A`, `+0x4C` | `uint16` | skills: trade, building, healing, curios, smithing (status-line phrases above 30 / 4 / 100 / 10 / 4). Healing (знахарство, ≤ 500) grows with potion making and sets potion strength ([potions](konung_potions.md)) |
| `+0x12` | byte | **weapon mode** = active weapon slot: 0 axe/club, 1 bow, 2 blade; selects HUD button 0's image (`0x0042911A`); Volk = 2 |
| `+0x50` | `int16` | non-zero while poisoned (HP bar colour) |
| `+0x66` | 6 × `uint16` | **equipment**: item indices for slots 0–5 (0 = empty), drawn as overlays |
| `+0x72` | 32 × `uint16` | **bag**: carried item indices (0 = empty) |
| `+0xB2` | 5 × `uint16` | **jewellery**, drawn from the jewellery table; by box position probably necklace, two bracelets, two rings |
| `+0x4E` | `int16` | armour without gear ([character window](konung_character_window.md#derived-values)) |
| `+0xC4`, `+0xC6`, `+0xC8`, `+0xD1` | `int16` ×3, byte | enchantment bonuses (armour, attack, stat 9) and their stack flags, written by `0x0041C750` |
| `+0xCC` | `uint16` | **ammunition** item (the box next to the bow) |
| `+0xD2`, `+0xD3` | `int8` | enchantments on the character (−1 = none) |
| `+0x5C` | `int16` | **HP**; 1600 = full (HUD bar divides by `0x640`); portrait state thresholds 1200/800/400 ([HUD](konung_interface_res.md)) |
| `+0x54`, `+0x58`, `+0x5E`, `+0x62` | `uint16` | effective stats Лов, Хар, Сил, Вын (status line of party members); bases at `+0x56`, `+0x5A`, `+0x60`, `+0x64` (`0x0041C750`) |
| `+0xD0` | byte | **dialogue ID** opened by a right click on the entity (`0x004364F0`, [dialogues](konung_dialogue.md)); `0xFF` = no dialogue |
| `+0xD4` | byte | **first name** index (exe table `0x0045E77C`, 32 bytes each; [names](konung_interface_res.md#hover-status-line-verified)) |
| `+0xD5` | byte | nickname index (`0x0045FCDC`), 0 = none |
| `+0xD6` | `int8` | model: `OBJECTS.RES` slot 0–29, or `GRAPH.RES` Type 0 block. **Earlier notes said `+0xD3`**; the code reads `dword[+0xD3] >> 24`, i.e. byte `+0xD6`. |
| `+0xD8` | `uint32` | palette byte offset (per-entity dye), passed to `0x0043ED09` by the draw at `0x004272CB` |

World position (VA `0x00439BA0`, fine-cell centre): `x = col*58 + (row odd ? 29 : 58)`, `y = row*16 + 16`.

## Animation types and facings (Type 0 bodies)

Body records come in two sets of six types: 0–5, and 6–11 with the same layout. The peaceful set (6–11) is used by the villagers and Volk; animated objects fold types ≥ 6 back onto 0–5. Seen in body model 0's records and frames:

| Type | Steps | Meaning |
|---|---|---|
| 6 (0) | 12, 6 frames held 2 steps each | idle |
| 7 (1) | 12 distinct frames | **walk**. The anchor records carry one cell of displacement per cycle, e.g. facing E: image dx −14 → +32 over the 12 steps; S: dy −77 → −47; NE: (−16, −79) → (6, −93). Snapping to the next cell continues the motion seamlessly (E: last step +32, next cycle starts at 58 − 14 = +44, a normal step). So a walking character advances one fine cell per walk cycle. |
| 8 (2) | 7 | hit reaction ([combat](konung_combat.md#action-types-verified)) |
| 9 (3) | 12 | death |
| 10 (4), 11 (5) | — | no peaceful records: the bow shot (4) and the melee swing (5) use the stance set 0–5 |

**Facing (variant):** 0 W, 1 NW, 2 N, 3 NE, 4 E, 5 SE, 6 S, 7 SW, i.e. clockwise from west. Identified visually from the walk frames, not from movement code. On the fine grid, the matching neighbour cells are: same row ±1 col (E/W, 58 px), row ±2 (N/S, 32 px), and row ±1 for the diagonals (±29, ±16 px; the column depends on row parity). See `mapdata.Neighbors`.

## Frame and anchor lookup (model initializer `0x00414078`)

Each (type, variant) has a record of `int16` words: `[count, frame index for step 0, step 1, …]`. Animated objects have 19 words (up to 18 steps), Type 0 has 13 (up to 12). The idle sets seen so far have 12 steps that cycle 6 frames, each held for 2 steps, e.g. Type 0 block 1 type 6 variant 5: `12, 24,24, 25,25, … 29,29`. In the table below, `d` is the **step**. Earlier notes called it direction.

| | Animated `OBJECTS.RES` (`0x00722D2C + slot*0x3228`) | `GRAPH.RES` Type 0 (`0x007145B4 + n*0x4D28`) |
|---|---|---|
| frame index | `int16[meta + 0x0A + t*0x130 + v*0x26 + d*2]` | `int16[meta + 0x0A + t*0xD0 + v*0x1A + d*2]` |
| frame pair (shadow, image offsets) | `meta + 0x728 + idx*8` | `meta + 0x1728 + idx*8` |
| anchor record (int16 shadow dx, dy, image dx, dy) | `meta + 0x1728 + t*0x480 + v*0x90 + d*8` | `meta + 0x2F28 + t*0x300 + v*0x60 + d*8` |
| image data base | `meta + 0x3228` | `meta + 0x4D28` |

`meta` is the block as stored in the file, including its 8-byte tag/size header (the runtime replaces `+4` with the data pointer). Image position = world position + image dx/dy (`0x00414277`). The shadow (pair dword 0, skipped when −1) goes into the same merged 50% shadow pass as static objects, at world position + shadow dx/dy (`0x004267BD`). Entities are depth-sorted with static objects.

## Inventory and items (verified)

Each entity owns its items directly: **6 equipment slots** at `+0x66`, a **32-item bag** at `+0x72` and **5 jewellery slots** at `+0xB2` (all `uint16` item indices, 0 = empty). The item-release loop at `0x004227A0` walks exactly these three ranges and marks each item free (`item +0 = 0xFF`). No party-wide inventory has been seen.

| Equipment slot | Content | Evidence |
|---|---|---|
| 0 | axe / club (weapon mode 0) | «Дубина», «Бердициум» in `GAME.0` |
| 1 | bow (weapon mode 1) | «Пехотный лук» |
| 2 | knife / sword (weapon mode 2) | «Нож» (Volk), «Спата» |
| 3, 4, 5 | armour, helmet, shield | «Доспех кожаный», «Шлем кожаный», «Деревянный щит» |

The slot meaning of 0–2 is verified by `0x0042192C` (mode *m* requires slot *m*); 3–5 by the item names and by the stance draw (slot 5 is drawn as a raised shield).

**Item record (16 bytes, `GAME.N` section 1).** Byte `+0x0F` and `+0x0E` bits 1–7 are unused: no code reads them (every test of `+0x0E` is bit 0) and they are 0 in every item of `GAME.0`–`GAME.2`.

| Offset | Type | Meaning |
|---|---|---|
| `+0x00` | byte | **category**: selects the item type table (`0x0042C870`, below). Equals the slot index for equipped gear; 9 = herbs/potions, 11 = quest items, 12 = arrows. `0xFF` = free (`0x004227A0`). |
| `+0x01` | `int8` | enchantment index (table `0x00458590`), −1 = none |
| `+0x02` | `int8` | special property (64-byte records at `0x0046238C`: name such as «, на червяков» at `+0`, value `+0x2C`, target kind `+0x2E`), −1 = none |
| `+0x03` | byte | item type (index into the type table below), verified by `0x004281A4` |
| `+0x04` | `int32` / `float32` | arrows (category 12): stack count (int32, `0x0041B898`); other items: current condition (float32) |
| `+0x08` | `float32` | maximum condition: the loot sets `+8` = `+4` = gear `+0x2E` (`0x00418843`); the smith (script 362) and weapon wear change them |
| `+0x0C` | `int16` | poisoning (description «, отравление:»; weapons and arrows) |
| `+0x0E` | byte | bit 0: **unidentified**: the description shows «Неведомое оружие / Неведомый лук / Неведомый предмет», the enchantment is ignored, the bag cell is tinted yellow |

**Item type tables (verified).** The category picks the table, the type the record (`0x0042C870`). Every record starts with its CP866 name.

| Categories | Table | Record | Icon (INTR slot) | Examples |
|---|---|---|---|---|
| 0–5, 12 | gear `0x00459BB0` | 64 B | `int16 +0x3A` | «Нож», «Дубина», «Доспех кожаный», «медные стрелы» |
| 6–8 | jewellery `0x00458110` | 128 B | `int16 +0x7C` | «Ожерелье», «Браслет» |
| 9 | herbs, potions `0x004594B0` (14 types, [potions](konung_potions.md#herb-types)) | 128 B | `int16 +0x7C` | «Лечебный бальзам», «Белый корень» |
| other | misc `0x0046053C` | 128 B | `int16 +0x7C` | «Левая часть амулета Дракона», «Труды Гиппократа» |

Gear records also hold the damage or armour value (`uint16 +0x2C`), the maximum condition (`int16 +0x2E`, the loot's new items), a required stat id and minimum (`+0x30`, `+0x32`), the bow range (`+0x34`; the arrow spawn copies its low byte to the arrow's lifetime, `0x0041C204`), the base price (`int16 +0x36`, `0x0041B36A`; other tables `+0x78`), the weight in grams (`+0x38`; other tables `+0x7A`), the icon (`+0x3A`), the ground image (`+0x3C`) and the equipment overlay `k` at `+0x3E` (−1 = invisible; `0x004281A4` reads it as `int16[0x00459BEE + type*64]` by type alone). The character window uses the gear table for equipment and the jewellery table for the `+0xB2` items regardless of category. Code: `resfmt.ItemTableOf`, `ExeTables.ItemName`, `ItemIcon`, `ItemOverlay`; `gamestate.Game.Item`.

`GAME.0`: Volk has only the knife equipped (slot 2); his bag holds «Левая часть амулета Дракона» and five «Лечебный бальзам». So keys 2 and 3 do nothing for him until he gets an axe or a bow.

## Weapons and combat stance (verified)

**Controls.** The keyboard switch (compare chain `0x00437301`–`0x00437368`): Space (`0x20`) → `0x0042179C`; `1` → `0x0042192C(2)`; `2` → `(0)`; `3` → `(1)`; `A` (`0x41`, when `0x00A3E754` = 0, `0x00436DB0`) → `0x00421698`. HUD hotspots ([interface](konung_interface_res.md#hud-hotspots-verified)): 1 (button 0, weapon) → `0x0042192C(−1)`; 2 (button 1, stance) → `0x0042179C`; 5 → `0x00421698`.

**Stance toggle `0x0042179C`.** Works on the selection list `0x0083B93C` (up to 10 entity pointers, ends at the first null). The **first** selected entity decides: bit `0x04` of its `+3` clear → everyone enters, else everyone leaves. Entities that are dead (`+4` bit `0x80`, type 3/9) are skipped.

* Enter (if not in stance): `type −= 6` (peaceful set 6–11 → combat set 0–5), `+3 |= 0x04`.
* Leave (if in stance): `type = 6` when `type > 3` (an attack ends in idle), else `type += 6`; `+2 &= 0xFA`, becoming `0x40` if 0; clear the bit.
* Walking entities (type 7/1) re-init their model (`0x00414078`), others restart their action (`0x004143DC(e, type)`). Then the HUD stance button image becomes 10 (stance) or 15.

**Whole party `0x00421698`.** For every living party member: `+6 = 0`, `+2 = 0x44` (bow) or `0x41`, `+0x14 = 0xFF`, and enter the stance as above. Ported as the hero's attack order (A / HUD button 4, [combat: attack order](konung_combat.md#attack-order-verified)).

**Weapon select `0x0042192C(mode)`.** For the selection (10 entries if `0x00A3E754` ≠ 0, else the first only; `0x00A3E754` not traced): modes 0–2 set `+0x12 = mode` only if equipment slot *mode* is non-empty. Any other mode cycles 0 → 1 → 2 → 0, skipping empty slots (nothing happens with all three empty). Afterwards, if `+0x12` ≠ 1 and `+2` bit `0x04` is set, the bit is cleared and `0x00414AB4` (not traced) is called.

**Combat animation types.** 0 idle, 1 walk, 3 dead (combat set; +6 for peaceful). From the action sounds in `0x004143DC` (jump table `0x00414768`): 2/8 play the model's cry `model*8+0x24` (hypothesis: hit), 3/9 `model*8+0x22` (death), 4 sound 3, 5 sound 8 with the blade else 2 (hypothesis: 4/5 are attacks). Not verified visually.

## Equipment overlays (verified)

Type 0 bodies wear equipment drawn as overlay layers by `0x004281A4`, called right after the body blit (`0x0042739B` / `0x004273DD`).

* **Blocks.** `GRAPH.RES` Type 0 blocks 0–2 are the three **bodies** (loaded to `0x007145B4`); blocks 3–26 are 24 **equipment overlays** (loaded to `0x006A09F4`). Each overlay gets a fixed palette from the exe table `0x00465DCC[k]` (loader `0x0043B797`): `2,2,2,2,4,4,3,3,3,4,4,4,4,4,4,4,4,4,2,2,5,5,5,5`.
* **Slots.** Equipment slots 0–5 (`+0x66`); empty slots and items with overlay −1 are skipped.
* **Peaceful order** (`+3` bit `0x04` clear, `0x00428484`): `0x00462E1C + variant*0x18` (6 `int32`, e.g. facing 5: `3,2,5,4,1,0`).
* **Stance order** (`0x004281B3`): action *a* = 3 for type 1, 2 for type 5, 1 for type 4, else 0. With weapon mode 1 (bow) and *a* ≠ 2 the table is `0x00462B1C + a*0xC0 + variant*0x18` and only the bow slot (1) is drawn with overlay **`k+1`**. Otherwise the table is `0x0046281C + …` (same layout) and the weapon slot (`+0x12`, unless it is 1) and the shield slot 5 use `k+1`. Overlay `k+1` is the drawn version of the item (raised club, shield on the arm, drawn knife), confirmed by rendering.
* **Frame and position.** The overlay frame comes from the overlay block's own `(type, variant, step)` sequence. The position is world position + the body's per-step attachment point (`int16 x, y` at `body + 0x828 + type*0x180 + variant*0x30 + step*4`) + the overlay anchor's image dx/dy (`overlay + 0x2F28 + type*0x300 + variant*0x60 + step*8`, words `+4`/`+6`).

Code: `resfmt.ExeTables` (`ItemOverlay`, `DrawOrder`, `CombatDrawOrder`, `OverlayPalette`), `resfmt.ComposeEquipment`, `Game.Overlays`; stance and weapons in `gamestate/stance.go` (`ToggleStance`, `SelectWeapon`, `CombatAction`). The port composites body and overlays into one image per step. Space and the HUD stance button toggle the selection's stance, `1`/`2`/`3` and the HUD weapon button set the first selected member's weapon (all of them with Shift); see [party](konung_party.md#orders-verified).

`KONUNG.1` in `GAME.0`: villagers wear helmets, armour, axes and shields (e.g. villager 355: overlays 17, 9, 20, 0 by facing order). The new-game code does not overwrite the hero's items, so `GAME.0`'s entity 0 is Volk's starting kit.

## NPC AI (village AI traced)

**Dispatcher `0x00421DA4`**, called once per game tick from the main loop (`0x00437854`/`0x00437888`, next to the animation step). It resets the path-search budget `0x00A3E720` and walks groups 0–199 that are active on the current map. Kind (`+0x1E`) bit `0x40` → creature AI `0x004123BC` ([below](#creature-ai-traced), rules in [combat](konung_combat.md#creature-ai-verified)). Otherwise bit `0x80` → village AI `0x00416058`. Otherwise → party AI (contains `0x0041F11C`).

**Budget `0x004116CC`.** An entity may think only while `0x00A3E720 < 3` (incremented by each path search) and while it comes after the last served entity (`0x00A3E690`), so members are served round-robin.

### Village AI (verified against code and `GAME.0`)

`0x00416058(group)` calls `0x00416934(entity)` for every idle member (`+2` bit `0x40`, not dead: type ≠ 3/9) within the budget. While the group is calm (group `+0x1D` = 0):

1. **Women and children** (entity `+4` = `0x52` / `0x51`): roll `rand() % 100`; on 75–99, pick a random step target (`0x00413A84`) and walk (`0x004142D4(e, row, col, 8)`); otherwise stay idle.
2. **Everyone else:** if combat stance `+3` bit `0x04` is set, clear it and switch to idle type 6. **Apprentice** (`0x00416A7A`): `+4` bits `0x38` hold k = 1–3, the post whose holder the entity learns from (dialogue script 354, [dialogues](konung_dialogue.md#script-functions-verified)). In the start files these bits are 0 for every villager except women and children (`0x51`/`0x52`, bits `0x10`), who never reach this test. If settlement post k is empty (`+0x3D2 + 8k` = 0), the bits clear (`0x00416BEE`) and the post test below follows. Otherwise the apprentice belongs on one of the 8 cells around post k's cell (`0x0041165C(row, col, post row, post col)` gives that neighbour's direction or −1; the post cell itself does not count). Elsewhere it walks to the post cell's first free neighbour (`0x00410FFC`, `0x004142D4(e, row, col, 8)`); without one it stays idle. Next to the post it turns to the post's facing (`+0x3D0 + 8k`, `0x004391BC`), then stays idle. Then `0x0041182C` looks the entity up in the settlement's posts. **Posted:** if the entity is not on the post cell, walk straight to it (`0x004142D4`). Otherwise turn to the post's facing (`0x004391BC`), then stay idle. **Not posted:** roll `rand() % 100`; on 50–99, take a random step target and walk.

**Random step `0x00413A84`.** The base is the entity's cell when it lies inside the group rectangle (bounds inclusive), otherwise the rectangle centre. `rand() % 4` picks a quadrant (jump table `0x00413CE4`: 0 = −row −col, 1 = −row +col, 2 = +row −col, 3 = +row +col). The base moves by `rand() % (2·R)` rows and `rand() % (2·C)` columns in that direction (0 when the modulus is < 2). The result is accepted if it lies strictly inside the rectangle and the grid cell's occupant bits (`& 0xFFF`) are 0. Up to 10 tries; on success the target goes to `+0x0C`/`+0x0E` and `+2 = 0x48`. R = `0x00A3E818` and C = `0x00A3E7DC` come from the view size (`0x00427B54` / `0x00427C10`): 640×480 → R 17, C 7; 800×600 → R 20, C 9.

**Settlements** (section 6, `0x00835B18`, 20 × `0x4B5`). The map loader picks the record whose `+3` equals the current map (`0x0043CAC6` → pointer `0x00A3E6A8`).

| Offset | Type | Meaning |
|---|---|---|
| `+0x02` | byte | village group index |
| `+0x01` | byte | fence pieces (non-zero = fortified; `Settlement.Fence`) |
| `+0x03` | byte | map number (`KONUNG.N`); records 15–19 are empty |
| `+0x10`, `+0x14` | int32 | tribute pool and counter ([trade: production](konung_trade.md#shop-stock-production)) |
| `+0x18 + k*8`, k = 0..118 | 8 bytes | building records: int8 type (−1 empty), int8 state (0–2 building, 3 built or burning with a timer, 4–5 burning, 6 ruin), `uint16` placement index (the map object shows type object + state), `uint16` unused, int16 timer. A settlement uses 7 + `+0` + `+1` records (the fence pieces are buildings; up to 112 in the data); the other slots up to `+0x3CF` are empty records (type −1). `+0x40`/`+0x41` = record 5, the map-exit gate. See [buildings](konung_buildings.md). |
| `+0x4B2` | byte | flags: hostile, strangers, independent, loan, surrendered (`Settlement.Flags`; [village anger](konung_combat.md#village-anger-verified)) |
| `+0x3D0 + k*8`, k = 0..4 | 8 bytes | **post**: byte facing, byte dialogue ID (copy of entity `+0xD0`), `uint16` entity index (0 = empty), `uint16` row, `uint16` col |

Further fields: `+0` extra buildings, `+4`/`+8`/`+0xC` healer, smith and voevoda timers, `+0x10`/`+0x14` tribute pool and counter, `+0x18 + 8k` building records, `+0x3F8 + 4k` apprentice times, `+0x40C` healer flags and stock, `+0x444` merchant stock, `+0x484` smith stock, `+0x4B4` smith job (read as `dword[+0x4B1] >> 24`, `0x00417A8D`; `0xFF` = none) ([dialogues](konung_dialogue.md#script-functions-verified), [trade: production](konung_trade.md#shop-stock-production)). `+0x4B3` is unused (`0xFF` in every record, never read on its own). With these the whole record is accounted for (`+0x3D2` per slot is also tested by the dismiss path `0x00433AF8`). In every village, post 0 is the elder (unique dialogue) and posts 1–4 hold the role characters with dialogues 101, 103, 102 and 100, which recur in every village (posts 1 healer, 2 merchant, 3 smith, 4 voevoda, from the dialogue scripts; see [dialogues](konung_dialogue.md#script-functions-verified)). Post cells and facings equal the entities' file position and variant. `KONUNG.1` = settlement 9, group 27: posts 355 (elder), 356, 357, 358; slot 4 is empty. Child 371 has its own dialogue but no post, so it roams.

**Dismissing a party member** (`0x00433AF8`). The entity is copied into the next slot of the settlement's village group, gets `+5` = that group and `+2 = 0x60`, and the party count drops. That matches the observed behaviour: the dismissed member runs into the village and stays there.

### Creature AI (traced)

`0x004123BC(group)` runs for every creature group (`+0x1E` bit `0x40`) on the current map, once per game tick. The alarm check, the decisions by alarm state and creature kind, and the action cases are in [combat: creature AI](konung_combat.md#creature-ai-verified). Calm creatures do not move.

Creature kinds `+4` and their `OBJECTS.RES` models in `GAME.0`: `0x41` model 0, `0x42` 1, `0x43` 2, `0x44` 3 (the man-eating flower: walk set of 1 frame, stationary; maps 3, 37, 40), `0x45` 4 (a skeleton-like walker, maps 32 and 36), `0x46` 5, `0x47` 6, `0x48` 7, `0x49` 8/23, `0x4A` 9, `0x4B` 10, `0x4C` 11, `0x4D`/`0x4F` 12, `0x4E`/`0x50` 13. Kinds `0x46`, `0x4C`, `0x55` start dormant (action type 4, step 0). Humanoid members of creature groups (bandits, `+4` = 1, 2, 3, 5) start at type 6.

### Party/return routine `0x0041F5D8(group, entity)` (partly traced)

It is reached only from `0x0041F11C` (AI step `0x00422415`), and only when entity `+2` bit `0x20` is set. That bit is set by `0x00414AB4` (idle, when the entity's group `+5` is the hero's) and by the party-join paths (`0x00422762`, `0x00432B21`, `0x00433B92`: `+2 = 0x60`).

* **Group kind bit `0x40`:** the group rectangle (rows `+0x08`..`+0x0C`, cols `+0x10`..`+0x14`) is a home area. A member inside gets `+2 = 0x40` (bit `0x20` cleared). When all members are inside, group `+0x1D = 0x80` and the routine returns 0. A member outside gets the centre target, shifted on both axes by `rand() % (span/4)` when that is at least 2, with one coin flip for the sign. It then scans forward (column first, wrapping rows) to the first free cell. Target → `+0x0C`/`+0x0E`, `+2 |= 0x48`.
* **Other groups (`0x0041FA5D`):** a member at least 10 cells from the group's first entity walks to a formation slot near it (offset table `0x0045FF79`); otherwise nothing happens. Traced in full in [party](konung_party.md#idle-members-0x004223ad--0x0041f11c-verified).

### Port

`internal/game` `npc` with `npcMode`, and `gamestate` `Settlements`, `PostOf`, `VillageMoveThreshold`, `VillageStepTarget`. Village groups run the village AI above once per game tick (60 ms), with the 3-search budget served round-robin and the 800×600 ranges. Paths are not limited to the rectangle; only targets are. Creature groups (kind bit `0x40`) run the creature combat AI ([combat](konung_combat.md#creature-ai-verified)). Groups without a rectangle stand still too. The party branch is ported ([party](konung_party.md)). Characters reserve the cell they step into, so they don't overlap. An alarmed village (group `+0x1D` ≠ 0) runs `0x00416DA6` instead (`game.villagerTick`, [combat: village anger](konung_combat.md#village-anger-verified)). Apprentices follow their master's post (`Game.ApprenticePost`, `npc.apprenticeTick`).

## Placement of entities without coordinates

Entities with row = col = 0 are positioned at runtime around their group's position. The party code at `0x00411B8C` uses random formation offsets from tables near `0x0045FF79`. The port places them deterministically on the nearest free fine cells, using the `KONUNG.N` walk grid (`mapdata.WalkGrid.Blocked`). These positions are an **approximation**, but in `GAME.0`–`GAME.2` no living entity of a non-creature group lacks coordinates, so it does not come up at game start; the party is placed by the traced `0x004118DC` after a map change. Creature groups are re-placed on map entry by the traced `0x00411DE4` (random cell in the territory, `Game.PlaceInTerritory`, [combat](konung_combat.md#upkeep-poison-and-respawn-verified)), so only non-creature groups without coordinates remain approximate.

## `KONUNG.1` (Volk's starting location) in `GAME.0`

The user identified `KONUNG.1` as Volk's (Волк) initial location.

| Group | Entities | Content |
|---|---|---|
| 0 | 1 | the player (Volk): drawn as Type 0 model 0 with palette 74 (the file's model 1 / palette 33 are overwritten on new game), at row 86 col 33 inside the village |
| 27 | 18 | villagers with fixed positions: Type 0 model 0 men; `OBJECTS` slot 14 women, slot 15 children (1-based index IDs 15/16) |
| 64 | 1 | a single Type 0 villager |
| 3 | 4 | giant spiders (slot 12, palette 148) at the nest around row 184 col 40 |
| 105–109 | 26 | giant ants (slot 0, palette 56), five groups along the stream |

`GAME.1` and `GAME.2` are the other heroes' games. Their map-1 groups are similar, with more monsters (spiders 4 → 8 → 16), but those heroes start on `KONUNG.4` / `KONUNG.7`.

Validation: `go test ./internal/gamestate` checks the player group, the 50 drawable entities on map 1, sprite decoding, and that spawned entities sit on distinct free cells. `go run ./cmd/map-render -game assets/Konung/GAME.0 -all output/maps` renders 987 `GAME.0` entities across all maps with no decode errors.

## Map exits (verified)

Section 4 holds 250 × `0x11`-byte records; the loader (`0x0043BCEF`) counts the records before the first with byte `+4` = 0 into `0x00A3E81C` (118 in `GAME.0` and `GAME.2`, 121 in `GAME.1`). Decoder: `gamestate.Game.Exits` (`exits.go`).

| Offset | Type | Meaning |
|---|---|---|
| `+0x00`, `+0x01` | bytes | unused: every reader loads a dword and keeps its top byte (`dword[+0] >> 24` = map `+3`, `dword[+1] >> 24` = target `+4`); 0 in all three files |
| `+0x02` | int8 | arrival facing: copied to party group `+0x0C`, which the placement routine `0x004118DC` writes to the hero's `+0x01` (`0x00411A1D`) |
| `+0x03` | byte | map the exit is on (`KONUNG.N`) |
| `+0x04` | byte | target map |
| `+0x05`, `+0x07` | uint16 | arrival fine-grid row, column (party group `+0x08`, `+0x10`) |
| `+0x09`, `+0x0D` | uint16 | trigger rectangle rows, min..max inclusive |
| `+0x0B`, `+0x0F` | uint16 | trigger rectangle columns, min..max inclusive |

* **Teleport tiles.** 109 exits are single cells under a teleport decoration, `GRAPH.RES` tile 285 or 286 in `KONUNG.N` section 2, whose top-left is (−42, −21) ± 4 px from the exit cell's centre. The other 9 are larger areas (map edges, a gate) or use other tiles. `KONUNG.1` has three: (158, 4) → `KONUNG.3`, (155, 59) → `KONUNG.11`, (218, 17) → `KONUNG.42`. 25 exits lead to the same map (in-map teleports on maps 3, 5, 27, 32, 36 and 39, the same in all three `GAME.N`).
* **Map load** (`0x0043D11B`): every rectangle of the current map sets bit `0x1000` in the fine-grid word map `0x00505C70`.
* **Trigger** (`0x00422346`): when a party member's walk ends on a cell with that bit and the pick-up intent is not set, `0x00421BB8` runs. It acts for the hero, or a member standing on the hero's destination (`+0x0C`/`+0x0E`), and takes the first exit in record order with `+3` = party map and the cell inside the rectangle. If that is exit *i* and the party is in settlement *i* (`0x00A3E6A8`), it needs the settlement's bridge (record 5: type `+0x40` < 0 or state `+0x41` = 3); otherwise nothing happens. Only maps 14, 19 and 22 have such an exit ([buildings: the bridge gate](konung_buildings.md#the-bridge-gate)).
* **Taking the exit** (`0x00421CCD`): `0x00A3E810 = −target`, party group `+0x08`/`+0x10` = arrival row/col, `+0x0C` = facing. The main loop (`0x00437799`) sees `0x00A3E810 < 0`: the same map (`0x004377A6`) clears `0x00A3E810` and only re-places the party (`0x004118DC`), with no cleanup or load; another map runs the leave cleanup (`0x00436AD0`), loads the target (`0x0043C0FC`) and sets `0x00A3E7FC` (current map).
* **Placement** (`0x004118DC`): the hero goes to the group cell when it is inside the map and free (occupant bits `0xFFF` = 0); the other members are placed around it ([party](konung_party.md#placement-after-a-map-change-0x004118dc-verified), `Game.PlaceParty`).
* **No per-map save.** Every entity lives in the one global array `0x007AB378` all game; the AI (`0x00421DA4`) runs only for groups whose map byte `+0x1B` is the current map, so other maps stay frozen and characters keep their position, HP, inventory and deaths.
  * Leaving a map (`0x00436AD0(map)`): for each group on it, the party included (group 0 is still on the old map at that point), dead members (`+4` bit `0x80`) are removed. Their items are freed (`0x00422778`), the later members move down (memmove, group count `+0x1C` − 1) and the settlement's post indices (`+0x3D2`, 5 × 8 bytes, settlement from `0x0043A1F8(map)`) are fixed up. If post 0 is then empty, the first living member with model `+0xD6` < 3 (the code reads `dword[+0xD3] >> 24`), `+4` bits `0x38` clear and `0x0041182C` = 0 takes it: its dialogue `+0xD0` becomes settlement byte `+0x3D1`, `+2` = 0, then `0x00414AB4`.
  * Entering a map (`0x0043C0FC(0)`, group loop `0x0043C612`): villagers and the party go back on their stored cells; animated models get `+2` = `0x40` (idle) and every member's animation is reset (`0x004143DC`). Creature groups get their alarm `+0x1D` cleared, every creature is placed at random in its territory (`0x00411DE4`), and respawn runs (`0x00420CCC`). See [combat: upkeep and respawn](konung_combat.md#upkeep-poison-and-respawn-verified); port `Game.EnterMap`. The port keeps the villagers' cells across visits (`startScene.storeWorld`).
* **Travel by orb** (`0x00422AD8`, the map window of HUD button 2 «Карта (M)»): a click on a known location uses up a «Шар магического прыжка» and takes the first exit whose target is that map ([world map](konung_world_map.md#travel-by-orb-0x00422ad8)). `Game.Travel`.

**Port** (`startScene.step`, `startScene.changeMap`, `gamestate.Game.ExitAt`, `Game.TakeExit`): a hero walk that ends on an exit without the pick-up intent writes the party back into the game, moves group 0 and rebuilds the scene for the target map. The hero is placed on the arrival cell or the nearest free one. Before the change, `startScene.storeWorld` writes the party and every other character's cell and facing back into `gamestate.Game`, so a map looks as it was left on the next visit (`OnMap` uses the stored cells). Piles, the held item, the game clock and speed carry over. The bridge gate is checked (`Game.ExitOpen`). The leave cleanup (`Game.LeaveMap`) and the enter-time placement and respawn (`Game.EnterMap`) run on every change to another map. An exit or jump to the current map keeps the scene and only places the party again (`startScene.placeAgain`, `Game.SetArrival`, [party](konung_party.md#placement-after-a-map-change-0x004118dc-verified)).

Validation: `go test ./internal/gamestate -run Exits` checks the count, the `KONUNG.1` exits, `TakeExit` + `OnMap` placement on `KONUNG.3`, and the 109 teleport tiles; `go test ./internal/game -run StoreWorld` checks that a moved villager is back on its cell after `storeWorld`.

## Open questions

* None. Answered (see the tables above and the linked docs): the settlement record (every byte placed, `+0x4B3` unused); item `+0x0E` bits 1–7 and `+0x0F` (unused); the group kind `+0x1E`; map exit bytes `+0`/`+1` (unused); the item condition floats `+4`/`+8`; the gear fields `+0x2E`, `+0x34`, `+0x36`; the jewellery boxes (box 0 necklace, 1–2 bracelets, 3–4 rings, [character window](konung_character_window.md#moving-items)); `0x00A3E754`/`0x00A3E714` = Shift/Ctrl held (key down `0x004375A3`/`0x0043741A`, key up `0x004373DA`/`0x004373CE`); `0x00414AB4` ([party](konung_party.md)); travel by item `0x00422AD8` ([world map](konung_world_map.md#travel-by-orb-0x00422ad8)); spawn positions without coordinates (none occur in the start files); entity `+0x11`/`+0x14` ([movement](konung_movement.md)).
