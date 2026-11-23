# Browser (WebAssembly) build

The game runs in a browser as `GOOS=js GOARCH=wasm`. The game code is the same as the desktop build; only file access differs.

## Build and run

```bash
$C go run ./cmd/web-build                  # -> output/web
$C go run ./cmd/web-build -serve :8080     # build, then serve http://localhost:8080
./web-build -build=false -serve 8080       # host without Go: serve an earlier build (binary built with GOOS/GOARCH of the host)
./web-build                                # the same: without Go on PATH it serves output/web on :8080 (the page files come from the binary itself)
```

`output/web` is a static site. You can host it on any HTTP server that sends `.wasm` as `application/wasm`.

| File | Content |
|---|---|
| `index.html`, `konungfs.js` | Page and file shim, embedded from `cmd/web-build/web/` |
| `wasm_exec.js` | Go's JS runtime glue, copied from `$GOROOT/lib/wasm` |
| `konung.wasm` | `./cmd/konung` (~25 MB, `-ldflags=-s -w`) |
| `assets/Konung/…` | Game files the port reads, hard-linked or copied from `-assets` (default `assets/Konung`; a file not readable by others is copied and made readable, so a server container reading as another user can serve it). The allowlist is the `gameFile` pattern in `cmd/web-build/main.go`: `konung.exe`, the 800×600 RES files, `GRAPH`/`OBJECTS`/`LIGHTS`/`FLAMES`/`SOUNDS`/`MUSICS`/`QUESTR.RES`, `KONUNG.N`, `LIGHTS.N`, `GAME.N`, `KONUNG.CFG`/`SAV`/`SA0–9`. About 125 MB. |
| `assets/Konung/Video/*.webm` | The films: `<name>.ivf` (AV1) + `<name>.ogg` (Vorbis) remuxed with `ffmpeg -c copy`, so nothing is re-encoded (`remuxFilms`). Without ffmpeg the `.ivf`/`.ogg` are served instead. |
| `manifest.txt` | The served game files, one path per line |

## File access (`internal/gamefs`)

All game-file I/O in `internal/game` goes through `gamefs` (`ReadFile`, `WriteFile`, `Exists`, `ReadDir`, `ReadFileFold`).

* **Native** (`os.go`): the OS file system.
* **Browser** (`js.go`):
  * On first use it fetches `manifest.txt` and loads the stored files from IndexedDB (`konungFS.load`).
  * Reads return a stored file first. Otherwise a file listed in the manifest is fetched relative to the page (`konungFS.fetch`). Anything else is `fs.ErrNotExist` and costs no request.
  * Writes (`KONUNG.CFG`, `KONUNG.SAV`, `KONUNG.SA<i>`) go to IndexedDB (database `konung`, store `files`, key = path). They therefore persist per browser and origin.
  * Without IndexedDB (some private modes), writes last only for the session.
* Fetches block the calling goroutine on a promise. This is safe because Ebitengine runs `Update` in a goroutine, not in a JS callback. On a map change the game waits for `KONUNG.N` and `LIGHTS.N`; a film waits for its `.ivf`/`.ogg`.
* `FindGameResource` returns the page-relative path when the manifest lists it.

## Network game

The LAN client works in the browser; the dedicated server (`cmd/konung-server`, [LAN version](konung_lan.md)) serves browsers and desktop games on the same port.

* **Transport.** A browser cannot open TCP or UDP, so `netplay` connects over **WebSocket** (`github.com/coder/websocket`, which uses the browser's `WebSocket` in WASM). The JSON lines are the same, one per text message (`websocket.NetConn`).
  * Client: `dialServer` in `internal/netplay/dial_js.go` dials `ws://host:port/`, or `wss://` when the page came over HTTPS (browsers refuse `ws://` there). The desktop build keeps TCP (`dial.go`; a `ws://` address works there too, for tests).
  * Server (`internal/netplay/websocket.go`): the game port's listener reads each new connection's first bytes (`sniff`). An HTTP request (`GET`, `HEAD`, `OPTIONS`) goes to a small web server; anything else is the JSON-line protocol as before. A connection silent for the join timeout is the JSON protocol too, so its own timeout answers it. The web server answers **`GET /info`** with the `SessionInfo` (with `Access-Control-Allow-Origin: *`) and upgrades any other request to the WebSocket session. Any origin may connect (a LAN game has no origin to trust).
* **The server serves the page.** `konung-server -web output/web` (default: `<data>/web` when it exists) serves the browser build on the game port (`netplay.Server.ServeFiles`: every request that is neither `/info` nor a WebSocket goes to the files, with `Cache-Control: no-cache` so a rebuilt `konung.wasm` is fetched), so `http://server:7777/` is the game and the page finds its server by itself. `scripts/konung-server.sh` builds `output/web` in a Go container (`web`; `start` builds it on the first run, `update` rebuilds it; ffmpeg is installed there for the films) and mounts it read-only into the server's container (`-web /web`). Test: `netplay` `TestServeFiles` (the page, `.wasm` as `application/wasm`, `/info` and a WebSocket player on one port).
* **Server list.** UDP discovery is impossible in the browser. `Discover` there asks the page's host and any typed address for `/info` (`DiscoverHTTP`; in the browser through `fetch` directly, `getInfo` in `dial_js.go`). With the page served by `web-build -serve` on the server's machine, the server shows up without typing anything; another server is reached by typing its address in the address field.
* **Files.** The LAN screens read and write through `gamefs` like the rest. `web-build` serves `LAN_KONG.EXE`, `ME800R`/`PLAY800R`/`NEW800R`/`MAPS`/`GAME`/`INTO640R.RES` and `CLANS.CFG`. It never serves `LAN.CFG` (the player's login, with a saved password) or `HISCORES.DAT` (other players' names); the browser writes its own into IndexedDB (`TestGameFileLAN`).
* **Limits.**
  * A page served over HTTPS needs the server behind TLS (`wss://`), for example a reverse proxy.
  * Protocol version 19 as the desktop game: browser and desktop players meet on one server.
* **Verified** under Node, which has the browser's `WebSocket` and `fetch`. `netplay`'s `TestBrowserTransport` (`browser_js_test.go`, run with `GOOS=js` against a running server, `KONUNG_TEST_SERVER`) discovers the server, joins and gets its chat line back. The native `TestWebSocketAndInfo` checks `/info`, a WebSocket player with a TCP player on one port, and `DiscoverHTTP`; `TestWebSocketServerClose` checks that `Close` ends WebSocket players. The game itself has not been run in a browser from the container.

## Behaviour differences

* There is no fullscreen at start (the browser requires a user gesture). The canvas fills the page and Ebitengine scales the 800×600 screen.
* The page waits for a click or key press before starting the game ("Click to start"; the download continues meanwhile): browsers block sound until the page gets one, and the start-up film plays before any game input.
* Music is played by the browser.
  * `newWebMusic` (`internal/game/audio_js.go`, `konungMusic` in `konungfs.js`) puts the `MUSICS.RES` track's Ogg into an `<audio>` element.
  * The game's rules stay the same (`soundSystem.playTrack`/`update`/`setMusic`): a random track, the next one when it ends, and «Музыка» off only setting the volume.
  * Decoding Vorbis and resampling 22050 → 44100 Hz in Go ran without a break on the only WASM thread, because the music never stops.
  * Sound effects are still mixed in Go.
* Start-up downloads about 105 MB (`OBJECTS`, `SOUNDS`, `GRAPH`, `MUSICS`). `-serve` does no compression; a production host should gzip/brotli the files.
* The original `.flm` films are not served. Convert them first with `cmd/video-convert`.
* Films are played by the browser, not decoded in Go.
  * The pure-Go AV1 decoder (`gav1d`) uses SIMD natively, but in WASM it is scalar and single-threaded, which is far too slow.
  * `openWebFilm` (`internal/game/film_js.go`) shows `Video/<name>.webm` in a `<video>` element over `resfmt.FilmRect`, placed like Ebitengine's centred, scaled screen (`konungFilm` in `konungfs.js`). The frozen screen is drawn around it.
  * Any key still ends the film (`filmPlayer.update`).
  * Should the browser still block sound (no gesture yet), the film plays muted rather than not at all.
  * Without a `.webm` the Go decoder is used.

## Verification

* `GOOS=js GOARCH=wasm go vet ./cmd/konung ./internal/game ./internal/gamefs`.
* `gamefs` was run under Node with a stub `konungFS`: reads, case-insensitive lookup, a missing file, write and read back, directory listing.
* The full game was not run in a browser from the container, because there is no browser there.

## Measuring performance

* **F3** (on every screen, a port addition, `internal/game/perf.go`) shows:
  * FPS and TPS.
  * The CPU time of `Update` and `Draw`, averaged over the last second.
  * The scene's draw calls in the last frame (`drawAt` plus the tinted `drawTinted`).
* Compare the browser numbers with the native build.
* The CPU time of Draw does not include Ebitengine's flush to WebGL, which runs after `Draw`. A low FPS with a small Draw time therefore points at the GPU or WebGL side.
* `konung.wasm` keeps its name section even with `-ldflags=-s -w`, so a browser profile shows Go function names. Use the Firefox profiler (`about:profiling`) or Chrome DevTools → Performance.
* In the browser the menu showed 33 FPS / 53 TPS with Update 0.00 ms and Draw 0.88 ms, so the time went outside the game's code. That led to moving the music out of Go (TPS 50 → 60). The Zen browser (a Firefox fork) on Linux stayed below the display rate (about 46 FPS even on testufo), while Firefox itself ran acceptably on the same machine, Chromium reached about 60 FPS and Zen on a Mac 120: a slow Zen on Linux is that browser's problem, not the game's.
* One native game tick (`startScene.step` on `KONUNG.1`) took about 0.1 ms (2026-11, measured with a temporary benchmark). The world simulation is not the browser bottleneck.
