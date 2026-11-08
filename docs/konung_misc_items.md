# Misc items (category 11)

How the quest items, books, orbs, keys and other misc items (category 11, type table `0x0046053C`, 128-byte records, CP866 name at +0) are used. Status: every case of the dispatcher is **verified** against `konung.exe` and **ported** (`gamestate.UseMisc`, `internal/gamestate/misc.go`; the scene side is `game.sceneWorld`, `internal/game/misc.go`).

## Entry points

`0x00414E64(e, target, item)`: `e` uses `item` on the item `target` (0 = none). It refuses (returns 0, changes nothing) unless item `+0` = 11. The type (`+3`) indexes the jump table `0x00415ADC`. Types above 53 (`cmp 0x35; ja` at `0x00415BBA`) and types without a case go to `0x00415ACD` (return 0).

| Caller | Target | After a use |
|---|---|---|
| Bag right click `0x00423E26` (category 11 case `0x00423EDB`) | 0 | nothing; a refusal is silent. The port: `Game.UseBagItem`; after the shovel the hand becomes `CursorDig` |
| In-hand box, hotspot 19 (`0x00422D50`), cursor mode 9 | the boxed item `0x00A3E7A0` | cursor mode 4, so the shovel's dig cursor does not survive. A refusal shows «Предмет не доступен для использования» and returns the item (`0x00429194(1)`). `Game.ClickBox` |
| Portrait, hotspots 30–39 (`0x0042054C`), on a dead member or the holder | 0 | a kept item returns to the holder's bag. `Game.ClickPortrait` → `useOn` |

**Common tail** `0x00415BD3`, after every case that does not refuse:
* with a target: a kept item (`+0` > −1) goes to the front of `e`'s bag (`0x00424918`, no weight check, silently lost when the bag is full); a used-up one (`+0` = `0xFF`) leaves `e`'s bag (`0x00421D24`);
* without one: a used-up item leaves `e`'s bag;
* sound 10 (`0x00415C6C`); for a party member, the portrait is redrawn (`0x0042D574`).

The return value is 1.

## Cases

Entity fields: skills `+0x48` healing, `+0x4C` smithing, `+0x42` trade, `+0x46` building, `+0x4A` curios; base stats `+0x5A` Хар, `+0x60` Сил, `+0x56` Лов, `+0x64` Вын. "Used up" = item `+0` = `0xFF`. Skill and stat words wrap at 16 bits; caps compare unsigned.

| Type | Name | Case | Effect | Used up |
|---|---|---|---|---|
| 0 | Потерянная карта | `0x00414E95` | a random location becomes known ([world map](konung_world_map.md#known-maps-0x00a3e0a8)) | yes |
| 1 | Труды Гиппократа | `0x00414F06` | healing +100, at most 500; message `0x0045C77C` «Ты открыл для себя секреты древних целителей» | yes |
| 2 | Чертежи древних пирамид | `0x00414F43` | building +5 (no cap); `0x0045C97C` «Ты познал секреты строительного искусства» | yes |
| 3 | Свиток кузнеца | `0x00414F64` | smithing +4 (no cap); `0x0045CB7C` «Ты познал секреты кузнечного дела» | yes |
| 4 | Трактат о торговле | `0x00414F85` | trade +10, at most 100; `0x0045CD7C` «Ты узнал много нового об искусстве торговли» | yes |
| 5 | Ягода Третий Глаз | `0x00414FC0` | curios +100, at most 100; `0x0045CF7C` «Ты узнал много интересного о неведомых вещах» | yes |
| 6 | Слова Воскрешения | `0x00414FFB` | revives every dead character on the map ([party](konung_party.md#revival-0x00414b3c-verified)) | yes |
| 7 | Дупликатор | `0x004150AD` | needs a target. The 16-byte item record of the target is copied over the duplicator's, so the duplicator becomes a copy of the boxed item and goes to the bag front | no (it is the copy) |
| 8 | Соколиная лапка | `0x004150CE` | base Сил +3 | yes |
| 9 | Свиток ведуна | `0x004150E1` | `0x0041BABC(e)`: every unidentified item `e` carries (item `+0xE` bit 0; equipment, ammunition, bag, jewellery) is identified, each adding 1 to the **hero's** curios, then capped at 100. The window and the strip are redrawn if they show `e` | yes |
| 10 | Яблоко познания | `0x0041513A` | 200 free experience (`0x0041079C`, auto-level applies) | yes |
| 12 | Вино | `0x00415156` | like «Брага»: n = base Лов / 3, at most 5. With the potion timer `+0xCE` at 0 the base stats are saved (`+0xBC`…`+0xC2`). Timer = 30n, `+0xCA` = 0, base Лов −3n, Хар/Сил/Вын +n. No refusal while a potion runs | yes |
| 13 | Великий магнит | `0x0041521A` | for every character (dead or alive) of every group on the map other than `e`'s: its gold `+0x34` goes to the hero, and its bag items, from slot 31 down, go to the front of `e`'s bag until the bag is full (then the next character) | yes |
| 14–19, 21 | Магический шар волхва … / Подземного города | `0x00415329`–`0x0041539B`, `0x004153D9` | `0x004106E0` jump (below) to the magus's map | no |
| 20 | Шар магического прыжка | `0x004153B0` | opens the [map window](konung_world_map.md#opening-and-closing), plays sound 10 once more | no |
| 22, 23 | Рукоять / Лезвие древнего меча | `0x004153EE`, `0x00415479` | needs the other part (category 11, type 23 / 22) as the target: 30 free experience, and the target becomes gear: category 2, type 27, `+1` = −1, `+2` = 14, condition `+4`/`+8` = the gear record's `+0x2E` (`0x0045A29E`), `+0xC` = 0 | yes |
| 24 | Лопата | `0x00415504` | window 0 (closed), cursor mode 7 ([dig mode](konung_ground_items.md#hover-and-pick-up)), sound 10 once more | no |
| 25–27, 41–43 | parts of the dragon's amulet | `0x0041552D` | needs a target: `0x00410010(e, target, item)` (below) | yes |
| 28 | Берестяная грамота | `0x0041556E` | quest flag 0 set (`0x00625068` bit `0x80`); `0x0045D17C` «Ты прочитал Берестяную Грамоту» | yes |
| 30 | Медная лампа с именем Короля скелетов | `0x0041558E` | every living character (not dead, action not 3/9) of the other groups on the map that are neither villages (`+0x1E` bit `0x80`) nor kind `0x53` dies: `+0x12` = 0, `+2` = 0, HP = 0, death action (3 in the stance, else 9), loot `0x00418510(m, 0, hero's level +0x33)`. No experience | yes |
| 31 | Факел | `0x004156E6` | the [loot highlight](konung_potions.md#loot-highlight-0x00a3e758) | no |
| 33 | Кукла | `0x004156F5` | every living character of the other groups on the map (villages included) gets enchantment 19 at `+0xD2` (stat 5 −5) | yes |
| 35, 37 | Свисток, Дудочка | `0x004157D2` | the party's enemy group `0x0041FD74` (below); unless it is a village, its living members with `+4` bit `0x40` (animals) and `+4` ∉ {`0x44`, `0x53`, `0x54`, `0x57`} run: `0x004101E4` (below), then a walk there with `+2` = 8 (`0x004142D4`) | no |
| 36 | Чаша воеводы | `0x00415914` | base Хар +10 | yes |
| 38 | Святые мощи | `0x00415927` | enchantment 70 at `+0xD3` (as dialogue script 276) | yes |
| 39 | Детские игрушки | `0x0041593C` | base Хар +5 | yes |
| 44, 52, 53 | Ключ Воды, Луны, Огня | `0x0041594F`, `0x00415A34`, `0x00415A7F` | on the key's map with `e` exactly on its cell, a `0x004106E0` jump; anywhere else nothing (still a use: sound, item kept) | no |
| 48 | Медное зеркало колдуна | `0x0041599A` | every pile record on the map with gold `+0x10` < 0 (buried) and every active pile with gold < 0 gets the gold negated: the buried piles come up | yes |
| 11, 29, 32, 34, 40, 45–47, 49–51 | Сын Луны, Медная лампа, Неразменный пятак, Заячий хвост, Амулет Дракона, Перо Жар-птицы, … | `0x00415ACD` | no use (32 counts as money in [trade](konung_trade.md)) | — |

### Jumps (`0x004106E0`)

`0x004106E0(map, row, col, facing)` sets the pending map change `0x00A3E810` = −map and the party group's `+0x08` = row, `+0x10` = col, `+0x0C` = facing. That is what taking a [map exit](konung_game_format.md#map-exits-verified) does, and the main loop changes the map the same way.

| Item | Condition | Map | Row | Col | Facing |
|---|---|---|---|---|---|
| 14 волхва Бережков | — | 28 Волхв у Бережков | 184 | 18 | 1 |
| 15 волхва Старого лагеря | — | 5 Волхв у Старого лагеря | 60 | 68 | 2 |
| 16 волхва Византийского лагеря | — | 10 Волхв у Византийского Лагеря | 147 | 66 | 1 |
| 17 волхва Черного бора | — | 20 Волхв у Черного Бора | 49 | 41 | 2 |
| 18 волхва Родников | — | 3 Волхв у Родников | 141 | 54 | 1 |
| 19 волхва Лесного лагеря | — | 40 Волхв у Лесного лагеря | 42 | 31 | 1 |
| 21 Подземного города | — | 43 Пещера Зверя | 18 | 24 | 2 |
| 44 Ключ Воды | map 50, `e` on (34, 11) | 46 Гнездо Дракона | 68 | 13 | 2 |
| 52 Ключ Луны | map 48, `e` on (23, 9) | 49 Гнездо Дракона | 106 | 17 | 2 |
| 53 Ключ Огня | map 49, `e` on (15, 17) | 50 Гнездо Дракона | 134 | 12 | 2 |

Port: `orbJumps`, `keyDoors`; `World.Jump` stores the exit and `startScene.miscRequests` takes it with `changeMap` after the click.

### Amulet (`0x00410010`)

The target must be category 11. Two parts, in either order, turn the **target** into the result. Then 100 free experience (`0x0041079C`), return 1. Any other pair returns 0.

| Parts | Result |
|---|---|
| 25 right + 26 left | 41 left and right |
| 25 right + 27 bottom | 42 right and bottom |
| 25 right + 43 left and bottom | 40 Амулет Дракона |
| 26 left + 27 bottom | 43 left and bottom |
| 26 left + 42 right and bottom | 40 |
| 27 bottom + 41 left and right | 40 |

### Enemy group (`0x0041FD74`) and flight (`0x004101E4`)

**`0x0041FD74(group)`** for the party (`+0x1E` bit `0x20`): the box is the leader's (`word[group+0]`) cell ± (W + W/2) rows and ± (C + C/2) columns, with W = `0x00A3E818` = 20 and C = `0x00A3E7DC` = 9 at 800×600. Among the groups on the map other than the party that are not villages (unless `+0x1D` = 1, alarmed) and not kind `0x53`, it picks the group whose living member inside the box is nearest to the leader (`0x00439944`). It returns 0 when there is none. Creature groups (bit `0x40`) take another branch: they return the party if a living party member is inside their leash. Port: `startScene.nearestEnemy` on the leader's cell → `sceneWorld.EnemyGroup`.

**`0x004101E4(e, &dr, &dc)`** picks `rand() % 4` (jump table `0x004103F4`):
* 0 / 1: dr = −2W / +2W. dc scans from −C up to C − 1 and stops at the first cell (row + dr, col + dc) whose fine-grid word `0x00505C70[row·0x140 + col·2] & 0xFFF` is 0.
* 2 / 3: dc = −2C / +2C. dr scans from −W up to W − 1 in the same way.

A negative row or column counts as blocked (the bounds helper `0x00447CAA` skips the test). The return value is 1 unless the scan ran out (dr = W or dc = C). Port: `gamestate.FleeOffset`. The same routine is the kind-`0x49` retreat after a creature's swing that did not kill (`0x004126BD`, [combat](konung_combat.md#creature-ai-verified)), which the port does not use yet.

## Port notes

* `gamestate.World` (`Game.World`, set by the scene every frame to `sceneWorld`) gives the map-wide cases the hero's live record, the party copies, live cells, the death and loot (`Kill`), walk orders (`Walk`), cell tests, the enemy group and the map change (`Jump`). With no world (tests without one), cases 9, 13, 14–19, 21, 30, 33, 35, 37, 44, 52 and 53 refuse.
* The shovel asks the scene to close the window (`Game.CloseWindow`; the bag strip stays). The jump orb and the words keep their request fields `OpenMap` and `ReviveAll`.
* A map change by an orb to the map the party is on rebuilds the scene like a map exit (`changeMap`). The original only re-places the party in that case (`0x004118DC`).

Validation: `gamestate.TestMiscTablesMatchExe` reads the jump table, the types without a use and the bound, the orbs' and keys' `push` arguments and compares, and the amulet compares from `konung.exe`. `TestUseMiscBooks`, `TestUseMiscCombine`, `TestUseMiscSelf`, `TestUseMiscMapWide` (magnet, doll, lamp, keys, orb, mirror), `TestUseMiscWhistle`, `TestUseMiscNoUse` and `TestFleeOffset` cover the behaviour through a fake `World`. In the scene, `game.TestMagicOrbJumps` (orb 18 → `KONUNG.3` at (141, 54), orb kept) and `TestShovelClosesWindow` run the real thing.

## Open questions

* What «Сын Луны», «Медная лампа» (plain), «Заячий хвост», «Перо Жар-птицы», «Клык белого волка», «Паутина странного паука», «Волшебная вода», «Ларец колдуна», «Камень» and the assembled amulet do. They have no use case, so they are presumably for dialogues ([dialogues](konung_dialogue.md)).
