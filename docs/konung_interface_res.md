# Interface resources (`INTR640R.RES`, `Intr800r.res`) and the party HUD

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

**HP bar.** Only drawn when HP ≠ 0.
* Height = HP · H / 1600, at least 1; H = 40 (800) / 32 (640), from `0x00462F34`.
* Slot: `0xAF` if poisoned (`+0x50` ≠ 0); otherwise `0xB0` (red) at ≤ 400, `0xB1` (yellow) at ≤ 800, else `0xAE` (green).
* The bar image is drawn at the portrait's position + (6, 10) in 800 mode, or (4, 8) in 640 (`0x00465C34`), and clipped so only its bottom *height* rows show.

**Buttons (`0x004290D8`).** Slot bytes `0x00A3E090..96`, positions from `0x0045AF00`; in 800 mode (3,340), (1,374), (72,372), (1,455), (78,457), (1,514), (70,514).
* Button 0, weapon mode (hero `+0x12`): 0 → 8, 1 → 9, otherwise 16.
* Button 1, stance: 10 if hero `+3` bit `0x04`, else 15.
* Buttons 2–6 are fixed: 14, 11, 7, 12, 13.
* A pressed button has bit `0x80` set and is drawn 2 px higher.

**Game view.** The frame leaves x 140–799, y 0–555 (800 mode) for the map (`0x00427C10`: `0x00A3E874`, `0x00A3E880`, `0x00A3E890`, `0x00A3E89C`).

**Port.** The rules are in `gamestate` (`PortraitState`, `PortraitSlot`, `HPBar`, `HUDButtonSlots`, `Entity.HP`, `.Poison`, `.WeaponMode`, `.Dead`). Drawing is in `internal/game/hud.go`, 800×600 only. The camera centres on the view rectangle, clicks on the HUD don't move the hero, and the hero is always shown as selected. **H** is a debug key that takes 400 HP per press until combat exists. Button clicks are not implemented.

## Open questions

* The hero portrait depends on the body model. The port forces Volk to model 0 (slots 215–219); his model in `GAME.0` is 1, and which model the character-creation screen sets is not traced.
* The alternate portrait table (`0x0062F3E0` / `0x0062F3D8`), the palette effects for types 2/8, and the button actions.
