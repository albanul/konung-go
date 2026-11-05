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
| 11 | Чистая слеза | timer 30*n*, hit bonus `+0xCA` = trunc(√*n*·0.1) (0 for any *n* < 100); sets `0x00A3E758` (below) |
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

* **In-hand box** (window hotspot 19, item `0x00A3E7A0`, drawn centred at `0x00465864`, `0x0042E5C0`): the box only *points at* an item that stays where it is. A bag cell holding the boxed item can't be lifted (`0x004234A0`). The window clears it when it opens (`0x004246D1`). Click handler `0x00422D50`:
  * box empty or hand empty → `0x0041E96C`: an empty hand lifts the boxed item from its place (ammunition, equipment, jewellery, else the bag) and empties the box. A held item is put back from its backup (a bag item at the **end** of the bag) and the box points at it.
  * holding a herb (cursor 8) → `0x0041DB64(e, box, held)`: mixing, or poison/oil on a boxed weapon or arrows; the hand empties.
  * holding a misc item (cursor 9) → `0x00414E64(e, box, held)` (not ported).
  * otherwise, or refused → «Предмет не доступен для использования» (`0x0045C17C`) and the item returns.
* **Portrait** (hotspots 30–39, `0x00422E21`) while holding an item: on a dead member or the holder itself → `0x0042054C(member)`: a herb is used on the member (`0x0041DB64(m, 0, held)`), and the jar returns to the bag front (`0x00429194`). A misc item goes to `0x00414E64` (not ported). Anything else shows the message and returns the item. On another living member → `0x004246F8(member)` (give: weight and bag-full checks).
* **Right click on a bag herb** (`0x00423E26` case 9): `0x0041DB64(e, 0, item)` on the window's character; the jar stays in its cell; a refusal is silent.

Port: `gamestate.Game.Mix`, `UseHerb`, `ClickBox`, `ClickPortrait`, `UseBagItem`; `Hand.Box`. UI: `startScene.windowClick` (hotspot 19), `startScene.click` (portraits), `charWindow.draw` (box icon), `statusText` (box hover).

## `0x00A3E758` (Чистая слеза, not ported)

Set by Чистая слеза and by a misc item (`0x004156E6`), cleared on map load (`0x0043C11B`). While set, the pile pass (`0x00426554`, over the active piles `0x007A6CDC` on cells with grid bit `0x2000`, skipping buried piles) draws a 64×43 image `0x00625518` (hypothesis: a glow) under each pile and blits the pile with a palette from `0x0051ECB4` instead of the light-tinted one (`0x0043ED09`). Hypothesis: ground items stand out until the next map change.

## Open questions

* `0x00414B3C` (revival by Живая Вода); the port's party has no dead members (the hero's death ends the game).
* The pile glow `0x00A3E758` and its image `0x00625518`.
* Misc items used on the box or a portrait (`0x00414E64`, cursor 9).
* Herb sources other than the ground, loot and the start bag: the healer and merchants (dialogue).
