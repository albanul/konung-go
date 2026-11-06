# Konung Reverse Engineering Project Guide

## Project

This repository reimplements parts of the 1999 game «Князь: Легенды лесной страны» (Konung: Legends of the North) in Go/Ebitengine and provides tools for analyzing its Windows x86 executable (`assets/Konung/konung.exe`) and resource files (`assets/Konung/*.RES`, `KONUNG.N`, `LIGHTS.N`).

Use the existing findings as a starting point. Before reverse engineering anything, check the task map below and the referenced document; afterwards, record verified findings there so the work is not repeated.

## Working loop (fast path)

1. **Orient (≤ 5 min).** Find your row in the task map. Read only that doc plus the open questions. Don't re-derive anything a doc marks as verified.
2. **Clarify scope** when a request could mean "faithful port" or "visual preview". Those differ by hours of RE work, so ask.
3. **Trace, don't guess.** Find the loader (file-read buffer) → consumer (code that reads the buffer) → draw/parse routine. Record VAs as you go.
4. **Prototype engine-free.** Decode in `internal/resfmt` or `internal/mapdata`, render to PNG with a small `cmd/` tool or a temporary test, and look at the image. Ebitengine can't run in the container (no display).
5. **Integrate** into the Ebitengine package as a thin consumer.
6. **Done means:**
   * `go build ./...`, `go vet` and `go test ./...` pass in the container.
   * A new decoder has an asset-backed test that skips when assets are missing.
   * Docs are updated: the topic doc, plus the task map, resource overview, code map, open questions, and the [exe lookup table](docs/konung_exe_resource_loaders.md#known-routines-and-buffers) for new VAs.
   * Superseded claims are removed.
   * Temporary files are deleted.
   * Nothing is committed unless the user asks.

## Task map: where to start

| If you are working on… | Read first | Code |
|---|---|---|
| A resource file you haven't seen before | [Resource file overview](#resource-file-overview) below, then [RES formats](docs/konung_res_format.md) | `internal/resfmt/` |
| Map terrain, decorations, static objects, shadows, camera/projection | [Map format](docs/konung_map_format.md) | `internal/mapdata/`, `cmd/map-render/` |
| The New Game scene (map terrain/objects + GAME.N characters) | [Menu & scene implementation](docs/konung_menu.md), [Start location](docs/konung_start_location.md) | `internal/game/scene.go` |
| Character creation «Выбор героя» (`NEWH*R.RES`): hero, class, skills, stat trade, game start with the chosen hero | [New game](docs/konung_new_game.md) | `gamestate/newhero.go`, `game/newhero.go`, `sceneAssets.newGame` |
| Main menu, buttons, cursor | [Menu implementation](docs/konung_menu.md), [Menu RES catalog](docs/konung_menu_res.md) | `internal/game/menu.go`, `cursor.go` |
| In-game HUD: portrait grid, HP bars, panel buttons and click hotspots (`INTR*R.RES`), NPC names / hover status line, game fonts | [Interface resources and HUD](docs/konung_interface_res.md) | `resfmt.ParseInterface`, `resfmt/exetext.go`, `resfmt/hotspot.go`, `gamestate/portrait.go`, `gamestate/names.go`, `internal/game/hud.go`, `text.go` |
| `GRAPH.RES` (palettes, cursors, terrain tiles, Type 0 character models) | [GRAPH.RES layout and sprites](docs/konung_graph_sprites.md) | `mapdata.LoadTerrainTiles`, `resfmt.DecodeType0Frames`, `game/cursor.go` |
| `OBJECTS.RES` (buildings, animated objects) | [RES formats: OBJECTS](docs/konung_res_format.md#objectsres), [Start location: OBJECTS path](docs/konung_start_location.md#objectsres-image-path-and-object-lookup), [Sprite index](docs/konung_sprite_index.md) | `resfmt.DecodeStaticObject` |
| Characters, NPCs, monsters, player start (`GAME.N`) | [GAME.N format](docs/konung_game_format.md) | `internal/gamestate/`, `resfmt.LoadEntitySprite` |
| Combat: melee, bows and arrows, creature AI (alarm, territory/leash, return home), village anger and surrender, loot, experience and levels, poison and respawn, hero death | [Combat](docs/konung_combat.md) | `gamestate/combat.go`, `game/combat.go` |
| Saving and loading: `KONUNG.SAV` / `KONUNG.SA0`–`SA9`, the save/load screens (`SAVE*R.RES`), restore-mode map load | [Saved games](docs/konung_save_format.md) | `gamestate/save.go`, `gamestate/encode.go`, `game/saves.go` |
| Settings screen («Установки»), `KONUNG.CFG`, what each option does | [Settings](docs/konung_settings.md) | `gamestate/config.go`, `game/settings.go` |
| Day and night: light tint by time of day, tinted palettes, `LIGHTS.N`, the lit terrain path at night («Световые эффекты») | [Day and night](docs/konung_daylight.md) | `gamestate/daylight.go`, `resfmt/daylight.go`, `mapdata/lights.go`, `game/daylight.go` |
| Flames on burning objects, «Прозрачность» (transparency), `FLAMES.RES` | [Flames](docs/konung_flames.md) | `resfmt/flames.go`, `game/flames.go` |
| Sound effects, music, «Музыка» / «Звук», what plays when | [Sound](docs/konung_sound.md) | `gamestate/sound.go`, `resfmt/audio.go`, `game/audio.go` |
| Talking to NPCs, dialogues, quests and the quest log (Q), `QUESTR.RES`, dialogue script functions (hire, dismiss, heal, loans, buildings…) | [Dialogues and quests](docs/konung_dialogue.md) | `resfmt/quest.go`, `gamestate/dialogue.go`, `gamestate/scriptfuncs.go`, `gamestate/scriptvillage.go`, `game/dialogue.go` |
| Trading (dialogue «купить…») and the pile window when a pick-up is over the weight or bag limit: window 7, prices, coin balance, shop stock | [Trade](docs/konung_trade.md) | `gamestate/trade.go`, `game/trade.go`, `resfmt/trade.go` |
| Village buildings (construction stages, ruins, burning, the building sprite by state, setting them on fire with incendiary arrows, the bridge gate on map exits) and the voevoda's training in the settlement pass | [Buildings](docs/konung_buildings.md) | `gamestate/buildings.go`, `game/buildings.go` |
| World map window (M / HUD button 2): known locations, travel by «Шар магического прыжка» |  [World map](docs/konung_world_map.md) | `resfmt/worldmap.go`, `gamestate/worldmap.go`, `game/worldmap.go` |
| Moving between maps: teleports (tiles 285/286), map exits, map change | [GAME.N: map exits](docs/konung_game_format.md#map-exits-verified) | `gamestate/exits.go`, `startScene.changeMap` in `game/scene.go` |
| Items on the ground (piles), herb respawn, pick-up | [Ground items](docs/konung_ground_items.md) | `gamestate/piles.go`, `game/piles.go` |
| Character / inventory window (I), bag strip, moving/equipping items, derived stats (weight, armour, attack), level and stat raises, auto-level | [Character window](docs/konung_character_window.md) | `game/inventory.go`, `gamestate/charsheet.go`, `gamestate/stats.go`, `gamestate/inventory.go`, `gamestate/levels.go`, `resfmt/charwindow.go`, `resfmt/items.go` |
| Herbs and potions: mixing in the in-hand box, drinking, the healing skill (знахарство) | [Potions](docs/konung_potions.md) | `gamestate/potions.go`, `resfmt.ExeTables.Recipes`, `gamestate.Game.ClickBox`/`ClickPortrait` |
| Inventory, items, weapons, combat stance (Space, 1/2/3) | [GAME.N: inventory](docs/konung_game_format.md#inventory-and-items-verified), [weapons and stance](docs/konung_game_format.md#weapons-and-combat-stance-verified), [equipment overlays](docs/konung_game_format.md#equipment-overlays-verified) | `gamestate/stance.go`, `Game.Overlays`, `resfmt/items.go`, `resfmt.ExeTables.CombatDrawOrder` |
| Finding a sprite by appearance or ID | [Sprite index](docs/konung_sprite_index.md) + `docs/data/*.tsv` | `cmd/sprite-catalog/` |
| What is still missing: features not ported, approximations, format unknowns | [Not ported](docs/konung_not_ported.md) | — |
| Tracing code in `konung.exe` | [Executable loader notes](docs/konung_exe_resource_loaders.md), [RE workflow](#reverse-engineering-workflow) below | — |

## Resource file overview

One line per file family. Details live in the linked documents; "verified" means confirmed against both executable code and file bytes.

| File | Layout summary | Status | Doc |
|---|---|---|---|
| `FLAMES.RES`, `MENU*R.RES`, `NEWH*R.RES`, `SAVE*R.RES` | Sequential Type 5 frames (BE tag 5, LE size), RGB555 row-RLE; `FLAMES.RES` = 74 flame frames (7 types); `SAVE*R.RES` = load and save screen backgrounds; `NEWH*R.RES` = the 13 frames of the hero-creation screen | verified | [res format](docs/konung_res_format.md), [flames](docs/konung_flames.md), [menu RES](docs/konung_menu_res.md), [saves](docs/konung_save_format.md#save-and-load-screen), [new game](docs/konung_new_game.md#resources-newh800rres--newh640rres) |
| `GRAPH.RES` | 256 palettes → 11 Type 5 cursors → Type 1 terrain tile block (512-slot table) → 64 KiB gap → 27 Type 0 blocks (3 bodies + 24 equipment overlays) | verified except gap and Type 0 palette/dye | [GRAPH layout](docs/konung_graph_sprites.md) |
| `OBJECTS.RES` | 512-slot table; slots 0–29 animated (Type 2), others static | verified | [start location](docs/konung_start_location.md), [exe loaders](docs/konung_exe_resource_loaders.md) |
| `KONUNG.N` | 4 sections: terrain (160×80×2 tile bytes), 320×160 flag grid, flat decorations (16-byte records), static object placements (36-byte records) | verified except the flag grid | [map format](docs/konung_map_format.md) |
| `GAME.N` | Start state: 6 sections incl. 8192 items, 378 ground piles, map exits, 200 entity groups (group 0 = player) and 2000 × 256-byte entities (each with 6 equipment slots + 32-item bag) | groups (incl. alarm/respawn fields)/entities/inventory/piles/sprites/settlements (posts, flags)/map exits verified; item fields beyond the type open; spawn placement approximate; all six sections identified | [GAME.N](docs/konung_game_format.md) |
| `LIGHTS.N` | 64000 bytes: 160×80 terrain part (1 + `LIGHTS.RES` light mask for the lit terrain path) + 320×160 fine-grid lit cells (characters there are not day/night tinted) | verified; fine-grid values unused | [day and night](docs/konung_daylight.md#lightsn-lit-cells), [map format](docs/konung_map_format.md) |
| `LIGHTS.RES` | 64×64 incendiary arrow glow (`0x1000` bytes), 64×43 glow map (loot highlight), 19 × `0x1C80` light masks (114×64, lit terrain) | verified | [potions](docs/konung_potions.md#loot-highlight-0x00a3e758), [day and night](docs/konung_daylight.md#lit-terrain-path-0x004257a9) |
| `INTR640R.RES`, `Intr800r.res` | Type 1: 512 × (image offset, palette offset) table + indexed-RLE images (HUD frame, buttons, item icons, portraits) | verified | [interface](docs/konung_interface_res.md) |
| `KONUNG.SAV`, `KONUNG.SA0`–`SA9` | Saves: 10 × 50-byte slot names; per slot a raw dump of globals (settings, time, hero, quest flags, arrows, `GAME.N` sections 1/2/3/5/6, active piles); map exits come from `GAME.<hero model>` | verified against code; no original save files to compare | [saves](docs/konung_save_format.md) |
| `KONUNG.CFG` | 8 LE dwords: video mode, transparency, day/night, lighting, music, background tiling, shadows, sound | verified (the installed file has only 7) | [settings](docs/konung_settings.md) |
| `QUESTR.RES` | 7 blocks read by `0x0043BDD5`: 8000 dialogue nodes, 300 dialogue starts, string pool, condition and action lists, word-list texts, 300 quest flags (`0x00625068`) | verified | [dialogues](docs/konung_dialogue.md) |
| `SOUNDS.RES` | Type 1 header + 1000 `(offset,size)` entries, mono 16-bit PCM (22050 Hz, voices pitch-varied) | verified | [res format](docs/konung_res_format.md#soundsres--sounds_res), [sound](docs/konung_sound.md) |
| `MUSICS.RES` | Type 1 header + 9 `(offset,size)` entries, Ogg Vorbis tracks | verified | [sound](docs/konung_sound.md#files) |

## Code map

| Package / command | Responsibility |
|---|---|
| `internal/resfmt` | Engine-independent decoders: Type 5 RGB555 RLE, indexed RLE, RGB555 palettes, `INTR*R.RES` interface images (`ParseInterface`), exe strings/name tables and fonts (`exetext.go`), static OBJECTS entries and map objects with shadows (`LoadMapObject`), entity frames (`LoadEntitySprite`), equipment overlays (`ComposeEquipment`, exe tables via `ExeTables`: item types/names, peaceful and combat draw orders), HUD hotspot map (`HUDHotspot800`), item tables, ground images, enchantments and the herb recipes (`items.go`), the `LIGHTS.RES` glow maps (loot and incendiary arrows, `lights.go`), flames and the transparent flame blend (`flames.go`), `MUSICS.RES` and sound rendering (`audio.go`), day/night exe tables and tint tables (`daylight.go`, `TintTable`), the lit terrain path (`litterrain.go`), character window layout (`CharWindow800`), `QUESTR.RES` dialogues, the dialogue window layout and the building type table (`quest.go`), the trade window layout (`trade.go`), the world map window's region runs and composition (`worldmap.go`), item prices (`ItemPrice`), palette tint (`TintPalette`), `GRAPH.RES` Type 0 frames, sound entries |
| `internal/mapdata` | Engine-independent `KONUNG.N` parsing (terrain, decorations, object placements, walk grid), `LIGHTS.N` (`lights.go`), fine-grid geometry and A* pathfinding (`finegrid.go`), `GRAPH.RES` terrain tiles, projection, depth order, full-scene render |
| `internal/gamestate` | Engine-independent `GAME.N` parsing: groups, entities, settlements/posts, map exits (`exits.go`), known maps and travel by orb (`worldmap.go`), world positions, spawn placement, entity sprites, NPC target rules (`village.go`, `wander.go`), combat stance and weapon selection (`stance.go`), combat rules (`combat.go`, `bow.go`, `creature.go`, `anger.go`, `loot.go`, `upkeep.go`, `leave.go`), character sheet, derived stats, item moves and level/stat raises (`charsheet.go`, `stats.go`, `inventory.go`, `levels.go`), herb mixing, potions and the torch (`potions.go`), character creation and the game start (`newhero.go`), dialogues, quest flags and the dialogue script functions (`dialogue.go`, `scriptfuncs.go`, `scriptvillage.go`), trade and the pile window: lists, prices, coin balance, apply (`trade.go`), shop stock production, merchant restock, tribute and the settlement pass (`production.go`), village buildings and the voevoda's training (`buildings.go`), the day/night tint (`daylight.go`), sound selection and the rule sound queue (`sound.go`), ground items (`piles.go`), party HUD rules (`portrait.go`), names and status line (`names.go`), encoding back to `GAME.N` layout (`encode.go`), saved games (`save.go`) and `KONUNG.CFG` (`config.go`) |
| `internal/game` | Ebitengine menu, New Game scene (camera, drawing), HUD (`hud.go`), character window (`inventory.go`), save/load screens (`saves.go`), settings screen (`settings.go`), character creation (`newhero.go`), the dialogue window and quest log (`dialogue.go`), the trade and pile window (`trade.go`), the world map window (`worldmap.go`), building sprites by state (`buildings.go`), the day/night tint shader (`daylight.go`), flames (`flames.go`), audio (`audio.go`) and the combat controller (`combat.go`, `ranged.go`, `upkeep.go`); converts decoded `image.RGBA` to `ebiten.Image` |
| `cmd/konung` | Game entry point |
| `cmd/map-render` | `KONUNG.N` → PNG (terrain, decorations, shadows, objects, optional `GAME.N` characters and building states via `-game assets/Konung/GAME.0`, `-terrain` for terrain only, `-all` for every map + `index.html`) |
| `cmd/sprite-catalog` | Extract all indexed images into an HTML gallery under `output/site/sprites/` |
| `internal/sitenav` | Shared nav bar and landing page (`output/site/index.html`) that join the tools' HTML outputs into one site |
| `cmd/res-parser` | Extract Type 5 frames from a RES file to PNG |

Each tool is its own binary under `cmd/`, but tools must not carry private copies of format logic. Shared block access lives in `internal/resfmt/blocks.go` (`ObjectBlock`, `Type0Blocks`, `FrameRanges`, `Palettes`). Package names carry no `konung` prefix (`resfmt`, `mapdata`, `gamestate`, `game`).

Put new format decoders in an engine-independent package (`internal/resfmt` or a sibling such as `internal/mapdata`) with a test against the installed assets, and keep Ebitengine code a thin consumer. This keeps tools and tests runnable without a display.

## Environment and commands

The project is developed in a Fedora Podman devcontainer. The workspace is mounted at `/workspaces/konung1.5`. Go, Python 3 (without Pillow), and GNU `objdump` are available only inside the container; the host has no `objdump`.

```bash
podman ps                                   # find <container>
C="podman exec -w /workspaces/konung1.5 <container>"
$C go build ./... && $C go test ./...      # asset-backed tests skip if assets are missing
$C go run ./cmd/map-render assets/Konung/KONUNG.1 output/maps/konung1.png
$C go run ./cmd/map-render -all output/site/maps   # every KONUNG.N + index.html
$C go run ./cmd/sprite-catalog              # -> output/site/sprites; open output/site/index.html for both
$C go run cmd/res-parser/main.go <resfmt.RES> <output_directory>
$C go run ./cmd/konung                      # needs a display; run on the host if the container has none
```

Files that the container writes under `output/` belong to the container user and may not be deletable from the host. Remove them with `$C rm`. Viewing a rendered PNG is the fastest way to validate a graphics hypothesis.

## Reverse-engineering workflow

```bash
$C sh -c 'objdump -D -Mintel -j BEGTEXT assets/Konung/konung.exe > /tmp/kn/text.asm'   # mkdir -p /tmp/kn first
$C grep -n '^  43c0fc:' /tmp/kn/text.asm          # jump to a known VA (lowercase, two-space indent)
$C grep -n '0x627fd8' /tmp/kn/text.asm            # find every reference to a global buffer
$C sed -n '53935,54260p' /tmp/kn/text.asm | awk -F'\t' '{print $1, $3}'   # address + mnemonic only
```

* Code is in `BEGTEXT` (VA `0x410000`), data in `DGROUP` (VA `0x450000`). See [the exe notes](docs/konung_exe_resource_loaders.md) for PE offsets.
* The file-read helper is `call 0x44023e(handle, buffer, size)`. Following its buffer argument from a loader to the code that reads that buffer is the fastest way to find a format's consumer. Known global buffers and routines are listed in [the exe notes](docs/konung_exe_resource_loaders.md#known-routines-and-buffers).
* Functions start with `push ebx/esi/edi/ebp; mov ebp,esp`; locals are `[ebp-N]`. Compiler identity not verified.

## Reverse-engineering rules

* The game is a 32-bit Windows x86 application. Treat multi-byte resource values as little-endian unless the field is specifically documented otherwise. Some block tags are stored big-endian; follow the per-format specification.
* Do not guess an unknown format. Inspect references in `konung.exe`, trace the read/parse call sites, and validate the interpretation against the resource bytes.
* When recording a finding, distinguish observed facts from hypotheses. Include file name, offset, size, indexing convention (0- vs 1-based), and a reproducible validation method. Remove or mark superseded statements; don't leave a stale claim next to the new one.
* Map names come from the exe's location table ([map format](docs/konung_map_format.md#location-names-verified)): `KONUNG.1` = «Родники», Volk's initial location; `KONUNG.4` = «Старый лагерь» (Old Camp). «Новая игра» opens the [character-creation screen](docs/konung_new_game.md) (hero, class, skills, stat trade); «Ok» loads `GAME.<model>` and the start map `KONUNG.<model*3+1>` with the chosen hero. In the scene everything is data-derived: map content from `KONUNG.N`, characters from `GAME.N`. The hero is controllable (click to move with A* on the fine grid, C toggles camera follow, F «Все ко мне» (no-op with the hero alone), Space / HUD stance button toggles the combat stance, right click on a character in the stance attacks it and A / HUD button 4 attacks the nearest enemy (melee or the bow, [combat](docs/konung_combat.md)), F1 hides the key help, 1/2/3 / HUD weapon button select blade/axe/bow, I / HUD bag button toggles the character window (T toggles a solid tiled background behind it), where clicks move items between bag, equipment and jewellery and the raise buttons spend free experience on the level and stats («АВТО» toggles auto-level); right click on a character with a dialogue (out of the stance; Ctrl for a party member) opens the [dialogue](docs/konung_dialogue.md) at once without walking there, Q / HUD button 5 toggles the quest log, M / HUD button 2 the [world map](docs/konung_world_map.md) (known locations; with a «Шар магического прыжка» in the bag a click on one travels there, and using the orb opens the map; «Потерянная карта» reveals a random location); trade replies open the [trade window](docs/konung_trade.md), and a pick-up over the weight or bag limit opens the pile window to swap bag and ground items; otherwise right click walks and picks up from the ground (nuts/roots grow back), left click walks; ~ shows the bag strip, right click on a bag item uses/equips it (a herb is drunk), the shovel enters dig mode; in the window a herb dropped on the in-hand box's herb mixes a potion (trains знахарство), on a weapon/arrows applies poison/oil, and a herb dropped on the portrait is drunk; Чистая слеза or the torch (right click / portrait) makes the piles glow until the next map change; numpad +/− set the game speed; ending a walk on a teleport (map exit) loads the target map, unless the exit is a village's that needs its bridge built ([buildings](docs/konung_buildings.md#the-bridge-gate)); with incendiary arrows («Масло» of concentration ≥ 10 put on the boxed arrows) the bow stance's right click shoots the hovered building, which may catch fire, and incendiary arrows glow in flight; the selection is the hero alone); Villagers run the traced village AI (`0x00416934`): posted characters (elder, merchant, healer… from the settlement records) hold their post; others take random steps inside the village rectangle. Creatures run the traced combat AI (`0x004123BC`): calm ones stand still; a party member inside a group's territory alarms it, members chase and fight (bandits also shoot) within the leash rectangle and walk home afterwards, and respawn on map entry; villagers turn on a hero who hurts them and may surrender; kills drop loot; poison drains HP; the idle hero strikes back; the hero's death returns to the menu. The world is tinted by the time of day ([day and night](docs/konung_daylight.md)); burning objects show flames ([flames](docs/konung_flames.md)); sound effects and music play ([sound](docs/konung_sound.md)). Approximations: spawn-group member positions, animation and walking speed. `GAME.N` N = hero model; start map = model*3+1 (Volk 0 → `KONUNG.1`, Dragomir 1 → `KONUNG.4`, Michail 2 → `KONUNG.7`).
* Konung supports both 15-bit RGB 555 and 16-bit RGB 565 paths. Verify masks and conversion behavior before assigning a pixel format. All decoded assets so far use RGB555 with magenta `0x7C1F` as the transparent key.
* The cursor is `GRAPH.RES` Type 5 sprite 4 (plain blade); sprite 1 includes a crossed-swords marker.

## Documentation conventions

* Keep this file focused on project-wide instructions and the indexes above. Detailed specs, offsets, inventories, and investigation notes belong in `docs/`.
* One topic per document. When a new resource or subsystem is understood, add a doc, a row to the task map and resource overview, and a line in [open questions](#open-questions).
* Write docs in English. Lead with the spec (tables, offsets, decoder names) and keep investigation history to a short evidence line or a pitfall note.
* Each doc's code references must name the current function or package; update them when code moves.

## Open questions

Highest-value unknowns, roughly in priority order. The full list of missing features and approximations, with VAs, is in [not ported](docs/konung_not_ported.md).

1. The game's own movement code (speed, path rules); party play (multi-selection, the party AI `0x0041F11C`); combat leftovers (god mode, kind `0x49` retreat; see [combat](docs/konung_combat.md#open-questions)); misc item use other than the torch and shovel ([not ported](docs/konung_not_ported.md#other-misc-items)); spawn positions of non-creature groups without coordinates. See [GAME.N](docs/konung_game_format.md#open-questions).
2. Type 0 model palette/dye selection for NPCs (the heroes' palettes are [known](docs/konung_new_game.md#the-hero-template)) and the model ID → character mapping. See [exe notes](docs/konung_exe_resource_loaders.md).
3. `KONUNG.N` section 1 (320×160 flag grid): walls and arrow obstacles (bits `0xC0`) are known; the other bits are not.
4. The fine-grid `LIGHTS.N` values. See [day and night](docs/konung_daylight.md#open-questions).
5. The 64 KiB gap in `GRAPH.RES` at `0x1ED8B6`.
6. Dialogues: `SPEECHS.RES`; building record `+4` ([buildings](docs/konung_buildings.md#not-ported--open)). See [dialogues](docs/konung_dialogue.md#open-questions). Saves: the hero copy `0x00A3E584` after a load ([saves](docs/konung_save_format.md#open-questions)).
7. HUD: the alternate portrait table `0x0062F3D8`. See [interface](docs/konung_interface_res.md#open-questions).
8. Ground items: see [ground items](docs/konung_ground_items.md#open-questions).
9. Sound: dialogue greetings and `SPEECHS.RES` (not installed) are not ported ([sound](docs/konung_sound.md#not-ported)). All settings are honoured. See [settings](docs/konung_settings.md#open-questions).
10. Potions: revival by Живая Вода (`0x00414B3C`), misc items other than the torch, the rest of `LIGHTS.RES`. See [potions](docs/konung_potions.md#open-questions).
11. `cmd/sprite-catalog` has its own Type 0 walker that duplicates `resfmt.DecodeType0Frames`; consolidate when touching it.
