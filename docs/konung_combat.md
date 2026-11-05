# Combat

Melee is traced and ported for the hero (stage 1) and for creature groups, with the hero striking back and dying (stage 2). Bows, loot and village anger are stage 3. Code: `gamestate/combat.go` (rules), `gamestate/creature.go` (creature AI rules, group alarm), `game/combat.go` (scene controller), `walker.act` in `game/hero.go` (action animations).

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
| `0x0041B4CC(e, distance)` | `Game.HitChance` | creatures `100 − (100 − Лов·0.5)·0.5`. Characters in melee: `100 − (100 − Лов·0.5) / (Фехт·0.00015 + 1.5)`. Bow drawn and distance > 0: `100 − (distance/range + 1)·0.7·(100 − Лов·0.5) / (Метк·0.00015 + 1.5)`, range = bow gear `+0x34`. Then `+0xCA` while `+0xCE` ≠ 0, then the stat-9 bonus `+0xC8` (stack or replace by `+0xD1` bit 2). |
| `0x0041C480(target, attacker, attack)` | `Game.Damage` | −2 if god mode `0x00A3E768` and the target is the hero (not ported). Dodge −2: facing table `0x00450E14[target dir][attacker dir]` ≠ 0 (they roughly face each other) and roll ≤ `trunc(Лов·0.5)`. `x = max(0, attack·0.7 − Armour)` (float), `dmg = trunc(((attack·0.3 + x) / Вын + 1)·16)`. HP ≤ dmg → 1; else HP −= dmg, result 0 if dmg ≥ `0x30`, else −1. Then: target poison `+0x50` += `0x00A3E828`; armour/helmet/shield condition (item float `+4`) −= gear `+0x2C`·0.001·min(1, attack/Armour); attacker's axe or blade loses one charge (item `+0xC`) and attack·0.00025 condition. Condition ≤ 0 destroys the item (category `0xFF`, slot cleared). |
| `0x0041AF48` side effect | `Game.poisonOf` | `0x00A3E828` = creature `+0x52`, the melee weapon's item `+0xC`, or 0 |
| `0x0041B70C(victim)` | `Game.KillExperience` | creatures `trunc(Armour·0.1 + Лов·0.2 + Сил·0.3 + Вын·0.3)`; characters add `Фехт·0.002 + Метк·0.003`, then with a melee weapon `trunc(xp + gear·0.2)` |
| kill path `0x00422082`, `0x0041079C` | `Game.Kill` | killer «Свободный опыт» `+0x38` += xp/4 and Фехтование `+0x40` += xp. With `+3` bit `0x02` (not Volk) `0x0041079C` also spends free experience on stats (`0x00410B08`); not ported. |

Example (`gamestate.TestMeleeGame0`): Volk with the knife, hit 37 %, attack 120. Against giant ant 921 (Лов 1, Сил 8, Вын 3, armour 10) a hit does 602, so three hits kill it; it is worth 4 experience.

## Port

Right click on a character in the stance → `startScene.startFight`. `combatTick` runs once per game tick (`walkTicksPerStep` updates). Approach uses `walker.chase` (a path ending on the target's cell) and `walker.stopAt` (stop before entering it). Any other walk click or leaving the stance ends the fight. The dead stay on the map as corpses until the map is left; `buildScene` skips dead non-party entities, like the leave cleanup `0x00436AD0`.

Approximations: the roll uses Go's RNG, not the game's `rand()`; a walking target does not flinch; the creature path budget is separate from the villagers'; a chasing creature re-plans its whole path when the target moves (the game patches one step).

## Open questions

* Stage 3: bow shots (type 4, `0x0042255E`, creature `0x00410E84`, `0x0041130C`, ammunition `0x0041BFF8`), projectiles, loot `0x00418510`, auto-spend of experience `0x00410B08`, the dead-removal on leaving a map.
* Village anger (settlement `+0x4B2 |= 0x15`) and the village alarm AI (`0x00416058` with `+0x1D` ≠ 0, `0x00416DA6`); the village branch of `0x0041FD74`.
* Group upkeep `0x0041BDF0` (respawn via `+0x18`/`+0x6` and table `0x0045E37E`; HP drain by poison `+0x50`), HP regeneration, the god-mode toggle `0x00436E93`.
* Kind `0x49` retreat `0x004101E4`; HUD feedback `0x0042D574`; fields `+0x11`, `+0x14`, `+0xCA`/`+0xCE`.
