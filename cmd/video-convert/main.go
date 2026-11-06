// Command video-convert rewrites the game's films (Video/*.flm: AVI with
// WMV3 video and PCM audio, played through Windows' DirectShow in the
// original) into <name>.ivf (AV1) and <name>.ogg (Vorbis) next to them,
// which the port decodes in pure Go on every platform (docs/konung_films.md).
//
// It needs an ffmpeg with the WMV3 decoder, libsvtav1 and libvorbis (on
// Fedora: ffmpeg from RPM Fusion; ffmpeg-free lacks WMV3; on macOS:
// brew install ffmpeg).
//
//	go run ./cmd/video-convert [-crf 35] [-force] assets/Konung/Video
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	crf := flag.Int("crf", 35, "AV1 quality (lower is better and bigger)")
	audioQ := flag.Int("q", 3, "Vorbis quality")
	force := flag.Bool("force", false, "convert films that already have both outputs")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: video-convert [-crf N] [-q N] [-force] <Video directory>")
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *crf, *audioQ, *force); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(dir string, crf, audioQ int, force bool) error {
	films, err := filepath.Glob(filepath.Join(dir, "*.[fF][lL][mM]"))
	if err != nil {
		return err
	}
	if len(films) == 0 {
		return fmt.Errorf("no .flm files in %s", dir)
	}
	for _, src := range films {
		base := filepath.Join(dir, strings.ToLower(strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))))
		video, audio := base+".ivf", base+".ogg"
		if !force && exists(video) && exists(audio) {
			fmt.Println("skip", filepath.Base(src))
			continue
		}
		// The films are 640×360 at 12 fps; a key frame every 10 s.
		if err := ffmpeg("-i", src, "-map", "0:v", "-an", "-c:v", "libsvtav1", "-preset", "6",
			"-crf", strconv.Itoa(crf), "-g", "120", "-pix_fmt", "yuv420p", video); err != nil {
			return fmt.Errorf("%s video: %w", src, err)
		}
		if err := ffmpeg("-i", src, "-map", "0:a", "-vn", "-c:a", "libvorbis",
			"-q:a", strconv.Itoa(audioQ), audio); err != nil {
			return fmt.Errorf("%s audio: %w", src, err)
		}
		fmt.Println("ok  ", filepath.Base(src))
	}
	return nil
}

func ffmpeg(args ...string) error {
	cmd := exec.Command("ffmpeg", append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
