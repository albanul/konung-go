package game

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"

	"konung1.5/internal/gamefs"
	"konung1.5/internal/resfmt"
)

// FindGameResource finds a resource by its repository-relative path, in the
// working directory, the program's directory, the source directory or one of
// their parents. In the
// browser the path is relative to the page.
func FindGameResource(resourceRelativePath string) (string, error) {
	if runtime.GOOS == "js" {
		if gamefs.Exists(resourceRelativePath) {
			return resourceRelativePath, nil
		}
		return "", fmt.Errorf("%s is not served next to the page (see cmd/web-build)", resourceRelativePath)
	}
	var roots []string
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
	}
	// Next to the program too: a double-clicked game starts in the user's
	// home folder (macOS Finder, desktop shortcuts), not in its own.
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		roots = append(roots, filepath.Dir(exe))
	}
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		roots = append(roots, filepath.Dir(sourceFile))
	}

	seen := make(map[string]bool)
	for _, root := range roots {
		for dir := root; ; dir = filepath.Dir(dir) {
			if !seen[dir] {
				seen[dir] = true
				candidate := filepath.Join(dir, resourceRelativePath)
				if gamefs.Exists(candidate) {
					return candidate, nil
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}

	return "", fmt.Errorf("could not find %s next to the program, in the working directory or their parents: put the game files in assets/Konung next to the program", resourceRelativePath)
}

// loadType5File reads a Type 5 .RES file and decodes all its frames.
func loadType5File(name string) ([]*image.RGBA, error) {
	data, err := gamefs.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return resfmt.LoadType5Frames(bytes.NewReader(data))
}
