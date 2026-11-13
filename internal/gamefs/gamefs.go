// Package gamefs is the game's file access: the game directory's resources,
// KONUNG.CFG and the saves. Natively it is the OS file system. In the browser
// (GOOS=js) resources are fetched over HTTP relative to the page, the files
// that exist are listed by a manifest, and written files (settings, saves)
// are kept in IndexedDB; see cmd/web-build.
//
// Names are slash- or OS-separated relative paths such as
// "assets/Konung/GRAPH.RES".
package gamefs

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ReadFile returns the contents of the file name.
func ReadFile(name string) ([]byte, error) { return readFile(name) }

// WriteFile creates or replaces the file name.
func WriteFile(name string, data []byte) error { return writeFile(name, data) }

// Exists reports whether name is a regular file.
func Exists(name string) bool { return exists(name) }

// ReadDir returns the names of the files in dir.
func ReadDir(dir string) ([]string, error) { return readDir(dir) }

// ReadFileFold reads name from dir, matching the name case-insensitively
// (the game opens QUESTR.RES; the installed file is questr.RES).
func ReadFileFold(dir, name string) ([]byte, error) {
	if data, err := ReadFile(filepath.Join(dir, name)); err == nil {
		return data, nil
	}
	names, err := ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, n := range names {
		if strings.EqualFold(n, name) {
			return ReadFile(filepath.Join(dir, n))
		}
	}
	return nil, fmt.Errorf("%s not found in %s", name, dir)
}
