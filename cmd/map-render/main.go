// Command map-render writes the terrain of a KONUNG.N map to a PNG.
//
//	go run ./cmd/map-render assets/Konung/KONUNG.1 output/konung1.png
package main

import (
	"fmt"
	"image/png"
	"os"

	"konung1.5/internal/konungmap"
)

func main() {
	if len(os.Args) < 3 || len(os.Args) > 4 {
		fmt.Fprintln(os.Stderr, "usage: map-render <KONUNG.N> <out.png> [GRAPH.RES]")
		os.Exit(2)
	}
	graphPath := "assets/Konung/GRAPH.RES"
	if len(os.Args) == 4 {
		graphPath = os.Args[3]
	}
	if err := run(os.Args[1], os.Args[2], graphPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(mapPath, outPath, graphPath string) error {
	mapData, err := os.ReadFile(mapPath)
	if err != nil {
		return err
	}
	m, err := konungmap.Parse(mapData)
	if err != nil {
		return err
	}
	graph, err := os.ReadFile(graphPath)
	if err != nil {
		return err
	}
	tiles, err := konungmap.LoadTerrainTiles(graph)
	if err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	img := konungmap.Render(m, tiles)
	fmt.Printf("%s: last row %d, last col %d, %dx%d px\n", mapPath, m.LastRow, m.LastCol, img.Bounds().Dx(), img.Bounds().Dy())
	return png.Encode(f, img)
}
