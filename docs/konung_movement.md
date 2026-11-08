# Movement: walking and path search

How characters walk in `konung.exe`: the game tick, the path search, a walk step, orders and stops. Status: **traced** against the code (VAs below); the step layout (`0x0047FD2C`) is checked against `mapdata.Neighbors`. **Ported**: the 60 ms tick, the path search, turning and the walk step, the party's re-search and the creature extras; the remaining differences are listed at the end.

## Timing

* The game draws a frame every 60 ms (`0x00A3E6F4`, [menu](konung_menu.md)) and runs 1 + speed game ticks per frame, so a tick is 60 ms at normal speed (16.7 per second).
* Every walking character runs its walk case once per tick (party `0x0042218D`, creatures `0x00412717`, village AI the same type through `0x00416058`).
* **One walk frame per tick.** `0x00413DE4` advances `+0x10`; a step into the next cell lasts the walk animation's frame count `+0xD7` (12 for the bodies), so 12 ticks = 0.72 s per cell, whatever the direction.
* **Turning.** At the start of each step, a character that does not face the step's direction turns one octant (`0x004391BC`, the shorter way) and does nothing else that tick. A path that bends by k octants costs k extra ticks.

## Setting a destination

* **Order** (`0x004142D4(e, row, col, state)`, AI walks): goal `+0x0C/+0x0E`, `+2` = state, `+0x14` = `0xFF`. It frees the walker's own cell, searches (`0x0043EDB7`), takes the cell back, and on success clears the idle bit and starts the walk action (1 in the stance, else 7).
* **Left click on the map** (`0x004229F7`): every selected member gets the clicked cell as its goal (all the same cell), `+2` = `0x48` (walk + idle bits), `+0x14` = `0xFF`. The party planner (`0x0041ED8C`, walk bit `0x08`) then searches. No search on the click itself.
* **No nearest-free fallback.** A goal that is a wall, or holds a character other than the walker's target (`+6`, only when `+2 & 7`), fails the search: the walk bit is dropped and the member stays idle.
* **New order while walking.** `+0x14` = `0xFF` cuts the path after the step in progress (`+0x13`). The member finishes that step, finds the path empty away from the goal, and the planner searches again from the new cell.

## Path search (`0x0043EDB7`, hand-written assembly)

Input: the entity (cell `+0x08/+0x0A`, goal `+0x0C/+0x0E`). Grid: the fine-grid word map `0x00505C70` (320 rows × 160 columns); a cell is free when `& 0xFFF` is 0 ([map format](konung_map_format.md#flag-grid-section-1-verified)).

1. **Fails** when the goal is the current cell, a wall (`0xFFF`), or occupied by anyone but the target as above, or when none of the goal's 8 neighbours is free (or the walker's own cell).
2. Clears the path (`+0x13`, 32 bytes of `0xFF`) and counts the search (`0x00A3E720`, the AI's budget of 3 per tick, `0x004116CC`).
3. **Flood.** A FIFO queue (`0x0051ECD0`, 16-byte entries: row, column, cost; a ring of 4096 entries, 16-bit indices) starts at the walker's cell. The node table `0x0052ECD0` keeps per cell the best cost and the direction it was reached by; a generation stamp (`0x00592CD0`, −`0x10000` per search, a full reset to `0x7F7F7F7F` on wrap) avoids clearing it. For each cell taken from the queue, the neighbours are tried in this order, with this cost:

   | Row parity | Order (direction code) | Cost |
   |---|---|---|
   | odd row | NE 3, NW 1, SE 5, SW 7 | +10 |
   | even row | NW 1, NE 3, SW 7, SE 5 | +10 |
   | both | W 0, E 4, N 2, S 6 | +14 |

   A neighbour that is the goal is recorded when its cost improves and sets "found" (`0x0051EC80`); it is never queued. Any other neighbour is queued and recorded when it is free, nothing has been found yet, and its cost improves. The queue then drains.
4. **No search limit**: the flood covers the reachable area until the goal is found. On four maps (300 random pairs each), the queue never held more than 497 entries, far below the 4096-entry ring.
5. **Path.** From the goal back to the start by the recorded directions (step back = direction + 4, table `0x0047FD6C`); the **first 31** directions are written to `+0x13` (byte `0xFF` ends the path). The return value is the full step count (0 = no path).

The step layout (`0x0043ECBA`, table `0x0047FD2C`, by row parity): W (0, −1); NW odd (−1, −1) even (−1, 0); N (−2, 0); NE odd (−1, 0) even (−1, +1); E (0, +1); SE odd (+1, 0) even (+1, +1); S (+2, 0); SW odd (+1, −1) even (+1, 0). This is `mapdata.Neighbors`.

## A walk step

Walk case, once per tick (party `0x0042218D`; creatures `0x00412717`):

1. **Creatures only:** with throttle `+0x11` = m ≠ 0, the walker acts only on ticks where `time & m == m` (`0x0041271A`).
2. Path empty (`+0x13` = `0xFF`): party, the planner searches again within the budget; creatures go idle (`+2 |= 0x40`) and decide again.
3. Facing ≠ `+0x13`: turn one octant, end of tick.
4. Advance the walk frame (`0x00413DE4`). While frames remain, end of tick.
5. **End of step** (frames played out): the position is set from the cell (`0x00439BA0`), the path shifts by one (`+0x13` ← `+0x14…`, `+0x32` = `0xFF`). Party: `0x0041F258` (keep attacking). Creatures: kinds `0x4D`/`0x4F` pause first (below), then the chase check `0x00411FD0`.
   * Path empty at the goal: party, a pick-up (`+3` bit `0x40`) or a map exit (grid bit `0x1000`, `0x00421BB8`), then the walk bit clears and the member idles (`0x00414AB4`); creatures idle (`0x00410450`).
   * Path empty elsewhere (the 31-step limit, or a cut path): idle bit, step `0xFF`; the planner or AI searches again.
   * Otherwise the next step starts (`0x004143DC(e, type)`).
6. **Start of a step** (`0x004143DC`, walk types 1/7, `0x0041492E`): the next cell from `+0x13` (`0x0043ECBA`).
   * **Occupied:** if it holds the walker's target (`0x00411758`), the walker attacks it (`+6`, goal = that cell, path cleared, stance on, swing 5). Otherwise the walk stops: idle bit, path cleared; the planner or AI decides again (the party's walk bit stays, so it searches around).
   * **Free:** the walker **moves into the next cell at once**: its old cell is freed and the new one taken at the start of the step. The body is drawn from the old cell's centre (`+0xE8/+0xF0`) plus the walk frame's offset, which carries it across during the step.

**Pauses of kinds `0x4D`/`0x4F`** (`0x004127BD`): at each step end, if the counter `+0x12` is 0 it is set to (rand() % 2) · (rand() % `+0xD7`, or 0 when `+0xD7` < 2) + 1 and the tick ends; while it is non-zero it counts down one per tick, and the walk goes on when it reaches 0.

## Port

* **Tick:** `startScene.Update` runs 1 + speed game ticks (`startScene.step`) whenever a 60 ms game frame has passed (updates · 5/18 at 60 Hz); the game time `0x00A3E770` is the tick count (`gameTime`).
* **Search:** `mapdata.FindPath` is `0x0043EDB7` (order, costs, goal rules, 4096-entry ring, at most `mapdata.PathSteps` = 31 cells). Tests: `mapdata.TestFindPathRules`, `TestFindPathKonung1` (a long walk in 31-step segments).
* **Walk step:** `walker.update`, once per tick: turn one octant toward the next cell, else the next walk frame; after the last frame the next cell and the next step. A taken next cell stops the walk (`walker.startStep`). Test: `game.TestHeroWalksToGoal` (ticks = frames per cell + octants turned).
* **Party:** a member whose walk stops short of its goal with the walk bit searches again (`startScene.step`, the planner's search); the pick-up and map exits happen only at the goal.
* **Creatures:** the throttle `+0x11` (`startScene.updateNPCs`) and the `0x4D`/`0x4F` pauses (`npc.pauseHold` as `walker.hold`). The pause counter `+0x12` is kept on the scene's `npc`, not in the entity, so a save does not keep it.

## Remaining differences

| | Original | Port |
|---|---|---|
| Goal blocked (player's order) | no walk | nearest free cell within 6 (`mapdata.NearestFree`), kept as a convenience |
| Cells held during a step | the new cell from the step's start; the old one freed at once | the next cell reserved, the old one freed at the step's end (both held) |
| The party's re-search | within the AI budget of 3 searches per tick | at once, outside the budget |
| A blocked step | stop; the planner (party) or the AI (creatures) decides on a later tick | the same, but the party searches again in the same tick |

Before the port, the port's A* (Euclidean pixel costs) found the same step counts but zigzagged: on `KONUNG.1`, `.4`, `.30`, `.36` (300 random pairs each) only 2–4 % of its routes matched the game's, with about 26 octants of turning per path against 2.
