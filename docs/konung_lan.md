# LAN version (network play)

The single-player `konung.exe` has no network code (imports only GDI32, KERNEL32, USER32, WINMM, DSOUND, DDRAW; dynamic loads only `MP.DLL` and `VORBISFILE.DLL`). Network play is a separate program, `LAN_KONG.EXE` (PE timestamp 23.03.2003, `BEGTEXT` 0x54200 bytes against 0x37E00 in `konung.exe`). Fan accounts call the online variant "сетевой Князь": up to 16 players, 3 nations, 180-minute sessions, amulet carrying and PvP ([fan page](https://gospitaliers.narod.ru/knyaz_server.htm), unverified).

**Port decision:** our own network layer (not wire-compatible with `LAN_KONG.EXE`). The game rules come from the LAN exe; the transport is ours.

## Layout

The LAN files live in the game folder (`assets/Konung`) next to the single-player files, as in the original install: the GOG folder already held the LAN version's `GAME.RES`, `HISCORES.DAT`, `SERVER.LOG` and `Servers.cfg`, which `konung.exe` never opens. Names do not clash; `GAME.RES`, `LIGHTS.RES` and `SERVER.LOG` were byte-identical, and `HISCORES.DAT` is the LAN one (the GOG one was all zeros). The port is one binary: hosting and joining happen from the game UI, with no command-line modes.

## Port status

* Main menu «Сетевая игра» (port addition, `internal/game/lan.go`): opens the LAN menu (below). Shown only without a running game and only when `LAN_KONG.EXE`, `ME800R.RES` and `PLAY800R.RES` are in the game folder. The label is the large exe font (`FontLargeVA`) scaled 2×, tinted like the painted buttons (normal ≈ `MENU800R.RES` frame 9 lettering, selected ≈ frame 10), at (262, 196), where «Сохранить игру» is drawn in a game.
* Images: `go run ./cmd/res-gallery` → `output/site/lan`. Maps: `go run ./cmd/map-render -mapsres assets/Konung/MAPS.RES -all output/site/lanmaps`.
* LAN screens (`internal/game/lanui.go`, data from the exe via `gamestate.ParseLANExe`): the LAN menu, the rating screen (frame only), the login screen with typing, Tab, «Сохранить пароль», the portraits, the server list (LAN search on entering; Enter in the address field searches that address too), hosting («Новый сервер»: server + discovery in the game process, `netplay.Host`, named from `SERVER`), joining, the message box, «Ok» → wait, and `LAN.CFG` read and written in the original layout (`gamestate.LANConfig`). Verified by running two game copies under Xvfb: host, discovery, join, the duplicate-name refusal, the live ` [n]` count, wait and cancel.
* Network layer `internal/netplay` (adapted from the author's go-tetris `pkg/network`, MIT): newline-delimited JSON messages over TCP (`Conn`: ordered, keep-alive pings, a stalled peer is disconnected instead of messages being dropped, graceful close with a TCP half-close), a session server (`Listen`: join handshake with protocol version, name, password, clan and hero model; up to 16 players; chat; `MsgGame` handed to `OnGame`; membership messages in one order for everyone), a client (`Dial`) and LAN discovery (`ServeDiscovery`/`Discover`: UDP broadcast on the game port, replacing DirectPlay's session list). Port 7777 for both, as the original's TCP/IP port.
* Port choices: one connection type (the port's TCP/IP), so the list skips the original's DirectPlay connection stage; the list shows session names (with the host's address only when names repeat); the scroll arrows are clickable (the original's 800×600 rectangles were copied from the NEW screen); a duplicate name is refused with a port message («Ошибка: Игрок с таким именем уже в игре»), since accounts are not ported; Escape in the LAN menu returns to the port's main menu; text uses the single-player fonts (`FontSlotVA`, focused `FontLargeVA`) — the LAN exe's fonts `0x7A5142`/`0x7AB36F` are not mapped.
* Not ported: the game start from the wait (the server's start message is not traced), «Новый герой» (state 4), «Установки» (state 8), the rating rows, the login screen's stat values, accounts and passwords (`LANUSERS.DAT`; the password is sent but ignored), in-game chat, gameplay.

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
| `LANUSERS.DAT` | Server accounts, `0x0043167B`: `0x2000` header (clan names, 32 bytes each) + `0x138800` records | sizes verified; record layout open |
| `CLANS.CFG` | «Византийцы», «Русские», «Викинги» | verified |
| `HISCORES.DAT` | `0x4D8` bytes (`0x004313EC`): team names and scores | size verified |
| `SAVITEMS.DAT` | Created by the server (`0x00431784`); players' items between sessions | open |
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
* Server rules: session timer and reset, teams and starts, amulet scoring, respawn, rating, `SAVITEMS.DAT`.
* Which `MAPS.RES` map is which cell of the a1–d5 grid; the six `SERVER` lines; the `LANUSERS.DAT` record; `GAME.RES`; `Quest_r.res`; item differences from single-player.
