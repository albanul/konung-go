package game

import (
	"bytes"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

// Sound effects and music (docs/konung_sound.md). The game plays SOUNDS.RES
// entries through DirectSound buffers (0x0042A664) and streams a random
// MUSICS.RES track (0x0042A474), picking another when it ends (0x00429BC4).

const (
	audioRate       = 44100
	maxSoundBuffers = 45     // 0x0042A79E
	musicOnVolume   = -1000  // 0x0042A420: music on
	musicOffVolume  = -10000 // music off: silent, but it keeps playing
)

// soundSystem is the game's audio. sfx is nil when the resources are
// missing (and in tests), which makes every call a no-op.
type soundSystem struct {
	ctx     *audio.Context
	sounds  map[int][]byte // SOUNDS.RES PCM by id
	active  []*audio.Player
	tracks  [][]byte
	music   *audio.Player
	track   int // 0x00A3E7D4
	musicDB int
	soundOn bool // setting 0x00A3E75C
	rng     *rand.Rand
}

var sfx *soundSystem

// startAudio loads SOUNDS.RES and MUSICS.RES from the game directory and
// starts a random track (0x00438E80), at the volume of the music setting.
func startAudio(dir string, musicOn, soundOn bool) {
	a := &soundSystem{sounds: map[int][]byte{}, soundOn: soundOn, rng: rand.New(rand.NewPCG(7, 9))}
	if f, err := os.Open(filepath.Join(dir, "SOUNDS.RES")); err != nil {
		log.Print(err)
	} else {
		st, _ := f.Stat()
		entries, err := resfmt.LoadSoundEntries(f, st.Size())
		f.Close()
		if err != nil {
			log.Printf("SOUNDS.RES: %v", err)
		}
		for _, e := range entries {
			if len(e.Data) >= 2 { // 0x0042A695: entries below 2 bytes are skipped
				a.sounds[e.Index] = e.Data
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, "MUSICS.RES")); err != nil {
		log.Print(err)
	} else if a.tracks, err = resfmt.LoadMusicTracks(data); err != nil {
		log.Print(err)
	}
	a.ctx = audio.NewContext(audioRate)
	a.musicDB = musicOffVolume
	if musicOn {
		a.musicDB = musicOnVolume
	}
	sfx = a
	if len(a.tracks) > 0 {
		a.playTrack(a.rng.IntN(len(a.tracks)))
	}
}

// play is 0x0042A664(id, volume, pan, 0): nothing with sound off, below
// −4000 or for a missing entry, or when 45 buffers are playing. Voices and
// a few effects pick one of three sample rates at random (a pitch change).
func (a *soundSystem) play(id, volume, pan int) {
	if a == nil || !a.soundOn || volume < gamestate.SoundMinVolume {
		return
	}
	pcm := a.sounds[id]
	if pcm == nil {
		return
	}
	live := a.active[:0]
	for _, p := range a.active {
		if p.IsPlaying() {
			live = append(live, p)
		} else {
			p.Close()
		}
	}
	a.active = live
	if len(a.active) >= maxSoundBuffers {
		return
	}
	rate := gamestate.SoundRates[0]
	if gamestate.SoundVariants(id) {
		rate = gamestate.SoundRates[a.rng.IntN(len(gamestate.SoundRates))]
	}
	l, r := resfmt.DSPanGains(pan)
	g := resfmt.DSGain(volume)
	p := a.ctx.NewPlayerFromBytes(resfmt.RenderSound(pcm, rate, audioRate, l*g, r*g))
	p.Play()
	a.active = append(a.active, p)
}

// playTrack is 0x0042A474: decode and play track i once, at the current
// music volume (0x0042A617).
func (a *soundSystem) playTrack(i int) {
	if a.music != nil {
		a.music.Close()
		a.music = nil
	}
	stream, err := vorbis.DecodeWithSampleRate(audioRate, bytes.NewReader(a.tracks[i]))
	if err != nil {
		log.Printf("MUSICS.RES track %d: %v", i, err)
		return
	}
	p, err := a.ctx.NewPlayer(stream)
	if err != nil {
		log.Printf("MUSICS.RES track %d: %v", i, err)
		return
	}
	a.track, a.music = i, p
	p.SetVolume(resfmt.DSGain(a.musicDB))
	p.Play()
}

// setMusic is 0x0042A420: the music setting changes the volume only.
func (a *soundSystem) setMusic(on bool) {
	if a == nil {
		return
	}
	a.musicDB = musicOffVolume
	if on {
		a.musicDB = musicOnVolume
	}
	if a.music != nil {
		a.music.SetVolume(resfmt.DSGain(a.musicDB))
	}
}

// update starts the next track when the current one has ended (0x00429BC4):
// a random one, the following one if that repeats the current.
func (a *soundSystem) update() {
	if a == nil || len(a.tracks) == 0 || a.music == nil || a.music.IsPlaying() {
		return
	}
	next := a.rng.IntN(len(a.tracks))
	if next == a.track {
		next = (next + 1) % len(a.tracks)
	}
	a.playTrack(next)
}

// playAt plays a sound from a fine cell, with the volume and pan of its
// distance from the hero (0x00439F4C, 0x00439EEC).
func (s *startScene) playAt(id int, c mapdata.Cell) {
	if sfx == nil {
		return
	}
	vol, pan := 0, 0
	if s.hero != nil {
		hx, hy := mapdata.CellCenter(s.hero.cell)
		x, y := mapdata.CellCenter(c)
		vol, pan = gamestate.SoundVolume(x-hx, y-hy), gamestate.SoundPan(c.Col-s.hero.cell.Col)
	}
	sfx.play(id, vol, pan)
}

// actionSound is the sound 0x004143DC plays when a walker starts action act.
func (s *startScene) actionSound(w *walker, act int) {
	if sfx == nil {
		return
	}
	if id := gamestate.ActionSound(s.entity(w.entity), act, w == s.hero, sfx.rng.IntN); id > 0 {
		s.playAt(id, w.cell)
	}
}

// soundTick runs once per game frame (60 ms): queued rule sounds, idle
// sounds of walkers that just stopped (0x00410450), and the map's ambient
// sound (0x004376F4).
func (s *startScene) soundTick() {
	if sfx == nil {
		s.game.Sounds = s.game.Sounds[:0]
		return
	}
	for _, id := range s.game.Sounds {
		sfx.play(id, 0, 0)
	}
	s.game.Sounds = s.game.Sounds[:0]
	settle := func(w *walker, dead bool) {
		busy := w.walking || len(w.path) > 0 || w.act != 0
		if w.busy && !busy && !dead {
			s.actionSound(w, gamestate.ActIdle)
		}
		w.busy = busy
	}
	if s.hero != nil && len(s.party) > 0 {
		settle(s.hero, s.party[0].Dead())
	}
	for _, n := range s.npcs {
		settle(&n.walker, s.game.Entities[n.entity].Dead())
	}
	if id := gamestate.AmbientSound(s.mapNumber, sfx.rng.IntN); id >= 0 {
		sfx.play(id, 0, 0)
	}
}
