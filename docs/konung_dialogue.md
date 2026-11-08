# Dialogues and quests (`QUESTR.RES`)

Status: the file layout, the dialogue engine (open, branch, lines, replies, conditions, actions, quest flags), the dialogue window, the quest log and every script function used by the data are traced against `konung.exe` and checked against the file. Ported: `resfmt.ParseQuests` (decoder), `gamestate.Talk` (engine and script functions, `dialogue.go`, `scriptfuncs.go`, `scriptvillage.go`), `game.talkWindow` (windows 1 and 2, `internal/game/dialogue.go`). Nothing is left to port (`SPEECHS.RES` voices were never shipped). The settlement pass builds what 355–361 start ([buildings](konung_buildings.md)). Script 275 opens the [trade window](konung_trade.md).

Validation: `resfmt.TestParseQuests`, `TestDialogueLayout`; `gamestate.TestDialogueElder` (the elder of «Родники» through the Amulet chain), `TestDialogueRoles`, `TestQuestLog`, `TestScriptMoney`, `TestRecruitAndDismiss`; `game.TestRightClickTalks` (right click opens the dialogue and the hero does not move), `TestHireRebuildsScene`.

## `QUESTR.RES` layout (verified)

The game opens `QUESTR.RES` (installed as `questr.RES`) next to `GAME.N`. The loader `0x0043BDD5` reads seven blocks back to back; the sizes sum to the file (337,800 bytes):

| File offset | Size | Buffer | Contents |
|---|---|---|---|
| `0x00000` | `0x1F400` | `0x005D2D90` | 8000 × 16-byte **nodes** |
| `0x1F400` | `0x258` | `0x005F2190` | 300 × `uint16`: dialogue id (entity `+0xD0`) → first node |
| `0x1F658` | `0xFA00` | `0x005F23E8` | **string pool**: zero-terminated CP866 pieces |
| `0x2F058` | `0x2EE0` | `0x00601DE8` | **condition lists**: `uint16` terms, `0xFFFF` ends a list |
| `0x31F38` | `0x2EE0` | `0x00604CC8` | **action lists**: same |
| `0x34E18` | `0x1D4C0` | `0x00607BA8` | **texts**: lists of `uint16` pool offsets, `0xFFFF` ends |
| `0x522D8` | `0x4B0` | `0x00625068` | **quest flags**, 300 dwords ([saves](konung_save_format.md)) |

A list or text id is the `uint16` index of its first entry within its block.

**Node** (16 bytes, `resfmt.QuestNode`):

| Offset | Type | Meaning |
|---|---|---|
| `+0` | `uint32` | text id; `0xFFFFFFFF` = none |
| `+4` | `uint16` | next node; `0xFFFE` = end of dialogue; `0xFFFF` = back (a reply) / stop (a branch) |
| `+6` | `uint16` | condition list; `0xFFFF` = always |
| `+8` | `uint16` | `SPEECHS.RES` entry; `0xFFFF` = none. The file was never shipped; 22 nodes still name one |
| `+0xA` | `uint16` | action list; `0xFFFF` = none; in a header `0xFFFE` marks a **branch node** |
| `+0xC` | `uint16` | header: number of replies |
| `+0xE` | `uint16` | header: number of NPC lines (branch node: number of branches) |

A **dialogue node** is a header followed by its NPC lines, then its replies. A **branch node** (`+0xA` = `0xFFFE`) has `+0xE` branch records starting with the header itself; the first whose condition holds gives the next node (`+4`).

**Text** (`0x004363E8`): the pieces are concatenated. Every piece after the first gets a leading space when it starts with a letter (byte ≥ `0x41`, so CP866 Cyrillic too), a digit, or is a lone `-`. This is a word dictionary; punctuation pieces attach without a space. `resfmt.Quests.Text`.

**Quest flag** *i* (dword at `0x00625068 + 4i`): byte 0 bit `0x80` = set; `uint16 +2` = the quest-log text id (`0xFFFF` = none). 125 of the 300 flags have a log text; none is set at the start. `Game.QuestFlag`, `SetQuestFlag`, `QuestLog`.

## Condition and action terms (verified)

A term is a `uint16`: bits `0x01FF` = flag index or script function id; `0x2000` = call script function `0x0043259C(id)` instead of reading the flag.

* **Condition** (`0x00436248`): the result starts true; terms run left to right. A term with `0x4000` (AND) is evaluated only while the result is true and replaces it; a term without it (OR) only while the result is false. `0x8000` negates the term. A skipped term does not call its function (relevant for the random 277). `Talk.condition`.
* **Action** (`0x00435DBC`): each term calls its function, or sets (`0x8000`) / clears the flag. `Talk.actions`.

## Opening (`0x004364F0`)

* **Who.** Right click (`0x004238EE`) on a character under the cursor whose dialogue `+0xD0` ≠ `0xFF`:
  * anyone not in the party: Shift (`0x00A3E754`) not held and the hero not in the combat stance (`+3` bit 4) → `0x00423B6E`;
  * a party member: Ctrl (`0x00A3E714`, set by `VK_CONTROL` down `0x0043741A`, cleared on key up `0x004373CE`) held → `0x00423A41` (also from its portrait, `0x00423D93`).
  * The dialogue opens **at once, wherever the hero stands**: there is no walk to the character. The cursor shows sprite 5 (talk) over such a character.
* **Start.** The shown character `0x00A3E688` = the hero; node `0x00A3E7A4` = start node of `+0xD0`; previous node `0x00A5398C` = the same; NPC `0x00A3E69C` = the character.
* **Greeting** (`0x004365EF`, sound ids): `+4` bit `0x40` (women and children): `0x51` → 17, `0x52` → 18, `0x53` → 19, other → 16; men: `rand() % 4 + 12`, and if equal to the last greeting (`0x00A3E7C0`) the next one (16 wraps to 12).
* **Advance** `0x00435E78` runs; on 0 nothing opens; on 1 the greeting plays (−1, a line with speech: no greeting); then the window `0x0042DBBC(0)`.

## Advance (`0x00435E78`)

1. While the node is a branch node: take the first branch whose condition holds (or has none). If none holds, or its next is `0xFFFE`/`0xFFFF`, return 0 (no dialogue).
2. For each NPC line whose condition holds: run its actions; if it has text, list it (`0x007AA0DC`, 16 × 32 bytes, count `0x00A3E80C`); a listed line with speech plays it (`0x0042A9C4`) and makes the result −1.
3. For each reply whose condition holds and that has text: list it (`0x007A9EDC`, count `0x00A3E808`): `+0` node, `+8` text, `+0xC` speech, and when its next is `0xFFFF`, `+4` = the previous node `0x00A5398C`.
4. No reply listed → 0 (the window closes). Otherwise previous node = current node; return 1 (or −1).

Lines' actions run when the node is shown, **before** any reply: many nodes pay or give experience this way. `Talk.advance`.

## Choosing a reply (`0x00423501` → `0x0041F18C`)

A left click on reply *k* (hotspot `k<<8 | 0x64`, hit test `0x0043969C`): if the reply has speech, the game sets `0x00A3E704` and calls the voice player, and the click acts when the voice ends. `SPEECHS.RES` was never shipped, so no voice starts (`0x00A3E7A8` stays −1) and the main loop acts on its next pass (`0x004228C0`). The port acts at once. Then: run the reply's actions; next `0xFFFF` → back to the stored previous node; `0xFFFE` → close (`0x00A3E740` = 0, redraw `0x004292E4`); else that node. Advance; on 0 close, else redraw. `Talk.Choose`.

## Window 1: the dialogue (`0x0042DC50`)

`0x00A3E740` = 1. Layout table `0x00462EE4`, 8 bytes per video mode (`resfmt.DialogueLayout`); 800×600:

| Item | Position |
|---|---|
| INTR slot 1 (frame, 651×470, transparent inside) | (144, 0) |
| INTR slot 0 (divider) | (161, 187) |
| NPC lines, small font `0x005A31F4` | from (166, 25) |
| Replies; the one under the mouse in font `0x005A2EB0` (yellow) | from (166, 232) |
| Wrap width | 606 |

Each entry is drawn wrapped (`0x0042CBBC`, 15 px per line); the next starts 6 px below its last line. The entry's box (`+0x10..+0x1C`: x, x + width, top, bottom + 6) is the reply's click area (strict comparisons). With background tiling (T) the view is tiled first (`0x0043141C`). While window 1 is open the hit test offers only the replies, all other clicks are ignored (`0x00422EF5`), Escape does nothing (`0x00437451`) and the status line shows no hover text. The world keeps running. Port: `talkWindow.show`, `draw`, `replyAt`; `startScene.talkTo`, `chooseReply`.

## Window 2: the quest log (`0x0042DEDB`)

Q or HUD button 5 «Информация к размышлению (Q)» (hotspot 6) → `0x0042459C`: only with no window or the log open; plays sound 4, presses button 5 with image 12, and toggles window 2 (opening resets the scroll `0x00A3E7B4`). The log draws INTR slot 1 at the frame position, then for every set flag with a text (in flag order) after the first `0x00A3E7B4`: the text wrapped from (166, 25), 3 px apart, until y reaches the bag strip top − 45. Clicks: the areas of `0x0045B0AC` (800×600: (175,0)–(400,30) up, (410,0)–(800,30) down; hotspots `0xCA`, `0x1CA`) scroll by one entry (`0x00422A83`). Escape closes it. Port: `talkWindow.toggleLog`, `drawLog`, `scrollLog`.

## Script functions (verified)

`0x0043259C(id)`: five families of 56 ids, then the pointer table `0x004657F4 + id·4`. They are conditions (non-zero = true) or actions (result ignored). H = hero, N = NPC, S = current map's settlement (`0x00A3E6A8`; functions testing S return 0 without one). Port: `Talk.script`; ids without a Go function are reported in `TalkEvents.Unported` and logged.

| ids | Meaning |
|---|---|
| 0–55 | `0x00434160(map = id + 1)`: the first creature group (`+0x1E` bit `0x40`) on the map is below its target (`+0x1C` < `+0x18`) or has a dead member |
| 56–111 | `0x004351FC(map = id − 55)`: every member of the map's creature groups is dead (or of type 3/9) |
| 112–165 | `0x00434B60(type = id − 112)`: take quest item (category 11) of that type from H's bag; the bag closes up |
| 166–219 | `0x004359D4(type = id − 166)`: new quest item to the front of H's bag (`0x00424918`), else on the ground (`0x004138C8`) |
| 220–273 | `0x00434C90(type = id − 220)`: H's bag holds it (returns slot + 1) |
| 274 | N's group alarm `+0x1D` = 1; if it is S's village group, S `+0x4B2 \|= 0x15` (the port also sets the group's enemy to the party) |
| 275 | open the [trade window](konung_trade.md) (`0x00432FD0`): village posts 1–3 sell S's stock, others barter with their own gold and bag |
| 276 | H `+0xD3` = 70 (blessing: Хар +10, needs Вын ≥ 30) |
| 277 | `rand() % 100 < 50` |
| 278–281 | party has a warrior / healer / smith / merchant: `0x004323B0(4/1/3/2)`, members after the hero, alive, with role bits `+4 & 0x38` = k·8 or healing `+0x48` > 100 (k 1), trade `+0x42` > 30 (2), smithing `+0x4C` > 4 (3), Сил `+0x5E` > 10 and fencing `+0x40` > 10 (4) |
| 282 | repay the loan: with gold ≥ 120, gold −120 and clear S bit `0x20`, else 0 |
| 283 | collect tribute: gold += S `+0x10`, which becomes 0 |
| 284 | loan: S `+0x4B2 \|= 0x20`, gold +100; returns 0 |
| 285–290 | gold −5, −10, −20, −50, −100, −250 (no balance check; the conditions check first) |
| 291 | **hire N** (`0x00432820`): a trainee (role k = `+4 & 0x38` >> 3, 1–3) first gains skill for the days since S `+0x3F8 + 4k` (time/16/1440; healing and smithing move 3 %/day towards the post holder's, healing ≤ 500; trade +1/day, at most the merchant's and 100). N is appended to group 0 (`+5` = 0, `+2` = `0x60`, `+0x14` = `0xFF`, portrait variant `+0xF8` = (model·36 + (face−1)·9 + r)·5 + 1 when 0, r a free `rand() % 9`), leaves its group (memmove) and the posts are fixed up |
| 293–298 | gold +5, +10, +20, +50, +100, +250 |
| 299–301 | `0x0041079C(H, 10 / 20 / 50)` experience |
| 302 | S is independent (`+0x4B2` bit `0x10`) |
| 303, 325 | H has the blessing (`+0xD3` ≥ 0) / the curse (`+0xD2` ≥ 0) |
| 304, 314, 319, 321 | post 4 (voevoda), 1 (healer), 3 (smith), 2 (merchant) occupied (`+0x3D2 + 8k` ≠ 0) |
| 305, 307, 308, 309 | gold ≥ 30, 50, 100, 500 |
| 310 | gold ≥ hire price: 500 if N's level ≥ 4 (352) else 300, halved unless 302 |
| 311, 312 | S tribute `+0x10` ≠ 0; S exists |
| 313, 318, 329 | H's bag holds herb (category 9) type 1, 2, 0 |
| 315, 322, 320, 316, 317 | building record 1, 2, 3, 4, 6 exists (state `+1` > 0 or timer `+6` > 0) |
| 324 | H carries an unidentified item (`+0xE` bit 0) |
| 326 | S's village group has building skill `+0x46` ≥ 5 (`0x00432520`, the best member) |
| 328, 337–343 | something / healer's hut, barracks, well, smithy, shop, bridge, fence can be started: record type ≥ 0 (not checked for the fence), state 0, timer 0, best builder ≥ the type's skill (`0x00458A62 + type·64`). 341 uses record 1's type (game bug). The fence record is S `+0` + 7 and needs S `+1` ≠ 0 |
| 330 | H's HP < 1600 |
| 331 | identify everything H carries; H's curios `+0x4A` +1 each, at most 100 |
| 332 | **dismiss N into S's village** (`0x00433AF8`): fails if the group is full (`+0x1C` = `+0x19`); appended (`+2` = `0x60`), takes an empty elder post (dialogue = `+0x3D1`, `+2` = 0, `+4 &= 0xC7`), the party closes up |
| 333 | S loan outstanding (bit `0x20`) |
| 334, 335 | heal H / every living party member to 1600; N's healing += trunc((1600 − HP)·0.015) |
| 336 | S has an apprentice candidate (human model < 3, no role bits, alive, no post); an independent S also needs gold ≥ 100 |
| 344 | curse H (`+0xD2` = 19, Сил −5) unless H carries quest item 34 «Заячий хвост»; then returns 0 |
| 345–348 | install the party's voevoda / healer / smith / merchant at the free post (`0x00432654(k)`): dismiss, role bits cleared, post dialogue `+0x3D1 + 8k`, stance off, post entity set |
| 349 | party full: group 0 `+0x18` = `0x00411078(H)` (`MaxParty`), count ≥ it |
| 350, 352, 353 | N is in the party; N's level ≥ 4; N has role bits |
| 354 | the NPC holding post 1–3 takes the first candidate (336) as apprentice: role bits k·8, S `+0x3F8 + 4k` = time. The village AI then keeps the apprentice next to post k ([GAME.N: village AI](konung_game_format.md#village-ai-verified-against-code-and-game0)) |
| 355–361 | start building the healer's hut / barracks / well / smithy / shop / bridge / every fence piece: record timer `+6` = build time (`0x00458A66 + type·64`); no cost |
| 362 | smith repairs the party's gear (equipment, bag items of category ≤ 5): max condition `+8` above L = smithing·10 drops to L + (max − L)·0.7, condition `+4` = max |
| 363 | N vanishes: its post is cleared (later posts −1 only then), items freed, group closes up |
| 364 | lift the curse (`+0xD2` = −1) |
| 365 | S becomes the hero's vassal: `+0x4B2 &= 0xEA`, `+0x10` = `+0x14` = 0 |
| 366 | H's Хар `+0x58` > 60 |

Unused by the data: 285, 292 (a jewel into the bag), 293, 295, 306, 323, 327, 351 (N's HP ≤ 800).

**Settlement fields found here** (`gamestate.Settlement.Raw`): `+0` number of extra buildings; `+0x10` tribute pool, `+0x14` tribute counter; `+0x18 + 8k` building records (int8 type, state, …, int16 timer `+6`): 0 elder's house, 1 healer's hut, 2 shop, 3 smithy, 4 barracks, 5 bridge (the map-exit gate), 6 well, then the extra buildings and the fence pieces; `+0x3F8 + 4k` apprentice start times; `+0x412`/`+0x444`/`+0x484` shop stock; `+0x4B2` bit `0x20` loan. Posts: 1 healer (dialogue 101), 2 merchant (103), 3 smith (102), 4 voevoda (100).

**Building type table** `0x00458A30`, 64 bytes per type: CP866 name, `+0x32` required building skill (1/3/5), `+0x34` OBJECTS id, `+0x36` build time. `resfmt.ExeTables.BuildingSkill`, `BuildTime`, `BuildingName`.

## Port notes

* The scripts work on `Game.Entities`, as the game works on its global array: the scene writes its party copies back before a dialogue step (`startScene.newTalk`) and reads them afterwards. When the party changes (291, 332, 345–348, 363), entity records move, so the scene is rebuilt in restore mode (`startScene.rebuild`: no respawn, alarms kept, same piles, clock and arrows).
* The dialogue keeps the NPC index it was opened with, as the game keeps the pointer `0x00A3E69C`, even when a script moved the record.

## Open questions

* None. (`SPEECHS.RES` and `FACES<width>.RES`, which entity `+0xF8` would select a face from, were never shipped with the game; see [sound](konung_sound.md#files) and [interface](konung_interface_res.md#hired-faces-faceswidthres-never-shipped).)
