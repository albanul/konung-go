# Character window (inventory)

Window 3: the character sheet with equipment, jewellery, stats and the bag strip. Opened and closed with **I** or HUD button 6 (the bag, hotspot 7). Both call `0x00424610`, which works when no window or window 3 is open (`0x00A3E740` = 0 or 3). It presses button 6 with image 13 (`0x004290AC(6, 13)`) and toggles the window. It shows the first selected character (`0x0083B93C[0]` → `0x00A3E688`), resets the bag scroll `0x00A3E840`, then draws the window (`0x0042DBBC(2)`; the dispatcher's jump table at `0x0042F4C8` is indexed with *argument + 1*, so this is case 3, `0x0042E04C`) and the bag strip (`0x0042F50C(entity, 0)`).

Status: drawing verified against code and rendered PNGs. Values verified against hand calculation for `GAME.0` entities 0 and 355. [Moving items](#moving-items) between the bag, the equipment and the jewellery is ported, and so are the in-hand box (hotspot 19, `0x00A3E7A0`) and herb use ([potions](konung_potions.md)). Misc item use is ported for the shovel and the torch only.

## Window (`0x0042E04C`, 800×600)

Positions come from exe tables of `(x640, y640, x800, y800)` int32 rows; `resfmt.ExeTables.CharWindow800` reads the 800 column. Coordinates are screen pixels.

| Element | Rule |
|---|---|
| Recompute | `0x0041C750(entity)` first ([derived values](#derived-values)) |
| Background | INTR slot 2 (642×460) at the view's top-left (140, 0). Printed labels (Опыт, Харизма, Броня, …) are part of the art. |
| Name | large font `0x00592CFC`, centred on the view, y = 10·600/640 = 9 |
| Title | only for Type 0 models: string `0x0045AE20 + (face−1)·16` (face = entity `+4` & 7: «Храбрец», «Купец», «Охотник», «Вождь», «Воин»), centred, y = 31 |
| Equipment | slot → position row `int32[0x0046573C + slot·4]` → `0x004657B4 + row·16`. Icon = gear table icon by type; x = row x + (cell width + 1 − icon width)/2, y = row y. Axe (slot 0) and blade (slot 2) share the hand box: the axe is drawn unless weapon mode is 2 and the blade is present; the blade only when the axe isn't drawn (`gamestate.SheetEquipment`). |
| Ammunition | item word `+0xCC`, gear-table icon, top-left at `0x00465804` (the box next to the bow) |
| Jewellery | items `+0xB2`…`+0xBA`, jewellery-table icons, centred on `0x00465814 + i·16` |
| In-hand box | item `0x00A3E7A0`, icon by category centred on `0x00465864` (`0x0042E5C0`); the item is hidden from its own box or bag cell meanwhile; see [potions](konung_potions.md#controls) |
| 25 numbers | small font `0x005A31F4`, top-left at `0x00465874 + i·16`, see below |
| Raise column | `0x0042F1F5`–`0x0042F48F`: «Свободный опыт», raise buttons and the auto-level toggle, see [below](#raise-column-verified) |

**The 25 numbers** (`gamestate.CharSheet`, constants `Sheet*`): 0 `+0x3C` Опыт · 1 `+0x33` Уровень · 2–5 effective Хар/Сил/Лов/Вын (`+0x58`, `+0x5E`, `+0x54`, `+0x62`, column «сейчас») · 6 Здоровье = (HP + 15)/16 · 7–10 base values (`+0x5A`, `+0x60`, `+0x56`, `+0x64`, «базовая») · 11 Меткость `+0x44`/100 · 12 Фехтование `+0x40`/100 · 13–17 skills `+0x4A`, `+0x42`, `+0x48`, `+0x4C`, `+0x46` (identification, trade, healing, smithing, building) · 18 Текущий вес · 19 Максим. вес (both `%4.1f` of grams·0.001) · 20 Отравление `+0x50` · 21 Броня · 22 Удар · 23 Золото `+0x34` · 24 Свободный опыт `+0x38` (only its y is used; see the [raise column](#raise-column-verified)).

## Bag strip (`0x0042F50C`)

A row of cells along the bottom of the game view showing the bag (`+0x72`, 32 slots). Geometry is computed when the INTR file loads (`0x0042D2D8`–`0x0042D396`):

* cell width `0x00A3E860` = width(slot 17) − 1 = 69; arrow size `0x00A3E864`/`0x00A3E784` = slot 0x12's 20×70;
* strip top `0x00A3E88C` = 555 − 70 + 1 = 486; space between the arrows W = 659 + 2 − 2·20; visible cells = W / 69 = 9; first cell x `0x00A3E870` = 139 + 20 + (W − 9·69)/2 = 159.

Arrows: left at (139, 486), slot 0x12 (0x13 pressed); right at (780, 486), slot 0x14 (0x15 pressed). The argument picks the pressed arrow (−1 left, +1 right). Each cell draws INTR slot 17, possibly tinted, and the item icon (by category, `0x0042C870`) centred: x + (69 + 1 − w)/2, y + (70 + 1 − h)/2. Bag index = (scroll + k) & 31.

**Scrolling.** Hotspot 120 (left arrow) decrements the scroll, not below 0 (`0x004235E4`). Hotspot 121 (right arrow) increments it only if `0x0041EB9C` reports an item in the bag slot just past the last visible cell (`scroll + visible < 32`) (`0x0042367F`).

**Cell tint (`0x0042C914`).** The first matching rule wins: the character can't use the item (`0x00418364`) → tint (24, 0, 0); unidentified (item `+0xE` bit 0) → (15, 15, 0); enchanted (item `+1` ≥ 0) → (0, 12, 0). `0x0043FD18(r, g, b)` tints each 5-bit channel by n %: c + trunc((31 − c)·n/100); negative n darkens. `resfmt.TintPalette` ports it.

## Moving items

Clicks in the open window resolve through the hotspot routine (`0x004395E6`, `0x00A3E730` ≠ 0): ids ≥ 120 first (bag arrows); then a bag cell when x > first-cell x and strip top < y < 555, returned as `(cell << 8) | 0x32`; then window ids 8–19 and 40–45. The click handler `0x00422864` dispatches:

| Hotspot | Box | Handler |
|---|---|---|
| 8, 10, 11, 12 | helmet, shield, armour, bow | `0x0041E500(4 / 5 / 3 / 1)` |
| 9 | hand (axe or blade) | `0x0041E500(slot)`, slot from `0x00422CB4` (`gamestate.HandSlot`): holding, category 0 → 0 else 2; empty hand, no axe → 2, axe mode → 0, blade present → 2, else 0 |
| 13 | ammunition (`+0xCC`) | `0x0041E500(12)` |
| 14–18 | jewellery `+0xB2`[0–4] | `0x0041E778(id)` |
| 19 | in-hand box | `0x00422D50`: `0x0041E96C` (put in / take out) or herb use on the boxed item (`0x0041DB64`); misc use `0x00414E64` for the torch only ([potions](konung_potions.md#controls)) |
| 40–44, 45 | raise buttons, auto-level toggle | [raise column](#raise-column-verified) |
| bag cell | `+0x72`[(cell + scroll) & 31] | `0x0042342A` |

**The hand** (`gamestate.Hand`): item `0x00A3E528`, source `0x00A3E524` (= source array length: 6 equipment, 32 bag, 5 jewellery, 1 ammunition), entity `0x00A3E520`, backups `0x00A3E52C` (equipment), `0x00A3E538` (bag), `0x00A3E578` (jewellery). Cursor mode `0x00A3E790` = 4 when empty; picking up sets it by category (`0x00425104`, table `0x00425170`): 0, 2 → 10 (large sword); 1, 12 → 2 (arrow); 3–5 → 0 (crate); 9 → 8 (globe with hand); others → 9 (starburst). Over the HUD and the window the cursor is GRAPH.RES Type 5 sprite number `0x00A3E790` (`0x00428AC0`).

* **Lift** (`0x0041E210(e, n, item)`): find the item among the first *n* entries of the source and clear it. A bag item is removed and the bag closed up; the bag backup is the closed-up bag with the item appended after the last item.
* **Return** (`0x00429194`): restore the equipment or jewellery backup; a bag item goes to the **front** of the bag (backup minus the appended item, shifted by one); ammunition is restored. The stance toggle, weapon select and window toggle call it first.
* **Bag insert** (`0x00424918`): if any of the 32 slots is free, shift the bag right by one and store at index 0; otherwise fail without storing.
* **Equipment** (`0x0041E500(slot)`): empty hand → lift the slot's item. Holding → if the entity is a creature or the item's category ≠ slot, return it; if the character can't use it (`0x00418364`), show «Предмет не доступен для использования» (`0x0045C17C`) and return it; otherwise the old item goes through bag insert (its result is ignored, so with a full bag the old item is lost), the new item is stored and `0x0041C750` recomputes the stats.
* **Jewellery** (`0x0041E778`): box 0 takes category 6, boxes 1–2 category 7, boxes 3–4 category 8; unusable or wrong items return silently; otherwise as equipment.
* **Bag** (`0x0042342A`): empty hand → lift the clicked item, unless it is the in-hand box's item (`0x004234A0`). Holding → `0x004246F8`: if carried weight + the item's weight (`0x0041B164`, same per-category rule as the bag) > max weight, «Превышен вес» (`0x0045C57C`); if the bag is full, «Мешок у тебя не резиновый!» (`0x0045BD7C`); either way the item returns. Otherwise it goes to the front of the bag. The clicked cell doesn't matter.

**Messages** (`0x0042F9B8`): small font at (view left + 10, view bottom + 5), the status-line position; counter `0x00A3E84C` = 12 main-loop passes. The port shows them for 12 animation steps. Sounds: [sound](konung_sound.md).

## Raise column (verified)

The rules are `0x00410B08` (`Game.RaiseCost`) and `0x0041079C` (`Game.GainExperience`), see [combat: experience and levels](konung_combat.md#experience-and-levels-verified). Costs, caps and the level check use the **base** stats (`+0x56`, `+0x5A`, `+0x60`, `+0x64`), not the effective ones. A new hero starts above the level-1 caps of its non-favoured stats, so at first only the favoured stat's button shows ([why](konung_combat.md#experience-and-levels-verified)).

**Drawing** (`0x0042F1F5`–`0x0042F48F`, table `0x00465754`, rows of `(x640, y640, x800, y800)`, `resfmt.CharWindow.Raise`/`AutoLevel`; 800×600 values):

| Row | Id (`0x00410B08`) | Point | Hotspot | Click |
|---|---|---|---|---|
| 0 level | 0 | (722, 89) | 40 | `0x004230D3`: `+0x33` += 1, free |
| 1 Хар | 3 | (716, 138) | 41 | `0x0042310E`: base `+0x5A` |
| 2 Сил | 5 | (716, 158) | 42 | `0x00423167`: base `+0x60` |
| 3 Лов | 2 | (716, 181) | 43 | `0x004231C0`: base `+0x56` |
| 4 Вын | 6 | (716, 203) | 44 | `0x00423219`: base `+0x64` |
| 5 toggle | — | (644, 379) | 45 (shaped like the icon) | `0x00423272` |

* **«Свободный опыт»** (`+0x38`): small font, x = row 0 x + (width of INTR slot 171 − text width)/2 (the width is the image's first word, `0x0043FE18`), y = the 25th number's y (63). The 25-number table's x for it (700) is unused.
* **Rows 0–4** (jump table `0x0042F328`): if `0x00410B08(e, id)` = 0 nothing is drawn. Otherwise a stat row prints the cost (small font) at the point and draws INTR slot 171 (a 10×10 marker, `resfmt.RaiseButtonSlot`) at (x + 24, y + 2); the level row draws only the marker, at (x, y + 2). The labels («Поднять уровень:», «поднять») are part of the art.
* **Toggle**: INTR slot `0xAA` («АВТО» lit) when `+3` bit `0x02` is set, else `0xA9`, top-left at row 5.

**Clicks** (left button, handler `0x00422864`, any hand state): hotspots 40–44 call `0x00410B08` again and do nothing on 0; a stat moves its cost from `+0x38` to `+0x3C` and its base goes up by one. The level row is free. Hotspot 45 clears a set bit `0x02`; otherwise it sets it and calls `0x0041079C(e, 0)`, which spends the free experience at once. Every change redraws the window (`0x0042DBBC(2)`, which recomputes the stats) and the bag strip. No hover text (`0x004244B3` has no case for 40–45).

Port: `gamestate.Game.RaiseCosts`, `Raise`, `ToggleAutoLevel` (`levels.go`, test `TestRaiseGame0`); drawing `charWindow.drawRaise`, clicks `startScene.windowClick`. Validation: `resfmt.TestItemTablesAndCharWindow` checks that each drawn button falls on its hotspot; a composite of slot 2 with the markers puts them next to the printed labels.

## Right click and the quick bag

**Right click on a bag cell** (`0x00423E26`, jump table `0x00423F28` by category; `gamestate.Game.UseBagItem`). A held item is returned first (`0x00423919`).

| Category | Action |
|---|---|
| 0–5, 12 | equip at once: lift from the bag, then `0x0041E500(category)` (with all its checks) |
| 6 | lift, then jewellery box 0 (`0x0041E778(14)`) |
| 7, 8, 10 | nothing |
| 9 | use the herb on the character, `0x0041DB64(e, 0, item)` ([potions](konung_potions.md)) |
| 11 | use the misc item, `0x00414E64(e, 0, item)` (jump table `0x00415ADC` by type; ported: the shovel, type 24, [dig mode](konung_ground_items.md#hover-and-pick-up); the torch, type 31, [loot highlight](konung_potions.md#loot-highlight-0x00a3e758)) |

**Quick bag (`` ` ``/`~`, VK 0xC0, `0x004371D2`).** Only with no window open (`0x00A3E740` = 0). It toggles the bag strip (`0x0042F50C`) for the first selected member: if the strip already shows that member (`0x00A3E730`) it is hidden, otherwise the scroll resets and the strip is drawn. The strip covers the bottom of the game view, just above the status bar. Its cells and arrows work as in the window (hotspot routine `0x004395E6` tests `0x00A3E730`, not the window). The window toggle saves and restores the strip state (`0x00A3E760`), and the shovel leaves the strip open. While an item is held, a left click on the map drops it ([ground items](konung_ground_items.md#hover-and-pick-up)). Port: `charWindow.quick`.

## Hover texts (verified)

The hover handler (`0x00423FBA`, switch on the hotspot at `0x004244B3`) writes the status line. With no window open, a pile and then a character under the mouse come first ([ground items](konung_ground_items.md)). Otherwise:

* **Panel buttons** (hotspots 1–7): «Сменить оружие (1, 2, 3)», «Достать/Убрать оружие (Пробел)», «Карта (M)», «Все ко мне (F)», «Атаковать (A)», «Информация к размышлению (Q)», «Панель персонажа (I)» (`0x0045E17C`, `0x0045D77C`, `0x0045DF7C`, `0x0045D97C`, `0x0045D57C`, `0x0045DB7C`, `0x0045DD7C`).
* **Items**: doll boxes 8–18 (helmet, hand, shield, armour, bow, ammunition, jewellery; the hand box shows the blade if it is the active weapon or there is no axe, `0x00424375`), the in-hand box 19 (`0x00A3E7A0`) and bag cells (`0x004241A1`, skipped while windows 1, 2 or 5 are open) call **`0x004305E4(item)`**.

**Item description (`0x004305E4`, `gamestate.Game.ItemDescription`).** It sets the message counter `0x00A3E84C` to 12, redraws the bottom bar, builds the text in `0x00A3E120` by category (jump table `0x0043138C`) and draws it wrapped (`0x0042CBBC`, small font, at (view left + 10, view bottom + 5), width view right − view left − 15).

| Category | Text |
|---|---|
| 0–5 | name, «, убойная сила » (0–2) or «, броня: » + uint16 `+0x2C`, «, износ » + trunc(float `+4`, at least 1) + «/» + trunc(float `+8`), «, вес » + `%4.2f` of `+0x38`·0.001, «, отравление: » + item `+0xC` (0 and 2 only, when ≠ 0), the special property's name, the enchantment, «, требует: » + stat name + «:» + minimum (`+0x30`/`+0x32`, when the stat is ≠ 0) |
| 12 | «Колчан стрел: » + count (`+4`) + name, damage, weight × count, poisoning, special property |
| 6–8 | name, weight (`+0x7A`), enchantment |
| 9 | name, «, концентрация » + `%5.2f` of float `+4` (when record `+0x78` < 0), weight |
| 11 | name, weight |
| 10, > 12 | nothing (the buffer keeps its previous text) |

An enchantment adds, per effect with a stat, the stat's name from `0x00465A24` (5 bytes each, with a leading space: « Хар», « Лов», « Хар», « Здр», « Сил», « Вын», « Брн», « Удр», « Вер»), then «+» for a positive stacking value or «=» for a replacing one, then the value. Its requirement follows as «, требует: » + name + «:» + minimum. No separator precedes the first effect («вес 0.20 Вын+1»). Unidentified items (gear 0–8, misc 11) show only «Неведомое оружие» (categories 0, 2), «Неведомый лук» (1) or «Неведомый предмет»; arrows show the count and « неведомые стрелы». Herbs have no unidentified variant.

Validation: `gamestate.TestItemDescriptionGame0` checks 14 `GAME.0` items, e.g. Volk's knife «Нож, убойная сила 60, износ 16/16, вес 0.20, требует:  Сил:3» and «Богатырские доспехи, броня: 200, износ 110/110, вес 5.00 Лов=100 Вын-20, требует:  Хар:35, требует:  Вын:95». `GAME.0` has no unidentified or poisoned items; those branches follow the code only.

Port: `startScene.statusText` (a click message still wins for its short lifetime).

## Derived values

| Routine | Port | Rule |
|---|---|---|
| `0x0041C750` | `Game.Recompute` | Sum the enchantments (table `0x00458590`, 16 B: `+2` required stat, `+3` minimum, three `{stat, mask, int16 value}` at `+4/+8/+0xC`) of worn gear, then jewellery (skipping items with `+0xE` bit 0), then the character's own `+0xD2`, `+0xD3`. Mask 1 stacks the value; mask 0 replaces it and locks the stat. Writes effective Лов/Хар/Сил/Вын from their bases (`+2` each), HP (its own base, capped at 1600; bonus ×16), and bonuses `+0xC4` armour, `+0xC6` attack, `+0xC8` stat 9, `+0xD1` = stack flags. |
| `0x0041B244` | `Game.StatValue` | requirement stat ids: 1/3 Хар, 2 Лов, 4 HP/16, 5 Сил, 6 Вын, 7 armour, 8 attack, 9 hit chance `0x0041B4CC(e, 1)` (`Game.HitChance`, [combat](konung_combat.md#formulas-verified)) |
| `0x0041B898` | `Game.CarriedWeight` | grams: gear table `+0x38` for equipment, jewellery `+0x7A` for `+0xB2`, bag by category (≤5 gear, 6–8 jewellery, 9 herbs, 11 misc `+0x7A`, 12 gear × count, 10 and others 0), ammunition `+0xCC` × count |
| `0x0042EF48` | `MaxWeight` | Вын·1000/3 + 10000 |
| `0x0041AC1C` | `Game.Armour` | `+0x4E` + gear `+0x2C` of slots 3–5; bonus `+0xC4` added if `+0xD1` bit 0, else replaces |
| `0x0041AF48(e, kind)` | `Game.Attack` | creatures: trunc(Сил·0.04·HP·0.0625). Axe or blade active: gear `+0x2C` (or the special property's value, record `0x0046238C` `+0x2C`, when item `+2` ≥ 0 and its kind matches) + trunc(Сил·0.04·HP·0.0625). Otherwise trunc(Сил·0.02·HP·0.0625). Bonus `+0xC6` by `+0xD1` bit 1. |
| `0x0041ADE4` | `Game.SheetDamage` | window only, with ammunition, bow mode and a bow: bow `+0x2C` × arrow `+0x2C`, same bonus rule |
| `0x00418364` | `Game.CanUse` | gear (categories ≤5, 12) and jewellery (6–8): the enchantment's requirement; gear also its type's (`+0x30` stat id, `+0x32` minimum). Others usable. Returns 1 when `0x00A3E768` ≠ 0 (not traced; the port assumes 0). |

`GAME.0` check (`gamestate.TestCharSheetGame0`): Volk weight 2.8, armour 10, attack 120 (knife 60 + 60); villager 355 weight 6.8, armour 52 (12 + 24 + 4 + 12), attack 120 (club 80 + 40). No start item is enchanted, so `Recompute` changes nothing in `GAME.0`.

## Port

`internal/game/inventory.go` (`charWindow`, `bagStrip`) draws the window after the HUD every frame; `startScene.windowClick` routes clicks to `gamestate` (`ClickEquipment`, `ClickJewellery`, `ClickBag`, `Hand.Return`). The worn equipment is part of the walker's look, so the character's overlays update. While it is open, clicks in the view don't move the hero and the hover status line is off. The game redraws windows only on change; whether the map keeps animating underneath is not traced.

## Open questions

* `0x00A3E768` (god mode, see [combat](konung_combat.md#open-questions)).
* Misc items on the in-hand box or a portrait (cursor mode 9, `0x00414E64`). (Background tiling, **T**, is traced and ported: [settings](konung_settings.md#background-tiling-t).)
