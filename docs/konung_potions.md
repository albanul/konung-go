# Herbs and potions (знахарство)

Category 9 items are herbs and potions (herb table `0x004594B0`, 128-byte records; see [GAME.N](konung_game_format.md#inventory-and-items-verified)). Potions are made by pouring one herb onto another in the character window and are drunk from the bag or the portrait. Making them trains the **healing skill** `+0x48` (знахарство, sheet number 15), which in turn sets the strength of new potions.

Status: mixing `0x0041BC24` and use `0x0041DB64` traced and ported (`gamestate/potions.go`); the recipe table is read from the exe (`resfmt.ExeTables.Recipes`). Checked against `GAME.0` (`gamestate.TestMixGame0`, `TestPotionTimerGame0`, `TestInHandBoxGame0`).

## Herb types

The item's float `+4` is its **concentration** *f* (the description shows «, концентрация» when record `+0x78` < 0, i.e. types 5–13). Weight `+0x7A`: 1000 g for ingredients, 100 g for the jar, 500 g for potions.

| Type | Name | Use (`0x0041DB64`, jump table `0x0041E110` by type − 4) |
|---|---|---|
| 0 | Ядовитое жало | ingredient (creature loot, *f* = 1.0) |
| 1 | Земляной орех | ingredient (ground piles, grows back) |
| 2 | Белый корень | ingredient (ground piles, grows back) |
| 3 | Пустая банка | ingredient; what every potion leaves behind |
| 4 | Непонятная смесь | HP halved (C division), at least 1. Applies to the user even when used on a boxed item |
| 5 | Лечебный бальзам | HP = trunc(*f*·160 + HP), at most 1600 |
| 6 | Яд | on a character: poison `+0x50` += *f* (truncated). On a boxed axe, blade or arrows (categories 0, 2, 12): item poisoning `+0xC` += *f*. Other targets refuse |
| 7 | Масло | boxed arrows only, *f* ≥ 10: item `+2` (special property) = 0 |
| 8 | Противоядие | if poisoned: poison = max(0, trunc(poison − *f*·10)) |
| 9 | Брага | *n* = effective Лов `+0x54` / 3, lowered to trunc(*f*) if *f* is smaller. Refused while a potion timer runs. Base Лов −3*n*; Хар, Сил, Вын +*n*; timer 30*n* |
| 10 | Зелье | refused while a timer runs. *n* = trunc(*f*): base Лов, Сил, Вын +*n*; timer 30*n*; HP += *f*·160 (≤ 1600) |
| 11 | Чистая слеза | timer 30*n*, hit bonus `+0xCA` = trunc(√*n*·0.1) (0 for any *n* < 100); [loot highlight](#loot-highlight-0x00a3e758) until the next map load |
| 12 | Элексир Мудрости | refused for *n* < 6. Free experience trunc(√(*n*−5)·100) (`0x0041079C`); timer 60*n*; base Сил −5*n* (≥ 1); HP −160*n* (≥ 160) |
| 13 | Живая Вода | *f* ≥ 10: HP = 1600; a dead user (`+4` bit `0x80`) is revived by `0x00414B3C` |

* **Timer.** `+0xCE` counts upkeep passes (16 ticks each). The timed potions first save the base stats into `+0xBC`…`+0xC2` if no timer runs. Брага and Зелье set the hit bonus `+0xCA` to 0. When the timer reaches 0, upkeep restores the base stats and clears `+0xCA` ([combat](konung_combat.md#upkeep-poison-and-respawn-verified)). While it runs, nothing can be raised (`0x00410B08`) and `+0xCA` is added to the hit chance.
* **After a use** the potion's type becomes 3 (Пустая банка). With a boxed target the held item (if not used up) goes to the front of the user's bag through `0x00424918`, without a weight check. Then sound 9 plays and, for a party member, the portrait, window and strip are redrawn. The routine returns 1; ingredients (types 0–3) and refusals return 0 and change nothing.
* All float → int conversions truncate (`0x0043FE4C`, `frndint` with RC = chop); `0x0043FECE` is `sqrt`.

## Mixing (`0x0041BC24`)

`0x0041DB64(e, target, held)` with a herb as *target* calls `0x0041BC24(e, target, held)`. The **recipe table** at `0x00450E54` has 33 rows of 6 bytes: target type, held type, new target type, new held type (`0xFF` = used up, category set to `0xFF`), `int8` skill gain, concentration mode. The first row matching (target type, held type) applies. Let *s* = healing `+0x48` · 0.01.

| Mode | Target concentration |
|---|---|
| 0 | *s* |
| 1 | target + *s* |
| 2 | target + held |
| 3 | min(target, held) |
| 4 | held + *s* |

The result is capped at 15.0 (`0x00450134`), the two types are rewritten, and `+0x48` += gain (at most 500). **No matching row:** the target becomes 4 (Непонятная смесь) and the held herb is used up; there is no skill gain.

| Target + held (either order where listed twice) | Result (held becomes) | Skill | Mode |
|---|---|---|---|
| Банка + Жало / Жало + Банка | Яд | 3 | 0 |
| Яд + Жало; Жало + Яд | Яд | 3 | 1; 4 |
| Банка + Орех / Орех + Банка | Масло | 2 | 0 |
| Масло + Орех; Орех + Масло | Масло | 2 | 1; 4 |
| Банка + Корень / Корень + Банка | Лечебный бальзам | 0 | 0 |
| Бальзам + Корень; Корень + Бальзам | Лечебный бальзам | 0 | 1; 4 |
| Бальзам + Бальзам | Лечебный бальзам | 1 | 2 |
| Яд + Яд, Масло + Масло | same | 3, 2 | 2 |
| Бальзам + Яд | Противоядие (Банка) | 3 | 3 |
| Бальзам + Масло | Брага (Банка) | 3 | 3 |
| Бальзам + Брага | Зелье (Банка) | 0 | 3 |
| Яд + Масло | Чистая слеза (Банка) | 3 | 3 |
| Яд + Брага | Элексир Мудрости (Банка) | 4 | 3 |
| Масло + Противоядие | Живая Вода (Банка) | 4 | 3 |
| any potion 8–13 + itself | same | 3 (8–11), 4 (12, 13) | 2 |

The two-potion recipes also exist with target and held swapped. Rows 21–32 cover these and the herb-onto-jar/potion cases.

So the skill sets the starting strength: a jar plus a root gives a balm of *s* (Volk, healing 101: 1.01), and further roots add *s* each. Doubling (potion + same potion) adds the concentrations and trains the skill most. `GAME.0` has no empty jars; the first one comes from drinking.

## Controls

* **In-hand box** (window hotspot 19, item `0x00A3E7A0`, drawn centred at `0x00465864`, `0x0042E5C0`): the box only *points at* an item that stays where it is. Every draw skips the boxed item: the bag strip leaves a plain, untinted empty cell (`0x0042F72F`, `0x0042F7FA`), and the equipment, ammunition and jewellery icons are not drawn (`0x0042E29F`, `0x0042E3EF`, `0x0042E4B7`). It is described only over the box (bag hover `0x00424294`, hand `0x004243C6`, ammunition `0x00424403`). So it looks as if it had moved into the box. A bag cell holding it can't be lifted (`0x004234A0`). The box is cleared when the window opens (`0x004246D1`) or the quick strip is shown (`0x00437253`), but not when the window closes. The item then shows in its cell again; it never left it. Click handler `0x00422D50`:
  * box empty or hand empty → `0x0041E96C`: an empty hand lifts the boxed item from its place (ammunition, equipment, jewellery, else the bag) and empties the box. A held item is put back from its backup (a bag item at the **end** of the bag) and the box points at it.
  * holding a herb (cursor 8) → `0x0041DB64(e, box, held)`: mixing, or poison/oil on a boxed weapon or arrows; the hand empties.
  * holding a misc item (cursor 9) → `0x00414E64(e, box, held)` (not ported).
  * otherwise, or refused → «Предмет не доступен для использования» (`0x0045C17C`) and the item returns.
* **Portrait** (hotspots 30–39, `0x00422E21`) while holding an item: on a dead member or the holder itself → `0x0042054C(member)`: a herb is used on the member (`0x0041DB64(m, 0, held)`), and the jar returns to the bag front (`0x00429194`). A misc item goes to `0x00414E64` (not ported). Anything else shows the message and returns the item. On another living member → `0x004246F8(member)` (give: weight and bag-full checks).
* **Right click on a bag herb** (`0x00423E26` case 9): `0x0041DB64(e, 0, item)` on the window's character; the jar stays in its cell; a refusal is silent.

Port: `gamestate.Game.Mix`, `UseHerb`, `UseMisc` (torch), `ClickBox`, `ClickPortrait`, `UseBagItem`; `Hand.Box`. UI: `startScene.windowClick` (hotspot 19), `startScene.click` (portraits), `charWindow.draw` (box icon), `statusText` (box hover).

## Loot highlight (`0x00A3E758`)

Status: traced and ported (`Game.Lit`, `resfmt.LightsGlow`, `startScene.drawPiles`); glow map checked against `LIGHTS.RES` (`resfmt.TestLightsGlow`) and a rendered preview.

* **Set** by Чистая слеза (`0x0041DF12`) and by **«Факел»** (the torch, misc type 31: case 31 of `0x00414E64`'s jump table `0x00415ADC`, at `0x004156E6`). The torch is not used up: the common tail `0x00415BD3` frees nothing, plays sound 10 and returns 1. It works from the bag (right click), on the portrait (`0x0042054C`, cursor 9, then returned to the bag front) and on the in-hand box (then inserted at the bag front). `GAME.0` has two torches (items 1021, 1038).
* **Cleared** by the map loader (`0x0043C11B`), so the light lasts until the next map change. Port: `buildScene` clears `Game.Lit`.
* **Draw** (`0x00426554`, in the pile pass over visible cells with grid bit `0x2000`, piles from the active list `0x007A6CDC`, buried ones skipped): before each pile, the glow map is blended at (pile x − 32 + w/2, pile y − 21 + h/2), centred on the pile image, via `0x0043DE4D`. The pile is then blitted with its palette from `[0x0051ECB4]` instead of the light-tinted one (`0x0043ED09`); the port draws piles untinted anyway.
* **Glow map**: `LIGHTS.RES` (one file, unrelated to `LIGHTS.N`; loader `0x0043B7FC`, size 145472 = `0x1000` + `0xAC0` + 19 × `0x1C80`). Bytes `0x1000`–`0x1ABF` are the 64×43 intensity map (`0x00625518`): a soft ellipse, values 0–43, 2334 non-zero. The 19 light masks at `0x1AC0` (`0x005B1010`) are decoded in [day and night](konung_daylight.md#lit-terrain-path-0x004257a9). The first `0x1000` bytes (`0x005B0010`, a 64 × 64 glow used for list-2 sprites, `0x004274C7`) are not.
* **Blend** `0x0043DE4D(dst, map, x, y, 64, 43)`, clipped to the view: for each non-zero intensity *k*, the RGB555 path replaces red and green by `c + trunc((31 − c)·f_k)` and keeps blue (tables `0x004A17F0` red, `0x0047CB2C` green, built at `0x0043B27C`: 100 rows, *f* accumulated in float32 steps of 0.01 from `0x00450721`). So it is a yellow brightening of up to 43 % (`resfmt.Brighten555`). The RGB565 path (`0x00A3E824` = 16) uses the 565 tables built at `0x0043ADF1`.
* **Port**: `resfmt.GlowMask` holds (*f*, *f*, 0) per pixel, and `game.glowBlend` draws it with dst' = src + dst·(1 − src) per channel, which is the same lerp without the 5-bit truncation.

## Open questions

* `0x00414B3C` (revival by Живая Вода); the port's party has no dead members (the hero's death ends the game).
* Misc items other than the torch used on the box or a portrait (`0x00414E64`, cursor 9).
* The rest of `LIGHTS.RES` (`0x1000` bytes to `0x005B0010`, 19 × `0x1C80` to `0x005B1010`).
* Herb sources other than the ground, loot and the start bag: the healer and merchants (dialogue).
