# LAN version (network play)

The single-player `konung.exe` has no network code (imports only GDI32, KERNEL32, USER32, WINMM, DSOUND, DDRAW; dynamic loads only `MP.DLL` and `VORBISFILE.DLL`). Network play is a separate program, `LAN_KONG.EXE` (PE timestamp 23.03.2003, `BEGTEXT` 0x54200 bytes against 0x37E00 in `konung.exe`). Fan accounts call the online variant "сетевой Князь": up to 16 players, 3 nations, 180-minute sessions, amulet carrying and PvP ([fan page](https://gospitaliers.narod.ru/knyaz_server.htm), unverified).

**Port decision:** our own network layer (not wire-compatible with `LAN_KONG.EXE`). The game rules come from the LAN exe; the transport is ours.

## Layout

The LAN files live in the game folder (`assets/Konung`) next to the single-player files, as in the original install: the GOG folder already held the LAN version's `GAME.RES`, `HISCORES.DAT`, `SERVER.LOG` and `Servers.cfg`, which `konung.exe` never opens. Names do not clash; `GAME.RES`, `LIGHTS.RES` and `SERVER.LOG` were byte-identical, and `HISCORES.DAT` is the LAN one (the GOG one was all zeros). The game only joins: sessions run on the dedicated server, a separate binary (`cmd/konung-server`, below); the game has no command-line modes and does not host.

## Port status

* Main menu «Сетевая игра» (port addition, `internal/game/lan.go`): opens the LAN menu (below). Shown only without a running game and only when `LAN_KONG.EXE`, `ME800R.RES` and `PLAY800R.RES` are in the game folder. The label is the large exe font (`FontLargeVA`) scaled 2×, tinted like the painted buttons (normal ≈ `MENU800R.RES` frame 9 lettering, selected ≈ frame 10), at (262, 196), where «Сохранить игру» is drawn in a game.
* Images: `go run ./cmd/res-gallery` → `output/site/lan`. Maps: `go run ./cmd/map-render -mapsres assets/Konung/MAPS.RES -all output/site/lanmaps`.
* LAN screens (`internal/game/lanui.go`, data from the exe via `gamestate.ParseLANExe`): the LAN menu, the rating screen (frame only), the login screen with typing, Tab, «Сохранить пароль», the portraits, the server list (LAN search on entering; Enter in the address field searches that address too), joining, the message box, «Ok» → wait, and `LAN.CFG` read and written in the original layout (`gamestate.LANConfig`). Verified under Xvfb (then still with in-game hosting): discovery, join, the duplicate-name refusal, wait and cancel.
* Network layer `internal/netplay` (adapted from the author's go-tetris `pkg/network`, MIT): newline-delimited JSON messages over TCP (`Conn`: ordered, keep-alive pings, a stalled peer is disconnected instead of messages being dropped, graceful close with a TCP half-close), a session server (`Listen`: join handshake with protocol version, name, password, clan and hero model; up to 16 players; chat; `MsgGame` handed to `OnGame`; membership messages in one order for everyone), a client (`Dial`) and LAN discovery (`ServeDiscovery`/`Discover`: UDP broadcast on the game port, replacing DirectPlay's session list). Port 7777 for both, as the original's TCP/IP port.
* Server settings (`cmd/konung-server/config.go`): name, port, max players, session minutes, world speed and team damage in one file, `DIR/server.conf` or `-config FILE` (`key = value`, `#` comments; [`konung-server.conf.example`](../konung-server.conf.example) shows every key; an unknown key or bad value stops the server). Order: defaults, the `SERVER` file (name, minutes), the config, then the flags given. `scripts/konung-server.sh` copies `konung-server.conf` from the repo root into the server at every start and takes the host port from its `port`.
* Dedicated server `cmd/konung-server` (`-data DIR`, `-config`, `-port`, `-max`, `-name`, `-minutes`, `-speed`, `-team-damage`; `Containerfile` for a container, `scripts/konung-server.sh` to run it: `KONUNG_NAME="…" scripts/konung-server.sh restart` names the server, and the container is re-created at each start so settings apply): accounts and progress in SQLite (`DIR/konung.db`, `internal/lanstore`), session name and length from `DIR/SERVER` if present, LAN discovery on the game port, the session clock with the rating update at each end. First login creates the account (the server logs «Новый игрок <name> клан <clan>»); a wrong password shows «Неправильный пароль»; after joining, the login screen shows the account's hero in the «Параметры»/«Навыки» boxes (at the anchor points).  Verified end to end under Xvfb with the real server: account created, wrong password refused, rejoin with the stored hero, leave, session ends.
* Entering the world: «Ok» sends `MsgEnter`; the server answers `MsgEntered` with the start ([game start](#game-start)), from `GAME.RES` (`netplay.StartFromGameRes`; the dedicated server needs `GAME.RES` in its data folder); the client builds the world (`game.lanWorld`): the `MAPS.RES` map, the hero from the model's single-player start (`SelectHero` + `StartHero`, with `GAME.<model>` as the template) and the account's values on top (`applyLANHero`: level, gold, experience, stats, skills, palette; HP 1600). Saving is hidden in a LAN world; starting or loading a single-player game drops the connection. Verified under Xvfb with the dedicated server: login, «Ok», the hero on the arrival pad of its village map, walking. Map changes use `GAME.RES`'s 192 map exits (`EnterLANWorld`; every map has 8 teleports, map 12 has 40; 177 sit under a teleport stone of their `MAPS.RES` map); a walk ending on one loads the target map at the exit's arrival cell. Port choice: the client changes maps itself — the original client skips the exits block and the server moves players between maps; that moves to the server with the world sync. Verified under Xvfb: from the start pad of map 1 through exit 9 to map 3. The map window (M) shows the LAN map ([map window](#map-window-m)) with the number of heroes per map (the count uses the small single-player font).
* Other players (`netplay` `MsgAppear`/`MsgMove`/`MsgMoved`, `game/lanremote.go`): on entering, the server sends the new hero's start, then every hero already in the world to it, and the new hero to everyone else (as the original's `0x8C` then `0x5A`); each client reports its hero's map, cell and facing when they change, and the server relays it to the others (as `0x19`; moves before entering are dropped, cells off the grid too). A remote hero on the own map is a walker (`npcRemote`) made from the own hero's template with the player's model and palette and the account's values, in an emptied group (no party, village or combat rule applies to it); it walks to each reported cell, is placed at once after a jump of more than 24 cells, leaves the map when its map changes, comes back after the own map change, and goes when the player leaves. It cannot be clicked (no orders, dialogue or selection); the cursor over it shows «<name>, клан <clan>». Heroes are named by their accounts (`Game.Names`, read by `Game.EntityName`): the own hero's status line and character window show the player's name, not the template's first name (Волк, Драгомир, Михаил). Verified under Xvfb with two game copies and the dedicated server: each sees the other, and a walk on one shows on the other. `GAME.RES`'s characters are not placed yet, and nothing beyond positions is synchronised (no combat, items or chat in the world).
* Port choices: no hosting from the game — the list leaves out the original's row 0 «Новый сервер» and shows only the sessions found; one connection type (the port's TCP/IP), so the list skips the original's DirectPlay connection stage; the list shows session names (with the host's address only when names repeat); the scroll arrows are clickable (the original's 800×600 rectangles were copied from the NEW screen); a name already in the session is refused with a port message («Ошибка: Игрок с таким именем уже в игре»; the original's reconnect path `0x00447CF8` is not ported); Escape in the LAN menu returns to the port's main menu; text uses the single-player fonts (`FontSlotVA`, focused `FontLargeVA`) — the LAN exe's fonts `0x7A5142`/`0x7AB36F` are not mapped.
* Server differences, on purpose: passwords are stored as salted PBKDF2-SHA256 hashes (the original kept them in plain text); accounts and items are SQLite rows, not fixed `0x80` / `0x200`-byte records; there is no 7-character password limit on the server (the client field still allows 7); a participant is marked at join, not at «Ok» (`0x4C` has no port counterpart yet).
* «Новый герой» (`gamestate/lanhero.go`, `game/lannewhero.go`; [new hero](#new-hero)): the NEW800R screen with the class, combat and peaceful skill, the stat trade on the LAN tables, «Макс. груз» / «Макс. отряд», the clan field and list (`CLANS.CFG`, typed clans and the server's list from the welcome), the hero preview and its look; «Ok» keeps the build, and the next join warns «Старый персонаж будет удален!» (message 1, Ok/Отмена) and sends it (`JoinInfo.NewHero`). The server checks the choices with the screen's own rules (`LANHeroChoice.Build`) and rebuilds the account's hero. Verified under Xvfb with the dedicated server: the screen, the warning, the rebuilt hero on the login screen.
* LAN mode (`game/menu.go`, `lanui.go`): «Сетевая игра» opens the LAN menu, which stays until «Выход»; Escape in a LAN world opens it with «Вернуться в игру» and «Закончить игру» (ME800R frames 3 and 15, as the original in a game), the hero staying online meanwhile; «Закончить игру» logs out to the LAN menu; «Новая игра», «Новый герой» and «Выход» end the game first. A lost connection and the hero's death also return to the LAN screens. No single-player menu items are reachable from a LAN game.
* The hero type is the login portrait at «Ok» (`Client.Enter(model)`, as the original's `0x4C` carries the model), not at the server pick: it sets the start village, the team and the look's palette, and the server keeps it (`ModelSetter`).
* World clock (`netplay/clock.go`, `game/lanclock.go`; port rule — the original's clients each ran their own): the server counts game ticks (60 ms, 1 + speed per 60 ms) while anybody is in the world and stops when it is empty; it sends the clock (`MsgClock`: tick, speed, session seconds left) on every entry, when the world empties or fills, at each session start and every 30 s. A client's scene runs ticks to keep up with it and jumps when more than 40 ticks off, so time of day and AI timers agree; the world speed is the server's (`-speed`, numpad +/− do nothing); the moonlit night is decided from the day number, the same everywhere; the session's time left is shown in the view's top-left corner («До конца сессии»). Verified under Xvfb with the dedicated server, together with the hero type at «Ok» and the LAN menu.
* Not ported: arrows between players drawn in flight on the other clients (the shooter's client draws its arrow), hit and death for heroes off the own map, items changing hands, healing synced to the server (the server's HP is the truth: a potion drunk between hits is undone by the next hit), `GAME.RES`'s villagers and creatures (the LAN entity layout), the account's items in the world (`SAVITEMS.DAT` → bag), the LAN HUD (`INTO640R`; the single-player HUD stands in), per-map single-player tables keyed by map number (fixed light, location names) applied to LAN maps, the rating rows on «Рейтинг» (the store has them: `lanstore.Store.Ratings`), «Установки» (state 8), in-game chat, gameplay — so no kills, amulets or items are produced yet; the store's `Score`, `SaveItems` and `LoadItems` wait for it.

## Files

All text files are CP866. VAs below are in `LAN_KONG.EXE` (`BEGTEXT` at `0x410000`, `DGROUP` at `0x470000`, file offset `0x54600`).

| File (all in `assets/Konung`) | Content | Status |
|---|---|---|
| `PLAY800R.RES`, `PLAY640R.RES` | Type 5, 14 frames (loader `0x004245B4`, image slots `0x1F2`+k): 0 login screen, 1 rating screen, 2–7 portraits (pairs colour = selected / grey), 8/9 «Отмена» normal/pressed, 10/11 «Ok» normal/pressed, 12 message box panel (513×214), 13 «Сохранить пароль» mark | verified |
| `ME800R.RES` | Type 5, 17 frames (loader `0x0042474C`): 0 LAN menu background, 1 settings background, 2 settings marker, 3–16 label overlays (353×345, drawn at (209,132)) in normal/hover pairs: 3 «Вернуться в игру», 5 «Рейтинг», 7 «Новая игра», 9 «Новый герой», 11 «Установки», 13 «Выход», 15 «Закончить игру» | verified |
| `NEW800R.RES`, `NEW640R.RES` | Type 5, 7 frames (loader `0x00424944`, image slots `0x1F9`+k): 0 «Новый герой» background, 1 100×100 tile repeated under it, 2 marker (13×13), 3/4 «Отмена», 5/6 «Ok» (normal/pressed) | verified |
| `INTO640R.RES` | Interface layout (`resfmt.ParseInterface`), 275 images: HUD frame, panels, item icons, portraits; drawn with the single-player `GRAPH.RES` palettes | decoded |
| `MAPS.RES` | 20 × (`KONUNG.N` + `LIGHTS.N`), map n at `(n−1) × 0x2F120` (loader `0x00417DB0`); none matches a single-player map ([map format](konung_map_format.md#lan-version-mapsres-verified)) | verified |
| `SOUNDS_.RES` | Same layout as `SOUNDS.RES` (`resfmt.LoadSoundEntries`): 244 sounds; 145 identical to single-player, 52 differ, 47 new | decoded |
| `LIGHTS.RES`, `GAME.RES` | One copy shared with the single-player files (the LAN copies were byte-identical); `GAME.RES` is the LAN world file, unused by `konung.exe` | verified by compare; `GAME.RES` layout open |
| `Quest_r.res` | Differs from `questr.RES`; the exe opens `QUESTR.RES` | open |
| `SPEECHS_.RES` | Voices (Type 1 header) | open |
| `MP3_SND.DLL` | `MP3_init`, `MP3_converttoWAV22M`, `MP3_close` | not analysed |
| `SERVER` | Server settings: `TEAM 3 180` (keyword `TEAM`, `%s %i %i`: mode, teams, minutes), `Home Server` (session name), 6 × `%i %i %i` (map x y; probably starts and scoring spots), `//` comments. Parsed at `0x00431481`; default name «Стандартный Сервер с Командной игрой» | keywords verified; line meanings open |
| `LAN.CFG` | Client settings (`gamestate.LANConfig`), read `0x00431176`, written `0x00432898` after every login edit; 486 bytes: dword 0 = resolution index `[0xA6EB2C]` with bit 31 = «Сохранить пароль» `[0xA6EB0C]`; dwords 1–6 = settings `0xA6EAC4`, `0xA6EAD8`, `0xA6EAEC`, `0xA6EAF4`, `0xA6EB10`, `0xA6EB18` (meanings open); name 33 (`0xA6E3B4`); password 9 (`0xA6E3D8`, zero unless «Сохранить пароль»); clan 32 (`0xA6E33C`); IP, modem, phone 128 each (`0xA83214`, `0xA83294`, `0xA83314`) | verified; not shipped (created by the game) |
| `LANUSERS.DAT` | Server accounts: 256 × 32-byte clan names, then 10000 × `0x80`-byte account records ([accounts](#accounts)) | verified (layout matches the shipped «HERO» record) |
| `CLANS.CFG` | «Византийцы», «Русские», «Викинги» | verified |
| `HISCORES.DAT` | Client-side cache of the 10 «Рейтинг» rows: 10 × `0x7C` (read `0x004313EC`, written `0x004173CE`; ratings ÷ 10); the server never reads it | verified |
| `SAVITEMS.DAT` | Server: 10000 × `0x200` bytes, one per account ([items](#items-between-sessions-savitemsdat)) | verified against code (not shipped) |
| `CLIENT.LOG`, `SERVER.LOG` | `%s rc=%i` | — |

`GRAPH.RES`, `OBJECTS.RES` and `FLAMES.RES` were not shipped with the LAN files; the single-player ones render the LAN maps and images without errors.

## Screens

States are `[0x9EB9EC]`; clicks go through the jump table `0x0042F7F8`, Escape through `0x0042D90D` (table `0x0042DA5C`). `[0xA6EAE0]` = 1 means no game is running. Hotspot tables hold 32-byte entries (640×480 rectangle, then 800×600; inclusive; ended by −1); the hit test `0x00417934` returns the first match. The port reads the tables and texts from the exe (`gamestate.ParseLANExe`).

| State | Screen | Draw / click |
|---|---|---|
| 0 | message box (`[0xA6EADC]` ≠ 0), or waiting for the server | `0x00424DE4` / `0x0042DB89` |
| 1 | in game (INTO HUD) | — |
| 2 | rating: PLAY frame 1, 10 best players and 10 best clans; any click → menu | `0x004271AC` / `0x0042E384` |
| 3 | LAN menu (ME800R) | `0x00427970` / `0x0042DE9C` |
| 4 | «Новый герой» ([new hero](#new-hero)): clan field and list, 4 classes, the combat and peaceful skill, −/+ for 4 stats, the hero preview stepping 8 looks; «Ok» keeps the build and opens the login | `0x00427D14` / `0x0042E3CA` |
| 5 | login «Новая игра» (PLAY frame 0) | `0x00429598` / `0x0042EC30` |
| 6 | quit | `0x0042F2FD` |
| 8 | settings (ME frames 1, 2; hotspots `0x0047EC04`) | `0x0042A1A8` / `0x0042F317` |
| 9, 10, 11 | transients: → menu (loads ME), → game (loads INTO), → login (loads PLAY, not hosting, list = connections) | `0x0042F596`, `0x0042F620`, `0x0042F727` |

Escape: 0 closes; 1 → in-game menu; 2 → game if one runs; 3, 4 → menu; 5, 6 → nothing; in a text field it only ends the edit.

**LAN menu** (hotspots `0x00479884`, all from (209,132), nested so the first match wins: bottoms 249, 299, 349, 399, 440; actions `0x0042E358`): 0 «Рейтинг» (in a game «Вернуться в игру»), 1 «Установки» (in a game «Закончить игру»: logout, or server shutdown `0x00432C18` when hosting), 2 «Новая игра» → login, 3 «Новый герой» → NEW, 4 «Выход» (logged in: quit request `0x54`).

The address line after joining shows the session name and ` [n]` when `[0x9EBA24]` ≠ 0 (`0x004298EA`): n = the online count from the login answer `0x87` (set at `0x00420927`), taken before the new player counts, so alone on a server only the name shows. The port shows the name only: the count read as part of the server's name, and the LAN map (M) shows the players.

**Login** (hotspots `0x0047E7A4`, switch `0x0042F269`; `gamestate.Login*`):

| # | 800×600 | What |
|---|---|---|
| 0–2 | (434,158), (544,158), (652,158), ~57×66 | portraits: hero models 1, 2, 0 (frames 2/3, 4/5, 6/7) |
| 3, 4 | (291,532)–(391,565), (400,532)–(500,565) | «Ok», «Отмена» |
| 5, 6 | (82,95)–(360,119), (448,95)–(727,119) | name (focus 2), password (focus 3) |
| 7, 8 | (549,49), (582,49) in the exe — a copy of the NEW screen's boxes | list scroll; the drawn arrows are at (346,192)–(362,207) and (346,481)–(362,492) |
| 9 | (76,161)–(354,186) | address line: the joined session's name, or a typed address (focus 6) |
| 10–18 | (76,207)–(354,230) … (76,454)–(354,478), ~31 px apart | 9 list rows |
| 19–32 | points x = 537 and 700, y = 284 … 398 | stat value anchors («Параметры», «Навыки»), shown after login from the hero record `0xA6E874` |
| 33 | (453,125)–(478,150) | «Сохранить пароль» (frame 13 centred when set) |

Texts are drawn at a hotspot's top-left +5,+5; the password is shown unmasked; the focused field uses font `0x7AB36F`, others `0x7A5142`, with a blinking `_` cursor. Fields (`0x0042CB94`; the limit includes the cursor): clan 31 bytes, name 32, password 8, chat 79, address 63; a character is also refused when the text + 15 px would not fit the field. Typing converts CP1251 → CP866; Tab switches name ↔ password; Enter/Escape end the edit and save `LAN.CFG`; Enter in the address field enumerates sessions at that address; a click anywhere ends the edit first.

List: on entering, the DirectPlay connections; picking one enumerates sessions once — row 0 «Новый сервер» (`0x00470695`), then the session names. Picking a row needs a name (message 4): row 0 hosts (`[0xA83398]` = 1, `0x00430800(−1)`, session name from `SERVER` line 2), others join. The client then logs in (`0x47`: name 32 + password 8) and sends ten `0x46` (not traced). The login answer (`0x004205B0`) gives the player slot (`0xFF` = refused); errors (`0x0042062C`) show a message. «Ok»: no name → 4; no server → 6; a new hero → message 1 (Ok/Отмена); else load `GAME.RES` and wait (state 0) until the server starts the game (state 10 → 1). «Отмена» → menu, address cleared.

**Message box**: PLAY frame 12 at (144,193); lines centred on x = 400 from y = 233; buttons (hotspots `0x00478F90`): message 1 «Ok» (241,347) and «Отмена» (453,347), others one «Ok» (347,347)–(452,393). Texts (`0x0047E264`, 0x80-byte records, line 2 at `+0x40`): 1 «Предупреждение: Старый персонаж будет удален!», 2 «Ошибка: Неправильная версия клиентской части» / «Выполните обновление Вашей программы», 3 «Ошибка: Слишком много игроков», 4 «Ошибка: Не указано имя игрока», 5 «Ошибка: Сервер не доступен», 6 «Ошибка: Не указан сервер», 7 «Ошибка: Неправильный пароль», 8 «На сервере идет выход персонажа из игры» / «Повторите вход в игру через некоторое время».

## New hero

State 4, drawn by `0x00427D14`, clicks `0x0042E3CA` (jump table `0x0042EB84`, hit test `0x00417934`). It is the single-player creation screen ([new game](konung_new_game.md)): the same 33-hotspot layout and the same class, skill and stat-trade handlers, with the LAN tables below. The hero portraits are replaced by the clan picker, and the preview switches the look. Opening it (`0x0042E0DB`) logs out (`0x44`), clears the server address and resets the template `0xA6E874` (LAN entity layout): the single-player template (Воин; Хар 3, Сил 15, Лов 3, Вын 10; Фехтование, Идентификация; no free points) at **level 13** (single-player 10), gold 1000, no items.

Hotspots `0x0047DE24` (33, 800×600 rects; `LANExe.NewHeroRects`): 0 clan field (31 bytes), 1/2 clan list scroll, 3–6 Купец/Охотник/Вождь/Воин, 7–10 values Хар/Сил/Лов/Вын, 11 «Макс. груз», 12 «Макс. отряд», 13 Кузнечное дело, 14 Плотницкое дело, 15 Фехтование, 16 Меткость, 17–19 Идентификация/Торговля/Знахарство, 20 «Ok» (85,545), 21 «Отмена» (195,545), 22 the hero (610,160; 160×160), 23 clan rows (33,176–361,521; +5, 15 px per row), 24 free points, 25–32 −/+ for Хар, Сил, Лов, Вын. Values in font `0x7A5142` at the rect's top-left; markers as in single-player (the class, the higher combat skill, Идентификация > 10, Торговля > 10, Знахарство > 50).

| Rule | LAN | Code / table |
|---|---|---|
| Class preset, skills | as single-player (`SelectClass`, `SelectCombatSkill`, `SelectSkill`) | `0x00437E86` handlers |
| Raise cost | first row of (favourite threshold, other threshold, cost) with value < threshold: (12,9,5) (19,14,15) (26,20,25) (34,28,35) (43,36,60) (53,44,100) (63,52,200) (74,60,300) (80,65,400) (90,70,500) (95,75,600) (100,80,800); paid from the free points, 0 if they don't cover it | `0x0044CDF0`, `0x00481E24`; `LANRaiseCost` |
| Favourite stat | Купец Вын, Охотник Лов, Вождь Хар, Воин Сил | `0x00481DDC` |
| Cap | value < cap at level 13: favourite 105, others 42 (every class) | `0x00481EC0` + class·0x68 + stat·0x1A + level·2 − 1 |
| − | a non-zero stat loses a point and the free points get that point's cost back | as single-player; `LANTradeStat` |
| «Макс. отряд» | 1 + the number of the class's thresholds base Хар reaches (Купец, Вождь 15/21/28; Охотник 17/23/30; Воин 20/28/36), at most 3 | `0x0044D798`, `0x00482190`; `LANMaxParty` |
| «Макс. груз» | 10 + base Вын/3 kg, `%4.1f` | `0x00427D14`; `LANMaxWeight` |
| Looks | the preview click steps a look index 0–7 (`[0xA6EB4C]`); palette = table[model][look]: 70,71,72,70,73,74,113,114 / 28,29,30,31,32,33,109,110 / 34,35,36,37,38,39,111,112 | `0x004821CC`, `0x004821EC`, `0x0048220C`; `LANLooks` |

Because free points start at 0, a build is a trade: the cost of each point depends only on its value, so any final stats are reachable when each is within 0 … cap and the points refunded cover the points spent (`LANHeroChoice.Build`). `TestLANHeroTables` checks the hard-coded tables against `LAN_KONG.EXE`.

«Ok» (`0x0042E756`) sets level 1 and the pending flag `[0xA6EAF8]`, backs the hero up (`0xA6E974`), saves `LAN.CFG`, adds a new clan to the list and opens the login; nothing is sent. With the flag set the login answer (`0x00420750`) keeps the built hero, and the login «Ok» shows message 1 «Предупреждение: Старый персонаж будет удален!»: «Ok» (`0x0042DBDB`) connects and sends `0x4C`; «Отмена» (`0x0042D9D8`) clears the flag for the LAN menu. «Отмена» on the screen (`0x0042E826`) returns to the LAN menu. The model is not chosen here: the login portraits (`0x0042ECA2`/`0x0042ECD2`/`0x0042ED02`) set the name index (124/151/10), the model (1/2/0) and the palette of the same look index.

`0x4C` (`0x004179B4`, 0x73 bytes) carries the name, password, clan and the whole hero on every entry. The server's check `0x00447460` accepts a level-1 hero with every stat ≤ 30 and free points ≤ 50 as long as gold and `+0x28` equal the record's; otherwise it needs the record's free points and, above level 1, its stats and class. Skills, model and palette are never checked. A rejected hero (`0x0044B513`) has its record reset (free points 0, gold 100, level 1, every stat 1) and gets message 7.

Port, with a server that decides: the client sends only the choices (`netplay.HeroChoice`: class, combat skill, peaceful skill, the four stats, the look) with the next join after the warning; the server rebuilds them with the screen's rules and refuses anything the screen cannot produce (`ErrHero`, port message «Ошибка: Сервер не принял нового героя»). The rebuild resets level, gold, stats, skills, free points and items, and keeps the rating and counters; the clan changes only with a rebuild (and on the first login). As the original, the preview opens on Михаил and one look index serves every hero type; the type stays a per-session choice at login («Ok»). The preview is drawn like the original: the hero image's top-left at (15, 20) in the 160×160 square at hotspot 22 (`0x004286CA` → `0x0044EE76`, no anchor; the single-player screen uses (60, 20)). Port addition: a right click on the preview shows the look on the next type (Михаил → Волк → Драгомир), a demonstration only that changes neither the build nor the login's type.

## Accounts

The LAN server (`[0xA83398]` set) keeps `LANUSERS.DAT` in memory (clans `0xC447B0`, accounts `0xE257C0`) on handle `[0xF5EC54]` (open/create `0x0043167B`; a new file gets the three default clans and zeroed records). Account index = (record − `0xE257C0`) / `0x80`; it also selects the `SAVITEMS.DAT` record. Online slot (0–15) → record: `[0xF5DFC0 + 4·slot]`. Port: `lanstore` (`accounts`, `clans` tables).

| Off | Size | Field | Default |
|---|---|---|---|
| `+0x00` | 32 | name (CP866; compared with `strcmp`, case-sensitive) | — |
| `+0x20` | 8 | password, plain text | — |
| `+0x28`, `+0x2C`, `+0x30` | i32 ×3 | kills: player heroes, hirelings, monsters | 0 |
| `+0x34` | i32 | amulet deliveries | 0 |
| `+0x38` | i32 | free points (entity `+0x12`, `0x004475F8`) | 0 |
| `+0x3C` | i32 | gold | 1000 |
| `+0x40` | i32 | palette = dye ÷ 512 (per-model tables `0x004821CC`/`0x004821EC`/`0x0048220C`, 8 each; first entries 70, 28, 34) | 34 |
| `+0x44` | i32 | rating, 0 … 10,000,000 | 1000 |
| `+0x48` | i32 | session points: 0 not in the session, −1 joined without points, > 0 points | 0 |
| `+0x4C` | i32 | unknown (entity `+0x16`) | 0 |
| `+0x50`…`+0x5C` | u16 ×7 | Фехтование ×100, Торговля, Меткость ×100, Плотницкое дело, Знахарство, Идентификация, Кузнечное дело | 100, 10, 0, 1, 50, 20, 10 |
| `+0x5E`, `+0x60` | u16 | Ловкость, Харизма | 3, 3 |
| `+0x62` | u16 | unknown (entity `+0x28`, checked at «Ok») | 10 |
| `+0x64`, `+0x66` | u16 | Сила, Выносливость | 15, 10 |
| `+0x68` | u8 | clan index | 0 |
| `+0x69` | u8 | level 1–13 (0 = re-init the record at load, `0x00447067`) | 1 |
| `+0x6A` | u8 | hero model (`0x004475F8`) | 2 |
| `+0x6B` | u8 | class (2–5) | 5 |
| `+0x6C` | 20 | unused | 0 |

Defaults: create path `0x00432E2B`. Saved (`0x004475F8`) on create, every score change (`0x0043B7EA`), logout/quit (`0x00443C1C`), the session's end (`0x0043459D`) and shutdown. The shipped record 0 («HERO»/«HERO», level 1, rating 1000, model 2, class 2, traded stats) fits the layout.

Port (`lanstore`): `+0x38` is the column `free_points` (schema version 2); the column `look` keeps the look index (version 3; version 2's per-type `look0`–`look2` are folded into it from the type last played). The model and palette are this session's: the type picked at «Ok» and the look's palette for it.

**Login flow** (server dispatcher `0x0043319C`, table `0x00482914`; packets: dword length, sender slot, then sub-messages with a type byte, `& 0x3F` = handler, bit `0x80` = sequenced): `0x47` login (`0x00432CF0`: name, client version byte = 6 else error 2, password; unknown name → new account; other password → 7; reply `0x87` with the hero, then the clan list); `0x46` ×10 rating rows (`0x0044A964`: rows 0–8 the table's top, row 9 the requester's own; reply `0x86`); `0x4C` enter the game at «Ok» (`0x0044ADB4`: 7 wrong password, 3 sixteen online, 8 still leaving; clan registered; «Новый игрок» printed; hero checked against the record by `0x00447460` — a failed check resets the hero to level 1 and answers 7; items loaded); `0x44` logout and `0x54` quit (save, leaving timer `0xA8C`). Errors: reply `0x85`, byte `+4` = message id.

## Items between sessions (`SAVITEMS.DAT`)

One `0x200`-byte record per account (handle `[0xF5EC58]`, create `0x00431784` fills `0xFF`), written by `0x00444918` from the hero, read by `0x0044444C` at «Ok» and at a new session. Port: `lanstore` `items` table.

| Off | Size | Content |
|---|---|---|
| `+0x00` | 6 × 7 | equipment: item `+1` (enchantment), `+3` (type), `+0` (category, `0xFF` empty) \| `0x80` (guess: identified by this player), high words of condition `+4` and maximum `+8` |
| `+0x2A`, `+0x2B` | 1, 1 | boxed arrows: type, count |
| `+0x2C` | 5 × 3 | jewellery: `+1`, `+3`, category \| flag |
| `+0x3B` | 32 × 7 | bag, as equipment (arrows keep their count) |

On load, bag items are dropped when category > 12, category ≤ 5 with type ≥ 70, category 9 with type ≥ 14, or category 11 with type ≥ 10 (amulet parts).

## Score and rating

`0x0043B67C(killer, victim, points)` (`gamestate.HeroKillPoints`, `lanstore.Store.Score`): an amulet part delivered on a scoring spot (table `0x004826D0`) = 25 points; a player hero's kill = max(1, 2·victim level − killer level + 1), doubled when the victim carries an amulet part; hireling and monster kills are counted with 0 points. Session end `0x00444150` (`gamestate.SessionRatings`, `lanstore.Store.EndSession`): over the participants (session points ≠ 0), R̄ = mean rating, P = sum of positive points, p̄ = P / n; each rating += (max(points, 0) − p̄) · R̄ / P, clamped to 0 … 10,000,000. Rating table `0x00446FA0` (10000 × `0x7C`, by rating; clan rating = members' mean, a guess): name, clan, amulets, the three kill counters, rating, clan rating, ranks.

## Map window (M)

Draw `0x0041E8EC`, the single-player window's scheme with LAN tables (`gamestate.LANExe.Map`, `MapMarkers`; drawn by `worldMapWindow.drawLAN`): parchment = interface slot 4, picture = `INTO640R.RES` slot 179 (502×361, a 5 × 4 grid of the 20 maps, each framed by its 8 teleports), clipped to row runs. Area per video mode `0x0047EFA4` (16 bytes: int32 x, y, width, height; 800×600: (80, 60), 502 × 361), row offsets `0x00477750`, runs `0x004714D0` — **region = map number** (1–20; 0 = none). Unlike single player there is no known-map gate: every location is drawn; the current map (`[0x9EBA30]`) with the palette tinted (0x30, 0, 0) (`0x0044FF51`). Then the number of players per map (the 16 player groups' `+0x1B`) is drawn at `0x0047F004`/`0x0047F058` (int32 x, y per map, from the area's top-left) + 3 (font `0x0078C1A0`). The hit test `0x004176AB` (`0x0044D3D0` on the same runs) only gives a hover id; there is no travel by click. Validation: `gamestate.TestLANMapWindow` (each map's count spot lies in its own region), `game.TestLANWorldMap` (all maps drawn, only the current one tinted); checked under Xvfb (map 1, then map 3 after a teleport).

## World (`GAME.RES`)

Client loader `0x0041DC84` («File with game resources not found»); `gamestate.LANExits`. Six blocks in `GAME.N`'s order, 365,509 bytes:

| Offset | Size | Content |
|---|---|---|
| `0x00000` | `0x20000` | 8192 × 16-byte items, as `GAME.N` |
| `0x20000` | `0xA00` | 80 × 32-byte groups, as `GAME.N`; `+0x1F` = group index. Groups 0–15 are the player slots (one entity each, entity index = slot); 16–18 the villages on maps 7, 4, 1; 19–24 lone characters; 25–78 creature groups on maps 2–20 |
| `0x20A00` | `0x20D00` | 2100 ground piles of `0x40` bytes (size inferred) |
| `0x41700` | `0x109A` | 250 × `0x11` map exits, as `GAME.N` (192 used); the client skips them, the server keeps them |
| `0x4279A` | `0x14000` | 320 × 256-byte entities, a LAN layout: `+0x05` group, `+0x06`/`+0x08` row/col, `+0x0D` level, `+0x0E` gold, `+0x1A`… skills, `+0x2E`/`+0x30`/`+0x34`/`+0x36` Ловкость/Харизма/Сила/Выносливость, `+0x32` HP, `+0x3F` hero model (the team), `+0x41` palette offset, `+0x45`/`+0x51`/`+0x91` equipment/bag/jewellery, `+0xBD` own index, `+0xE5`/`+0xED` world pixel x/y |
| `0x5679A` | `0x2C2B` | 3 × `0xEB9` settlements (maps 7, 4, 1) |

## Game start

Client «Ok» (`0x004179B4`) sends `0x4C` and waits (state 0, variant 10). Server `0x0044ADB4`: a free slot, the hero from the account, **start map = hero model · 3 + 1** (`0x0044401C`: the model's village), **start cell and facing = the arrival of the first map exit leading to that map** (`gamestate.LANStart`; shipped: model 0 → map 1 (110, 32) facing 2, model 1 → map 4 (66, 57) facing 0, model 2 → map 7 (65, 8) facing 4), placed by `0x00435830` when the cell is free (the fallback search is not traced), items from `SAVITEMS.DAT`, then `0x5A` (the hero's appearance, name, clan) to everyone and `0x8C` to the client: slot, map, entity word (bits 0–11 index, 12–13 weapon mode → entity `+0x0C`: 0 axe, 1 bow, 2 blade; 15 combat stance), (type << 4) | facing, row and col as single bytes. Client handler `0x0042105C`: its hero = entity `slot`, filled from its hero record (HP 1600), the map loaded from `MAPS.RES`, the hero placed; state 10 (`0x0042F620`) loads `INTO`, centres the camera on the hero and enters the game. **Team = hero model**, not the clan. `SERVER` lines 0–2 are the amulet parts' spawn spots (category 11, types 10–12; `0x004349FC`), lines 3–5 the delivery spots of models 0–2 (type 13, 25 points).

Other players reach the client as `0x5A`/`0x1A` (appearance: entity word, HP/16, model and kind, level, palette, equipment, jewellery, bag; players add name and clan, handler `0x00422C9C`) and `0x19` (9 bytes: entity word, (type << 4) | facing, HP/16, row and col bytes; applied on the current map, handler `0x0042294C`); items as `0x00`/`0x40` (`0x0042008C`).

## Fights

The original's server runs the whole world (tick `0x445988` over all 80 groups: groups 0–15 are the players' parties, group = slot; creature and village AI on the server; arrows `0x444CD0`); clients send orders and draw what the server reports. Verified in code:

* A world click sends `0x55` (builder `0x418E90`, server `0x449B24` → `0x446264`, sub-command 0): `+0xC`/`+0xD` row/col, `+0xE` target index + 1, `+0x10` map. A right click on an entity opens trade (`0x43`) only out of the stance, for a group ≥ 16, without Ctrl; otherwise it is an order. With a target in the stance the server shoots (bow `+0x0C` = 1 with arrows) or fights in melee, target in `+0xBB` (index + 1); targets in the own group are ignored.
* Hit `0x43E33C`, damage `0x43E968` on the server; a heavy hit plays the flinch (2 in the stance, 8 out of it), a kill experience `0x43E394` and the death (3/9, `0x4386A0`); the attacker gets xp/4 in `+0x12`. A player attacker scores `0x43B67C(attacker, victim, 0)` (melee `0x445D18`, arrows `0x444FF2`), then `0x5C` to it (`0x448970`, its stats; the account is saved), then loot `0x43BD54` (creatures drop their gold; players keep theirs).
* `0x19` action nibble: 0 idle, 1 walk, 2 flinch, 3 death, 4 shoot, 5 melee in the stance, + 6 out of it; the client starts the action (`0x412450`). `0x1A`/`0x5A` (`0x448524` → `0x422C9C`): entity word, HP/16, (model << 4) | class, level, palette/512, 2 bytes of `+0x38`, equipment 12 bytes, jewellery 10, bag 64, then name 32 and clan 32 for players (`0xA1` bytes, else `0x61`). The draw (`0x4197A2`) reads `+0x0C` as the weapon slot.
* Same team (model) heroes can hurt each other: only the group is compared. No safe areas. A hit on a player marks the attacker in the map record's `+0x4E` when the map's village side (`+0x1E & 3`) is the target's model (orders) or always (arrows) — read as the village's aggressor list (guess).
* Death: at the end of the death action (`0x436A46`) `+4 |= 0x80` and the hero leaves the grid. No respawn: HP 1600 is written on entry (`0x44B263`), at the session's end (`0x448BB8`: every hero back to its village, full HP, out of the stance), and by heals and items.

Messages client → server (type | `0x40`, table `0x482914`): `0x4B`/`0x55` orders (`0x449B24`), `0x4A` item and HUD events (`0x44AC78`), `0x4D` give an item (`0x44BA34`), `0x42`/`0x43` trade (`0x449DB4`/`0x44A3BC`), `0x53` dialogue script function (`0x44BCC4`), `0x57` chat (`0x44C9B0`), `0x5D` stat raise (`0x44CB48`). Server → client: `0x19` (`0x448410`), `0x1A` (`0x448524`), `0x1B` (`0x44875C` → `0x422ED8`, probably a walk path), `0x1C`/`0x5C` (`0x448970` → `0x423180`), `0x12`/`0x52` (`0x448182` → `0x42219C`, probably a shot), `0x00`/`0x40` items (`0x4478B4`). Not traced: `0x08`/`0x48`, `0x09`/`0x49`, `0x4E`, `0x4F`, `0x50`, `0x51`, `0x16`/`0x56`, `0x5E`, `0x58`.

**Port** (`netplay/fight.go`, `internal/lanfight`, `game/lanfight.go`). Each client runs its own world, so only fights between heroes are decided on the server:

* Look: the client reports its stance, weapon mode and equipment (`MsgLook`); the server keeps the stance and weapon through `Combat.Stance` (a weapon slot without an item is not taken) and sends the look to everyone; `MsgAppear` carries the look, HP and death. A remote hero is drawn with its own item records (`Game.SetItemKind`), out of the stance at type 6, in it at the combat set with the stance bit (`EnterStance`/`LeaveStance`).
* Attack: in the stance a remote hero is a target (right click, A); a blow at the end of the swing or an arrow that reaches it is reported (`MsgAttack`) instead of resolved. The server checks both are in the world, alive, on one map, the attacker in the stance, a blow within 2 cells, at most one strike per 3 ticks, and lets `lanfight.World` decide: every hero is the hero type's `GAME.N` hero (gear, quiver) with the account's values (`lanfight.Hero`, which the client uses for its own hero too), in one shared `Game`, struck with `Strike` or `Shoot` + `ArrowAttack` + `ArrowDamage` (the bow's range), and `Kill`/`KillByArrow`. The verdict (`MsgHit`: result, damage, HP, death) goes to everyone; the attacker's swing or shot, the target's flinch or death play. A kill scores `HeroKillPoints` (`OnKill` → `lanstore.Score`, the killer's experience `SaveProgress`, `MsgHero` to the killer). The own hero's HP changes only by verdicts; its death is the game over (`GAMEOVER.FLM`, then the LAN menu), and it stays dead until it enters again, as the original.
* Team damage is a server option: `-team-damage` (default true, as the original); with `-team-damage=false` (`ServerConfig.TeamSafe`, announced in `SessionInfo.TeamSafe`) the server refuses blows between heroes of one hero type, and clients don't target teammates (hover, A).
* The fighters carry the account's class (`ApplyHero` sets entity `+4` from record `+0x6B`), on the server and in the own game.
* The server needs `konung.exe` and `GAME.0`–`GAME.2` in its data folder for this (`scripts/konung-server.sh` copies them); without them heroes cannot hurt each other.
* Verified under Xvfb with two game copies and the dedicated server: the stance and drawn sword of the other hero, a fight to the death («Ivar killed Olaf: 2 points»), the victim's game over, its re-entry alive.

## Networking (verified in code)

| What | Detail | VA |
|---|---|---|
| API | `DPLAYX.dll` ordinals 2 and 4, `CoCreateInstance(CLSID_DirectPlay, IDirectPlay4A)` | `0x004303C9` |
| Address | Service provider chosen from the enumerated list (`0xA82FD4`, `0x90` bytes each). TCP/IP: `DPAID_INet` = typed IP (empty searches the LAN), `DPAID_INetPort` = 7777 (`0x1E61`). Modem: `DPAID_Phone`, `DPAID_Modem`. IPX/serial: provider only | `0x004300DF` |
| Join | `EnumSessions` with the app GUID at `0x004712EC`, retried while `DPERR_CONNECTING`; sessions listed at `0x9889EC` (`0x50` bytes: name + GUID at `+0x40`) | `0x004300DF` |
| Host | `Open(DPOPEN_CREATE)`, session flags `0x1040` (client/server, keep-alive), session name `0xE23CC4`, `CreatePlayer(DPPLAYER_SERVERPLAYER)` | `0x00430803` |
| Send | 1 byte type + payload (≤ `0x200`), guaranteed, to the server player | `0x0042FDE4` |
| Receive | `Receive(DPRECEIVE_ALL)` into a `0x200` buffer | `0x0042FCD4` |
| Server mode | `[0xA83398]`: send/receive go through local queues (`0xA8FDE8`, `0xA83458`, `0x204` per entry), apparently for the host's own player (hypothesis) | `0x0042FCD4`, `0x0042FDE4` |

## Open

* The client's side of the rating rows (`0x86` answers to the ten `0x46` requests) and the «Рейтинг» screen layout (`0x004271AC`).
* The settings screen (state 8); the LAN fonts `0x7A5142`/`0x7AB36F`; the hero fields `+0x16`, `+0x28` (record `+0x4C`, `+0x62`) and `+0x3E`.
* The message types and payloads (callers of `0x0042FDE4`: `0x417CD2`, `0x41FFD5`, `0x42E054`, `0x42E0D1`, `0x42E147`, `0x42E34C`, `0x42F09C`, `0x42F1EF`, `0x42F8CF`; receive dispatch from `0x42F8A1`).
* Server rules still open: the `0x53` broadcast, the `0x1B` move message, how a client entering gets the others' positions, the free-cell search at entry, respawn, row/col sent as single bytes although rows reach 319, the saves at `0x00448A6A` and `0x0044CCF3`, the account fields `+0x4C` and `+0x62`.
* Which `MAPS.RES` map is which cell of the a1–d5 grid; the six `SERVER` lines; `GAME.RES`; `Quest_r.res`; item differences from single-player.
