package main

import (
	"os"
	"path/filepath"
	"strings"
)

// Where the server finds things when it is not told (a release bundle run
// by a double click, whose working folder may be the user's home):
//
//	konung-server(.exe)
//	konung-server.conf        the settings (optional)
//	web/                      the browser game's page (web-build -page-only)
//	assets/Konung/            the game files the user copies in
//	server-data/              accounts and progress (made on the first run)

// exeDir is the folder of the running program ("" if unknown).
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return filepath.Dir(exe)
}

// isBundle reports a release bundle's layout: the page in web/ next to the
// program.
func isBundle() bool {
	d := exeDir()
	if d == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(d, "web", "index.html"))
	return err == nil
}

// defaultDataDir is server-data next to the program in a bundle, else the
// working folder.
func defaultDataDir() string {
	if isBundle() {
		return filepath.Join(exeDir(), "server-data")
	}
	return "."
}

// findGameDir is assets/Konung in the working folder, next to the program,
// or in a folder above either ("" if none): where the game finds its files.
func findGameDir() string {
	var roots []string
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
	}
	if d := exeDir(); d != "" {
		roots = append(roots, d)
	}
	for _, root := range roots {
		for dir := root; ; dir = filepath.Dir(dir) {
			c := filepath.Join(dir, "assets", "Konung")
			if st, err := os.Stat(c); err == nil && st.IsDir() {
				return c
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return ""
}

// findFile is name in the first of dirs that has it, its case ignored (the
// installed names differ: questr.RES); "" if none.
func findFile(name string, dirs ...string) string {
	for _, d := range dirs {
		if d == "" {
			continue
		}
		p := filepath.Join(d, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.EqualFold(e.Name(), name) {
				return filepath.Join(d, e.Name())
			}
		}
	}
	return ""
}
