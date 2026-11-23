package webgame

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// The LAN client's files are served; the player's login (LAN.CFG, with a
// saved password) and the rating cache (other players' names) never are.
func TestGameFileLAN(t *testing.T) {
	for _, name := range []string{"LAN_KONG.EXE", "ME800R.RES", "PLAY800R.RES", "NEW800R.RES", "MAPS.RES",
		"GAME.RES", "INTO640R.RES", "CLANS.CFG", "GRAPH.RES", "GAME.0", "konung.exe"} {
		if !GameFile.MatchString(name) {
			t.Errorf("%s not served", name)
		}
	}
	for _, name := range []string{"LAN.CFG", "HISCORES.DAT", "Servers.cfg", "SERVER", "SERVER.LOG", "LANUSERS.DAT", "SAVITEMS.DAT", "INTR640R.RES"} {
		if GameFile.MatchString(name) {
			t.Errorf("%s served", name)
		}
	}
}

// A bundle's page (no manifest.txt of its own): the list comes from the
// game folder and the files from there, only the allowed ones; a full
// web-build output serves its own.
func TestHandler(t *testing.T) {
	page, game := t.TempDir(), t.TempDir()
	write := func(dir, name, body string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(page, "index.html", "page")
	write(page, "konung.wasm", "wasm")
	write(game, "GRAPH.RES", "graph")
	write(game, "questr.RES", "quests")
	write(game, "Video/intro.ivf", "film")
	write(game, "LAN.CFG", "secret")
	write(game, "HISCORES.DAT", "names")
	write(game, "KONUNG.SAV", "host's saves")
	write(game, "KONUNG.CFG", "host's settings")
	h := Handler(page, game)
	get := func(path string) (int, string) {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code, rec.Body.String()
	}
	if code, body := get("/"); code != http.StatusOK || body != "page" {
		t.Errorf("/: %d %q", code, body)
	}
	if code, body := get("/manifest.txt"); code != http.StatusOK ||
		body != "assets/Konung/GRAPH.RES\nassets/Konung/Video/intro.ivf\nassets/Konung/questr.RES\n" {
		t.Errorf("manifest: %d %q", code, body)
	}
	for path, want := range map[string]string{
		"/assets/Konung/GRAPH.RES": "graph", "/assets/Konung/questr.RES": "quests", "/assets/Konung/Video/intro.ivf": "film",
	} {
		if code, body := get(path); code != http.StatusOK || body != want {
			t.Errorf("%s: %d %q", path, code, body)
		}
	}
	for _, path := range []string{"/assets/Konung/LAN.CFG", "/assets/Konung/HISCORES.DAT", "/assets/Konung/../../etc/passwd",
		"/assets/Konung/Video/../LAN.CFG", "/assets/Konung/KONUNG.SAV", "/assets/Konung/KONUNG.CFG"} {
		if code, body := get(path); code == http.StatusOK {
			t.Errorf("%s served: %q", path, body)
		}
	}
	// A full web-build output: its own manifest and files.
	write(page, "manifest.txt", "assets/Konung/KONUNG.SAV\nassets/Konung/MAPS.RES\n")
	write(page, "assets/Konung/MAPS.RES", "maps")
	write(page, "assets/Konung/KONUNG.SAV", "host's saves")
	h = Handler(page, game)
	if code, body := get("/manifest.txt"); code != http.StatusOK || body != "assets/Konung/MAPS.RES\n" {
		t.Errorf("full build manifest: %d %q", code, body)
	}
	if code, _ := get("/assets/Konung/KONUNG.SAV"); code == http.StatusOK {
		t.Error("full build: the host's saves served")
	}
	if code, body := get("/assets/Konung/MAPS.RES"); code != http.StatusOK || body != "maps" {
		t.Errorf("full build file: %d %q", code, body)
	}
}
