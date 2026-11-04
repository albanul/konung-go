# Character window (inventory)

Window 3: the character sheet with equipment, jewellery, stats and the bag strip. Opened and closed with **I** or HUD button 6 (the bag, hotspot 7). Both call `0x00424610`, which works when no window or window 3 is open (`0x00A3E740` = 0 or 3). It presses button 6 with image 13 (`0x004290AC(6, 13)`) and toggles the window. It shows the first selected character (`0x0083B93C[0]` → `0x00A3E688`), resets the bag scroll `0x00A3E840`, then draws the window (`0x0042DBBC(2)`; the dispatcher's jump table at `0x0042F4C8` is indexed with *argument + 1*, so this is case 3, `0x0042E04C`) and the bag strip (`0x0042F50C(entity, 0)`).

Status: drawing verified against code and rendered PNGs. Values verified against hand calculation for `GAME.0` entities 0 and 355. Dragging items (`0x00A3E7A0`) and equipping are not ported.

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
| Held item | `0x00A3E7A0` (being dragged), centred on `0x00465864`; not ported |
| 25 numbers | small font `0x005A31F4`, top-left at `0x00465874 + i·16`, see below |
| Raise column | `0x0042F2B6`: XP pool and level/stat-raise costs (`0x00410B08`) with INTR slot 171; not ported |
| Stance icon | INTR slot `0xAA` if `+3` bit `0x02`, else `0xA9`; not ported |

**The 25 numbers** (`gamestate.CharSheet`, constants `Sheet*`): 0 `+0x3C` Опыт · 1 `+0x33` Уровень · 2–5 effective Хар/Сил/Лов/Вын (`+0x58`, `+0x5E`, `+0x54`, `+0x62`, column «сейчас») · 6 Здоровье = (HP + 15)/16 · 7–10 base values (`+0x5A`, `+0x60`, `+0x56`, `+0x64`, «базовая») · 11 Меткость `+0x44`/100 · 12 Фехтование `+0x40`/100 · 13–17 skills `+0x4A`, `+0x42`, `+0x48`, `+0x4C`, `+0x46` (identification, trade, healing, smithing, building) · 18 Текущий вес · 19 Максим. вес (both `%4.1f` of grams·0.001) · 20 Отравление `+0x50` · 21 Броня · 22 Удар · 23 Золото `+0x34` · 24 Свободный опыт `+0x38`.

## Bag strip (`0x0042F50C`)

A row of cells along the bottom of the game view showing the bag (`+0x72`, 32 slots). Geometry is computed when the INTR file loads (`0x0042D2D8`–`0x0042D396`):

* cell width `0x00A3E860` = width(slot 17) − 1 = 69; arrow size `0x00A3E864`/`0x00A3E784` = slot 0x12's 20×70;
* strip top `0x00A3E88C` = 555 − 70 + 1 = 486; space between the arrows W = 659 + 2 − 2·20; visible cells = W / 69 = 9; first cell x `0x00A3E870` = 139 + 20 + (W − 9·69)/2 = 159.

Arrows: left at (139, 486), slot 0x12 (0x13 pressed); right at (780, 486), slot 0x14 (0x15 pressed). The argument picks the pressed arrow (−1 left, +1 right). Each cell draws INTR slot 17, possibly tinted, and the item icon (by category, `0x0042C870`) centred: x + (69 + 1 − w)/2, y + (70 + 1 − h)/2. Bag index = (scroll + k) & 31.

**Scrolling.** Hotspot 120 (left arrow) decrements the scroll, not below 0 (`0x004235E4`). Hotspot 121 (right arrow) increments it only if `0x0041EB9C` reports an item in the bag slot just past the last visible cell (`scroll + visible < 32`) (`0x0042367F`).

**Cell tint (`0x0042C914`).** The first matching rule wins: the character can't use the item (`0x00418364`) → tint (24, 0, 0); item `+0xE` bit 0 → (15, 15, 0); enchanted (item `+1` ≥ 0) → (0, 12, 0). `0x0043FD18(r, g, b)` tints each 5-bit channel by n %: c + trunc((31 − c)·n/100); negative n darkens. `resfmt.TintPalette` ports it.

## Derived values

| Routine | Port | Rule |
|---|---|---|
| `0x0041C750` | `Game.Recompute` | Sum the enchantments (table `0x00458590`, 16 B: `+2` required stat, `+3` minimum, three `{stat, mask, int16 value}` at `+4/+8/+0xC`) of worn gear, then jewellery (skipping items with `+0xE` bit 0), then the character's own `+0xD2`, `+0xD3`. Mask 1 stacks the value; mask 0 replaces it and locks the stat. Writes effective Лов/Хар/Сил/Вын from their bases (`+2` each), HP (its own base, capped at 1600; bonus ×16), and bonuses `+0xC4` armour, `+0xC6` attack, `+0xC8` stat 9, `+0xD1` = stack flags. |
| `0x0041B244` | `Game.StatValue` | requirement stat ids: 1/3 Хар, 2 Лов, 4 HP/16, 5 Сил, 6 Вын, 7 armour, 8 attack, 9 `0x0041B4CC` (not traced) |
| `0x0041B898` | `Game.CarriedWeight` | grams: gear table `+0x38` for equipment, jewellery `+0x7A` for `+0xB2`, bag by category (≤5 gear, 6–8 jewellery, 9 herbs, 11 misc `+0x7A`, 12 gear × count, 10 and others 0), ammunition `+0xCC` × count |
| `0x0042EF48` | `MaxWeight` | Вын·1000/3 + 10000 |
| `0x0041AC1C` | `Game.Armour` | `+0x4E` + gear `+0x2C` of slots 3–5; bonus `+0xC4` added if `+0xD1` bit 0, else replaces |
| `0x0041AF48(e, kind)` | `Game.Attack` | creatures: trunc(Сил·0.04·HP·0.0625). Axe or blade active: gear `+0x2C` (or the special table `0x004623B8` value when item `+2` ≥ 0 and its kind matches) + trunc(Сил·0.04·HP·0.0625). Otherwise trunc(Сил·0.02·HP·0.0625). Bonus `+0xC6` by `+0xD1` bit 1. |
| `0x0041ADE4` | `Game.SheetDamage` | window only, with ammunition, bow mode and a bow: bow `+0x2C` × arrow `+0x2C`, same bonus rule |
| `0x00418364` | `Game.CanUse` | gear (categories ≤5, 12) and jewellery (6–8): the enchantment's requirement; gear also its type's (`+0x30` stat id, `+0x32` minimum). Others usable. Returns 1 when `0x00A3E768` ≠ 0 (not traced; the port assumes 0). |

`GAME.0` check (`gamestate.TestCharSheetGame0`): Volk weight 2.8, armour 10, attack 120 (knife 60 + 60); villager 355 weight 6.8, armour 52 (12 + 24 + 4 + 12), attack 120 (club 80 + 40). No start item is enchanted, so `Recompute` changes nothing in `GAME.0`.

## Port

`internal/game/inventory.go` (`charWindow`, `bagStrip`) draws the window after the HUD every frame. It has no click handling beyond the arrows. While it is open, clicks in the view don't move the hero and the hover status line is off. The game redraws windows only on change; whether the map keeps animating underneath is not traced.

## Open questions

* The raise column (`0x00410B08`: per-face stat caps `0x0045B10A`, costs `0x0045B010`), the XP-pool text, and the stance icon slots 0xA9/0xAA.
* Item `+0xE` bit 0, `0x00A3E768`, `0x0041B4CC` (stat 9, `+0xC8`).
* Dragging and equipping (`0x00A3E7A0`, hotspots 8–19 cover the doll boxes by position), and the background tiling flag `0x00A3E744` (`0x0043141C` tiles INTR slot 5).
