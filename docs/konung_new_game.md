# New game: the character-creation screen «Выбор героя»

Menu item «Новая игра» opens the hero-creation screen (screen state `0x00A3E6D8` = 2). There the player picks one of three heroes, a class, a combat skill and a peaceful skill, and trades stat points. «Ok» starts the game with that hero. Status: **verified** against `konung.exe` (code and tables) and `NEWH800R.RES`. The 800×600 layout is ported; the 640×480 halves of the tables are listed but unused.

Port: `gamestate/newhero.go` (rules: `NewHeroTemplate`, `SelectHero`, `SelectClass`, `SelectCombatSkill`, `SelectSkill`, `TradeStat`, `MaxParty`, `Game.StartHero`, `NewHeroHit`, `NewHeroItems`), `game/newhero.go` (screen), `Menu.updateNewHero`, `sceneAssets.newGame`. Tests: `gamestate.TestNewHero`, `game.TestNewGameEachHero`.

## Resources: `NEWH800R.RES` / `NEWH640R.RES`

Sequential Type 5 frames ([RES formats](konung_res_format.md)), loaded into frame slots `0x1F3 + i` (pointer table `0x006304B8`, 8 bytes per slot).

| Frame | Slot | Content (800×600 file) |
|---|---|---|
| 0 | `0x1F3` | 800×600 background with all labels; transparent where the tile shows (including the preview box) |
| 1 | `0x1F4` | 100×100 tile, drawn first over the whole screen |
| 2 | `0x1F5` | 13×13 marker (chosen class, combat skill, peaceful skill) |
| 3, 4 | `0x1F6`, `0x1F7` | Драгомир portrait, chosen / not chosen (76×87) |
| 5, 6 | `0x1F8`, `0x1F9` | Михаил, chosen / not chosen |
| 7, 8 | `0x1FA`, `0x1FB` | Волк, chosen / not chosen |
| 9, 10 | `0x1FC`, `0x1FD` | «Отмена», normal / pressed (105×46) |
| 11, 12 | `0x1FE`, `0x1FF` | «Ok», normal / pressed (102×46) |

## Hotspots `0x004600FC`

32 bytes per entry: the 640×480 rectangle `x1, y1, x2, y2`, then the 800×600 one. The list ends with −1. Hit test `0x004398C4`: the first entry with x1 ≤ x ≤ x2 and y1 ≤ y ≤ y2 (both edges inclusive). Click switch `0x00437E86`, jump table `0x00438828` (33 cases). `gamestate.NewHeroRects800`, `NewHeroHit`.

| # | 800×600 | Meaning | Click |
|---|---|---|---|
| 0–2 | 44,108–120,190 · 155,108–236,190 · 265,108–346,190 | portraits Драгомир, Михаил, Волк | `0x00437EBE`, `0x00437F83`, `0x0043803D`: select hero |
| 3–6 | 582,81 · 549,81 · 549,49 · 582,49 (20×20) | Купец, Охотник, Вождь, Воин | `0x00438102`, `0x00438196`, `0x0043822A`, `0x004382BE`: class preset |
| 7–10 | 545,195–575,210 … 545,259–575,274 | Хар, Сил, Лов, Вын values | none |
| 11, 12 | 607,311–643,326 · 607,333–643,348 | «Макс. груз», «Макс. отряд» | none |
| 13, 14 | 695,414–725,429 · 695,436–725,451 | «Кузнечное дело», «Плотницкое дело» | none |
| 15, 16 | 542,413–562,433 · 542,434–562,454 | «Фехтование», «Меткость» | `0x00438352`, `0x004383C0` |
| 17–19 | 542,466 · 542,487 · 542,509 (20×20) | «Идентификация», «Торговля», «Знахарство» | `0x00438410`, `0x0043844C`, `0x00438488` |
| 20 | 85,527–185,560 | «Ok» | `0x004384C4`: state 8, redraw with «Ok» pressed, then the game start |
| 21 | 195,527–295,560 | «Отмена» | `0x004384DD`: menu (state 7), `[0x00A3E728]` = 0 |
| 22 | 570,160 (a point) | preview origin | none |
| 23 | 40,266–357,566 | description box | none |
| 24 | 543,164–577,179 | free experience (next to «Параметры») | none |
| 25–32 | −/+ pairs: 515/575,195 · 515/575,218 · 515/575,239 · 515/575,259 | − and + for Хар, Сил, Лов, Вын | `0x0043850A` … `0x004387D2` |

**Without a click** (`0x004388CE`): hotspots 0–19 show their own description, 25–32 that of their stat (7–10), others leave the last one. **Escape** (`0x004374DB`) acts like «Отмена». Both end a running game: the menu returns with only its four start items.

## The hero template

The screen edits the hero entity `0x00A3E694` in place. `0x00437B91` initialises it:

| Field | Value |
|---|---|
| `+0x33` level | 10 (so the stat trade uses the level-10 caps) |
| `+0x01` facing, `+0x03` | 5 (SE), `0x04` (combat stance) |
| `+0x04` class | 5 Воин; base Хар 3, Сил 15, Лов 3, Вын 10 (effective = base); `+0x38` free experience 0 |
| `+0x40` Фехтование / `+0x44` Меткость | 100 / 0 |
| `+0x42` trade, `+0x48` healing, `+0x4A` curios | 10, 50, 20 (Идентификация chosen) |
| `+0x46` building, `+0x4C` smithing | 1, 10 |
| `+0xD4`, `+0xD6`, `+0xD8`, `+0x12`, `+0x66` | Драгомир (as hotspot 0) |
| `+0xCE`, `+0x08`/`+0x0A` | 0 |

Then it writes 11 preview items (`0x00437D34`, items 1–11 at `0x0067AC34`, category `+0`, type `+3`): 1 armour `3/0x24`, 2 armour `3/0x22`, 3 axe `0/3`, 4 bow `1/6`, 5 blade `2/0`, 6–8 helmets `4/0x34`, `4/0x2B`, `4/0x37`, 9–11 shields `5/0x25`, `5/0x26`, `5/0x3D`. `GAME.N` replaces them when the game starts.

**Hero** (hotspots 0–2): name `+0xD4`, model `+0xD6`, palette `+0xD8` (byte offset, 512 bytes per `GRAPH.RES` palette), preview gear `+0x66` (slots axe, bow, blade, armour, helmet, shield), weapon in hand `+0x12` = bow if Меткость is chosen, else axe for Драгомир and blade for the others. Then `0x004143DC(hero, 0)` (stance idle, step 0). Stats and skills stay.

| Hotspot | Hero | Name | Model | Palette | Preview gear (items) |
|---|---|---|---|---|---|
| 0 | Драгомир | 124 | 1 | `0x4200` = 33 | axe 3, bow 4, armour 1, helmet 8, shield 11 |
| 1 | Михаил | 151 | 2 | `0x4E00` = 39 | bow 4, blade 5, helmet 7, shield 10 |
| 2 | Волк | 10 | 0 | `0x9400` = 74 | bow 4, blade 5, armour 2, helmet 6, shield 9 |

**Class** (hotspots 3–6): `+0x04` and the base stats (effective = base), free experience 0. Храбрец (1) can't be chosen.

| Hotspot | Class `+4` | Хар | Сил | Лов | Вын |
|---|---|---|---|---|---|
| 3 | 2 Купец | 3 | 10 | 3 | 12 |
| 4 | 3 Охотник | 3 | 3 | 15 | 10 |
| 5 | 4 Вождь | 25 | 7 | 3 | 7 |
| 6 | 5 Воин | 3 | 15 | 3 | 10 |

**Combat skill.** Фехтование: `+0x40` = 100, `+0x44` = 0, building 1, smithing 10, weapon axe (Драгомир) or blade. Меткость: `+0x40` = 0, `+0x44` = 100, building 2, smithing 5, weapon bow.

**Peaceful skill** (17–19): trade 10, healing 50, curios 10, then Идентификация sets curios 20, Торговля trade 20, Знахарство healing 100.

**Stats** (25–32, ids of `0x00410B08`: Хар 3, Сил 5, Лов 2, Вын 6; [levels](konung_combat.md#experience-and-levels-verified)):
* **+**: cost = `0x00410B08(hero, id)`. If non-zero: base + 1 and free experience − cost.
* **−**: only if the base is non-zero. Base − 1, then cost = `0x00410B08` evaluated with free experience + 1000 (always affordable). If non-zero, free experience + cost (the refund of that point); otherwise the base goes back up.
* So points move between stats at the class's level-10 costs and caps (a Воин's Сил stops at 95, his other stats at 37; a Купец's other stats at 75). A stat can go down to 0.
* The game also moves the effective value for Хар and Вын only (`+0x58`, `+0x62`). The screen shows base values, and the game start sets effective = base, so the port keeps both in step.

## Drawing `0x0042FA58(ok)`

1. Frame 1 tiled at 100 px over the screen, then frame 0 at (0, 0).
2. Portraits at hotspots 0–2's top-left: the chosen hero's frame 3/5/7, otherwise 4/6/8.
3. The marker in hotspot `+4` + 1 (class), in 15 or 16 (`+0x40` > `+0x44`), and in 17 if curios > 10, 18 if trade > 10, 19 if healing > 50. Offset: half of (hotspot 3's size − marker size), i.e. 3 px into each 20×20 box.
4. Numbers in the small font `0x005A31F4` at each hotspot's top-left (`0x00427A80`): 7–10 base Хар, Сил, Лов, Вын; 11 «Макс. груз» `%4.1f` of (effective Вын·1000/3 + 10000)·0.001, as the character window's maximum weight; 12 «Макс. отряд» `0x00411078`; 13 smithing; 14 building; 24 free experience.
5. «Ok» at hotspot 20 (frame 12 if the argument is 1, else 11) and «Отмена» at 21 (frame 10 if −1, else 9).
6. **Preview** (`0x004303B2`): a 160×320 buffer (`0x00505C70`) is cleared. The hero's current body frame goes at (60, 20) with palette `0x0051ECB4 + [+0xD8]`, then the equipment overlays (`0x004281A4`, stance order). The buffer is blitted at hotspot 22 (570, 160), clipped to 160×160.

**«Макс. отряд»** `0x00411078(e)`: class = (`+4` & 7) − 1; count the class's thresholds (10 × `int32` at `0x0045BCB4` + class·40) that effective Хар `+0x58` reaches; +1 unless all 10 are reached. Thresholds: classes 0, 1, 3: 15 21 28 36 45 55 65 74 83 95; class 2 (Охотник): 17 23 30 38 47 57 67 76 85 97; class 4 (Воин): 20 28 36 45 55 65 74 83 95 110. `gamestate.MaxParty`.

**Description** `0x00430564(i)`: CP866 text `0x00462F3C + i·512` (20 texts), wrapped (`0x0042CBBC`, small font) into hotspot 23 from its top-left. 0–2 the heroes, 3–6 the classes, 7–10 the stats, 11 «Макс. груз», 12 «Макс. отряд», 13 smithing, 14 building, 15 Фехтование, 16 Меткость, 17–19 the peaceful skills.

## Game start `0x00438F3F`

State 8 runs the start:
1. The hero (`0x00A3E694`) is copied to `0x00A3E584` (64 dwords).
2. `FLAMES.RES` and `GAME.<model>` load (`0x0043BA50`, `0x0043BB6C`; the model is byte `0x00A3E65A`).
3. The new hero record (group 0's first member) takes from the copy: `+0x40`, `+0x42`, `+0x44`, `+0x46`, `+0x48`, `+0x4A`, `+0x4C`, the base stats `+0x56`, `+0x5A`, `+0x60`, `+0x64` with the effective values equal to them, free experience `+0x38`, name `+0xD4`, model `+0xD6`, palette `+0xD8`, and the whole class byte `+0x04`. It also gets facing `+0x01` = 5 and level `+0x33` = 1.
4. Gear, bag, HP and position stay as in the file. The preview gear and `+0x12` are not copied.
5. The party's map `+0x1B` = model·3 + 1, and the map loads (`0x0043C0FC(0)`).

`gamestate.Game.StartHero`, `sceneAssets.newGame`. Start maps: Волк `KONUNG.1`, Драгомир `KONUNG.4`, Михаил `KONUNG.7`.

## Port notes

* «Ok» is drawn pressed for one frame, then the game loads. The old scene is dropped when the hero dies or a new game starts. Without a running game, `Menu.world` is nil.
* `cmd/map-render -game` still shows a `GAME.N` as stored, with only model, name and palette set (`Game.HeroModelFromGame`).

## Open questions

* Sounds on this screen were not looked for.
* The 640×480 layout is not ported (all screens run at 800×600).
