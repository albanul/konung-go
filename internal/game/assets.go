package game

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func FindGameResource(resourceRelativePath string) (string, error) {
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
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
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
