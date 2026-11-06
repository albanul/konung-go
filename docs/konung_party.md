# Party play

Group 0 is the party; its first entity is the hero (`0x00A3E694`). Every member runs the same party AI, the party branch of the dispatcher `0x00421DA4` (action switch `0x00422654`). The hero differs in only two ways: it never follows, and its death ends the game. The player gives orders to the **selection list**. Status: traced against `konung.exe` and ported. Code: `gamestate/party.go`, `game/party.go`.

## Intent bits (entity `+2`, verified)

| Bit | Meaning | Set by |
|---|---|---|
| `0x01` | melee: go for `+6`, else the nearest enemy | right click in the stance (`0x00423CC2`), A (`0x00421698`) |
| `0x02` | retaliating (`0x0041EC0C`); also set on members other than the hero by a left-click walk order (`0x00422A4C`). It is part of `& 0x0F`, so such a member does not follow. Cleared after a kill (`0x004220FC`) | |
| `0x04` | shoot `+6` (or a building, `+6 < 0`) | right click with the bow ready, A with the bow active |
| `0x08` | walk to `+0x0C/+0x0E` | walk orders, following |
| `0x20` | follow the leader | `0x00414AB4` (idle) for members other than the hero when `+2 & 0x0F` = 0; «Все ко мне» and hiring (`+2 = 0x60`) |
| `0x40` | idle: the AI may decide | `0x00414AB4` |

## Selection (`0x0083B93C`, verified)

The list holds 10 entity pointers and ends at the first null. Shift is `0x00A3E754`; the character window's character is `0x00A3E688`; the bag strip's owner is `0x00A3E730` (index + 1, 0 = hidden).

* `0x00425084(e)` **select**: without Shift the list becomes `[e]`. With Shift `e` is appended to the first free slot.
* `0x00439230(e)` **deselect**: removes `e` and moves the later entries up. If `e` owned the bag strip, the strip hides.
* `0x00411708(e)` tests membership.
* **Left click on portrait k with an empty hand** (`0x00422EA8`). A dead member is ignored, and so is any click in windows 1, 2 and 5.
  * Window 7 switches the trading character (not ported).
  * Window 3 makes the member the window's character and selects it.
  * Window 0:
    * An unselected member is selected. If the strip shows, the member becomes its owner.
    * A selected member toggles its own bag strip: it hides when the member already owns the strip, otherwise the member becomes the owner.
* **Right click on portrait k** (`0x00423D02`): with Ctrl and a dialogue, the dialogue opens. Otherwise the member is deselected, and if it owned the strip, the first selected member takes it over.
* **Right click on a party member on the map** (`0x004239EE`): with Ctrl and a dialogue, the dialogue opens. With Shift the member is toggled in the selection. Otherwise it becomes the whole selection.
* **Portrait hotspot**: the dispatcher `0x004392F4` returns `(k << 8) | 0x1E` for cell k only when k < the party count.

## Orders (verified)

| Input | Routine | Effect |
|---|---|---|
| Left click on the map | `0x004229F7` | every selected member: `+0x0C/+0x0E` = cell, `+2 = 0x48` (plus `0x02` for members other than the hero), `+0x14 = 0xFF`, `+3 &= ~0x40` |
| Right click | `0x00423B89` | per selected member: with the bow ready (`+0x12` = 1, a bow, a quiver, the stance), shoot the hovered character, or a building with incendiary arrows; on a character, attack it in melee if the member is in the stance (out of the stance nothing happens); on empty ground, walk there and pick up (`+3 \|= 0x40`) |
| Space / stance button | `0x0042179C` | the selection's stance; the first member decides |
| 1/2/3 / weapon button | `0x0042192C` | the first selected member; with Shift the whole selection |
| A | `0x00421698` | every living member: `+6` = 0, `+2 = 0x44`/`0x41`, the stance |
| F «Все ко мне» | `0x004226C0` | every living member except the hero: `+2 = 0x60`, `+0x14 = 0xFF` |

The HUD buttons show the state of the character window's character in window 3, otherwise the first selected member, otherwise the hero (`0x004290D8`). Pick-ups (`0x004110EC(e)`) and map exits (`0x00421BB8`, at `0x00422346`) apply to whichever member's walk ends on the cell.

## Idle members (`0x004223AD` → `0x0041F11C`, verified)

An idle member (`+2` bit `0x40`) within the path budget runs the planner `0x0041ED8C` when `+2 & 0x0F` ≠ 0. Otherwise it runs `0x0041F11C(group, e)`:

1. `0x00410E84(e)`: with the bow active, a bow and arrows, the first living member of the enemy group (`0x0041FD74`) with a clear line becomes `+6`. Nothing else happens: the planner then finds no intent and runs retaliation. A bow-armed member with an enemy in sight therefore does not follow.
2. Retaliation `0x0041EC0C` ([combat](konung_combat.md#melee-flow-verified)).
3. With `+2` bit `0x20`: `0x0041F5D8`. For group 0 (kind without bit `0x40`) this is **follow `0x0041FA5D`**:
   * L = the group's first entity. If `0x00439944(L, e)` < 10 → nothing.
   * The table parity is p = L.row & 1. The direction is d = `0x00439AA4(e → L)` and d2 = d − 1 (mod 8).
   * Four rounds. Each round starts at slot k = rand()%12. For 12 slots it tries the offset `0x0045FF79[p·0xC0 + d·0x18 + k·2]` (int8 row, col) from L, then the same slot with d2. A cell qualifies if it is inside 0 < row < 0x13F, 0 < col < 0x9F and its occupant bits `& 0xFFF` are 0. Then k++ (mod 12). After the round, d++ and d2−− (mod 8).
   * The first free cell becomes `+0x0C/+0x0E`, with `+2 |= 0x48` (the follow bit stays) and `+0x14 = 0xFF`.

**`0x00439AA4(r1, c1, r2, c2)`**, with dr = r2 − r1 and dc = c2 − c1:
* |dr| ≤ |dc|/2 → 0 if c2 > c1, else 4.
* r2 < r1 → 6 if |dc| ≤ |dr|/4, else 5 if c2 < c1, else 7.
* Otherwise → 2 if |dc| ≤ |dr|/4, else 3 if c2 < c1, else 1.

This is a table index, not the facing enum (`gamestate.FormationDir`).

## Placement after a map change (`0x004118DC`, verified)

The routine is called after an exit or a jump.

1. If the group cell (`+0x08`, `+0x10`) is inside the grid and free, the hero stands on it with the arrival facing `+0x0C`, `+2 &= 0xF0`, idle.
2. Every other living member is placed around it. If the group cell was taken, the hero is placed this way too.
   * The direction is the hero's facing turned around (f + 4 if f < 4, else f − 4), one octant less (−1) on rand()%2.
   * Eight directions, each starting at slot rand()%6 and trying the first 6 formation slots, relative to the group cell. The table parity is the group row & 1.
   * A placed member gets the arrival facing and `+2 &= 0xF0`.

`gamestate.Game.PlaceParty`. When the scene is the same map, the routine first clears the members' occupant bits.

**Dead members stay behind.** The leave cleanup `0x00436AD0(map)` loops over all 200 groups from group 0 and runs while group 0 is still on the map it leaves: the exit (`0x00421CCD`) sets only the arrival cell and facing, and `0x004118DC` writes the new map to `+0x1B` after the load (`0x00411999`). So a dead party member is removed like any other dead character: its items are freed, the later members move down one slot and the count drops. Its body stays on the old map until the cleanup; it is not on the new one, and it can no longer be revived. Port: `Game.LeaveMap` (from group 0), called by `startScene.changeMap` before `TakeExit`; the selection and the window's character are renumbered.

## Revival (`0x00414B3C`, verified)

The routine runs from «Живая Вода» (herb 13, f ≥ 10) on a dead character (`0x0041E101`). **«Слова Воскрешения»** (misc item type 6, case `0x00414FFB`, verified) runs it for every dead character of every group on the current map: the loop goes over all 200 groups whose map `+0x1B` is the current one (`0x00A3E7FC`), and over their members with `+4` bit `0x80`. Villagers and creatures come back too. Then item `+0` = `0xFF` (used up), and the common tail `0x00415BD3` removes it from the bag and plays sound 10. Nothing checks that anyone was dead, so a use with no dead still consumes it. Port: `UseMisc` sets `Game.ReviveAll`; `startScene.reviveAll` revives the party, then the others in group order; a revived NPC returns to its AI (`npc.liveMode`).

Kind `0x4C`'s `+0x12` reset compares the byte `+4` before the dead bit is cleared, so it never applies to a dead character; the port keeps that order.

`GAME.0` has five in ground piles: «Верхний лагерь» (14, cell 202,6), «Волхв у Пропасти» (32, 74,23) and «Борье» (33, 127,41) lie in the open; «Лесовье» (16, 132,38) and «Пещера Зверя» (43, 12,17) are buried (dig with the shovel). `GAME.1` has seven and `GAME.2` four.

1. **Cell.** The character keeps its own cell if free. Otherwise it takes the first free neighbour (`0x00410FFC`, 8 directions). Otherwise it scans up to 100 cells from a random start in a rectangle, columns first and wrapping:
   * the group's rectangle when the kind has `0xC0`;
   * else the hero's cell ± 2·20 rows and ± 2·9 cols.

   With no free cell found, the revival fails.
2. **State.** Revival is only possible on the map where the member died; leaving the map removes it (above). HP 1600, poison 0. Kind `0x4C` gets `+0x12` = rand()%5 + 1. The dead bit clears, the character goes idle (`0x00414AB4`) and its portrait is redrawn.

Port: `gamestate.Game.Revive`, wired through `Game.Reviver` to `startScene.revive`.

## Hit reaction and the portrait (verified)

* On every heavy hit (damage ≥ `0x30`), the party swing (`0x00422131`), the creature swing (`0x00412657`) and the arrow hit all call `0x004143DC(victim, 2 or 8)`. There is no walking check, so a walking victim stops and flinches. The planner resumes a walk order afterwards, because `+2` bit `0x08` stays set.
* A victim in the hero's group then gets its portrait redrawn (`0x0042D574`). While the member's type is 2 or 8, that routine calls `0x0043FD18(0x18, 0, 0)` (red +24 %) and draws the portrait with the palette built by `0x0043F763`. The portrait therefore shows red for the whole hit reaction (`gamestate.HitTint`, `hud.Draw`).

## Enemies and the party

The creature and village AIs target the whole party:

* the alarm checks see every living member (`startScene.partyPositions`, `partyOnCells`);
* the selectors pick the nearest member (`0x004202A8`), a group-mate's target (`0x00420178`) or the nearest unattacked member (`0x004203A0`) (`startScene.creatureTarget`, `nearestMember`);
* stationary creatures swing at any adjacent member;
* shooters take the first member with a clear line.

## Port

* **`gamestate`:** `Select`, `Deselect`, `Selected`, `FormationDir`, `FollowTarget`, `Game.PlaceParty`, `Game.Revive`, the `Intent*` bits; `resfmt.ExeTables.FormationOffset`.
* **`internal/game` (`party.go`):**
  * `member` has the walker, the `+2` bits (`intent`), `+6` (`target`), the building target and the pick-up flag;
  * `startScene.members` holds one member per party entry, `sel` is the selection and `cur` the character window's character;
  * `memberTick` and `idleTick` run the AI, `followLeader` the following, `rightOrder` / `walkOrder` / `attackOrder` / `gatherOrder` the orders;
  * `portraitClick`, `portraitRightClick` and `selectInWorld` change the selection.
* **Saving.** `storeWorld` writes each member's cell, facing and `+2` back; on rebuild, the strike, follow and idle bits survive.

Port choices and approximations:

* A dead member leaves the selection.
* The planner's enemy search (`0x0041FD74`) is centred on the member asking, not on the leader.
* The portrait clicks in window 7 (switching the trader) are not ported.
* A walk interrupted by a hit reaction snaps back to the cell it was leaving.

## Open questions

* What `+0x14 = 0xFF` does besides ending a walk after its step.
* Whether the game removes dead members from the selection.
