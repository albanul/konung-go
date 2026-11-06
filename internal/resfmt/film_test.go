package resfmt

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// aviVideoFrames counts the '00dc' chunks of an AVI's movi list, less the
// trailing empty ones: an empty chunk repeats the previous frame, which the
// conversion keeps inside the film but cannot after its last real frame.
func aviVideoFrames(data []byte) (int, error) {
	if len(data) < 12 || string(data[8:12]) != "AVI " {
		return 0, fmt.Errorf("not an AVI")
	}
	i := bytes.Index(data, []byte("movi"))
	if i < 8 {
		return 0, fmt.Errorf("no movi list")
	}
	end := min(i-4+int(binary.LittleEndian.Uint32(data[i-4:])), len(data))
	n, trailing := 0, 0
	for o := i + 4; o+8 <= end; {
		size := int(binary.LittleEndian.Uint32(data[o+4:]))
		if string(data[o:o+4]) == "00dc" {
			n++
			if trailing = trailing + 1; size != 0 {
				trailing = 0
			}
		}
		o += 8 + size + size&1
	}
	return n - trailing, nil
}

// TestFilmsMatchSources checks each converted film against its .flm: the
// IVF holds as many frames as the source AVI (aviVideoFrames). FILM_ALL=1
// also decodes every film to the end.
func TestFilmsMatchSources(t *testing.T) {
	srcs, _ := filepath.Glob("../../assets/Konung/Video/*.flm")
	if len(srcs) == 0 {
		t.Skip("no films installed")
	}
	for _, src := range srcs {
		name := strings.TrimSuffix(filepath.Base(src), ".flm")
		raw, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		want, err := aviVideoFrames(raw)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		video, _ := FilmPaths("../../assets/Konung", name)
		data, err := os.ReadFile(video)
		if err != nil {
			t.Skip("converted films not available (run cmd/video-convert):", err)
		}
		d, err := NewFilmDecoder(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if n := len(d.IVF().Frames); n != want {
			t.Errorf("%s: %d frames, source has %d", name, n, want)
		}
		if os.Getenv("FILM_ALL") == "" {
			continue
		}
		n := 0
		for {
			_, ok, err := d.Next(nil)
			if err != nil {
				t.Fatalf("%s frame %d: %v", name, n, err)
			}
			if !ok {
				break
			}
			n++
		}
		if n != want {
			t.Errorf("%s: decoded %d frames, want %d", name, n, want)
		}
	}
}

// TestFilmDefeat1 decodes the converted DEFEAT1 film (cmd/video-convert):
// 101 frames of 640×360 at 12 fps, as in the original AVI header. Set
// FILM_PNG=<path> to write the middle frame for a look.
func TestFilmDefeat1(t *testing.T) {
	video, audio := FilmPaths("../../assets/Konung", DefeatFilm(1))
	data, err := os.ReadFile(video)
	if err != nil {
		t.Skip("converted film not available (run cmd/video-convert):", err)
	}
	if _, err := os.Stat(audio); err != nil {
		t.Errorf("audio track: %v", err)
	}
	d, err := NewFilmDecoder(data)
	if err != nil {
		t.Fatal(err)
	}
	if v := d.IVF(); v.Width != FilmRect.Dx() || v.Height != FilmRect.Dy() || v.FPS() != 12 {
		t.Errorf("header %dx%d at %v fps", v.Width, v.Height, v.FPS())
	}
	start := time.Now()
	n := 0
	for {
		img, ok, err := d.Next(nil)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		if img.Rect != FilmRect.Sub(FilmRect.Min) {
			t.Fatalf("frame %d: %v", n, img.Rect)
		}
		if p := os.Getenv("FILM_PNG"); p != "" && n == 50 {
			f, err := os.Create(p)
			if err != nil {
				t.Fatal(err)
			}
			png.Encode(f, img)
			f.Close()
		}
		n++
	}
	if n != 101 {
		t.Errorf("%d frames, want 101", n)
	}
	t.Logf("%d frames in %v", n, time.Since(start))
}
