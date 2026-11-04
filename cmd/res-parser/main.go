// Command res-parser extracts every frame of a Type 5 .RES file (FLAMES.RES,
// MENU640R.RES, SAVE800R.RES, ...) to frame_NNN.png files.
//
//	go run ./cmd/res-parser assets/Konung/FLAMES.RES output/flames
package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"

	"konung1.5/internal/resfmt"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: res-parser <resource.RES> <output_directory>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(resPath, outDir string) error {
	frames, err := resfmt.LoadType5File(resPath)
	if err != nil {
		return fmt.Errorf("%s: %w", resPath, err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for i, img := range frames {
		name := fmt.Sprintf("frame_%03d.png", i)
		f, err := os.Create(filepath.Join(outDir, name))
		if err != nil {
			return err
		}
		err = png.Encode(f, img)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		fmt.Printf("frame %2d: %dx%d -> %s\n", i, img.Bounds().Dx(), img.Bounds().Dy(), name)
	}
	fmt.Printf("extracted %d frames to %s\n", len(frames), outDir)
	return nil
}
