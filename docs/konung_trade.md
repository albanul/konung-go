# Trade and the pile window (window 7)

Status: the window, its hit test, clicks, coin balancing, prices and the apply step are traced against `konung.exe`; both entry points are ported. Trading opens from a dialogue (script 275). The pile window opens when a pick-up stops at the bag or weight limit. Port: `gamestate.Trade` (`trade.go`), `game.tradeWindow` (`internal/game/trade.go`). Validation: `gamestate.TestPileTradeSwap`, `TestPileTradeEmpties`, `TestBarterMoney`, `TestTradeTooLittle`, `TestSellToShop`; `game.TestOverweightOpensPileWindow`, `TestTradeFromDialogue`.

## Entry points

* **Trade, script 275** (`0x00432FD0`, from a dialogue reply; every such reply ends the dialogue, so window 7 replaces window 1):
  * Hero money `0x00A3E7C8` = hero gold + 10 per «Неразменный пятак» (misc type `0x20`) in the hero's bag. The coin is never paid away.
  * Lists cleared; list 0 = the shown character's bag (`0x00A3E688`, the hero in a dialogue); scrolls cleared.
  * Post slot p = `0x0041182C(NPC)` (1-based). For p = 2–4 (healer, merchant, smith) it is a **shop**: mode `0x00A3E76C` = p, or −p when the village is the hero's (not independent); the shop's money is 99999; list 2 = the settlement's stock (p 2: `+0x412`, 24 entries; 3: `+0x444`, 32; 4: `+0x484`, 23), without weapons (category ≤ 2) while the village refuses armed strangers (bit `0x04`).
  * Anyone else **barters**: mode 6, money = its gold, list 2 = its bag.
* **Pile window** `0x004170A0(active pile)`: the pick-up `0x004110EC` stops when the next item would exceed the weight limit or the bag is full. It sets the shown character to the picker, opens the window and shows «Превышен вес» or «Мешок у тебя не резиновый!» (`0x0041121E`). Mode = the active pile's index; no trader (`0x00A3E69C` = 0); both sides' money 0, the hero's = gold. List 2 = the pile's 22 items.

## Window (`0x00431D70`)

`0x00A3E740` = 7. With T the view is tiled first (`0x0043141C`). INTR slot 6 (656×560) covers the view. «Закрыть» (slot 180) and «Ok» (slot 182) are drawn at their button positions. The trader's name is drawn in the large font, centred, at y = screen height / 24. Then the four strips (`0x0042CCD4`), the coin balance and six numbers.

800×600 layout (`resfmt.TradeLayout`, tables `0x00465AB4`, `0x00465A54`, `0x00465B54`; each record has 640 then 800 values):

| Item | Position |
|---|---|
| List 2, the other side's goods | (288, 50)–(794, 125) |
| List 3, what is taken | (288, 150)–(794, 225) |
| List 1, what is offered | (288, 344)–(794, 419) |
| List 0, the bag | (288, 438)–(794, 513) |
| «Ok» (hotspot `0xC9`) | (427, 260)–(520, 300) |
| «Закрыть» (hotspot `0xC8`) | (537, 260)–(630, 300) |
| Numbers: other money, other pays, other total | (205, 94), (216, 202), (216, 261) |
| Numbers: hero pays, hero money, hero total | (214, 344), (206, 448), (216, 286) |

A strip is drawn like the bag strip: the left arrow (slot `0x12`, `0x13` pressed) at the list's top-left, then 5 + video mode cells (6 at 800×600; slot 17, tinted for the shown character by `0x0042C914`), the icons centred, then the right arrow (`0x14`, `0x15` pressed) at the cells' height.

**Hit test** (`0x004392F4`): inside list k's rectangle (inclusive), x < x1 + arrow width → left arrow (`k<<8 | 0x78`); x ≥ x1 + arrow width + 6 · cell width → right arrow (`| 0x79`); else cell (`k<<16 | cell<<8 | 0x32`). Then the buttons (`0xC8 + i`) and the portraits; nothing else.

## Clicks (left button)

* **Cell** (`0x004232C0`): a non-empty slot (scroll + cell) moves to the first free slot of the paired list (0 ↔ 1, 2 ↔ 3); the source list closes up.
* **Arrows** (`0x0042358C`, `0x00423612`): scroll by one; left stops at 0, right only while the slot after the last visible cell holds an item (`0x0041EB9C`); the arrow is drawn pressed once.
* **«Закрыть»** (`0x004236D9`): `0x00420634(0)`; on success the window closes.
* **«Ok»** (`0x004236FB`): `0x00420634(1)`; on success the window reopens with the new state. The pile window reopens while the pile has items (`0x004170A0`); a trade reopens through script 275.
* A right click does nothing (`0x00423E26`). Escape closes the window without applying (`0x00437447`): the lists are copies, so nothing has changed. Hovering a cell shows the item's description (`0x004241A1`).

## Prices

* **Item price** `0x0041B348` (`Game.ItemPrice`):
  * Gear and jewellery: the type's price (gear `+0x36`, jewellery `+0x78`), plus an identified enchantment's value (`int16` at enchantment `+0`), at least 1.
  * Arrows: gear price × count.
  * Herbs: herb `+0x78` × 8. A negative price means (−price × concentration) × 8.
  * Other items: misc `+0x78`.
* **Sell price** `0x0041AEA0` (the trader pays for the offer, sum v): v · 0.5 · (1 + 0.002 · (hero trade − trader trade)), × 2/3 when mode > 0 (an independent village's shop, or a barter). 0 stays 0; below 1 becomes 1; truncated. Without a trader: v.
* **Buy price** `0x0041B65C` (the hero pays for the take): v · (1 − 0.002 · difference), × 1.5 when mode > 0.
* **Balance** (in the draw, `0x00432041`), with A = sell price + hero's coins `0x00A3E82C` and B = buy price + trader's coins `0x00A3E830`. If A > B, the hero's coins come back one by one, then the trader pays coins (from its money `0x00A3E7CC`) until even. If A < B it is the other way round: the trader's coins come back, then the hero pays from `0x00A3E7C8`. No balancing in the pile window. `Game.Balance`.

## Apply (`0x00420634(accept)`)

1. The shown character's new bag is the take then the kept bag (accept), or the offer then the kept bag (close). More than 32 items → «Мешок у тебя не резиновый!». Weight (without the bag `0x0041B898`, plus each item `0x0041B164`) above effective Вын · 1000/3 + 10000 → «Превышен вес». For a trade with accept: A < B → «Слишком мало даешь!» (`0x0045BF7C`). On any of these nothing changes and the window stays.
2. Accept: hero gold += trader's coins − hero's coins, not below 0. The bag is written.
3. **Pile window**, accept: the pile's items become the offer and the remaining goods, interleaved slot by slot, at most 22. An emptied pile is removed (`0x0041093C`, with the herb respawn rule; the window then closes). Close: the pile is unchanged.
4. **Trade**, accept: both trade skills (`+0x42`) grow by (sell price + hero coins) / 1024, at most 100.
   * Trader without a post (barter): with accept its gold += hero coins − its coins. Its bag becomes the offer (accept) or the take (close), interleaved with the remaining goods, at most 32.
   * Trader with a post, accept: the take leaves the post's stock and the offered items are destroyed (category `0xFF`). The game takes this path for every post slot ≠ 0, so the elder (slot 1) and the voevoda (slot 5) also take it. Their lists came from their own bag, which is never updated (a game bug; the port does the same).

## Port notes

* The pile window keeps the game pile's index (the game keeps the active-list index and closes the window when that index is 0; the port closes it when the pile is removed).
* The scene writes its party copies back before opening or applying, and reads them afterwards (`startScene.applyTrade`).

## Shop stock production

The shop lists are empty in `GAME.0`–`GAME.2`; the game fills them at runtime. Port: `gamestate.Game.SettlementPass`, `Game.Restock` (`production.go`); validation `TestHealerProduces`, `TestSmithProduces`, `TestMerchantRestock`.

**When.**
* The settlement pass `0x0041D39F` (every 16 ticks, `(time + 7) & 0xF` = 0) loops over **all 20 settlements**. In order it runs:
  * tribute accrual;
  * buildings ([buildings](konung_buildings.md));
  * the healer `0x0041768C`;
  * the smith `0x00417A58`;
  * when no building stage started, the voevoda's training `0x00418014` ([buildings](konung_buildings.md#voevodas-training-0x00418014)).
* Healer and smith return at once when their post is empty or window 7 is open (`[0x00A3E740]` = 7), so trading freezes production everywhere.
* The map loader runs `0x00418F0C` (call `0x0043CB8B`) for the entered map's settlement, also when a save is restored: the healer's jars and the merchant's stock.

**Tribute** (`0x0041D88A`): in a village that is not independent, counter `+0x14` +1 per pass. At 10080 the pool `+0x10` = trunc(pool + extra buildings (`+0`) · 20 · √(hero's trade) + members · 5 + 1), and the counter resets.

**Healer** (post 1, entity word `+0x3DA`).
* It works on a `u16` array H from `+0x40C`. H[0..2] are make-flags for Яд, Масло, Лечебный бальзам. H[3..26] (`+0x412`) is what the trade shows. H[27] is produced but never shown.
* Timer `+4`: −1 per pass. At ≤ 0 it resets to 1440 and sets all three flags. At 720 it sets Масло and Бальзам; at 1080 and 360, Бальзам only.
* Then, for i = 5..27, each empty slot:
  * 5/19, 6/20, 7/21: while the flag is set, a new herb (Белый корень, Ядовитое жало, Земляной орех) is mixed into a jar with the healer's skill (`0x0041BC24`, the potion recipes). The flag drops by one.
  * 8/22 … 13/27: two potions from slots 19–23 are combined (`0x00417268`): 19+20 Противоядие, 19+21 Брага, 19+23 Зелье, 20+21 Чистая слеза, 20+23 Элексир Мудрости, 21+22 Живая Вода. The second item is freed.
* No randomness and no gold. The healer's healing skill grows by the recipes' gains.
* On map load: rand() % 3 + 1 jars go into H[3] and H[17] when they are empty.

**Smith** (post 3, `+0x3EA`; stock `+0x484`, 23 slots; job slot int8 `+0x4B4`, −1 idle; job timer `+8`).
* Style k = `0x00417220(tribe − 1)`, with tribe = village group `+0x1E & 3`.
* Slots:

  | Slots | Gear types | Category |
  |---|---|---|
  | 0–2 | arrows 30–32 (stack of 30) | 12 |
  | 3–5 | blades 9k + i − 3 | 2 |
  | 6–8 | axes 9k + i − 3 | 0 |
  | 9–11 | bows 9k + i − 3 | 1 |
  | 12–14 | armour 11k + 0x22 + i − 12 | 3 |
  | 15–18 | shields, same formula | 5 |
  | 19–22 | helmets, same formula | 4 |

* **Idle:** the first empty slot whose work value W (gear `+0x2E`) the smith masters (smithing `+0x4C` · 10 ≥ W) becomes the job, taking trunc(W · 60 / √smithing) passes.
* **All mastered slots stocked:** the first stocked item that a village member can use (`0x00418364`) and lacks is handed over and its slot restarted.
  * Arrows go only into an empty quiver `+0xCC`.
  * Other items go into an empty equipment slot, or replace a lower type there; the old item is destroyed.
* **Job done:** the item is made (no enchantment; condition = W). The skill becomes trunc(√(skill² + (trunc(W/√skill) + 1)/24)), which in practice never rises.

**Merchant** (`+0x444`, 32 slots, map load only; the merchant post isn't consulted).
* L = hero level; R(n) = rand() % n for n ≥ 2, else 0 without a call.
* Ej = 3·R(L/8+1) + 10·(rand()%4 + 1) + rand()%2; Ew0 is the same without the +1.
* Slots 0–13 are filled only when empty, each with a chance roll `rand() % 100 > x`:

  | Slots | x | Item |
  |---|---|---|
  | 0–2 | 70 | jewellery: ring 6+R(L/5), bracelet 3+R(L/5) or necklace R(L/5) (rand()%30 < 10 / < 20 / else), enchantment Ej |
  | 3–4 | 50 | misc item rand()%10 |
  | 5–7 | 69 | weapon R(L/5+1) + 9·style + 3·rand()%2 (style `0x00417170`, one of two at random); 10 % enchanted |
  | 8–13 | 50 | armour, shield or helmet; 7 % enchanted |

* Slots 14–31 are freed and remade every load: bracelet, necklace and ring (Ej); two weapons and two bows of the two fixed styles; armour, shield and helmet of each style; a weapon and a bow (Ew0); armour, shield and helmet of a random style (Ej).
* The type ranges overflow their groups at high levels (from L 15/16/20), as in the game. Enchantment of a random roll: R(level band `0x004171D4`) + 10·rand()%5.
* Port note: the game keeps the freed indices of slots 14–31 until they are overwritten; the port clears them, so a failed allocation leaves no dangling entry.

**When stock leaves:** see [apply](#apply-0x00420634accept). The bought items' entries are zeroed and get refilled; items sold to a shop are destroyed.

## Open questions

* Building construction, burning and the voevoda's training, which share the settlement pass, are in [buildings](konung_buildings.md#not-ported--open).
