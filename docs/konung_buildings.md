# Village buildings and the voevoda's training

Status: traced against `konung.exe`. The map-load object IDs are checked against every `GAME.N` and `KONUNG.N`. Ported: `gamestate.Game.SettlementPass` (`production.go`), `buildingPass`, `voevodaPass`, `BuildingObjects` (`buildings.go`), `startScene.updateBuildings` / `placeObjects` (`internal/game/buildings.go`). `cmd/map-render -game` draws the buildings in their state. Tests: `TestBuildingObjectsMatchMaps`, `TestRuinRebuilt`, `TestBuildingBurns`, `TestVoevodaTrains`, `TestBuildingSpriteFollowsState`.

Both run in the settlement pass `0x0041D39F`, every 16 ticks (`(time + 7) & 0xF == 0`), for all 20 settlements. The rest of the pass (tribute, healer, smith) is in [trade: production](konung_trade.md#shop-stock-production). The surrender and alarm checks are in [combat](konung_combat.md).

## Building records

Settlement `+0x18 + 8r`, for r < 7 + extra buildings (`+0`) + fence pieces (`+1`). The record roles are listed in [dialogues](konung_dialogue.md#script-functions-verified).

| Offset | Type | Meaning |
|---|---|---|
| `+0` | int8 | building type, −1 = none (table `0x00458A30`) |
| `+1` | int8 | state |
| `+2` | uint16 | `KONUNG.N` section-3 placement index (the map object) |
| `+4` | — | not used by this code |
| `+6` | int16 | timer in passes |

The states are:
* 0, 1, 2: under construction;
* 3: standing;
* 4, 5: burning (these objects carry 1–19 flames each, drawn animated over the image);
* 6: ruin.

Every building object (all 40 types × 7 states) has one image: there is no frame animation beyond the flames.

**Building type table** `0x00458A30` (64 bytes per type):

| Offset | Type | Meaning | Accessor |
|---|---|---|---|
| `+0` | CP866 string | name | `BuildingName` |
| `+0x32` | uint16 | required building skill | `BuildingSkill` |
| `+0x34` | int16 | map object ID of state 0 | `BuildingObject` |
| `+0x36` | int16 | build time | `BuildTime` |
| `+0x38` | int16 | burn-stage time | `BurnTime` |

All accessors are on `resfmt.ExeTables`. **The placement shows object ID `+0x34` + state**, so seven consecutive OBJECTS slots per type.

**Map load** (`0x0043CB4A`, in the settlement loop at `0x0043CAC6`): for each settlement on the entered map, each typed record's placement `+0` is set to that ID. This also runs on a save restore. Port: `buildScene` applies `Game.BuildingObjects` before decoding the objects.
* In `GAME.0`–`GAME.2`, 628 of 712 records match the file's ID.
* The other 84 are unbuilt (state 0–2) records that the map files draw standing. Most are the fence of map 33, which shows as a dug trench until dialogue 361 starts the fence.

## Pass `0x0041D939`

1. **Builders** (head of the pass, `0x0041D404`):
   * humans = the village group's members that are alive, whose `+0` is not 9 or 3, and whose model `+0xD6` < 3;
   * busy = those of them with `+4 & 0x38`;
   * posted = the filled posts.
   * Builders B = humans − (busy + posted), or 0 while the group is alarmed (`+0x1D` = 1).
   * A village with no humans calms down: group `+0x1D` = 0, `+0x4B2 &= 0xEA`, tribute counter `+0x14` = 0.
2. **Per typed record:**

   | Condition | Effect |
   |---|---|
   | timer ≤ 0, state 6, B > 0 | rebuild: state 0, timer = trunc(60 · build time / B), "started" |
   | timer > 0, state < 3, B > 0 | timer −1. At ≤ 0: state +1, and unless it reached 3: timer = 60 · build time / B, "started" |
   | timer > 0, otherwise | timer −1. At ≤ 0: state +1, and unless it reached 6: timer = burn time |

   * A state change on the current map calls `0x0041733C(record)`. That routine sets the placement's object to `+0x34` + state and keeps the OBJECTS slot reference counts (`0x0083CA7C`, lazy loading). It then picks a random variant (`+0x23`, only when header `+0x18` > 0; every building object has `+0x18` = 0 and one variant) and gives each of the object's flames (header `+8`) a random start frame in its cycle (`+8 + i`, table `0x0045AEC8`). The port's flames already start at a random frame ([flames](konung_flames.md)). Port: `SettlementPass` reports the change, and `startScene.updateBuildings` swaps the sprites.
   * **Quirk (faithful):** without builders (alarmed or no free humans), a running construction steps on with the burn time. At state 3 it keeps its timer and burns down to the ruin.
3. Healer and smith follow ([trade](konung_trade.md#shop-stock-production)).
4. **When no record set "started"**, the voevoda's training runs.

Timings with 9 builders:
* the healer's hut (build time 12): 80 passes per stage;
* burning the elder's house (burn time 24): 24 passes per stage.

The scripts 355–361 start a construction by setting the timer to the plain build time ([dialogues](konung_dialogue.md#script-functions-verified)). That first stage is therefore not divided by the builders.

## Voevoda's training `0x00418014`

Post 4 (`+0x3F2`). If it is empty, nothing happens. Timer `+0xC` −1 per pass; at ≤ 0 it is reset to 720 and then, for each member m of the village group that is alive, not in action 9 or 3, not the voevoda, and with model < 3:

1. If `0x00410B08(m, 0)` (may level up, `Game.RaiseCost`): level `+0x33` +1, at no cost.
2. For Лов `+0x56`, Хар `+0x5A`, Сил `+0x60` and Вын `+0x64` (raise ids 2, 3, 5, 6), whenever m's base value is below the voevoda's (signed int16 compare):
   * gap += the difference;
   * free experience `+0x38` += 1000;
   * c = `0x00410B08(m, id)`; if c ≠ 0: experience `+0x3C` += c and the stat +1;
   * free experience −= 1000.
3. The level check again.
4. If gap ≠ 0:
   * Фехтование `+0x40` += (voevoda − m) / gap + 1;
   * Меткость `+0x44` the same;
   * both use C int division of the unsigned words, so the difference can be negative;
   * then `0x0041C750` (Recompute).

Net effect: each member gains at most one point per stat per period, up to the class caps, and free experience is unchanged.

## Not ported / open

* Setting a standing building on fire (timer > 0 at state 3): incendiary arrows ([combat](konung_combat.md#open-questions)).
* The map-exit gate (record 5) and other building effects outside the pass.
* Record `+4`.
