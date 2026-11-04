// Command map-render writes the terrain of KONUNG.N maps to PNG files.
//
//	go run ./cmd/map-render assets/Konung/KONUNG.1 output/konung1.png
//	go run ./cmd/map-render -all output/maps   # every KONUNG.N + index.html
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

	"konung1.5/internal/konungmap"
)

func main() {
	all := flag.String("all", "", "render every KONUNG.N in -dir into this output directory")
	dir := flag.String("dir", "assets/Konung", "directory containing KONUNG.N files (with -all)")
	graphPath := flag.String("graph", "assets/Konung/GRAPH.RES", "path to GRAPH.RES")
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
	switch {
	case *all != "" && flag.NArg() == 0:
		err = renderAll(*dir, *all, tiles)
	case *all == "" && flag.NArg() == 2:
		_, err = renderOne(flag.Arg(0), flag.Arg(1), tiles)
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

type mapInfo struct {
	Name, File       string
	Width, Height    int
	LastRow, LastCol int
}

func renderOne(mapPath, outPath string, tiles []*image.RGBA) (mapInfo, error) {
	data, err := os.ReadFile(mapPath)
	if err != nil {
		return mapInfo{}, err
	}
	m, err := konungmap.Parse(data)
	if err != nil {
		return mapInfo{}, fmt.Errorf("%s: %w", mapPath, err)
	}
	img := konungmap.Render(m, tiles)
	f, err := os.Create(outPath)
	if err != nil {
		return mapInfo{}, err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return mapInfo{}, err
	}
	info := mapInfo{Name: filepath.Base(mapPath), File: filepath.Base(outPath),
		Width: img.Bounds().Dx(), Height: img.Bounds().Dy(), LastRow: m.LastRow, LastCol: m.LastCol}
	fmt.Printf("%s: last row %d, last col %d, %dx%d px -> %s\n", mapPath, m.LastRow, m.LastCol, info.Width, info.Height, outPath)
	return info, nil
}

func renderAll(dir, outDir string, tiles []*image.RGBA) error {
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
		info, err := renderOne(m.path, filepath.Join(outDir, fmt.Sprintf("konung_%02d.png", m.n)), tiles)
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
	return indexTemplate.Execute(f, infos)
}

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<meta charset="utf-8"><title>KONUNG.N terrain renders</title>
<style>
body{background:#1b1b1b;color:#ddd;font:14px sans-serif;margin:16px}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(320px,1fr));gap:16px}
figure{margin:0;background:#262626;padding:8px;border-radius:6px}
img{width:100%;height:auto;display:block;image-rendering:auto}
figcaption{margin-top:6px}
</style>
<h1>KONUNG.N terrain renders</h1>
<p>Terrain layer only (no objects, characters, or lighting). Click a map for the full-size PNG.</p>
<div class="grid">{{range .}}
<figure><a href="{{.File}}"><img src="{{.File}}" loading="lazy" alt="{{.Name}}"></a>
<figcaption><b>{{.Name}}</b> — {{.Width}}×{{.Height}} px, rows 0–{{.LastRow}}, cols 0–{{.LastCol}}</figcaption></figure>{{end}}
</div>
`))
