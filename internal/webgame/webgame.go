// Package webgame serves the browser build of the game (cmd/web-build's
// page: index.html, konungfs.js, wasm_exec.js, konung.wasm) together with
// the game files it reads, for cmd/web-build and cmd/konung-server.
//
// The page reads manifest.txt (the game files there are) and fetches each
// file as assets/Konung/<name> next to itself (internal/gamefs, js.go). A
// full web-build output has both inside; a release bundle's page has
// neither, and the server lists and serves the files straight from the game
// folder (Handler), so the game files never have to be copied or shipped.
package webgame

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// GameFile matches the files the port reads from the game directory
// (case-insensitive, relative slash paths); everything else (DLLs, other
// executables, the original .flm films, 640×480 resources) is left out. The
// LAN client's files are in (LAN_KONG.EXE's tables, its screens, MAPS.RES,
// GAME.RES, the map window's INTO640R.RES, CLANS.CFG); LAN.CFG (the
// player's login, with a saved password) and HISCORES.DAT (other players'
// names) are never served: the browser writes its own (IndexedDB).
var GameFile = regexp.MustCompile(`(?i)^(` +
	`konung\.exe|lan_kong\.exe|clans\.cfg|` +
	`(graph|objects|lights|flames|sounds|musics|intr800r|menu800r|newh800r|save800r|questr)\.res|` +
	`(me800r|play800r|new800r|maps|game|into640r)\.res|` +
	`(konung|lights|game)\.\d+|konung\.(cfg|sav|sa\d)|` +
	`video/[^/]+\.(ivf|ogg)` + // the converted films (web-build remuxes them to .webm when it can)
	`)$`)

// hostOwn are the host's own settings and saves: a server serving the game
// folder leaves them out (every player would get the host's saves); the
// browser keeps its own (IndexedDB).
var hostOwn = regexp.MustCompile(`(?i)^konung\.(cfg|sav|sa\d)$`)

// served reports whether the server serves game file rel from the game
// folder.
func served(rel string) bool { return GameFile.MatchString(rel) && !hostOwn.MatchString(rel) }

// AssetsPrefix is where the page looks for the game files.
const AssetsPrefix = "assets/Konung"

// ManifestFile is the page's list of game files (internal/gamefs).
const ManifestFile = "manifest.txt"

// Manifest lists the game files the server serves from dir as the page
// names them ("assets/Konung/<name>"), sorted: GameFile's, without the
// host's own settings and saves.
func Manifest(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel = filepath.ToSlash(rel); served(rel) {
			out = append(out, path.Join(AssetsPrefix, rel))
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// Handler serves the browser game: the page from page, and the game files.
// When page is a full web-build output (it has manifest.txt), everything
// comes from it, without the host's settings and saves it carries; otherwise manifest.txt is listed from game and
// assets/Konung/<name> served from game, only the files GameFile allows and
// not the host's own settings and saves.
// Every answer asks the browser to revalidate, so a new konung.wasm is
// fetched after an update.
func Handler(page, game string) http.Handler {
	pageFiles := http.FileServer(http.Dir(page))
	_, err := os.Stat(filepath.Join(page, ManifestFile))
	full := err == nil
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		p := path.Clean("/" + r.URL.Path)
		if full || game == "" {
			// The page's own files, without the host's settings and saves
			// that a full web-build output carries.
			switch {
			case p == "/"+ManifestFile && full:
				b, err := os.ReadFile(filepath.Join(page, ManifestFile))
				if err != nil {
					http.NotFound(w, r)
					return
				}
				var keep []string
				for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
					if !hostOwn.MatchString(path.Base(l)) {
						keep = append(keep, l)
					}
				}
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Write([]byte(strings.Join(keep, "\n") + "\n"))
			case strings.HasPrefix(p, "/"+AssetsPrefix+"/") && hostOwn.MatchString(path.Base(p)):
				http.NotFound(w, r)
			default:
				pageFiles.ServeHTTP(w, r)
			}
			return
		}
		switch {
		case p == "/"+ManifestFile:
			list, err := Manifest(game)
			if err != nil || len(list) == 0 {
				http.Error(w, "no game files in "+game, http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte(strings.Join(list, "\n") + "\n"))
		case strings.HasPrefix(p, "/"+AssetsPrefix+"/"):
			rel := strings.TrimPrefix(p, "/"+AssetsPrefix+"/")
			if strings.Contains(rel, "..") || !served(rel) {
				http.NotFound(w, r)
				return
			}
			f := filepath.Join(game, filepath.FromSlash(rel))
			if st, err := os.Stat(f); err != nil || st.IsDir() {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, f)
		default:
			pageFiles.ServeHTTP(w, r)
		}
	})
}
