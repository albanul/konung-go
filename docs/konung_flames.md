# Flames and transparency

Burning map objects and the «Прозрачность» setting. Status: **verified** against `konung.exe` code, `FLAMES.RES`, `OBJECTS.RES` and the exe tables, and ported. Port: `resfmt.FlameCycle`, `resfmt.TransparentFlame`, `resfmt.MapObject.Flames`, `mapdata.Sprite.Flames`, `game/flames.go` (`loadFlames`, `newFlames`, `drawFlames`). Tests: `resfmt.TestFlames`, `resfmt.TestTransparentFlame`.

## Which objects burn

The static object header dword `+0x08` is the object's **flame count** N (0–20). The 5-byte flame entries start at header `+0x1C`:

| Offset | Type | Meaning |
|---|---|---|
| `+0x1C + 5i` | int8 | flame type 0–6 |
| `+0x1D + 5i` | int16 | dx from the placement position |
| `+0x1F + 5i` | int16 | dy |

The same field decides the palette and the shadow ([day and night](konung_daylight.md)):

* N = 0: tinted, no flames.
* N = 1: tinted, with its flame.
* N ≥ 2: raw palette (the fire lights it) and no shadow.

`OBJECTS.RES` has 486 flame entries (types 0–4 mostly). Placed burning objects:

* `KONUNG.1`: 1 object, 1 flame.
* `KONUNG.17`: 3 objects, 23 flames.
* Most other maps: 1–2 objects.

Example: slot 48, a log house with flames (3, −169, 24), (4, −6, 91), (4, 132, 16), (4, −210, −24), (3, −82, −67).

## Flame types and frames

`FLAMES.RES` is 74 Type 5 RGB555 frames (loader `0x0043BA50`, pointers `0x0083B990`). The type table `0x0045AEC8` has 8 bytes per type: start and end frame (int32). The types are 0–10, 11–21, 22–32, 33–43, 44–54, 55–59 and 60–73 (`ExeTables.FlameCycle`).

* **Frame counters**: placement record bytes `+0x08 + i` (int8, one per flame; files hold zeros and `0xFF` there). The map loader (`0x0043CCC2`) sets each to start + rand() % (end − start + 1), or to start when that span is below 2. The counter can start at end, which shows the next type's first frame once.
* **Draw** (`0x0042708D`, after the object in the sprite pass, both kind-2 handlers): for each flame, draw frame k with its top-left at the placement's world position + (dx, dy). Then k + 1, and if k ≥ end, k = start. Flames advance once per drawn frame and only for objects the pass draws. They are never day/night tinted.

## Transparency `0x00A3E700`

* **Off**: the frame is copied (`0x0043E696`, every RLE pixel opaque).
* **On**: the loader converts every pixel at load (`0x00427D9C`, RGB555 branch). With L = (2·R8 + G8 + B8) / 32 = (2r + g + b) / 4 (8-bit channels c << 3, so 0–31), each channel becomes c·L/32. «Ok» on the settings screen reloads `FLAMES.RES` when the setting changed (`0x00438C2D`).
* **Blend** `0x0043D8EF`, per pixel: f = table `0x00465E2C`[pixel] / 2. Then screen' = pixel + `0x0047FEF0`[screen RG][f] + `0x0047902C`[screen B][f], 16-bit addition.
  * The tables (RGB555 build `0x0043B0CC`) hold screen·f/32 per channel. Red and green are masked with `0x1E` (low bit dropped); blue is not masked.
  * `0x00465E2C` is indexed by the converted pixel and stores 0x40 − 2L of the original colour. Colours that convert to the same value overwrite each other in index order, so the last one wins.
  * So the blend is premultiplied source-over with alpha 1 − f/32. Channel overflow carries into the next channel.

## Port

* `loadFlames` uploads the frames twice: as decoded (transparency off) and converted by `resfmt.TransparentFlame` (premultiplied RGBA, alpha (32 − f)/32). Ebitengine's default blend is premultiplied source-over, which is the game's formula. The game's dropped low bit of the screen's red/green and the carries between channels are not reproduced.
* `newFlames` seeds the counters with the scene's RNG. `drawFlames` advances them once per game frame, every 60 ms: the game draws a frame every `0x00A3E6F4` = 60 ms ([menu](konung_menu.md)), and the port derives the frame from its update count (updates · 5/18 at 60 updates/s). It advances every object's flames, not only visible ones.
* The menu sets `startScene.transparency` from the setting each frame; both image sets stay loaded, so no reload is needed.

## Open questions

* None specific to flames.
