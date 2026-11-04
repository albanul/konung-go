// Command map-render writes KONUNG.N maps (terrain, decorations, shadows and
// static objects) to PNG files.
//
//	go run ./cmd/map-render assets/Konung/KONUNG.1 output/konung1.png
//	go run ./cmd/map-render -all output/site/maps   # every KONUNG.N + index.html
package main

import (
	"flag"
	"fmt"
	"html/template"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"konung1.5/internal/konunggame"
	"konung1.5/internal/konungmap"
	"konung1.5/internal/sitenav"
)

func main() {
	all := flag.String("all", "", "render every KONUNG.N in -dir into this output directory")
	dir := flag.String("dir", "assets/Konung", "directory containing KONUNG.N files (with -all)")
	graphPath := flag.String("graph", "assets/Konung/GRAPH.RES", "path to GRAPH.RES")
	objectsPath := flag.String("objects", "assets/Konung/OBJECTS.RES", "path to OBJECTS.RES")
	terrainOnly := flag.Bool("terrain", false, "render the terrain layer only")
	gamePath := flag.String("game", "assets/Konung/GAME.0", "GAME.N start state for characters and creatures (empty = none)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: map-render [-graph GRAPH.RES] <KONUNG.N> <out.png>\n       map-render [-graph GRAPH.RES] [-dir assets/Konung] -all <outdir>")
		flag.PrintDefaults()
	}
	flag.Parse()

	graph, err := os.ReadFile(*graphPath)
	if err != nil {
		fail(err)
	}
	tiles, err := konungmap.LoadTerrainTiles(graph)
	if err != nil {
		fail(err)
	}
	r := &renderer{graph: graph, tiles: tiles, terrainOnly: *terrainOnly}
	if !*terrainOnly {
		if r.objects, err = os.ReadFile(*objectsPath); err != nil {
			fail(err)
		}
		if *gamePath != "" {
			data, err := os.ReadFile(*gamePath)
			if err != nil {
				fail(err)
			}
			if r.game, err = konunggame.Parse(data); err != nil {
				fail(err)
			}
			// The game opens GAME.<hero model>, so the player shows model N.
			if n, err := strconv.Atoi(strings.TrimPrefix(filepath.Ext(*gamePath), ".")); err == nil {
				r.game.HeroModelFromGame(n, konunggame.PlaceholderHeroPalette)
			}
		}
	}
	switch {
	case *all != "" && flag.NArg() == 0:
		err = r.renderAll(*dir, *all)
	case *all == "" && flag.NArg() == 2:
		_, err = r.renderOne(flag.Arg(0), flag.Arg(1))
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

type renderer struct {
	graph, objects []byte
	tiles          []*image.RGBA
	terrainOnly    bool
	game           *konunggame.Game
}

type mapInfo struct {
	Name, File             string
	Width, Height          int
	LastRow, LastCol       int
	Decorations, Objects   int
	ObjectErrors           int
	Entities, EntityErrors int
}

func (r *renderer) renderOne(mapPath, outPath string) (mapInfo, error) {
	data, err := os.ReadFile(mapPath)
	if err != nil {
		return mapInfo{}, err
	}
	m, err := konungmap.Parse(data)
	if err != nil {
		return mapInfo{}, fmt.Errorf("%s: %w", mapPath, err)
	}
	info := mapInfo{Name: filepath.Base(mapPath), File: filepath.Base(outPath), LastRow: m.LastRow, LastCol: m.LastCol}
	var img *image.RGBA
	if r.terrainOnly {
		img = konungmap.Render(m, r.tiles)
	} else {
		decorations, err := konungmap.ParseDecorations(data)
		if err != nil {
			return mapInfo{}, err
		}
		placements, err := konungmap.ParsePlacements(data)
		if err != nil {
			return mapInfo{}, err
		}
		objs, errs := konungmap.LoadPlacedObjects(placements, r.objects, r.graph)
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "%s: %v\n", mapPath, e)
		}
		sorted := konungmap.DrawOrder(placements, objs)
		sprites := konungmap.ObjectSprites(sorted)
		info.Decorations, info.Objects, info.ObjectErrors = len(decorations), len(sorted), len(errs)
		if r.game != nil {
			n, err := strconv.Atoi(strings.TrimPrefix(filepath.Ext(mapPath), "."))
			if err != nil {
				return mapInfo{}, fmt.Errorf("%s: cannot derive map number from the extension", mapPath)
			}
			grid, err := konungmap.ParseWalkGrid(data)
			if err != nil {
				return mapInfo{}, err
			}
			for _, p := range r.game.OnMap(n, grid.Blocked) {
				es, x, y, sx, sy, depth, err := p.LoadSprite(r.objects, r.graph)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s: %v\n", mapPath, err)
					info.EntityErrors++
					continue
				}
				sprites = append(sprites, konungmap.Sprite{Image: es.Image, X: x, Y: y, Shadow: es.Shadow, SX: sx, SY: sy, Depth: depth})
				info.Entities++
			}
			konungmap.SortSprites(sprites)
		}
		img = konungmap.RenderScene(m, r.tiles, decorations, sprites)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return mapInfo{}, err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return mapInfo{}, err
	}
	info.Width, info.Height = img.Bounds().Dx(), img.Bounds().Dy()
	fmt.Printf("%s: %dx%d px, %d decorations, %d objects, %d entities -> %s\n", mapPath, info.Width, info.Height, info.Decorations, info.Objects, info.Entities, outPath)
	return info, nil
}

func (r *renderer) renderAll(dir, outDir string) error {
	paths, err := filepath.Glob(filepath.Join(dir, "KONUNG.*"))
	if err != nil {
		return err
	}
	type numbered struct {
		n    int
		path string
	}
	var maps []numbered
	for _, p := range paths {
		if n, err := strconv.Atoi(strings.TrimPrefix(filepath.Ext(p), ".")); err == nil {
			maps = append(maps, numbered{n, p})
		}
	}
	if len(maps) == 0 {
		return fmt.Errorf("no KONUNG.N files in %s", dir)
	}
	sort.Slice(maps, func(i, j int) bool { return maps[i].n < maps[j].n })
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	var infos []mapInfo
	for _, m := range maps {
		info, err := r.renderOne(m.path, filepath.Join(outDir, fmt.Sprintf("konung_%02d.png", m.n)))
		if err != nil {
			return err
		}
		infos = append(infos, info)
	}
	f, err := os.Create(filepath.Join(outDir, "index.html"))
	if err != nil {
		return err
	}
	defer f.Close()
	page := struct {
		Style template.CSS
		Nav   template.HTML
		Maps  []mapInfo
	}{Maps: infos}
	root, inSite := sitenav.SectionRoot(outDir)
	if inSite {
		page.Style, page.Nav = template.CSS(sitenav.Style), sitenav.Nav("maps")
	}
	if err := indexTemplate.Execute(f, page); err != nil {
		return err
	}
	if inSite {
		return sitenav.WriteHub(root)
	}
	return nil
}

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<meta charset="utf-8"><title>KONUNG.N map renders</title>
<style>
body{background:#1b1b1b;color:#ddd;font:14px sans-serif;margin:16px}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(320px,1fr));gap:16px}
figure{margin:0;background:#262626;padding:8px;border-radius:6px}
img{width:100%;height:auto;display:block;image-rendering:auto}
figcaption{margin-top:6px}
{{.Style}}
</style>
{{.Nav}}
<h1>KONUNG.N map renders</h1>
<p>Terrain, flat decorations, shadows and static objects (no characters, animation, or lighting). Click a map for the full-size PNG.</p>
<div class="grid">{{range .Maps}}
<figure><a href="{{.File}}"><img src="{{.File}}" loading="lazy" alt="{{.Name}}"></a>
<figcaption><b>{{.Name}}</b> — {{.Width}}×{{.Height}} px, rows 0–{{.LastRow}}, cols 0–{{.LastCol}}, {{.Decorations}} decorations, {{.Objects}} objects{{if .ObjectErrors}}, {{.ObjectErrors}} undecodable{{end}}, {{.Entities}} characters/creatures{{if .EntityErrors}}, {{.EntityErrors}} undecodable{{end}}</figcaption></figure>{{end}}
</div>
`))
