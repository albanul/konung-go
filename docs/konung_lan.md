# LAN version (network play)

The single-player `konung.exe` has no network code (imports only GDI32, KERNEL32, USER32, WINMM, DSOUND, DDRAW; dynamic loads only `MP.DLL` and `VORBISFILE.DLL`). Network play is a separate program, `LAN_KONG.EXE` (PE timestamp 23.03.2003, `BEGTEXT` 0x54200 bytes against 0x37E00 in `konung.exe`). Fan accounts call the online variant "сетевой Князь": up to 16 players, 3 nations, 180-minute sessions, amulet carrying and PvP ([fan page](https://gospitaliers.narod.ru/knyaz_server.htm), unverified).

**Port decision:** our own network layer (not wire-compatible with `LAN_KONG.EXE`). The game rules come from the LAN exe; the transport is ours.

## Layout

The LAN files live in the game folder (`assets/Konung`) next to the single-player files, as in the original install: the GOG folder already held the LAN version's `GAME.RES`, `HISCORES.DAT`, `SERVER.LOG` and `Servers.cfg`, which `konung.exe` never opens. Names do not clash; `GAME.RES`, `LIGHTS.RES` and `SERVER.LOG` were byte-identical, and `HISCORES.DAT` is the LAN one (the GOG one was all zeros). The game is one binary: hosting and joining happen from its UI, with no command-line modes. A dedicated server that keeps the players' progress is a separate binary, `cmd/konung-server` (below).

## Port status

* Main menu «Сетевая игра» (port addition, `internal/game/lan.go`): opens the LAN menu (below). Shown only without a running game and only when `LAN_KONG.EXE`, `ME800R.RES` and `PLAY800R.RES` are in the game folder. The label is the large exe font (`FontLargeVA`) scaled 2×, tinted like the painted buttons (normal ≈ `MENU800R.RES` frame 9 lettering, selected ≈ frame 10), at (262, 196), where «Сохранить игру» is drawn in a game.
* Images: `go run ./cmd/res-gallery` → `output/site/lan`. Maps: `go run ./cmd/map-render -mapsres assets/Konung/MAPS.RES -all output/site/lanmaps`.
* LAN screens (`internal/game/lanui.go`, data from the exe via `gamestate.ParseLANExe`): the LAN menu, the rating screen (frame only), the login screen with typing, Tab, «Сохранить пароль», the portraits, the server list (LAN search on entering; Enter in the address field searches that address too), hosting («Новый сервер»: server + discovery in the game process, `netplay.Host`, named from `SERVER`), joining, the message box, «Ok» → wait, and `LAN.CFG` read and written in the original layout (`gamestate.LANConfig`). Verified by running two game copies under Xvfb: host, discovery, join, the duplicate-name refusal, the live ` [n]` count, wait and cancel.
* Network layer `internal/netplay` (adapted from the author's go-tetris `pkg/network`, MIT): newline-delimited JSON messages over TCP (`Conn`: ordered, keep-alive pings, a stalled peer is disconnected instead of messages being dropped, graceful close with a TCP half-close), a session server (`Listen`: join handshake with protocol version, name, password, clan and hero model; up to 16 players; chat; `MsgGame` handed to `OnGame`; membership messages in one order for everyone), a client (`Dial`) and LAN discovery (`ServeDiscovery`/`Discover`: UDP broadcast on the game port, replacing DirectPlay's session list). Port 7777 for both, as the original's TCP/IP port.
* Dedicated server `cmd/konung-server` (`-data DIR`, `-port`, `-max`, `-name`, `-minutes`; `Containerfile` for a container): accounts and progress in SQLite (`DIR/konung.db`, `internal/lanstore`), session name and length from `DIR/SERVER` if present, LAN discovery on the game port, the session clock with the rating update at each end. First login creates the account (the server logs «Новый игрок <name> клан <clan>»); a wrong password shows «Неправильный пароль»; after joining, the login screen shows the account's hero in the «Параметры»/«Навыки» boxes (at the anchor points). In-game hosting («Новый сервер») keeps no accounts. Verified end to end under Xvfb with the real server: account created, wrong password refused, rejoin with the stored hero, leave, session ends.
* Entering the world: «Ok» sends `MsgEnter`; the server answers `MsgEntered` with the start ([game start](#game-start)), from `GAME.RES` (`netplay.StartFromGameRes`; the dedicated server needs `GAME.RES` in its data folder, in-game hosting reads the game folder's); the client builds the world (`game.lanWorld`): the `MAPS.RES` map, the hero from the model's single-player start (`SelectHero` + `StartHero`, with `GAME.<model>` as the template) and the account's values on top (`applyLANHero`: level, gold, experience, stats, skills, palette; HP 1600). Saving is hidden in a LAN world; starting or loading a single-player game drops the connection. Verified under Xvfb with the dedicated server: login, «Ok», the hero on the arrival pad of its village map, walking. Only the hero is on the map: `GAME.RES`'s characters and the other players are not placed yet.
* Port choices: one connection type (the port's TCP/IP), so the list skips the original's DirectPlay connection stage; the list shows session names (with the host's address only when names repeat); the scroll arrows are clickable (the original's 800×600 rectangles were copied from the NEW screen); a name already in the session is refused with a port message («Ошибка: Игрок с таким именем уже в игре»; the original's reconnect path `0x00447CF8` is not ported); Escape in the LAN menu returns to the port's main menu; text uses the single-player fonts (`FontSlotVA`, focused `FontLargeVA`) — the LAN exe's fonts `0x7A5142`/`0x7AB36F` are not mapped.
* Server differences, on purpose: passwords are stored as salted PBKDF2-SHA256 hashes (the original kept them in plain text); accounts and items are SQLite rows, not fixed `0x80` / `0x200`-byte records; there is no 7-character password limit on the server (the client field still allows 7); a participant is marked at join, not at «Ok» (`0x4C` has no port counterpart yet).
* Not ported: the other players in the world (`0x5A` appearance, `0x19` position), `GAME.RES`'s villagers and creatures (the LAN entity layout), the account's items in the world (`SAVITEMS.DAT` → bag), the LAN HUD (`INTO640R`; the single-player HUD stands in), per-map single-player tables keyed by map number (fixed light, location names) applied to LAN maps, the hero check at «Ok» (`0x00447460`), the rating rows on «Рейтинг» (the store has them: `lanstore.Store.Ratings`), «Новый герой» (state 4), «Установки» (state 8), «Макс. груз» / «Макс. отряд» (derived), in-game chat, gameplay — so no kills, amulets or items are produced yet; the store's `Score`, `SaveItems` and `LoadItems` wait for it.

## Files

All text files are CP866. VAs below are in `LAN_KONG.EXE` (`BEGTEXT` at `0x410000`, `DGROUP` at `0x470000`, file offset `0x54600`).

| File (all in `assets/Konung`) | Content | Status |
|---|---|---|
| `PLAY800R.RES`, `PLAY640R.RES` | Type 5, 14 frames (loader `0x004245B4`, image slots `0x1F2`+k): 0 login screen, 1 rating screen, 2–7 portraits (pairs colour = selected / grey), 8/9 «Отмена» normal/pressed, 10/11 «Ok» normal/pressed, 12 message box panel (513×214), 13 «Сохранить пароль» mark | verified |
| `ME800R.RES` | Type 5, 17 frames (loader `0x0042474C`): 0 LAN menu background, 1 settings background, 2 settings marker, 3–16 label overlays (353×345, drawn at (209,132)) in normal/hover pairs: 3 «Вернуться в игру», 5 «Рейтинг», 7 «Новая игра», 9 «Новый герой», 11 «Установки», 13 «Выход», 15 «Закончить игру» | verified |
| `NEW800R.RES`, `NEW640R.RES` | Type 5, 7 frames (loader `0x00424944`): 0 «Новый герой» background, 1 portrait (100×100), 2 marker, 3–6 «Ok»/«Отмена» pairs | verified |
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
| 4 | «Новый герой» (NEW800R; hotspots `0x0047DE24`): clan field and list (from `CLANS.CFG`), 4 classes, 5 skill toggles, −/+ for 4 stats, portrait cycling 8 portraits per model; «Ok» saves the hero and opens the login | `0x00427D14` / `0x0042E3CA` |
| 5 | login «Новая игра» (PLAY frame 0) | `0x00429598` / `0x0042EC30` |
| 6 | quit | `0x0042F2FD` |
| 8 | settings (ME frames 1, 2; hotspots `0x0047EC04`) | `0x0042A1A8` / `0x0042F317` |
| 9, 10, 11 | transients: → menu (loads ME), → game (loads INTO), → login (loads PLAY, not hosting, list = connections) | `0x0042F596`, `0x0042F620`, `0x0042F727` |

Escape: 0 closes; 1 → in-game menu; 2 → game if one runs; 3, 4 → menu; 5, 6 → nothing; in a text field it only ends the edit.

**LAN menu** (hotspots `0x00479884`, all from (209,132), nested so the first match wins: bottoms 249, 299, 349, 399, 440; actions `0x0042E358`): 0 «Рейтинг» (in a game «Вернуться в игру»), 1 «Установки» (in a game «Закончить игру»: logout, or server shutdown `0x00432C18` when hosting), 2 «Новая игра» → login, 3 «Новый герой» → NEW, 4 «Выход» (logged in: quit request `0x54`).

**Login** (hotspots `0x0047E7A4`, switch `0x0042F269`; `gamestate.Login*`):

| # | 800×600 | What |
|---|---|---|
| 0–2 | (434,158), (544,158), (652,158), ~57×66 | portraits: hero models 1, 2, 0 (frames 2/3, 4/5, 6/7) |
| 3, 4 | (291,532)–(391,565), (400,532)–(500,565) | «Ok», «Отмена» |
| 5, 6 | (82,95)–(360,119), (448,95)–(727,119) | name (focus 2), password (focus 3) |
| 7, 8 | (549,49), (582,49) in the exe — a copy of the NEW screen's boxes | list scroll; the drawn arrows are at (346,192)–(362,207) and (346,481)–(362,492) |
| 9 | (76,161)–(354,186) | address line: the joined session + ` [n]`, or a typed address (focus 6) |
| 10–18 | (76,207)–(354,230) … (76,454)–(354,478), ~31 px apart | 9 list rows |
| 19–32 | points x = 537 and 700, y = 284 … 398 | stat value anchors («Параметры», «Навыки»), shown after login from the hero record `0xA6E874` |
| 33 | (453,125)–(478,150) | «Сохранить пароль» (frame 13 centred when set) |

Texts are drawn at a hotspot's top-left +5,+5; the password is shown unmasked; the focused field uses font `0x7AB36F`, others `0x7A5142`, with a blinking `_` cursor. Fields (`0x0042CB94`; the limit includes the cursor): clan 31 bytes, name 32, password 8, chat 79, address 63; a character is also refused when the text + 15 px would not fit the field. Typing converts CP1251 → CP866; Tab switches name ↔ password; Enter/Escape end the edit and save `LAN.CFG`; Enter in the address field enumerates sessions at that address; a click anywhere ends the edit first.

List: on entering, the DirectPlay connections; picking one enumerates sessions once — row 0 «Новый сервер» (`0x00470695`), then the session names. Picking a row needs a name (message 4): row 0 hosts (`[0xA83398]` = 1, `0x00430800(−1)`, session name from `SERVER` line 2), others join. The client then logs in (`0x47`: name 32 + password 8) and sends ten `0x46` (not traced). The login answer (`0x004205B0`) gives the player slot (`0xFF` = refused); errors (`0x0042062C`) show a message. «Ok»: no name → 4; no server → 6; a new hero → message 1 (Ok/Отмена); else load `GAME.RES` and wait (state 0) until the server starts the game (state 10 → 1). «Отмена» → menu, address cleared.

**Message box**: PLAY frame 12 at (144,193); lines centred on x = 400 from y = 233; buttons (hotspots `0x00478F90`): message 1 «Ok» (241,347) and «Отмена» (453,347), others one «Ok» (347,347)–(452,393). Texts (`0x0047E264`, 0x80-byte records, line 2 at `+0x40`): 1 «Предупреждение: Старый персонаж будет удален!», 2 «Ошибка: Неправильная версия клиентской части» / «Выполните обновление Вашей программы», 3 «Ошибка: Слишком много игроков», 4 «Ошибка: Не указано имя игрока», 5 «Ошибка: Сервер не доступен», 6 «Ошибка: Не указан сервер», 7 «Ошибка: Неправильный пароль», 8 «На сервере идет выход персонажа из игры» / «Повторите вход в игру через некоторое время».

## Accounts

The LAN server (`[0xA83398]` set) keeps `LANUSERS.DAT` in memory (clans `0xC447B0`, accounts `0xE257C0`) on handle `[0xF5EC54]` (open/create `0x0043167B`; a new file gets the three default clans and zeroed records). Account index = (record − `0xE257C0`) / `0x80`; it also selects the `SAVITEMS.DAT` record. Online slot (0–15) → record: `[0xF5DFC0 + 4·slot]`. Port: `lanstore` (`accounts`, `clans` tables).

| Off | Size | Field | Default |
|---|---|---|---|
| `+0x00` | 32 | name (CP866; compared with `strcmp`, case-sensitive) | — |
| `+0x20` | 8 | password, plain text | — |
| `+0x28`, `+0x2C`, `+0x30` | i32 ×3 | kills: player heroes, hirelings, monsters | 0 |
| `+0x34` | i32 | amulet deliveries | 0 |
| `+0x38` | i32 | experience (guess) | 0 |
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
| `+0x6A` | u8 | hero model (guess) | 2 |
| `+0x6B` | u8 | class (2–5) | 5 |
| `+0x6C` | 20 | unused | 0 |

Defaults: create path `0x00432E2B`. Saved (`0x004475F8`) on create, every score change (`0x0043B7EA`), logout/quit (`0x00443C1C`), the session's end (`0x0043459D`) and shutdown. The shipped record 0 («HERO»/«HERO», level 1, rating 1000, model 2, class 2, traded stats) fits the layout.

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

Client «Ok» (`0x004179B4`) sends `0x4C` and waits (state 0, variant 10). Server `0x0044ADB4`: a free slot, the hero from the account, **start map = hero model · 3 + 1** (`0x0044401C`: the model's village), **start cell and facing = the arrival of the first map exit leading to that map** (`gamestate.LANStart`; shipped: model 0 → map 1 (110, 32) facing 2, model 1 → map 4 (66, 57) facing 0, model 2 → map 7 (65, 8) facing 4), placed by `0x00435830` when the cell is free (the fallback search is not traced), items from `SAVITEMS.DAT`, then `0x5A` (the hero's appearance, name, clan) to everyone and `0x8C` to the client: slot, map, entity word (index, stance `0x8000`, side bits 12–13), (type << 4) | facing, row and col as single bytes. Client handler `0x0042105C`: its hero = entity `slot`, filled from its hero record (HP 1600), the map loaded from `MAPS.RES`, the hero placed; state 10 (`0x0042F620`) loads `INTO`, centres the camera on the hero and enters the game. **Team = hero model**, not the clan. `SERVER` lines 0–2 are the amulet parts' spawn spots (category 11, types 10–12; `0x004349FC`), lines 3–5 the delivery spots of models 0–2 (type 13, 25 points).

Other players reach the client as `0x5A`/`0x1A` (appearance: entity word, HP/16, model and kind, level, palette, equipment, jewellery, bag; players add name and clan, handler `0x00422C9C`) and `0x19` (9 bytes: entity word, (type << 4) | facing, HP/16, row and col bytes; applied on the current map, handler `0x0042294C`); items as `0x00`/`0x40` (`0x0042008C`).

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

* The server's message dispatcher: the password check, the ten `0x46` requests, the game-start message, team assignment (by clan or model?), the source of the ` [n]` count.
* «Новый герой»: the hero record `0xA6E874` layout, class defaults and stat costs; the settings screen (state 8); the LAN fonts `0x7A5142`/`0x7AB36F`.
* The message types and payloads (callers of `0x0042FDE4`: `0x417CD2`, `0x41FFD5`, `0x42E054`, `0x42E0D1`, `0x42E147`, `0x42E34C`, `0x42F09C`, `0x42F1EF`, `0x42F8CF`; receive dispatch from `0x42F8A1`).
* Server rules still open: the `0x53` broadcast, the `0x1B` move message, how a client entering gets the others' positions, the free-cell search at entry, respawn, row/col sent as single bytes although rows reach 319, the saves at `0x00448A6A` and `0x0044CCF3`, the account fields `+0x4C` and `+0x62`.
* Which `MAPS.RES` map is which cell of the a1–d5 grid; the six `SERVER` lines; `GAME.RES`; `Quest_r.res`; item differences from single-player.
