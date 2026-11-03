# Konung Reverse Engineering Project Guide

## Project

This repository reimplements parts of the 1999 game «Князь: Легенды лесной страны» (Konung: Legends of the North) and provides tools for analyzing its Windows x86 executable (`assets/Konung/konung.exe`) and resource files (`assets/Konung/*.RES`).

Use the existing findings as a starting point. Before reverse engineering a resource format, check the reference documents below and update them with verified findings so the same work does not need to be repeated.

## Legacy game data references

* [Resource formats, Type 5 frame/RLE details, FLAMES frame catalog, and parser usage](docs/konung_res_format.md)
* [GRAPH.RES sprite and cursor catalog](docs/konung_graph_sprites.md)
* [Original 640/800 menu RES frame catalog](docs/konung_menu_res.md)
* [Dragomir's starting location, candidate map, and person sprite findings](docs/konung_start_location.md)
* [Searchable sprite, object, terrain, and map-reference index](docs/konung_sprite_index.md)

Keep this file focused on project-wide working instructions. Put detailed format specifications, offsets, inventories, and investigation notes in the relevant reference document.

## Implementation notes

* [Ebitengine menu implementation](docs/konung_menu.md)

## Environment and commands

The project is developed in a Fedora Podman devcontainer. The workspace is mounted at `/workspaces/konung1.5`; Go and Python are available inside the container.

```bash
podman ps
podman exec -w /workspaces/konung1.5 <container_id> go run cmd/res-parser/main.go <resource.RES> <output_directory>
podman exec -w /workspaces/konung1.5 <container_id> go test ./...
```

## Reverse-engineering rules

* The game is a 32-bit Windows x86 application. Treat multi-byte resource values as little-endian unless the field is specifically documented otherwise. Some block tags are stored big-endian; follow the per-format specification.
* Do not guess an unknown format. Inspect references in `konung.exe`, trace the read/parse call sites, and validate the interpretation against the resource bytes.
* When recording a finding, distinguish observed facts from hypotheses. Include file name, offset, size, indexing convention, and a reproducible validation method when available. For the cursor and related sprites, use the [GRAPH.RES sprite catalog](docs/konung_graph_sprites.md); the plain blade cursor is sprite 4, while sprite 1 includes a crossed-swords marker.
* Keep the `KONUNG.N` filename-to-location mapping provisional unless verified. The New Game scene currently uses `KONUNG.1` as a candidate Old Camp map and object entry 15 with GRAPH sprite 10 as a temporary sword-bearing figure; see [the start-location data notes](docs/konung_start_location.md).
* Konung supports both 15-bit RGB 555 and 16-bit RGB 565 paths. Verify masks and conversion behavior before assigning a pixel format.
