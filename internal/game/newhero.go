package game

import (
	"fmt"
	"image"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/resfmt"
)

// The character-creation screen «Выбор героя» (screen state 2,
// docs/konung_new_game.md): opened by «Новая игра» (0x00437B91), drawn by
// 0x0042FA58, its descriptions by 0x00430564, clicks by 0x00437E86. The
// rules are in gamestate/newhero.go; this is the thin Ebitengine side.

// NEWH800R.RES frames (Type 5, loaded to frame slots 0x1F3 + i).
const (
	newhBackground = 0  // 800×600, transparent where the tile shows
	newhTile       = 1  // 100×100, tiled under the background
	newhMarker     = 2  // 13×13 blue marker of the chosen class and skills
	newhPortraits  = 3  // 3–8: Драгомир, Михаил, Волк; chosen, then not chosen
	newhCancel     = 9  // 9 normal, 10 pressed
	newhOK         = 11 // 11 normal, 12 pressed
	newhFrames     = 13
)

// newHeroModels maps the portrait hotspots 0–2 to hero models.
var newHeroModels = [3]int{gamestate.HeroDragomir, gamestate.HeroMichail, gamestate.HeroVolk}

// newHeroClasses maps the class hotspots 3–6 to class bytes.
var newHeroClasses = [4]byte{gamestate.ClassMerchant, gamestate.ClassHunter, gamestate.ClassChief, gamestate.ClassWarrior}

// Preview placement (0x004303B2–0x00430553): the body is drawn with its
// image top-left at (60, 20) in a 160×320 buffer, the buffer is blitted at
// hotspot 22 and clipped to 160×160.
const (
	previewBodyX = 60
	previewBodyY = 20
	previewSize  = 160
)

type newHeroScreen struct {
	frames [newhFrames]*ebiten.Image
	tables *resfmt.ExeTables
	font   *gameFont // small font 0x005A31F4
	graph  []byte
	items  *gamestate.Game // the preview items (gamestate.NewHeroItems)

	hero gamestate.Entity // the template the screen edits
	text int              // description shown (0x00462F3C + text·512), −1 = none
	ok   bool             // «Ok» pressed: drawn pressed for one frame, then the game starts

	preview    *ebiten.Image
	previewDX  int // preview image offset from the body's image top-left
	previewDY  int
	previewKey previewKey // what the preview was built from
}

type previewKey struct {
	model                int
	palette              uint32
	weapon, typ, variant int
	gear                 [6]int
}

func newNewHeroScreen(path string, a *sceneAssets) (*newHeroScreen, error) {
	frames, err := resfmt.LoadType5File(path)
	if err != nil {
		return nil, err
	}
	if len(frames) < newhFrames {
		return nil, fmt.Errorf("%s has %d frames; need %d", path, len(frames), newhFrames)
	}
	s := &newHeroScreen{tables: a.tables, font: a.font, graph: a.graph, items: gamestate.NewHeroItems()}
	for i := range s.frames {
		s.frames[i] = ebiten.NewImageFromImage(frames[i])
	}
	return s, nil
}

// open is «Новая игра» (0x00437B91): a fresh template, no description,
// and the template's idle action (0x00437D2C).
func (s *newHeroScreen) open() {
	s.hero = gamestate.NewHeroTemplate()
	s.text = -1
	s.ok = false
	s.idle()
}

// idle is 0x004143DC(hero, 0), the screen's only sound: the template
// starts its idle action, which plays the hero's idle voice (model·8 +
// 0x23) on rand() % 100 > 95. The hero is its own listener, so the voice
// plays at full volume, centred (0x00439F4C, 0x00439EEC). It is called
// when the screen opens and on every hero portrait click.
func (s *newHeroScreen) idle() {
	if sfx == nil {
		return
	}
	if id := idleVoice(s.hero, sfx.rng.IntN); id != 0 {
		sfx.play(id, 0, 0)
	}
}

// idleVoice is the sound id of the template's idle action, 0 = none.
func idleVoice(hero gamestate.Entity, rnd func(int) int) int {
	return gamestate.ActionSound(hero, gamestate.ActIdle, true, rnd)
}

// hover shows the description of the hotspot under the mouse
// (0x004388CE): hotspots 0–19 their own, the −/+ buttons their stat's
// (7–10). Elsewhere the last description stays.
func (s *newHeroScreen) hover(hit int) {
	switch {
	case hit >= 0 && hit < gamestate.NewHeroTextCount:
		s.text = hit
	case hit >= gamestate.NewHeroMinus:
		s.text = gamestate.NewHeroStat + (hit-gamestate.NewHeroMinus)/2
	}
}

// click runs hotspot hit (jump table 0x00438828) and reports «Ok» or
// «Отмена».
func (s *newHeroScreen) click(hit int) (ok, cancel bool) {
	e := &s.hero
	switch {
	case hit >= gamestate.NewHeroPortrait && hit < gamestate.NewHeroClass:
		gamestate.SelectHero(e, newHeroModels[hit])
		s.text = hit
		s.idle() // 0x00437F62, 0x0043801C, 0x004380E1
	case hit >= gamestate.NewHeroClass && hit < gamestate.NewHeroStat:
		gamestate.SelectClass(e, newHeroClasses[hit-gamestate.NewHeroClass])
		s.text = hit
	case hit == gamestate.NewHeroFencing || hit == gamestate.NewHeroArchery:
		gamestate.SelectCombatSkill(e, hit == gamestate.NewHeroArchery)
		s.text = hit
	case hit >= gamestate.NewHeroIdentify && hit <= gamestate.NewHeroHealing:
		gamestate.SelectSkill(e, hit-gamestate.NewHeroIdentify)
		s.text = hit
	case hit == gamestate.NewHeroOK: // 0x004384C4
		s.ok = true
		return true, false
	case hit == gamestate.NewHeroCancel: // 0x004384DD
		return false, true
	case hit >= gamestate.NewHeroMinus:
		k := hit - gamestate.NewHeroMinus
		if gamestate.TradeStat(e, s.tables, k/2, k%2 == 1) {
			s.text = gamestate.NewHeroStat + k/2
		}
	}
	return false, false
}

func rect800(i int) [4]int { return gamestate.NewHeroRects800[i] }

// draw is 0x0042FA58 followed by the description 0x00430564.
func (s *newHeroScreen) draw(dst *ebiten.Image) {
	tile := s.frames[newhTile]
	tw, th := tile.Bounds().Dx(), tile.Bounds().Dy()
	for y := 0; y < ScreenHeight; y += th {
		for x := 0; x < ScreenWidth; x += tw {
			drawAt(dst, tile, x, y)
		}
	}
	dst.DrawImage(s.frames[newhBackground], nil)
	e := s.hero
	// Portraits: the chosen hero's frame, the others' plain frames.
	for i, model := range newHeroModels {
		f := newhPortraits + 2*i + 1
		if e.Model == model {
			f--
		}
		r := rect800(gamestate.NewHeroPortrait + i)
		drawAt(dst, s.frames[f], r[0], r[1])
	}
	s.drawMarker(dst, int(e.Flags4)+1) // class byte 2–5 → hotspot 3–6
	// Base Хар, Сил, Лов, Вын (the high words of +0x58, +0x5E, +0x54, +0x62).
	for i, v := range e.BaseStats {
		s.number(dst, gamestate.NewHeroStat+i, strconv.Itoa(v))
	}
	s.number(dst, gamestate.NewHeroWeight, fmt.Sprintf("%4.1f", float64(gamestate.MaxWeight(e))*0.001))
	s.number(dst, gamestate.NewHeroParty, strconv.Itoa(gamestate.MaxParty(e, s.tables)))
	s.number(dst, gamestate.NewHeroSmithing, strconv.Itoa(e.Skills[1]))
	s.number(dst, gamestate.NewHeroBuilding, strconv.Itoa(e.Skills[3]))
	if e.Fencing > e.Marksmanship {
		s.drawMarker(dst, gamestate.NewHeroFencing)
	} else {
		s.drawMarker(dst, gamestate.NewHeroArchery)
	}
	if k := gamestate.ChosenSkill(e); k >= 0 {
		s.drawMarker(dst, gamestate.NewHeroIdentify+k)
	}
	s.number(dst, gamestate.NewHeroFreeExp, strconv.Itoa(e.FreeExp))
	ok := newhOK
	if s.ok {
		ok++
	}
	r := rect800(gamestate.NewHeroOK)
	drawAt(dst, s.frames[ok], r[0], r[1])
	r = rect800(gamestate.NewHeroCancel)
	drawAt(dst, s.frames[newhCancel], r[0], r[1])
	s.drawPreview(dst)
	if s.text >= 0 {
		r := rect800(gamestate.NewHeroText)
		text := s.tables.CString(0x00462F3C + s.text*512)
		s.font.drawWrapped(dst, text, r[0], r[1], r[2]-r[0])
	}
}

// number draws v with the small font at the hotspot's top-left (0x00427A80).
func (s *newHeroScreen) number(dst *ebiten.Image, hotspot int, v string) {
	r := rect800(hotspot)
	s.font.drawLine(dst, []byte(v), r[0], r[1])
}

// drawMarker draws the marker in hotspot i, offset like the game: by half
// the difference between the class box (hotspot 3) and the marker size
// (0x0042FA98), which centres it in every 20×20 box it marks.
func (s *newHeroScreen) drawMarker(dst *ebiten.Image, i int) {
	if i < 0 || i >= gamestate.NewHeroHotspots {
		return
	}
	m := s.frames[newhMarker]
	c, r := rect800(gamestate.NewHeroClass), rect800(i)
	dx := (c[2] - c[0] - m.Bounds().Dx()) / 2
	dy := (c[3] - c[1] - m.Bounds().Dy()) / 2
	drawAt(dst, m, r[0]+dx, r[1]+dy)
}

// drawPreview draws the hero in the stance with the preview gear, its body
// image top-left at hotspot 22 + (60, 20), clipped to 160×160.
func (s *newHeroScreen) drawPreview(dst *ebiten.Image) {
	e := s.hero
	key := previewKey{e.Model, e.PaletteOffset, e.WeaponMode, e.Type, e.Variant, e.Equipment}
	if s.preview == nil || key != s.previewKey {
		s.previewKey = key
		s.preview = nil
		spec := e.ModelSpec()
		body, err := resfmt.LoadEntitySprite(nil, s.graph, spec)
		if err != nil {
			return
		}
		img, err := resfmt.ComposeEquipment(s.graph, body, spec, s.items.Overlays(e, s.tables), s.tables)
		if err != nil {
			return
		}
		s.preview = toEbiten(img.Image)
		s.previewDX, s.previewDY = img.ImageDX-body.ImageDX, img.ImageDY-body.ImageDY
	}
	if s.preview == nil {
		return
	}
	r := rect800(gamestate.NewHeroPreview)
	clip := dst.SubImage(image.Rect(r[0], r[1], r[0]+previewSize, r[1]+previewSize)).(*ebiten.Image)
	drawAt(clip, s.preview, r[0]+previewBodyX+s.previewDX, r[1]+previewBodyY+s.previewDY)
}
