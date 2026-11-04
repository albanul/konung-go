# Combat

Melee is traced and ported (stage 1: the hero attacks). The target fighting back, creature alarm and attacks, bows, and loot are stage 2 and 3. Code: `gamestate/combat.go` (rules), `game/combat.go` (scene controller), `walker.act` in `game/hero.go` (action animations).

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
* **Retaliation** `0x0041EC0C`: an idle party member whose neighbour attacks it (neighbour `+2` bit `0x01`, target = it) takes that neighbour as target, sets `+2 |= 2` and the stance, and swings back. Not ported.

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

Approximations: the roll uses Go's RNG, not the game's `rand()`; a walking target does not flinch.

## Open questions

* Stage 2: creature alarm and attack handlers (`0x00420178`, `0x004203A0`, `0x00412F62`, `0x0041309D`, `0x004130EE`), retaliation `0x0041EC0C`, the village anger flag, the enemy finder `0x0041FD74`, return home.
* Stage 3: bow shot (type 4, `0x0042255E`), projectiles, loot `0x00418510`, auto-spend of experience `0x00410B08`, the dead-removal on leaving a map.
* Poison over time (`+0x50`), HP regeneration, the god-mode toggle `0x00436E93`.
* Fields `+0x14` and `+0xCA`/`+0xCE`.
