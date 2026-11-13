// Command web-build builds the browser (WebAssembly) version of the game into
// a static directory and optionally serves it:
//
//	go run ./cmd/web-build                   # -> output/web
//	go run ./cmd/web-build -serve :8080      # build, then serve http://localhost:8080
//	web-build -build=false -serve 8080       # serve an earlier build (no Go needed)
//
// The directory holds index.html, konungfs.js, wasm_exec.js, konung.wasm, the
// game files the port reads (copied from -assets to assets/Konung/, hard
// linked when possible) and manifest.txt listing them (internal/gamefs).
// Settings and saves written in the browser live in its IndexedDB.
package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

//go:embed web/index.html web/konungfs.js
var webFiles embed.FS

// gameFile matches the files the port reads from the game directory
// (case-insensitive); everything else (DLLs, other executables, the
// original .flm films, LAN files, 640×480 resources) is left out.
var gameFile = regexp.MustCompile(`(?i)^(` +
	`konung\.exe|` +
	`(graph|objects|lights|flames|sounds|musics|intr800r|menu800r|newh800r|save800r|questr)\.res|` +
	`(konung|lights|game)\.\d+|konung\.(cfg|sav|sa\d)|` +
	`video/[^/]+\.(ivf|ogg)` + // only without ffmpeg; see remuxFilms
	`)$`)

func main() {
	out := flag.String("out", "output/web", "output directory")
	assets := flag.String("assets", "assets/Konung", "game directory")
	serve := flag.String("serve", "", "after building, serve the output on this address or port (e.g. :8080 or 8080)")
	doBuild := flag.Bool("build", true, "build before serving; -build=false serves an earlier build without the Go toolchain")
	flag.Parse()

	if *doBuild {
		if err := build(*out, *assets); err != nil {
			log.Fatal(err)
		}
	} else if _, err := os.Stat(filepath.Join(*out, "konung.wasm")); err != nil {
		log.Fatalf("no earlier build in %s (run with -build in an environment with Go): %v", *out, err)
	}
	if *serve == "" {
		return
	}
	addr := *serve
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	log.Printf("serving %s on http://localhost%s", *out, addr)
	log.Fatal(http.ListenAndServe(addr, noCache(http.FileServer(http.Dir(*out)))))
}

func build(out, assets string) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(out, "konung.wasm"), "./cmd/konung")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return fmt.Errorf("go env GOROOT: %w", err)
	}
	if err := copyFile(filepath.Join(strings.TrimSpace(string(goroot)), "lib", "wasm", "wasm_exec.js"), filepath.Join(out, "wasm_exec.js")); err != nil {
		return err
	}
	for _, name := range []string{"index.html", "konungfs.js"} {
		data, _ := webFiles.ReadFile("web/" + name)
		if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
			return err
		}
	}

	_, lookErr := exec.LookPath("ffmpeg")
	haveFFmpeg := lookErr == nil
	if !haveFFmpeg {
		log.Print("ffmpeg not found: serving the films as .ivf/.ogg, which the browser build decodes in Go (very slowly)")
	}
	// assets/Konung/ under out, as FindGameResource looks it up.
	dst := filepath.Join(out, "assets", "Konung")
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	var manifest []string
	var total int64
	err = filepath.WalkDir(assets, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(assets, p)
		rel = filepath.ToSlash(rel)
		if !gameFile.MatchString(rel) || (haveFFmpeg && isFilmPart(rel)) {
			return nil
		}
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if os.Link(p, target) != nil {
			if err := copyFile(p, target); err != nil {
				return err
			}
		}
		info, _ := d.Info()
		total += info.Size()
		manifest = append(manifest, path.Join("assets/Konung", rel))
		return nil
	})
	if err != nil {
		return err
	}
	if len(manifest) == 0 {
		return fmt.Errorf("no game files found in %s", assets)
	}
	films, err := remuxFilms(assets, dst)
	if err != nil {
		return err
	}
	manifest = append(manifest, films...)
	sort.Strings(manifest)
	if err := os.WriteFile(filepath.Join(out, "manifest.txt"), []byte(strings.Join(manifest, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	log.Printf("built %s: %d game files (%.0f MB)", out, len(manifest), float64(total)/1e6)
	return nil
}

func isFilmPart(rel string) bool {
	ext := strings.ToLower(path.Ext(rel))
	return strings.HasPrefix(strings.ToLower(rel), "video/") && (ext == ".ivf" || ext == ".ogg")
}

// remuxFilms writes Video/<name>.webm for every converted film (<name>.ivf
// AV1 + <name>.ogg Vorbis from cmd/video-convert) without re-encoding; the
// browser plays it in a <video> element (internal/game/film_js.go). It
// returns the manifest entries; without ffmpeg none.
func remuxFilms(assets, dst string) ([]string, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, nil
	}
	ivfs, _ := filepath.Glob(filepath.Join(assets, "Video", "*.ivf"))
	var out []string
	for _, ivf := range ivfs {
		base := strings.TrimSuffix(filepath.Base(ivf), filepath.Ext(ivf))
		name := strings.ToLower(base) + ".webm"
		args := []string{"-v", "error", "-y", "-i", ivf}
		if ogg := strings.TrimSuffix(ivf, filepath.Ext(ivf)) + ".ogg"; fileExists(ogg) {
			args = append(args, "-i", ogg, "-map", "0:v", "-map", "1:a")
		}
		target := filepath.Join(dst, "Video", name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		cmd := exec.Command("ffmpeg", append(args, "-c", "copy", target)...)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("ffmpeg %s: %w", ivf, err)
		}
		out = append(out, "assets/Konung/Video/"+name)
	}
	return out, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	o, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(o, in); err != nil {
		o.Close()
		return err
	}
	return o.Close()
}

// noCache makes the browser revalidate, so a rebuilt konung.wasm is picked up.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}
