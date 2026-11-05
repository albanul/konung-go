# Original Menu Resources: `MENU640R.RES` and `MENU800R.RES`

This is a data catalog of the original Konung menu graphics. It records the Type 5 frames as stored in the game resources; it is separate from notes about the Ebitengine implementation.

## File structure

Both files are sequential Type 5 frame streams with no global file header. Each block starts with an 8-byte header: a 4-byte big-endian tag (`5`) followed by a 4-byte little-endian payload size. Offsets below point to the start of that block's header; payload sizes exclude the 8-byte header. The next block offset is `current offset + 8 + payload size`.

| Resource | File size | Frames | Background/settings dimensions |
|---|---:|---:|---:|
| `MENU640R.RES` | 1,312,566 bytes | 15 | `640 × 480` |
| `MENU800R.RES` | 2,047,604 bytes | 15 | `800 × 600` |

## Frame inventory

Button labels were read from the decoded images. Normal and selected states are paired consecutively. `Offset / payload` entries are shown as `hex / decimal`.

| Frame(s) | Visual content | `MENU640R.RES`: offset / payload / dimensions | `MENU800R.RES`: offset / payload / dimensions |
|---:|---|---|---|
| 0 | Scenic forest and rock background with decorative border | `0x000000 / 618,724 / 640×480` | `0x000000 / 966,004 / 800×600` |
| 1 | Settings screen artwork: resolution choices (640×480 / 800×600), transparency, shadows, day/night change, lighting effects, music, sound, OK and Cancel ([settings](konung_settings.md)) | `0x0970EC / 618,724 / 640×480` | `0x0EBD7C / 966,004 / 800×600` |
| 2 | Small cyan/blue orb or gem-like marker; exact role not established | `0x12E1D8 / 344 / 13×13` | `0x1D7AF8 / 344 / 13×13` |
| 3–4 | «Вернуться в игру» — Return to game; normal / selected | `0x12E338 / 7,203 / 192×31`; `0x12FF63 / 7,203 / 192×31` | `0x1D7C58 / 12,964 / 245×43`; `0x1DAF04 / 13,128 / 245×43` |
| 5–6 | «Сохранить игру» — Save game; normal / selected | `0x131B8E / 7,107 / 173×36`; `0x133759 / 7,101 / 173×36` | `0x1DE254 / 10,500 / 216×45`; `0x1E0B60 / 10,500 / 216×45` |
| 7–8 | «Загрузить игру» — Load game; normal / selected | `0x13531E / 7,739 / 178×42`; `0x137161 / 7,731 / 178×42` | `0x1E346C / 11,563 / 223×53`; `0x1E619F / 11,563 / 223×53` |
| 9–10 | «Новая игра» — New game; normal / selected | `0x138F9C / 5,453 / 128×41`; `0x13A4F1 / 5,457 / 128×41` | `0x1E8ED2 / 7,630 / 160×51`; `0x1EACA8 / 7,630 / 160×51` |
| 11–12 | «Установки» — Settings; normal / selected | `0x13BA4A / 5,715 / 118×39`; `0x13D0A5 / 5,715 / 118×39` | `0x1ECA7E / 8,696 / 148×49`; `0x1EEC7E / 8,696 / 148×49` |
| 13–14 | «Выход» — Exit; normal / selected | `0x13E700 / 4,115 / 72×44`; `0x13F71B / 4,115 / 72×44` | `0x1F0E7E / 6,131 / 90×55`; `0x1F2679 / 6,131 / 90×55` |

The phrase «Вернуться в игру» (“Return to game”) indicates that this resource set is for an in-game menu/pause flow, rather than a title screen with a “New game” first action. That is an interpretation from the visible text, not a claim based on tracing the executable's menu state code.

## Data observations

* Each file contains exactly 15 frames. Frames 0 and 1 are full-screen images; frame 2 is the same `13 × 13` image in both resolution variants; frames 3–14 are six pairs of button states.
* Button labels and order are identical between the two resolutions, while the frames are scaled and re-rendered for each screen size.
* Frame 1 embeds the settings labels and controls in a full-screen image. Frame 2 is the small blue marker that the settings screen centres in every box whose setting is on ([settings](konung_settings.md)).
* The plain sword mouse pointer is **not** frame 2 of either menu file. It is Type 5 sprite 4 in `GRAPH.RES`; see the [GRAPH.RES sprite catalog](konung_graph_sprites.md).
* The image descriptions are visual observations. Behavioral meanings beyond the visible Russian labels require executable call-site analysis.

## How these offsets were checked

Starting at byte offset zero, each block's big-endian tag and little-endian payload length were read, then the Type 5 image payload was decoded. Advancing by `8 + payload size` reproduces the next listed offset and ends exactly at each file's EOF. The dimensions in the table come from the first four payload bytes (`uint16` width and height, little-endian).
