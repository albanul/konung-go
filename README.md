# Konung 1.5

A reimplementation of the 1999 game «Князь: Легенды лесной страны» (Konung: Legends of the North) in Go with [Ebitengine](https://ebitengine.org). It runs on the desktop and in a browser (WebAssembly), and comes with tools that decode the game's resource files.

The game's files are **not** part of this repository. You need your own copy of the original game.

## Game files

Copy the installed game into `assets/Konung/` (the directory is git-ignored):

```
assets/Konung/
  konung.exe          # read for tables, fonts and texts
  GRAPH.RES  OBJECTS.RES  MENU800R.RES  …
  KONUNG.1 …  LIGHTS.1 …  GAME.0 …
  Video/*.flm
```

### Films (optional, once)

The original films are WMV3, which Go cannot decode. Convert them once to AV1 + Vorbis next to the originals:

```bash
go run ./cmd/video-convert assets/Konung/Video
```

This needs an `ffmpeg` with the WMV3 decoder, `libsvtav1` and `libvorbis`: on Fedora RPM Fusion's `ffmpeg` (`ffmpeg-free` lacks WMV3), on macOS `brew install ffmpeg`. Without converted films the game simply skips them.

## Requirements

* Go 1.26.4 or newer (see `go.mod`).
* Desktop on Linux: a C compiler and the X11/OpenGL/ALSA development headers Ebitengine needs, e.g. on Fedora:

  ```bash
  sudo dnf install gcc mesa-libGL-devel libXrandr-devel libXcursor-devel \
      libXinerama-devel libXi-devel libXxf86vm-devel alsa-lib-devel
  ```

  macOS and Windows need nothing extra. See [Ebitengine's install guide](https://ebitengine.org/en/documents/install.html) for other systems.

## Desktop

```bash
go run ./cmd/konung              # run from the repository root
go build -o konung ./cmd/konung  # or build a binary
./konung
```

The game starts fullscreen at 800×600 and finds `assets/Konung/` from the current directory or any parent. Settings and saves (`KONUNG.CFG`, `KONUNG.SAV`, `KONUNG.SA0`–`SA9`) are written there too.

## Browser

```bash
go run ./cmd/web-build -serve :8080   # build into output/web and serve it
```

Open <http://localhost:8080> and click to start (browsers block sound until the page gets a click).

* `go run ./cmd/web-build` only builds `output/web`, a static site you can host on any HTTP server that serves `.wasm` as `application/wasm`. Compress it (gzip/brotli) for real hosting: the game downloads about 125 MB.
* With `ffmpeg` installed, the films are remuxed to WebM (no re-encoding) and played by the browser.
* Saves and settings are stored in the browser's IndexedDB, per site.

### Serving from a machine without Go

Build the server binary once where Go is installed (for the target machine's OS/architecture), together with `output/web`:

```bash
go run ./cmd/web-build                    # output/web
go build -o konung-web ./cmd/web-build    # the server
```

Then, on the machine without Go, run it from the repository root:

```bash
./konung-web                              # serves output/web on :8080
./konung-web -serve 9000                   # another port
```

Details: [docs/konung_web.md](docs/konung_web.md).

## Controls (short)

| Input | Action |
|---|---|
| Left click | Walk |
| Right click | Pick up / talk / attack (in the combat stance) |
| Space, 1 / 2 / 3 | Combat stance; blade / axe / bow |
| A | Attack the nearest enemy (melee or the bow) |
| I, Q, M | Character window, quest log, world map |
| ~ | Bag strip |
| F | «Все ко мне» (the party follows) |
| Ctrl+S | Camera follows the hero |
| Numpad + / − | Game speed |
| F1 | Key help |
| F3 | FPS / frame time overlay |

The full list is in [AGENTS.md](AGENTS.md#reverse-engineering-rules).

## Tools

| Command | What it does |
|---|---|
| `go run ./cmd/map-render assets/Konung/KONUNG.1 output/maps/konung1.png` | Render a map to PNG (`-all output/site/maps` for every map) |
| `go run ./cmd/sprite-catalog` | Gallery of all sprites in `output/site/sprites/` |
| `go run ./cmd/res-parser <file.RES> <dir>` | Extract Type 5 frames to PNG |
| `go run ./cmd/video-convert <Video dir>` | Convert the films (see above) |
| `go run ./cmd/web-build` | Browser build (see above) |

## Tests

```bash
go build ./... && go vet ./... && go test ./...
```

Tests that need the game files skip when `assets/Konung/` is missing.

## Documentation

[AGENTS.md](AGENTS.md) is the index: project layout, the reverse-engineering workflow and a task map pointing into [docs/](docs/), one document per file format or subsystem. What is not ported yet is listed in [docs/konung_not_ported.md](docs/konung_not_ported.md).

## Legal

This project contains no code or data from the original game. «Князь» and its assets belong to their rights holders; use the port only with a copy you own, and check the licensing before hosting the game files publicly (for example the browser build).
