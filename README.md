# Konung 1.5

A remake of the 1999 game «Князь: Легенды лесной страны» (Konung: Legends of the North). It runs on Windows, macOS and Linux, and in a web browser, and adds a network game: you can play together on a home or office network, as in the original's network version («Князь 1.5»).

The game's own files (pictures, maps, sounds) are **not** included. You need your own copy of the original game.

**Contents**

* [The easy way: download a ready bundle](#the-easy-way-download-a-ready-bundle)
* [What do you want to do?](#what-do-you-want-to-do)
* [Step 1: the game files](#step-1-the-game-files)
* [Step 2: the tools](#step-2-the-tools)
* [Play alone on your computer](#play-alone-on-your-computer)
* [Host a network game](#host-a-network-game)
* [Join a network game](#join-a-network-game)
* [Films](#films)
* [Controls](#controls)
* [When something does not work](#when-something-does-not-work)
* [For developers](#for-developers)
* [Legal](#legal)

## The easy way: download a ready bundle

Ready-to-run downloads for every system are on the project's [Releases page](https://github.com/albanul/konung-go/releases): **«Latest build (main)»** is always the newest, numbered versions (`v1.0`, …) are the stable ones. Nothing to install, no terminal.

1. Download the file for your computer:

   | Your computer | File |
   |---|---|
   | Windows | `konung-…-windows.zip` |
   | Mac with Apple silicon (M1, M2, M3, …) | `konung-…-macos-apple-silicon.zip` |
   | Mac with an Intel processor | `konung-…-macos-intel.zip` |
   | Linux PC | `konung-…-linux.tar.gz` |
   | Linux on ARM (Raspberry Pi 4/5, …) | `konung-…-linux-arm64.tar.gz` |

2. Unpack it anywhere.
3. Copy **everything** from the original game's folder (the one with `konung.exe`, `GRAPH.RES`, `KONUNG.1`, …) into the bundle's `assets/Konung` folder.
4. **Play alone:** double-click `konung` (`konung.exe` on Windows).
5. **Play together on your network:** on one computer double-click `konung-server` (`konung-server.exe`). A window shows what it does; keep it open while you play. Then everyone opens `http://<that computer's address>:7777/` in a browser (`http://localhost:7777/` on that computer itself), or joins from the game's menu «Сетевая игра». See [Join a network game](#join-a-network-game).

* **macOS**, the first time: right-click the program → **Open** → **Open** (it is not from the App Store). If macOS says it is «damaged», run `xattr -dr com.apple.quarantine <the bundle folder>` in Terminal.
* **The address** of the server's computer: Windows: `ipconfig` in a Command Prompt («IPv4 Address»); macOS: System Settings → Network; Linux: `hostname -I`.
* **Firewall:** allow `konung-server` when the system asks, so the others can reach port 7777.
* **Server settings:** copy `konung-server.conf.example` to `konung-server.conf` next to the server, edit it, start the server again. Accounts and progress are kept in the folder `server-data` next to it.
* The films need a one-time conversion ([Films](#films)); without it the game skips them.

The rest of this guide builds everything yourself from this project, with containers, and runs the server in the background.

## What do you want to do?

| I want to… | You need | Go to |
|---|---|---|
| play alone on my computer | the game files, Podman or Docker (once, to build the game) | [Step 1](#step-1-the-game-files), [Step 2](#step-2-the-tools), [Play alone](#play-alone-on-your-computer) |
| host a game for friends on our network (they play in the browser or the desktop game) | the game files, Podman or Docker, a computer that stays on while you play | [Step 1](#step-1-the-game-files), [Step 2](#step-2-the-tools), [Host](#host-a-network-game) |
| join a game someone else hosts | a web browser, nothing else | [Join](#join-a-network-game) |

Only the person who hosts, or who builds the desktop game, needs the steps below. Players who join in the browser need nothing.

## Step 1: the game files

1. Install the original game (for example the GOG version), or find its folder on your disk. It contains files such as `konung.exe`, `GRAPH.RES`, `KONUNG.1` and a `Video` folder.
2. Get this project: on its GitHub page click **Code → Download ZIP** and unpack it, or, if you use git, `git clone https://github.com/albanul/konung-go.git`.
3. In the project folder, create the folder `assets/Konung` and copy **everything** from the game's folder into it:

   ```
   konung-go/                  ← the project folder
     assets/
       Konung/                 ← the game's files go here
         konung.exe
         GRAPH.RES  OBJECTS.RES  MENU800R.RES  …
         KONUNG.1  KONUNG.2  …  GAME.0  GAME.1  GAME.2
         LAN_KONG.EXE  MAPS.RES  GAME.RES  …   (the network version's files)
         Video/
     scripts/
     README.md                 ← this file
   ```

4. Check that everything is in place. Open a terminal in the project folder (see [Step 2](#step-2-the-tools)) and run:

   ```bash
   scripts/check-files.sh
   ```

   It lists what is there and what is missing, and what each missing file costs. For example, without `LAN_KONG.EXE` and its companions the menu item «Сетевая игра» stays hidden. «All set.» means you can go on.

## Step 2: the tools

Everything is built and run in **containers**, so you do not have to install a programming language. You need one of these programs:

| System | Install | Then |
|---|---|---|
| Linux (Fedora) | `sudo dnf install podman` | ready |
| Linux (Ubuntu, Debian) | `sudo apt install podman` | ready |
| macOS | [Podman Desktop](https://podman-desktop.io) or [Docker Desktop](https://www.docker.com/products/docker-desktop/) | start it once, so its engine runs |
| Windows | [Docker Desktop](https://www.docker.com/products/docker-desktop/) with WSL 2 (it offers to set it up) | run the commands in a **WSL** terminal (Ubuntu from the Start menu), in the project folder |

**How to open a terminal in the project folder:**

* **Linux:** right-click the project folder → «Open in Terminal».
* **macOS:** open Terminal, type `cd ` (with a space), drag the project folder onto the window, press Enter.
* **Windows:** in the WSL (Ubuntu) terminal type `cd /mnt/c/Users/<you>/Downloads/konung-go` (the path where you unpacked it; drive `C:` is `/mnt/c`).

Every command in this guide is typed in that terminal, followed by Enter. The first run of each command downloads what it needs (a few hundred megabytes) and takes a few minutes; later runs are quick.

## Play alone on your computer

1. Build the game for your system:

   ```bash
   scripts/build-game.sh
   ```

   It builds for the system you are on. You can also build for another one: `scripts/build-game.sh windows`, `mac`, `linux`, or `all`.

2. Start it:

   | System | The program | How |
   |---|---|---|
   | Windows | `output/game/windows/konung.exe` | double-click |
   | macOS (M1/M2/M3…) | `output/game/mac/konung-apple-silicon` | the first time: right-click → **Open** → **Open** (macOS warns about programs from outside the App Store); later: double-click |
   | macOS (Intel) | `output/game/mac/konung-intel` | as above |
   | Linux | `output/game/linux/konung` | double-click, or `output/game/linux/konung` in the terminal |

The game opens fullscreen at 800×600. It finds the game files in `assets/Konung` next to the program or in a folder above it. Your saves and settings (`KONUNG.SAV`, `KONUNG.SA0`…`SA9`, `KONUNG.CFG`) are kept in `assets/Konung`.

**Playing on another computer:** copy the program and the game files together, in this layout:

```
any-folder/
  konung.exe                  (or konung-apple-silicon, konung-intel, konung)
  assets/
    Konung/                   (the whole game folder)
```

**Playing in the browser without a server:** the network host below gives everyone the browser version too. If you only want the browser version for yourself, host a server (it needs no friends) and open the address it shows.

## Host a network game

One computer runs the **server**: the shared world everyone plays in. It also gives the **browser version** of the game, so players need nothing but a browser. Desktop players can join the same server.

### Start the server

```bash
scripts/konung-server.sh start
```

The first start takes a while (it builds the server and the browser version). When it is done it prints something like:

```
Started. Players connect to this machine's address, port 7777.
Browser game: on this machine http://localhost:7777/ ; for others on the network:
    http://192.168.1.20:7777/
```

* **You** open `http://localhost:7777/` in your browser.
* **Others on your network** open the other address (`http://192.168.1.20:7777/` in this example; yours will differ).
* The server keeps running in the background, also after you close the terminal.
* **After a restart of the computer:** Docker starts the server again by itself. With Podman, run `scripts/konung-server.sh start` again, or once `systemctl --user enable podman-restart.service` (Linux) so it starts by itself too.
* **Windows (WSL):** the address printed under «for others» is WSL's internal one and does not work from other computers. Use your Windows computer's address instead: run `ipconfig` in a Windows Command Prompt and take the «IPv4 Address» (for example `http://192.168.1.20:7777/`).

### Let the others in: the firewall

Other computers reach the server on **port 7777** (TCP, and UDP for the desktop game's server search). If they cannot reach it, open the port:

* **Fedora:** `sudo firewall-cmd --permanent --add-port=7777/tcp --add-port=7777/udp && sudo firewall-cmd --reload`
* **Ubuntu:** `sudo ufw allow 7777`
* **macOS, Windows:** allow it when the system asks; otherwise in the firewall settings, for Docker / Podman.

Playing over the internet (not just your network) needs port 7777 forwarded on your router to this computer. Look up "port forwarding" for your router.

### Everyday commands

| Command | What it does |
|---|---|
| `scripts/konung-server.sh status` | is the server running? |
| `scripts/konung-server.sh logs` | watch what happens (who joins, kills, sessions); Ctrl+C to leave |
| `scripts/konung-server.sh stop` | stop the server |
| `scripts/konung-server.sh start` | start it again |
| `scripts/konung-server.sh restart` | stop and start (after changing the settings) |
| `scripts/konung-server.sh update` | after downloading a new version of this project: rebuild everything and restart. Accounts and progress are kept |
| `scripts/konung-server.sh reset` | delete the server **and all accounts** (asks first) |

### Server settings

1. Copy `konung-server.conf.example` to `konung-server.conf` (in the project folder).
2. Edit it with any text editor: the server's name, the number of players, the session length, the world speed, whether teammates can hurt each other, how many amulets win a session. Every setting is explained in the file.
3. Run `scripts/konung-server.sh restart`.

**Where everything is kept:** accounts, heroes and the rating live in a container volume called `konung-data`. It survives `stop`, `restart` and `update`; only `reset` deletes it.

## Join a network game

**In the browser** (nothing to install):

1. Open the address the host gives you, for example `http://192.168.1.20:7777/`.
2. Click the page to start (browsers allow sound only after a click).
3. In the menu choose **«Сетевая игра»**, then **«Новая игра»**.
4. Type a name and a password. The first time, this creates your account on that server; remember the password.
5. The server appears in the list. Choose it, pick your hero by clicking one of the three portraits (Волк, Драгомир or Михаил: the hero decides your team) and press **Ok**.

**In the desktop game:** the same, from «Сетевая игра». The desktop game finds servers on your network by itself. A server elsewhere: type its address (for example `192.168.1.20`) in the address field and press Enter.

**Your own new hero:** «Новый герой» in the network menu creates a hero with your choice of class, skills, looks and clan.

## Films

The game's films (intro, endings) are in an old Windows video format. Convert them once, and the game plays them (also in the browser):

```bash
scripts/convert-films.sh
```

It takes a while (about 19 films). Without converted films the game simply skips them. After converting, run `scripts/konung-server.sh update` if you host, so the browser version gets them too.

## Controls

| Input | Action |
|---|---|
| Left click | walk |
| Right click | pick up, talk, attack (in the combat stance) |
| Space; 1 / 2 / 3 | combat stance; blade / axe / bow |
| A | attack the nearest enemy |
| I, Q, M | character window, quest log (in a network game also the players and the teams' score), map |
| ~ | the bag strip |
| F | «Все ко мне»: the party follows you again |
| Ctrl+S | the camera follows the hero |
| Numpad + / − | game speed (alone only; in a network game the server sets it) |
| F1 | key help |
| Escape | menu |

In a **network game** also:

| Input | Action |
|---|---|
| Enter | chat to everyone; Enter again sends, Escape cancels |
| Shift+Enter | chat to your team only |
| Right click on a villager (out of the combat stance) | talk: trade, or hire them |
| Ctrl + right click on your hireling (or their portrait) | talk to your hireling: let them go |

The goal of the network game: three parts of the dragon's amulet lie in the world. Join them into the amulet and bring it to your team's spot. Kills and amulets earn rating points; «Рейтинг» in the network menu shows the best players and clans.

## When something does not work

| Problem | What to do |
|---|---|
| A script says «Need podman or docker» | install one of them ([Step 2](#step-2-the-tools)); on macOS/Windows start Podman Desktop / Docker Desktop first |
| The game does not start, or `check-files.sh` reports MISSING | copy the whole game folder into `assets/Konung` ([Step 1](#step-1-the-game-files)) |
| No «Сетевая игра» in the menu | the network version's files are missing: `scripts/check-files.sh` names them |
| The browser does not show the server in the list | open `http://<server address>:7777/info`: it should show a short text with the server's name. If it does not load, the server is not reachable (firewall, wrong address). If it loads but shows something else, the server is an old version: `scripts/konung-server.sh update` |
| The desktop game does not find a server on another computer | type the server's address in the address field and press Enter; the automatic search may not pass every network. Hosts on Linux can also try `KONUNG_NETWORK=host scripts/konung-server.sh restart` |
| The page `http://…:7777/` does not load on other computers | the firewall ([Let the others in](#let-the-others-in-the-firewall)), or a wrong address: use the one `start` printed (on Windows, the Windows computer's address from `ipconfig`) |
| After an update the browser still shows the old game | reload the page with Ctrl+Shift+R (Cmd+Shift+R on macOS) |
| macOS: «cannot be opened because the developer cannot be verified» | right-click the program → Open → Open (only the first time) |
| Linux: «error while loading shared libraries» | install the graphics and sound libraries: `sudo dnf install mesa-libGL alsa-lib` (Fedora) or `sudo apt install libgl1 libasound2` (Ubuntu) |
| Films do not play | convert them ([Films](#films)) |
| The page is on `https://…` and finds no server | browsers block unencrypted game connections from secure pages: use `http://` on your network |

The server's log (`scripts/konung-server.sh logs`) usually tells what went wrong.

## For developers

### Requirements

* Go 1.26.4 or newer (see `go.mod`). Nothing else: every system's build is pure Go (`CGO_ENABLED=0`) and cross-compiles from any system; on Linux, Ebitengine and its sound library load the system's X11/OpenGL/ALSA libraries when the game starts.

### Run from source

```bash
go run ./cmd/konung                              # desktop game (run from the repository root)
go run ./cmd/konung-server -data /tmp/konung-data -web output/web
                                                 # server; -web serves the browser game on its port
go run ./cmd/web-build -serve :8080              # browser build into output/web, served on :8080
go run ./cmd/video-convert assets/Konung/Video   # convert the films (needs ffmpeg with WMV3, libsvtav1, libvorbis)
```

The server reads `GAME.RES`, `MAPS.RES`, `GRAPH.RES`, `OBJECTS.RES`, `konung.exe`, `GAME.0`–`GAME.2` and `QUESTR.RES` from its data folder, else from the game folder (`-game`; by default `assets/Konung` in the working folder, next to the program or above). In a bundle (a `web` folder next to it) its data folder defaults to `server-data` next to it, and it serves the bundle's page with the game files straight from the game folder (`internal/webgame`), never the host's `LAN.CFG`, `HISCORES.DAT`, settings or saves. Its settings: `konung-server.conf.example`. The browser build without Go on the host: build `output/web` with Go, then `./konung-web -build=false -serve :8080` (`go build -o konung-web ./cmd/web-build`); details in [docs/konung_web.md](docs/konung_web.md).

### The scripts

| Script | What it does |
|---|---|
| `scripts/check-files.sh [folder]` | checks the game files |
| `scripts/build-game.sh [windows\|mac\|linux\|all]` | builds the desktop game in a Go container → `output/game/` |
| `scripts/konung-server.sh <command>` | the server in a container (start, stop, update, logs, web, …; environment variables in the script's header) |
| `scripts/convert-films.sh [-force]` | converts the films in a container with ffmpeg |
| `scripts/bundle.sh` | builds the release bundles for every system into `dist/` (needs Go; the GitHub workflow `.github/workflows/release.yml` runs it on every push: a push to `main` replaces the pre-release «latest», a tag `v*` makes a release) |

All of them use Podman or Docker (`KONUNG_ENGINE=docker` to choose).

### Tools

| Command | What it does |
|---|---|
| `go run ./cmd/map-render assets/Konung/KONUNG.1 output/maps/konung1.png` | render a map to PNG (`-all output/site/maps` for every map) |
| `go run ./cmd/sprite-catalog` | gallery of all sprites in `output/site/sprites/` |
| `go run ./cmd/res-parser <file.RES> <dir>` | extract Type 5 frames to PNG |
| `go run ./cmd/res-gallery` | gallery of the network version's images in `output/site/lan/` |

### Tests

```bash
go build ./... && go vet ./... && go test ./...
```

Tests that need the game files skip when `assets/Konung/` is missing.

### Documentation

[AGENTS.md](AGENTS.md) is the index: the project layout, the reverse-engineering workflow and a task map into [docs/](docs/), one document per file format or subsystem. The network game: [docs/konung_lan.md](docs/konung_lan.md); the browser build: [docs/konung_web.md](docs/konung_web.md). What is not ported: [docs/konung_not_ported.md](docs/konung_not_ported.md).

## Legal

This project contains no code or data from the original game. «Князь» and its assets belong to their rights holders. Use the port only with a copy you own, and check the licensing before putting the game files on a public website (the browser version and the server send them to every player).
