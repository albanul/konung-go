# Interface resources (`INTR640R.RES`, `Intr800r.res`), the party HUD and the hover status line

## File layout (verified)

Loader near VA `0x0042D21A`: the name is built as `"INTR"` + screen width + `"R.RES"`. It reads 8 bytes, then `0x1000` bytes to the slot table `0x006304B8`, then the data size to `[0x00A3E6B8]`.

| File offset | Size | Content |
|---|---|---|
| `0x0000` | 4 | big-endian tag `1` |
| `0x0004` | 4 | little-endian data size (= file size − `0x1008`) |
| `0x0008` | `0x1000` | 512 × (`uint32` image offset, `uint32` palette byte offset); `0xFFFFFFFF` = unused |
| `0x1008` | data | images; offsets are relative to this point |

Both files use slots 0–274. Every image is **indexed row-RLE** (`resfmt.DecodeIndexedRLEFrame`), drawn with the `GRAPH.RES` palette at the slot's palette byte offset (palette index = offset / `0x200`). The HUD draws push `[0x0051ECB4] + offset`. Decoding slots 0–21 as RGB555 gives noise; as indexed they are clean.

Decoder: `resfmt.ParseInterface`, `InterfaceRes.Slot`, `InterfaceRes.Image`. Test: `resfmt.TestInterfaceResources`.

## Slot catalog (`Intr800r.res`; `INTR640R.RES` has the same slots at 640×480 scale)

| Slots | Content |
|---|---|
| 0–2, 4, 6 | Panel/dialog backgrounds (character sheet, framed windows, parchment) |
| 3 | **HUD frame**, 800×600: portrait grid (2×5), lower-left button panel, bottom bar; transparent game view |
| 5, 7–21 | Panel pieces and buttons: sword, axe, bow (weapon mode), fist/hand (stance), compass, book, bag, cross arrows |
| 22–168 | Inventory item icons (68×68 and smaller), various palettes |
| 169–199 | Buttons ("АВТО", "Ok", "Закрыть"), selection frame 173, HP bars 174–177 (3×40), arrows, world map 179 |
| 200–274 | **Portraits**, 62×62 (52×52 at 640), palette 1: 3 body models × 5 faces × 5 damage states |

## Party HUD (portrait routine `0x0042D574`, verified against code and data)

**Redraw (`0x0042DBD3`).** Draws slot 3 at (0, 0), then the 7 buttons (`0x0042DA30`), then `0x0042D574` for each party member (group 0 entities).

**Cell.** k = entity index − group 0's first entity. The positions come from table `0x0045AF70`: 16 bytes per cell, (x, y) for 640 then 800. In 800×600: x = 6 / 74, y = 4, 71, 139, 206, 273.

**Damage state.** Dead (entity `+4` bit `0x80`) → 4. Otherwise from HP (`int16` at `+0x5C`): ≤ 400 → 3, ≤ 800 → 2, ≤ 1200 → 1, else 0. The thresholds are absolute; full health is 1600 (`0x640`).

**Portrait slot.** Animated `OBJECTS.RES` models (`+4` bit `0x40`) get none. The slot depends on the Type 0 body model m (`+0xD6`). Hero: `0xD7 + m·25 + state`, i.e. face 4. Companion: `0xC3 + (+4 & 7)·5 + m·25 + state`, i.e. faces 1, 2, 3, 5, the values `GAME.0` villagers carry in `+4`. When `0x0062F3E0` is set, another table is used instead (`0x0062F3D8`, images at `0x0083E090`, index from `+0xF6`); this is not ported. Entity types 2 and 8 are drawn with a modified palette (`0x0043FD18`, `0x0043F763`); also not ported.

**Selection.** Members in the selection list `0x0083B93C` (10 entries, `0x00411708`) get slot 173 drawn over the portrait.

**Selection ellipse on the map (`0x00426146`, verified).** In the terrain pass, before the ground items, each selected member gets a flat 56×32 ellipse drawn with its INTR palette (`0x0043E242`). The colour follows HP (`+0x5C`): ≤ 400 slot 172 (red), ≤ 800 slot 126 (yellow), otherwise slot 178 (green). The top-left is the entity's position (`+0xE8`, `+0xF0`, the cell centre) + (−29, −16). While the entity walks (type 7 or 1) it moves along the facing by the step s (`+0x10`), jump table `0x00426420`: W/E ∓5s; N/S ∓(2s + s/2); diagonals ±(2s + s/2) across and ±(s + s/2) up or down. Code: `gamestate.SelectionMarker`, `SelectionOffset`, `startScene.drawSelection` (the hero is the port's whole selection).

**HP bar.** Only drawn when HP ≠ 0.
* Height = HP · H / 1600, at least 1; H = 40 (800) / 32 (640), from `0x00462F34`.
* Slot: `0xAF` if poisoned (`+0x50` ≠ 0); otherwise `0xB0` (red) at ≤ 400, `0xB1` (yellow) at ≤ 800, else `0xAE` (green).
* The bar image is drawn at the portrait's position + (6, 10) in 800 mode, or (4, 8) in 640 (`0x00465C34`), and clipped so only its bottom *height* rows show.

**Buttons (`0x004290D8`).** Slot bytes `0x00A3E090..96`, positions from `0x0045AF00`; in 800 mode (3,340), (1,374), (72,372), (1,455), (78,457), (1,514), (70,514).
* Button 0, weapon mode (hero `+0x12`): 0 → 8, 1 → 9, otherwise 16.
* Button 1, stance: 10 if hero `+3` bit `0x04`, else 15.
* Buttons 2–6 are fixed: 14, 11, 7, 12, 13.
* A pressed button has bit `0x80` set and is drawn 2 px higher. `0x004290AC(k, image)` stores `image | 0x80` and flags a redraw; the next main-loop pass (`0x0043792F`) calls `0x004290D8`, which restores all images from the hero, so a press lasts one loop pass. Presses: weapon select `(0, image shown so far)`, stance toggle `(1, 10 or 15)`, character window `(6, 13)`. The port holds a press for one animation step (`hud.press`; the loop rate is not traced). The game also plays sound 4 (`0x0042A664(4, …)`); the port has no audio.

**Game view.** The frame leaves x 140–799, y 0–555 (800 mode) for the map (`0x00427C10`: `0x00A3E874`, `0x00A3E880`, `0x00A3E890`, `0x00A3E89C`).

**Port.** The rules are in `gamestate` (`PortraitState`, `PortraitSlot`, `HPBar`, `HUDButtonSlots`, `Entity.HP`, `.Poison`, `.WeaponMode`, `.Dead`). Drawing is in `internal/game/hud.go`, 800×600 only. The camera centres on the view rectangle, clicks on the HUD don't move the hero, and the hero is always shown as selected. HP changes come from [combat](konung_combat.md). Of the button clicks, weapon (hotspot 1), stance (hotspot 2), gather (hotspot 4, the F key, `startScene.gatherOrder`), attack (hotspot 5, the A key, [combat](konung_combat.md#attack-order-verified)) and bag (hotspot 7) are implemented, with the pressed-button feedback. The tooltips (`gamestate.ButtonTooltips`) read: «Сменить оружие (1, 2, 3)», «Достать/Убрать оружие (Пробел)», «Карта (M)», «Все ко мне (F)», «Атаковать (A)», «Информация к размышлению (Q)», «Панель персонажа (I)». Hovering a button shows its tooltip ([hover texts](konung_character_window.md#hover-texts-verified)).

## HUD hotspots (verified)

Clicks are resolved through a run-length **hotspot map** in the exe, not through the button images. The view setup selects it (`0x00427B54` 640×480: rows `0x00457020`, runs `0x00452444`; `0x00427C10` 800×600: rows `0x004577A0`, runs `0x004543DC`; pointers `0x00A3E6AC` / `0x00A3E6B0`). Row *y* is an `int32` byte offset into the runs (−1 = empty row); the row is a list of `(id, length)` byte pairs, ending at a zero length. Lookup: `0x00439A28`; the main-screen dispatcher `0x004392F4` returns ids 1–7 directly (`0x00439584`).

Ids in the 800×600 map: **1–7 = panel buttons 0–6** (bounding boxes match the `0x0045AF00` positions); **30–39 = portrait cells**; 120/121 = the bag strip arrows; 8–19 = the character window's doll boxes ([moving items](konung_character_window.md#moving-items)); 40–44 = the character window's raise buttons and 45 = its auto-level toggle ([raise column](konung_character_window.md#raise-column-verified)). The click handler `0x00422864` switches on the id: 1 → weapon cycle `0x0042192C(−1)`, 2 → stance toggle `0x0042179C`, 5 → whole party into stance `0x00421698` ([GAME.N](konung_game_format.md#weapons-and-combat-stance-verified)), 7 → character window `0x00424610`, 120/121 → bag strip scroll ([character window](konung_character_window.md)).

Code: `resfmt.ExeTables.HUDHotspot800`, `game.buttonAt`. Validation: `resfmt.TestHUDHotspot800`.

## Hover status line (verified)

**Hover target.** At the start of each frame `0x0042AD18` clears `0x00A3E6A0`. Each entity's body is then blitted, followed by `0x0043F89C(image, x, y)`, which tests whether the cursor lies on an opaque pixel of that image; if so, `0x00A3E6A0` = entity (`0x00427309` / `0x00427404`). Characters are drawn in depth order, so the front-most one wins. Static objects don't block the test.

**Display.** While no window is open (`0x00A3E740` = 0), `0x00424054` calls `0x0043195C(entity)`. That routine redraws the bottom bar from frame slot 3, builds the line in `0x00A3E120`, and draws it with the **small font** wrapped (`0x0042CBBC`) at (view left + 10, view bottom + 5), width view right − view left − 15. In 800 mode that is (150, 560), width 644, with 15 px lines.

**Name (`0x0042CA90`).**
* Creatures (`+4 & 0x7F` in `0x41`–`0x50`): creature table `0x0045E3BC` [f − `0x41`], 32 bytes each: Гигантский Муравей, Аспид, … Паук (12), …, Ядовитый паук (14), …
* Everyone else, women and children included: first-name table `0x0045E77C` [`+0xD4`], plus `" "` + nickname table `0x0045FCDC` [`+0xD5`] when that byte is non-zero.

**Line.**
* Creatures: name + `0x0042C9F4` (`", Уров:" level(+0x33) ", Здор:" (HP+15)/16`).
* Women and children (`+4 & 0x7F` ≥ `0x51`): the name only.
* People: name + `", "` + class `0x0045AE20 + ((+4 & 7) − 1)·16` (Храбрец, Купец, Охотник, Вождь, Воин) + level/health, then:
  * party members (group = the player's): `", воин дружины"`, `", отравлен"` if `+0x50` ≠ 0, then ` Хар:` +0x58, ` Сил:` +0x5E, ` Лов:` +0x54, ` Вын:` +0x62;
  * hostile groups (group `+0x1D` bit `0x01`): `", враг"`;
  * posted villagers: role `0x0046233C + (slot − 1)·16`: староста, знахарь, купец, кузнец, воевода;
  * otherwise skill phrases: healing `+0x48` > 100, smithing `+0x4C` > 4, trade `+0x42` > 30, building `+0x46` > 4, curios `+0x4A` > 10.

`KONUNG.1`: «Бодрич, Купец, Уров:1, Здор:100, староста», «Волеслав, Храбрец, …, знахарь», «Световик, …, купец», «Всеслав, …, кузнец», «Гигантский Муравей, Уров:1, Здор:100».

**Fonts (embedded in DGROUP).** Large `0x00592CFC` (character window title) and small `0x005A31F4` (status line, dialogue).
* Header: byte first character (`0x20`), byte count (209), `int16` advance for characters without a glyph (4); then count `uint32` glyph addresses.
* The load-time relocation (`0x0043AA7F`) handles only count − 1 entries; the last entry is not a glyph.
* Each glyph is a Type 5 RGB555 RLE frame (`0x0042809C` converts its pixels in place for 565 modes). Text is CP866; glyph = char − first; the advance is the glyph width (`0x0043F821`); glyphs are top-aligned.
* Wrapping (`0x0042CBBC`) splits at spaces (each word keeps its space); a word whose addition reaches the width starts a new line.

**Port.**
* `resfmt`: `ExeTables.CString`, `TableString`, `Font`, `Font.Wrap`, `CP866ToUTF8`.
* `gamestate`: `Name`, `Game.StatusLine`, the new `Entity` fields.
* `internal/game`: `text.go` (glyph drawing) and `startScene.hitTest` (opaque-pixel test on the drawn character frames, last drawn wins).
* Tests: `resfmt.TestExeTextAndFonts`, `gamestate.TestStatusLines`.

## Open questions

* The alternate portrait table (`0x0062F3E0` / `0x0062F3D8`), the palette effects for types 2/8, and the actions of buttons 2–5 (hotspots 3–6) except 5.
