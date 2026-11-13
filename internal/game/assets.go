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
// working directory, the source directory or one of their parents. In the
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

	return "", fmt.Errorf("could not find %s from working directory or source location; run from the repository or pass the resource path as an argument", resourceRelativePath)
}

// loadType5File reads a Type 5 .RES file and decodes all its frames.
func loadType5File(name string) ([]*image.RGBA, error) {
	data, err := gamefs.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return resfmt.LoadType5Frames(bytes.NewReader(data))
}
