package game

import (
	"bytes"
	"image"
	"log"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"konung1.5/internal/resfmt"
)

// Films (docs/konung_films.md). 0x0042B690 mutes the music (0x0042A420(0)),
// opens the film in an MP.DLL window at FilmRect and blocks the game loop in
// GetMessage until the film ends (message 0x501 with lParam 2) or a key is
// pressed (WM_KEYDOWN, 0x0042BEAB); 0x0042B774 then closes it, restores the
// music volume and switches to the state queued in 0x00A3E7F0. The screen
// is not redrawn meanwhile, so the last game frame stays around the film.

// maxFilmDecodes bounds the frames decoded in one update, so a slow
// machine drops behind the audio instead of stalling the loop.
const maxFilmDecodes = 3

type filmPlayer struct {
	dec    *resfmt.FilmDecoder
	fps    float64
	frame  *image.RGBA
	img    *ebiten.Image
	shown  int // frames decoded so far
	ended  bool
	audio  *audio.Player
	start  time.Time
	frozen *ebiten.Image // the screen as it was when the film started
}

// openFilm loads a converted film from the game directory. It returns nil
// when the film is missing or broken, as 0x0042B690 goes straight on to the
// next state when CreateAviWnd fails.
func openFilm(gameDir, name string) *filmPlayer {
	videoPath, audioPath := resfmt.FilmPaths(gameDir, name)
	data, err := os.ReadFile(videoPath)
	if err != nil {
		log.Printf("film %s: %v", name, err)
		return nil
	}
	dec, err := resfmt.NewFilmDecoder(data)
	if err != nil {
		log.Printf("film %s: %v", name, err)
		return nil
	}
	f := &filmPlayer{dec: dec, fps: dec.IVF().FPS(), start: time.Now()}
	if sfx != nil && sfx.ctx != nil {
		if ogg, err := os.ReadFile(audioPath); err != nil {
			log.Printf("film %s: %v", name, err)
		} else if stream, err := vorbis.DecodeWithSampleRate(audioRate, bytes.NewReader(ogg)); err != nil {
			log.Printf("film %s: %v", name, err)
		} else if f.audio, err = sfx.ctx.NewPlayer(stream); err != nil {
			log.Printf("film %s: %v", name, err)
		} else {
			f.audio.Play()
		}
	}
	return f
}

// elapsed is the film clock: the audio position while the sound plays.
func (f *filmPlayer) elapsed() time.Duration {
	if f.audio != nil && f.audio.IsPlaying() {
		return f.audio.Position()
	}
	return time.Since(f.start)
}

// update advances the film and reports whether it is over: all frames shown
// and the sound finished, or any key pressed.
func (f *filmPlayer) update() bool {
	if len(inpututil.AppendJustPressedKeys(nil)) > 0 {
		return true
	}
	want := int(f.elapsed().Seconds()*f.fps) + 1
	fresh := false
	for n := 0; !f.ended && f.shown < want && n < maxFilmDecodes; n++ {
		img, ok, err := f.dec.Next(f.frame)
		if err != nil {
			log.Print(err)
		}
		if !ok {
			f.ended = true
			break
		}
		f.frame, fresh = img, true
		f.shown++
	}
	if fresh {
		if f.img == nil {
			f.img = ebiten.NewImage(f.frame.Rect.Dx(), f.frame.Rect.Dy())
		}
		f.img.WritePixels(f.frame.Pix)
	}
	return f.ended && (f.audio == nil || !f.audio.IsPlaying())
}

func (f *filmPlayer) draw(screen *ebiten.Image) {
	if f.frozen != nil {
		screen.DrawImage(f.frozen, nil)
	}
	if f.img != nil {
		drawAt(screen, f.img, resfmt.FilmRect.Min.X, resfmt.FilmRect.Min.Y)
	}
}

func (f *filmPlayer) close() {
	if f.audio != nil {
		f.audio.Close()
	}
}
