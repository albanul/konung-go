package game

import (
	"image"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/resfmt"
)

// talkWindow is window 1 (the dialogue, 0x0042DC50) and window 2 (the
// quest log «Информация к размышлению», 0x0042DEDB). Both draw INTR slot 1
// as a frame over the game view, with the background tiled first when
// background tiling is on (0x0043141C). The rules are in gamestate
// (dialogue.go); this is the drawing and the hit test (0x0043969C).
type talkWindow struct {
	layout   resfmt.DialogueLayout
	scroll   [2]image.Rectangle
	small    *gameFont // 0x005A31F4
	hi       *gameFont // 0x005A2EB0: the reply under the mouse
	d        *gamestate.Dialogue
	log      bool // window 2 open
	logFirst int  // 0x00A3E7B4: log entries scrolled past
	logCount int  // 0x00A3E7C4: entries the last draw counted
	greeting int  // 0x00A3E7C0: last greeting sound
	replies  []image.Rectangle
	q        *resfmt.Quests
}

// Quest log button (hotspot 6, «Информация к размышлению (Q)»): the toggle
// 0x0042459C presses button 5 with image 12.
const (
	hudLogButton = 5
	hudLogImage  = 12
)

func newTalkWindow(t *resfmt.ExeTables, q *resfmt.Quests, small *gameFont) *talkWindow {
	w := &talkWindow{layout: t.DialogueLayout(1), scroll: t.QuestLogScroll(1), small: small, hi: small, q: q}
	if f, err := t.Font(resfmt.FontHighlightVA); err == nil {
		w.hi = newGameFont(f)
	}
	return w
}

// open reports whether a window that hides the world's characters is open.
func (w *talkWindow) open() bool { return w.d != nil || w.log }

// toggleLog is 0x0042459C (Q, HUD button 5): only with no window or the
// log open; opening resets the scroll.
func (w *talkWindow) toggleLog() {
	w.log = !w.log
	w.logFirst = 0
}

// show sets the open dialogue (nil closes it) and lays out the reply
// boxes the way the draw stores them (list entries +0x10..+0x1C).
func (w *talkWindow) show(d *gamestate.Dialogue) {
	w.d, w.replies = d, nil
	if d == nil {
		return
	}
	l := &w.layout
	y := l.Replies.Y
	for _, r := range d.Replies {
		top := y
		y += len(w.small.font.Wrap(w.q.Text(r.Text), l.Width))*resfmt.LineHeight + 6
		w.replies = append(w.replies, image.Rect(l.Replies.X, top, l.Replies.X+l.Width, y))
	}
}

// replyAt is the hit test 0x0043969C: the reply whose box (left of the
// text to left + width, from its top to its last line + 6) strictly
// contains the point; −1 if none.
func (w *talkWindow) replyAt(x, y int) int {
	if w.d == nil {
		return -1
	}
	for k, r := range w.replies {
		if x > r.Min.X && x < r.Max.X && y > r.Min.Y && y < r.Max.Y {
			return k
		}
	}
	return -1
}

// scrollLog handles a click on the log's scroll areas (0x004398C4 on
// 0x0045B0AC → hotspot k<<8 | 0xCA, 0x00422A83): area 1 scrolls down while
// entries remain, area 0 up.
func (w *talkWindow) scrollLog(x, y int) {
	p := image.Pt(x, y)
	switch {
	case p.In(w.scroll[1]):
		if w.logCount-1 > w.logFirst {
			w.logFirst++
		}
	case p.In(w.scroll[0]):
		if w.logFirst > 0 {
			w.logFirst--
		}
	}
}

// draw draws the open window. tiled is the background tiling setting;
// stripTop is the bag strip's top (0x00A3E88C), below which the log stops.
func (w *talkWindow) draw(dst *ebiten.Image, h *hud, g *gamestate.Game, tiled bool, stripTop int) {
	q := w.q
	if !w.open() {
		return
	}
	l := &w.layout
	if tiled {
		tileBackground(dst, h, viewLeft, viewTop, viewRight, viewBottom)
	}
	h.draw(dst, resfmt.DialogueFrameSlot, l.Frame.X, l.Frame.Y)
	if w.d == nil {
		w.drawLog(dst, q, g, stripTop)
		return
	}
	h.draw(dst, resfmt.DialogueDividerSlot, l.Divider.X, l.Divider.Y)
	y := l.Lines.Y
	for _, id := range w.d.Lines {
		y = w.small.drawWrappedY(dst, q.Text(id), l.Lines.X, y, l.Width) + 6
	}
	mx, my := ebiten.CursorPosition()
	hover := w.replyAt(mx, my)
	for k, r := range w.d.Replies {
		f := w.small
		if k == hover {
			f = w.hi
		}
		f.drawWrappedY(dst, q.Text(r.Text), l.Replies.X, w.replies[k].Min.Y, l.Width)
	}
}

// drawLog is window 2: the text of every set quest flag that has one,
// skipping the first logFirst, 3 px apart, until 45 px above the strip.
func (w *talkWindow) drawLog(dst *ebiten.Image, q *resfmt.Quests, g *gamestate.Game, stripTop int) {
	l := &w.layout
	entries := g.QuestLog()
	w.logCount = 0
	y := l.Lines.Y
	for _, id := range entries {
		w.logCount++
		if w.logCount <= w.logFirst {
			continue
		}
		y = w.small.drawWrappedY(dst, q.Text(id), l.Lines.X, y, l.Width) + 3
		if y >= stripTop-45 {
			break
		}
	}
}

// canTalk is the right-click test of 0x004238EE for the character under
// the cursor: it has a dialogue (+0xD0 ≠ 0xFF) and QUESTR.RES is loaded;
// a party member needs Ctrl held (0x00A3E714, 0x00423A0F); anyone else
// needs Shift up (0x00A3E754) and the hero out of the stance (+3 bit 4,
// 0x00423B51). The port also skips the dead.
func (s *startScene) canTalk(hero gamestate.Entity) bool {
	if s.hovered < 0 || s.assets == nil || s.assets.quests == nil {
		return false
	}
	e := s.entity(s.hovered)
	if e.Dialogue == gamestate.NoDialogue || e.Dead() {
		return false
	}
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl)
	if e.Group == 0 {
		return ctrl
	}
	return !ebiten.IsKeyPressed(ebiten.KeyShift) && !hero.InStance()
}

// newTalk writes the scene's party back into the game (the scripts work on
// the global records, like the game) and returns the script context.
func (s *startScene) newTalk() *gamestate.Talk {
	s.storeWorld()
	return &gamestate.Talk{
		G: s.game, Q: s.assets.quests, T: s.tables,
		Hero: s.party[0].Index, Map: s.mapNumber, Time: s.gameTime(),
		Rand: s.rng.IntN, Drop: s.dropAtHero,
	}
}

// talkTo opens the dialogue of entity i (0x004364F0, then the window
// 0x0042DBBC(0)); the greeting plays when no line has speech.
func (s *startScene) talkTo(i int) {
	s.hand.Return(&s.party[0])
	tk := s.newTalk()
	d := tk.OpenDialogue(i, &s.talk.greeting)
	s.afterTalk(tk, d)
	if d != nil && tk.Events.Greeting != 0 {
		sfx.play(tk.Events.Greeting, 0, 0)
	}
}

// chooseReply is a click on reply k (0x00423501 → 0x0041F18C).
func (s *startScene) chooseReply(k int) {
	d := s.talk.d
	tk := s.newTalk()
	tk.NPC = d.NPC
	if !tk.Choose(d, k) {
		d = nil
	}
	s.afterTalk(tk, d)
}

// afterTalk takes the scripts' changes back into the scene: the party
// copies, a rebuilt scene when someone joined or left the party, and the
// window.
func (s *startScene) afterTalk(tk *gamestate.Talk, d *gamestate.Dialogue) {
	if len(tk.Events.Unported) > 0 {
		log.Printf("dialogue: script functions not ported: %v", tk.Events.Unported)
	}
	if tk.Events.PartyChanged {
		s.rebuild()
	} else {
		for k := range s.party {
			s.party[k] = s.game.Entities[s.party[k].Index]
		}
	}
	if tk.Events.Trade != nil {
		d = nil // the trade replies end the dialogue; window 7 replaces it
		s.trade.tr = tk.Events.Trade
	}
	s.talk.show(d)
}

// dropAtHero puts an item on the ground at the hero's cell (a full bag,
// 0x004359D4 → 0x004138C8).
func (s *startScene) dropAtHero(item int) {
	held := s.hand.Item
	s.hand.Item = item
	s.dropHeld(&s.party[0])
	s.hand.Item = held
}

// rebuild rebuilds the scene for the same map after the party changed
// (hire 291, dismiss 332, posts 345–348, removal 363): the scripts move
// entity records between groups, so every walker is made anew from the
// records, as a loaded save is (restore mode: no respawn, alarms kept).
func (s *startScene) rebuild() {
	st := s.saveState()
	st.Active = nil // the piles stay in s.game; keep their active list
	ns, err := s.assets.buildScene(s.game, s.mapNumber, s, &st)
	if err != nil {
		log.Printf("rebuild KONUNG.%d: %v", s.mapNumber, err)
		return
	}
	ns.pileAt = s.pileAt
	ns.camX, ns.camY, ns.follow = s.camX, s.camY, s.follow
	*s = *ns
}
