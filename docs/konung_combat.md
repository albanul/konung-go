# Combat

Combat is traced and ported: melee for the hero, creatures and angry villagers; bows with flying arrows; loot, experience and levels; village anger and surrender; poison, potion timers and creature respawn; the cleanup when leaving a map. The [port](#port) lists the code and what is left out.

## Action types (verified)

Entity `+0x00` is the action type, relative to the animation set: 0–5 in the combat stance (`+3` bit `0x04`), 6–11 otherwise. Animated `OBJECTS.RES` models fold ≥ 6 onto 0–5. Dispatch: party jump table `0x00422654`, creature table `0x00412BE0`.

| Type | Party case | Meaning | Steps (Volk / ant model 0) |
|---|---|---|---|
| 0 / 6 | `0x004223AD` | idle; with an intent in `+2 & 0x0F` the planner `0x0041ED8C` runs | 12 / 12 |
| 1 / 7 | `0x0042218D` | walk | 12 / 12 |
| 2 / 8 | `0x004224F8` | hit reaction, then idle (`0x00414AB4`) | 7 / 6 |
| 3 / 9 | `0x00422447` | death: at step count−2 `+2` = 0, `+4 \|= 0x80`, the cell's occupant bits are cleared; the last step stays | 12 / 12 |
| 4 | `0x0042255E` | bow shot (not traced) | 12 / 1 |
| 5 | `0x00421E8C` | melee swing; always type 5 | 12 / 12 |

`0x004143DC(e, type)` starts an action at step 0. `0x00413DE4(e)` advances `+0x10` once per game tick and returns 0 (staying on the last step) when the step would reach the count `+0xD7`. `0x004391BC(e, dir)` turns one octant toward `dir`, the shorter way round. `0x0043976C` gives the direction between two cells.

## Melee flow (verified)

* **Order.** Right click on a character with the hero in the stance (`0x00423C8B`): each selected member gets `+6` = target index + 1, `+2` = `0x41` (melee intent), `+0x14` = `0xFF`. With a bow, arrows and the stance it is `+2` = `0x44` (shoot). Without the stance a right click opens the character's dialogue (`0x004364F0`).
* **Approach.** The idle case runs the planner `0x0041ED8C`. With bit `0x01`, `0x0041E384` copies the living target's cell into `+0x0C/+0x0E` (a dead target is replaced via the group's enemy finder `0x0041FD74`, else the intent ends). The member then paths there (`0x0043EDB7`) and walks (type 1/7).
* **Engage.** When a walk step would enter an occupied cell, `0x004143DC` asks `0x00411758`. If the occupant is the walker's target (`+6`) and in another group, the walker stores it, clears the path, sets the stance bit and starts type 5. Any other occupant stops the walk (idle).
* **Swing** (type 5, `0x00421E8C`), per tick:
  1. `0x0041F258`: if the target no longer stands on `+0x0C/+0x0E`, back to idle. The intent remains, so the planner re-approaches. If it is dead, `+6` = 0.
  2. Turn toward the target if needed (one octant per tick).
  3. At step 2: if the target's group is a village (`+0x1E` bit `0x80`), settlement `+0x4B2 |= 0x15`; then `0x004248A0` alarms the target's group (`+0x1D` = 1, members' `+0x14` = `0xFF`).
  4. Advance the step; after the last step the blow lands: `0x0041B100` then `0x0041C480`.
  5. Kill (1): attacker `+6` = 0; victim HP = 0, `+2` = 0, death animation; experience (below); `0x00418510` (loot); attacker idle. Damage ≥ `0x30` (0): the victim flinches (type 2, or 8 outside the stance). Otherwise (−1 light hit, −2 dodge) nothing. A hit on a party member updates its portrait (`0x0042D574`). The swing restarts.
* **Retaliation** `0x0041EC0C`: an idle party member without an intent (`+2 & 9` = 0) scans its 8 neighbours. A neighbour of another group with `+2` bit `0x01` and target = it becomes its target; the member sets `+2 |= 2` and the stance, and swings back. Port: `startScene.retaliate` (hero only). Stationary creatures swing with `+2` = `0x02`, so they are not struck back automatically.
* **Hero death** (`0x004376A8`): the main loop sees the hero's `+4` bit `0x80`, sets `0xA3E7F0` = 7, plays `GAMEOVER.FLM` (`0x0042B690(0x00450650)`) and leaves the game loop. The film is not among the installed files. The port returns to the main menu 2 s after the death (`startScene.gameOver`), and "New Game" starts again.

## Attack order (verified)

**A** or HUD button 5 «Атаковать (A)» (key switch `0x00437272`, VK `0x41` → `0x00436DB0`):

* With `0x00A3E754` set (apparently Shift held: it is set and cleared around VK `0x10`), A selects every living party member.
* Without Shift it runs `0x00421698`: sound 4, return a held item (`0x00429194(1)`), press button 4 with image 7 (`0x004290AC(4, 7)`). Each living party member then gets `+6` = 0, `+2` = `0x44` with the bow active (else `0x41`), `+0x14` = `0xFF`, and enters the stance (type − 6, `+3 |= 4`).
* With no target, the planner's `0x0041E384` asks `0x0041FD74` for the enemy group. For the party (`+0x1E` bit `0x20`; group 0 has `0x20` in `GAME.0`) the search area is the leader's cell ± (W + W/2), with W = `0x00A3E818` rows and `0x00A3E7DC` cols. On 800×600 (`0x00427C8C`) these are the view's height/32 + 3 = 20 rows and width/116 + 4 = 9 cols.
* Eligible groups: on the current map, not the party, `+0x1E` bit `0x80` clear (no village) unless alarmed, and `+0x1E` ≠ `0x53`. The group whose living member inside the area is closest to the leader wins. Then `0x004203A0` / `0x004202A8` pick that member.
* When the target dies, the same search finds the next one. This also happens after a right-click attack, because the intent stays. With no enemy left, the intent ends (`+2 &= ~1`, `0x0041E456`).

Port: `startScene.attackOrder`, `nearestEnemy`, `startScene.melee` (the intent). Bows are not ported, so with the bow active the order does nothing.

## Creature AI (verified)

`0x004123BC(group)` runs for each creature group on the current map, once per game tick. The group has two rectangles: the **territory** (words `+0x08`..`+0x0C` rows, `+0x10`..`+0x14` cols; `Group.WanderArea`) and the **leash** (`+0x0A`..`+0x0E`, `+0x12`..`+0x16`; `Group.LeashArea`). The leash is usually larger.

* **Alarm check** `0x00412C30(group)`, from `0x0041D0FC` every 16 game ticks (`0xA3E770 & 0xF` = 0), for creature groups on the current map (`Group.AlarmState`):
  * a living party member inside the territory → `+0x1A` = party group, `+0x1D` = 1 (attack);
  * else a living member outside it → `0x20` (return);
  * else 0 (calm).

  A hit or a dying member also sets 1 (`0x00412A21` at the end of the hit reaction, `0x00412989` in the death), and a party attacker's swing does too (`0x004248A0`). Each check overwrites it.
* **Members.** An idle member (`+2` bit `0x40`, type not 2/3/9) is decided by `0x00412E00` when the path budget `0x004116CC` allows (3 searches per tick, round-robin). Otherwise its action runs (table `0x00412BE0`): idle `0x0041291C`, walk `0x00412717`, hit reaction `0x00412A0E`, death `0x00412944`, type 4 `0x00412A36` (dormant pose and wake-up; the bow shot for humanoids), swing `0x0041248D`.
* **Decision `0x00412E00` by alarm state:**
  * 0 calm: back to idle (type 0/6, except a dormant type 4). Calm creatures never move.
  * 1 attack: target by kind (`gamestate.TacticOf`), then path to it with `+2` = 1 (`0x0041326B` → `0x004142D4`):

    | Kind `+4` | Rule |
    |---|---|
    | `0x41`, `0x4D`, `0x4F` | a group-mate's living target (`0x00420178`), else the nearest enemy (`0x004202A8`) |
    | `0x43`, `0x45` | the nearest enemy no group-mate is attacking (`0x004203A0`), else the nearest |
    | `0x44`, `0x57` | never move: swing at an adjacent member of the enemy group, `+2` = 2 (`0x00412F62`) |
    | `0x46`, `0x4C`, `0x55` | dormant (type 4 at step 0): first wake up (play type 4); then as `0x43` |
    | others | animated: the nearest. Humanoids (bandits) first enter the stance (re-equip `0x004105A0`); with a bow and arrows they shoot (type 4, `0x00410E84`; not ported), else the nearest. |

    Distance `0x00439944` (`gamestate.Dist`): the larger axis difference, plus 1 if the smaller one exceeds 1.
  * `0x20` return: a member with the melee intent keeps fighting while a party member is inside the leash (`0x0041FD74`). Kinds `0x41`/`0x4D`/`0x4F` and `0x43`/`0x45` use their selector without the nearest fallback; stationary ones stop; others find nothing. Otherwise the intent ends. A member outside the territory walks to a free cell picked like `0x0041F5D8` (`gamestate.WanderTarget`) with `+2` = 8.
* **Chase** `0x00411FD0` (walking with the melee intent): a dead target ends it. A target still on `+0x0C/+0x0E` keeps it going. A target outside the leash drops it (leash). A target that moved one cell patches the path's last step; otherwise the member goes idle and re-decides.
* **Swing** `0x0041248D`: as the party's, without the step-2 alarm. A kill also adds xp to the creature's `+0x40`. Kind `0x49` steps back after a blow (`0x004101E4`, not ported).
* **Revival:** a dying kind-`0x4C` creature with `+0x12` > 0 gets HP 1600, `+0x12` − 1 and action 4 (dormant) instead of dying (`gamestate.Revives`).
* **Walk extras** (not ported): `+0x11` slows walking (ticks where `time & m == m`); kinds `0x4D`/`0x4F` pause a random number of ticks between steps.

Port: `startScene.combatTick` → `alarmCheck`, `creatureTick` (`creatureChase`, `creatureDecide`, `goFor`, `stationarySwing`, `returnTarget`), `actTick`. The port's party is the hero, so every selector resolves to the hero. Corrected kind routing: `0x4E` and `0x54` belong to "others".

## Formulas (verified)

All conversions truncate (`0x0043FE4C`); `rand()%101` is the roll. Stats: Лов `+0x54`, Сил `+0x5E`, Вын `+0x62`, HP `+0x5C`, Фехтование `+0x40`, Меткость `+0x44`, each after `Game.Recompute` (`0x0041C750`) for characters.

| Routine | Port | Rule |
|---|---|---|
| `0x0041B100(target, attacker)` | `Game.Strike` | miss when roll > hit chance; else the attack value `0x0041AF48(attacker, target +4)` (`Game.Attack`) |
| `0x0041B4CC(e, distance)` | `Game.HitChance` | creatures `100 − (100 − Лов·0.5)·0.5`. Characters in melee: `100 − (100 − Лов·0.5) / (Фехт·0.00015 + 1.5)`. Bow drawn and distance > 0 (character window only; arrows have no hit roll, see [bows](#bows-verified)): `100 − (distance/range + 1)·0.7·(100 − Лов·0.5) / (Метк·0.00015 + 1.5)`, range = bow gear `+0x34`. Then `+0xCA` while `+0xCE` ≠ 0, then the stat-9 bonus `+0xC8` (stack or replace by `+0xD1` bit 2). |
| `0x0041C480(target, attacker, attack)` | `Game.Damage` | −2 if god mode `0x00A3E768` and the target is the hero (not ported). Dodge −2: facing table `0x00450E14[target dir][attacker dir]` ≠ 0 (they roughly face each other) and roll ≤ `trunc(Лов·0.5)`. `x = max(0, attack·0.7 − Armour)` (float), `dmg = trunc(((attack·0.3 + x) / Вын + 1)·16)`. HP ≤ dmg → 1; else HP −= dmg, result 0 if dmg ≥ `0x30`, else −1. Then: target poison `+0x50` += `0x00A3E828`; armour/helmet/shield condition (item float `+4`) −= gear `+0x2C`·0.001·min(1, attack/Armour); attacker's axe or blade loses one charge (item `+0xC`) and attack·0.00025 condition. Condition ≤ 0 destroys the item (category `0xFF`, slot cleared). |
| `0x0041AF48` side effect | `Game.poisonOf` | `0x00A3E828` = creature `+0x52`, the melee weapon's item `+0xC`, or 0 |
| `0x0041B70C(victim)` | `Game.KillExperience` | creatures `trunc(Armour·0.1 + Лов·0.2 + Сил·0.3 + Вын·0.3)`; characters add `Фехт·0.002 + Метк·0.003`, then with a melee weapon `trunc(xp + gear·0.2)` |
| kill path `0x00422082`, `0x0041079C` | `Game.Kill` | killer «Свободный опыт» `+0x38` += xp/4 and Фехтование `+0x40` += xp. With `+3` bit `0x02` (not Volk) `0x0041079C` also spends free experience on stats (`0x00410B08`); not ported. |

Example (`gamestate.TestMeleeGame0`): Volk with the knife, hit 37 %, attack 120. Against giant ant 921 (Лов 1, Сил 8, Вын 3, armour 10) a hit does 602, so three hits kill it; it is worth 4 experience.

## Bows (verified)

A bow shot spawns a **projectile**. The hit is decided by what the arrow flies into; there is no hit roll. `0x0041B4CC`'s distance branch and `0x0041ADE4` are used only by the character window.

* **Order** (`0x00423BB8`): right click with slot `+0x12` = 1, a bow `+0x68`, a quiver `+0xCC` and the stance.
  * On a character: `+6` = index+1, `+2` = `0x44`.
  * On a settlement building: only with incendiary arrows (quiver item `+2` = 0). `+6` = −(object index + 1), and `+0x0C/+0x0E` = the clicked cell.
  * The A order sets `+2` = `0x44` when the bow is active.
* **Planner** `0x0041ED8C` (bit `0x04`): without bow, quiver or slot 1 → drop the bit and go idle. With no target, `0x00410E84` takes the **first** living member of the enemy group (`0x0041FD74`) with a clear line. Then:
  * `0x0041130C` > 0 → action 4;
  * < 0 → drop the bit;
  * 0 → `+6` = 0 and the hero drops the bit.

  The shooter never walks closer. The intent stays after a shot, so it keeps shooting until the target dies or the line is lost.
* **Line of fire** `0x0041130C(e, r, c, t)`:
  * Grid distance (`0x00439944`) < 3 → −1 (too close).
  * Otherwise a ray from the shooter's pixel (`0x00439BA0`: x = col·58 + 29·(2 − (row&1)), y = row·16 + 16) plus the launch offset for its facing (int8 pairs at `0x0045AD30`: (−70,−65) (−48,−99) (12,−85) (68,−90) (80,−58) (52,−26) (−8,−10) (−60,−31)), toward the target cell's pixel with y − 30.
  * The ray moves 4 px per sub-step on the major axis, for range·8 sub-steps (range = bow gear `+0x34`). Each point is looked up as the cell at (x, y + 30):
    * the target cell → 1;
    * a cell with `0xC000` set → 0 (buildings: sprite hit test);
    * any other occupant (not 0, not `0xFFF`) → 0.

    Running out of sub-steps → 0.
* **Shot** (action 4, party `0x0042255E`; creatures `0x00412AE5` and villagers `0x004165B2` the same):
  * At step 0, turn toward `+0x0C/+0x0E` (`0x00439AA4`) before advancing.
  * At step count − 6, `0x0041BFF8` spawns the arrow. If it fires, the quiver stack (`+4` int32) loses one; at 0 the item is freed and `+0xCC` = 0.
  * At the end: idle. Creatures and villagers also drop bit `0x04`.
* **Projectile** (`0x0041BFF8`; list `0x0082C1F8`, 32-byte records, count `0x00A3E7D8`, at most 100):

  | Offset | Field |
  |---|---|
  | `+0`/`+4` | float velocity: 4 px per sub-step on the major axis toward the aim point (target cell pixel, y − 30, fixed at the shot) |
  | `+8`/`+0xC` | float position: the shooter's pixel + launch offset |
  | `+0x10`/`+0x1A` | sprite draw offset (int8 pairs at `0x00458100`) |
  | `+0x14` | shooter index (0-based) |
  | `+0x16` | bow gear value |
  | `+0x18` | the arrow item's poison `+0xC` |
  | `+0x1C` | INTR image slot `0xB8` + 2·facing |
  | `+0x1D` | lifetime = bow range, in ticks |
  | `+0x1E` | arrow special |
  | `+0x1F` | arrow type |

  The bow loses 0.1 condition per shot and is destroyed at ≤ 0.
* **Flight** `0x00421170`, every game tick before the AI: lifetime − 1 (removed at 0), then 8 sub-steps (32 px per tick). At each point (x, y + 30):
  * A cell with `0xC000` → removed. A building hit with incendiary arrows → that building's record `+1` = 3, `+6` = 1 on a 21 % roll; the village gets angry (below) and is alarmed.
  * An occupant other than the shooter → victim V. If V is not in the hero's group: a village V angers its village (`+0x4B2 |= 0x15`), and V's group is alarmed (`0x004248A0`). A dead V lets the arrow pass. Otherwise:
    * attack = `0x0041ACE0`: the slayer special's value (`0x0046238C` + s·64: `+0x2C` value, `+0x2E` target kind) when the arrow's special matches V's kind, else the arrow's gear value; times the bow value; then the `+0xC6` bonus;
    * damage = `0x0041C29C(V, attack)`, which is `0x0041C480` without god mode, the facing condition and weapon wear (dodge roll ≤ trunc(Лов·0.5); poison += the arrow's `+0xC`);
    * kill → as melee, but Меткость `+0x44` grows by xp; heavy hit → flinch.

    The arrow is removed in every case. **Friendly fire exists.**
  * Otherwise it moves on.
* **Drawing** (`0x00426D90`): INTR image slot `+0x1C` + (game time & 1) (slots 184–199: two frames per facing; 32×6, 29×17, 6×23), at the position plus the sprite offset, depth y + 0x5E. Incendiary arrows also get a 64×64 glow (`0x005B0010`, unknown).
* **Bandits and alarmed villagers** (`0x004130EE`, `0x00416F65`): first `0x004105A0` re-equips (quiver from the bag, best gear; slot `+0x12` = 1 with bow and arrows, else 0 with an axe, else 2) and enters the stance. Then they shoot via `0x00410E84` if possible, else go for the nearest in melee.
* Arrows are not recoverable. Slayer specials 1–11: оборотень 5000 (kind 85), муравьи 500 (65), ичетик 3000 (70), пауки 2000 (77), червяки 3000 (78), морок 3500 (73), омутник 4000 (75), скелеты 4000 (76), нави 4000 (74), мертвяк 4500 (69), моховик 5000 (72); special 0 = incendiary.

## Loot (verified)

`0x00418510(victim, A, L)` on every kill: A = the killer's kill xp (party kills; 0 for creature kills), L = the killer's level (hero's level for poison deaths; 0 for creature kills). Helpers: g(n) = rand()%(n+1) when n+1 ≥ 2, else 0 (no rand call); f(L) = 3 (L ≤ 3), 6 (≤ 6), 8 (≤ 9), else 10 (`0x004171D4`); ench(L) = rand()%f(L) + 10·(rand()%5).

1. If the bag is empty and gold `+0x34` is 0: continue only if rand()%101 ≤ 30 or the victim has poison `+0x52`.
2. Use the pile on the victim's cell (cell bit `0x2000`, first 22 item slots free), or start a new runtime pile (at most 200 active; `+0xE` = `0xFFFF`).
3. Pile gold += victim gold + g(A − 1) (rand()%A when A ≥ 2). The victim's gold is not cleared.
4. Bag items move into the pile in order, up to 22. Equipment is not dropped; it is freed with the corpse on leaving the map.
5. If there is room and L ≠ 0, r = rand()%101. On r > 85, a new item (`0x004399AC`, the first free record ≥ 1) is generated:

   | r | Item |
   |---|---|
   | 86 | weapon: type g(L/5) + 3·(rand()%9); category by type%9 (< 3 blade 2, < 6 axe 0, else bow 1); condition `+4` = `+8` = gear `+0x2E`. On rand()%101 < 10: unidentified (`+0xE` bit 0) if rand()%100 > hero curios `+0x4A`, else curios + 1 (cap 100); enchantment `+1` = ench(L). |
   | 87 | armour: s = rand()%3. s=0: body armour type `0x22` + g(L/5) + 11·(rand()%3), category 3. s=1: shield `0x25` + 11·(rand()%3) + g(L/4), category 5. s=2: helmet `0x29` + …, category 4. Condition as above. On rand()%100 < 7: unidentified test and enchantment. |
   | 88–94 | ring (category 8), type 6 + g(L/5) |
   | 95–97 | bracelet (7), type 3 + g(L/5) |
   | 98–100 | necklace (6), type g(L/5) |

   Jewellery is always enchanted and tested for identification; its condition is not set.
6. A victim with poison `+0x52` also drops «Ядовитое жало» (category 9, type 0, `+4` = 1.0, `+0xC` = the poison).
7. An empty pile is dropped again.

## Experience and levels (verified)

`0x0041079C(e, xp)`: free experience `+0x38` += xp. With `+3` bit `0x02` (the character window's auto-level toggle, `0x00423272`) it then loops: level up while `0x00410B08(e, 0)` allows it, and buy Вын, Сил, Лов, Хар in turn (ids 6, 5, 2, 3 → base `+0x64`, `+0x60`, `+0x56`, `+0x5A`; `+0x38` −= cost, `+0x3C` += cost) until nothing is affordable.

`0x00410B08(e, id)`, also used by the character window's raise buttons (`0x00422864`) and character creation:
* 0 while a potion timer `+0xCE` runs.
* race = 1 for animated models, else (kind & 7) − 1. The favoured stat is `0x0045AE88[race]` = {3, 6, 2, 3, 5}.
* Requirement and cap pairs at `0x0045B10C` + race·0x68 + stat·0x1A + level·2 (stat 0 Лов, 1 Хар, 2 Сил, 3 Вын).
* **id 0:** level < 13 and all four stats ≥ min[level].
* **ids 2/3/5/6:** stat < cap[level − 1]; then the cost from the 13-row table `0x0045B010` {threshold favoured, threshold other, cost}: thresholds 13/9, 19/14, 26/20, 34/28, 43/36, 53/44, 63/52, 74/60, 80/65, 90/70, 95/75, 100/80; costs 2, 5, 11, 14, 18, 23, 29, 35, 72, 100, 150, 200 (999 beyond). The first row whose threshold exceeds the stat applies; the cost is returned if free experience covers it.

## Village anger (verified)

Settlement byte `+0x4B2` (`Settlement.Flags`):

| Bit | Meaning |
|---|---|
| `0x01` | hostile: any party member inside the village rectangle alarms it |
| `0x04` | armed strangers not tolerated: a party member of another tribe (model `+0xD6` ≠ village group `+0x1E & 3`) with a weapon equipped or a weapon/bow in the bag (category ≤ 2) alarms it; the merchant also hides weapons |
| `0x10` | independent (not a vassal); no tribute |
| `0x20` | loan outstanding (dialogue) |
| `0x80` | surrendered |

* **Load** (`0x0043BD5C`): `0x10`, plus `0x04` for a fenced village (`+1` ≠ 0) of another tribe.
* **Anger** `|= 0x15` (hostile + strangers + independent): a villager's hit reaction ending or death (step count − 2), whoever caused it (`0x0041658E`, `0x0041651C`); the party's melee swing at step 2; an arrow hitting a villager or a building; the dialogue's "attack" op.
* **Peace** `&= 0xEA`: surrender, a village without men, the dialogue's peace op.
* **Alarm check** `0x00416700`, every 16 ticks offset by 7 (`(time + 7) & 0xF` = 0, `0x0041D39F`) for the village on the current map. Without bits `0x05` → calm. Otherwise a living party member inside the village group's rectangle alarms it (`+0x1A` = party, `+0x1D` = 1) when bit `0x01` is set, or when bit `0x04` is set and the member is an armed stranger. Otherwise calm. Hostility outlives the alarm: the village re-attacks on re-entry.
* **Alarmed villagers** (`0x00416934` → `0x00416DA6`):
  * Children and women (`+4` = `0x51`/`0x52`) never fight; they take random steps every tick (their flee routine `0x00413DC4` is a stub).
  * Men first re-equip and enter the stance (`0x004105A0`). Then they shoot (`0x00410E84`) or go for the nearest party member (`0x004202A8`) in melee.
* **Surrender** (the settlement tick, while alarmed and not yet surrendered):
  * P = Σ `0x0041B70C` over the party, V = Σ over the living men (model < 3).
  * P / V ≥ 2.0 → alarm 0; living members idle, targets cleared; flags `|= 0x80`, `&= 0xEA`; message `0x0045D37C` «Довольно крови, князь. Деревня сдается на твою милость!»; film `DEFEAT<map>.FLM`; the party's targets are cleared.
  * A village without men: alarm 0, flags `&= 0xEA`.
* Tribute, buildings (records from settlement `+0x18`, 8 bytes; `+0x40`/`+0x41` = building 5's type/state, the map-exit gate) and the economy are not part of combat; see the open questions.

## Upkeep, poison and respawn (verified)

**There is no passive HP regeneration.** HP goes up only through items, the healer, revival and HP enchantments.

* **Every 16 ticks** (`0x0041D0FC`), for every active group on every map:
  * Creature groups run `0x0041BDF0`; the party and villages run an inline copy with HUD updates.
  * **Poison:** every living member with poison `+0x50` ≠ 0 loses that much HP. The poison never wears off. At HP ≤ 0: `+2` = 0, HP 0, and on the current map the death action and the loot `0x00418510(e, 0, hero level)`. Off the current map the member stays at HP 0 without dying.
  * **Potion timer** `+0xCE` (set by timed potions, [potions](konung_potions.md)): −1; at 0, `+0xCA` = 0 and base stats are restored from `+0xBC`, `+0xBE`, `+0xC0`, `+0xC2` (into `+0x56`, `+0x5A`, `+0x60`, `+0x64`).
  * **Creature respawn counters** (kinds `0x40`–`0x50`):
    * While target `+0x18` < maximum `+0x19`, countdown `+6` − 1; below 0 → target + 1, countdown += period.
    * While target > count `+0x1C`, debt `+2` (int32) − 1.
    * Table `0x0045E278` + kind·4: int16 cost, int16 period (kind `0x41` 1440/3600 … `0x43` 4800/12000; `resfmt.RespawnRule`).
* **Map entry** (`0x0043C612`, map changes and new game):
  * Every member of a non-village group with kind < `0x51` (creatures) is placed at random in the territory (`0x00411DE4`). Up to 100 tries: centre ± rand()%(half span) per axis with a random sign, on a cell without an occupant. If that fails, the group is cut at that member.
  * Then, skipping the **first** eligible creature group on the map (literal code), each group with debt < 0 and count < target gets new members (`0x00420CCC`) and pays cost per member. Remaining debt is reset to 0.
  * A new member is written at slot first + count:
    * averages over the group of Лов, Хар, Сил, Вын, `+0x4E` armour, `+0x52` poison, level, gold, `+0x11`;
    * model and palette from a random member;
    * HP 1600, kind = group kind;
    * equipment and bag cleared;
    * action 4 for the dormant kinds `0x46`/`0x4C`/`0x55`, else 0;
    * random facing, stance, dialogue `0xFF`, no name, revives rand()%5 + 1 for kind `0x4C`.

## Port

Engine-free rules live in `gamestate`:
* `combat.go`: melee, `Damage`/`applyDamage`, `Kill`.
* `creature.go`: alarm, tactics, revival.
* `bow.go`: `CanShoot`, `LineOfFire`, `Shoot`, `Arrow.Fly`, `ArrowAttack`, `ArrowDamage`, `KillByArrow`, `ReEquip`.
* `loot.go`: `DropLoot`, `RaiseCost`, `GainExperience`.
* `anger.go`: `InitSettlementFlags`, `Anger`, `VillageAlarm`, `Surrenders`.
* `upkeep.go`: `Upkeep`, `EnterMap`, `PlaceInTerritory`, `NewMember`.
* `leave.go`: `LeaveMap`.

The scene (`internal/game`) consumes them:
* `combat.go`: `combatTick` each game tick, the swing, `kill`, `provoke`, the creature AI and the hero's orders.
* `ranged.go`: shooting and arrows. Arrows are drawn over the characters.
* `upkeep.go`: the 16-tick upkeep, the settlement tick, loot piles, the villagers' fight.
* `scene.go`: `buildScene` runs `EnterMap`; `changeMap` runs `LeaveMap`.

Orders:
* Right click on a character in the stance: shoot it when `CanShoot`, else attack in melee.
* A: attack; with the bow active it shoots.
* The dead stay as corpses until the map is left.

Approximations:
* The rolls use Go's RNG, not the game's `rand()`.
* A walking target does not flinch.
* The creature path budget is separate from the villagers'.
* A chasing creature re-plans its whole path when the target moves (the game patches one step).
* Arrow points use the nearest cell centre instead of `0x00439BDC`.
* `ReEquip` does not equip the best bag gear (`0x004104D4`).
* A respawned member's untouched fields start empty instead of stale.
* The party is the hero alone, so every target selector resolves to the hero.

Not ported:
* incendiary arrows and buildings (burning, the map-exit gate);
* tribute, loans and the village economy;
* the surrender film;
* item use and the healer (the only HP gains);
* the character window's raise buttons (`RaiseCost` is ready for them).

## Open questions

* Grid bits `0xC000` (obstacle kinds); the incendiary glow buffer `0x005B0010`; `0x004104D4` (equip best), `0x00418364` (usable).
* Settlement buildings: type table `0x00458A60`, burning and rebuilding, building 5 (types 14/15) as the map-exit gate; tribute (`+0x10`, `+0x14`, every 10080 settlement ticks, `0x0043FECE` = sqrt).
* The god-mode toggle `0x00436E93`; kind `0x49` retreat `0x004101E4`; HUD feedback `0x0042D574`; field `+0x11` (walk throttle), `+0x14`.
* Films `GAMEOVER.FLM`, `DEFEAT<n>.FLM` (`0x0042B690`), not in the installed files.
