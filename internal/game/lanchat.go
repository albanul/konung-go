package game

import (
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
)

// The chat in a LAN world (docs/konung_lan.md#chat; LAN_KONG.EXE): the
// last three lines at the view's top left, and a typed line sent to
// everyone or to the own team.

// Chat input modes (the original's window states 4 and 5, 0x00A6EAE4).
const (
	chatOff  = 0
	chatAll  = 4 // to everyone (sent with flag 0x80)
	chatTeam = 5 // to the own nation
)

// chatPrompt starts the typed line (0x00470344 «>_»: the prompt and the
// cursor).
const chatPrompt = '>'

// chatState is the view's chat: the shown lines and the typed line. It
// lives in the view, so a map change keeps it.
type chatState struct {
	log   gamestate.LANChat
	mode  int
	field gamestate.TextField
}

// chatTyping reports whether the chat line takes the keyboard (the game's
// keys are off meanwhile).
func (s *startScene) chatTyping() bool {
	return s != nil && s.view != nil && s.view.chats.mode != chatOff
}

// applyChat shows a chat line from the server (0x0042240C): «name : text»
// (0x0047032A « : »), a server's own text alone; a line to everyone in the
// small font, a team line in the highlight font.
func (s *startScene) applyChat(p netplay.ChatPayload) {
	c := &s.view.chats
	switch {
	case p.Clear: // a session starts (0x00422459)
		c.log.Clear()
		return
	case p.End:
		// The session is over (0x0042257F): the chat holds this line
		// alone, the end sound plays, and the client's windows close
		// (0x00410384; the server closes the dialogue and trade itself).
		c.log.Clear()
		c.log.Add(cp866(p.Text), true)
		c.mode = chatOff
		s.inv.open, s.inv.quick, s.wmap.open, s.talk.log = false, false, false, false
		if s.Hero != nil {
			s.playAt(gamestate.SessionEndSound, s.Hero.Cell)
		}
		return
	}
	text := p.Text
	if !p.System {
		text = p.Name + " : " + p.Text
	}
	c.log.Add(cp866(text), !p.Team)
}

// updateChat is the chat's keyboard (port keys: the original opens the
// line with two buttons of its LAN HUD, which the port does not have):
// with no window open, Enter opens a line to everyone, Shift + Enter one to
// the own team; while
// typing, the characters go to the line (0x0042CB94 with the 0x0042CC20
// limits: FieldChat bytes and the view's width), Backspace deletes, Enter
// sends and closes (0x0042CCF6), Escape closes (0x0042CDC6), and a full
// line is sent and a new one opened (0x0042CECF).
func (s *startScene) updateChat() {
	c := &s.view.chats
	if s.windowOpen() {
		// Only while playing (a port rule: the line is drawn with the
		// world, under the windows): a window opened by a click closes
		// the line, and Enter over a window does nothing.
		c.mode = chatOff
		return
	}
	enter := inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter)
	if c.mode == chatOff {
		if enter {
			c.mode = chatAll
			if ebiten.IsKeyPressed(ebiten.KeyShift) {
				c.mode = chatTeam
			}
			c.field = gamestate.TextField{Max: gamestate.FieldChat}
		}
		return
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		c.mode = chatOff
		return
	case enter:
		s.sendChat()
		c.mode = chatOff
		return
	}
	for _, r := range ebiten.AppendInputChars(nil) {
		b, ok := resfmt.RuneToCP866(r)
		if !ok {
			continue
		}
		if !c.field.Type(b, s.chatWidth, viewRight-viewLeft-10) {
			s.sendChat() // full: sent, and a new line opens (the character is lost, as in the original)
			c.field.Text = nil
			break
		}
	}
	if repeatKey(ebiten.KeyBackspace) {
		c.field.Backspace()
	}
}

// chatWidth is the typed line's width as drawn: the prompt, the text and
// the cursor.
func (s *startScene) chatWidth(text []byte) int {
	if s.font == nil {
		return 0
	}
	return s.font.width(slices.Concat([]byte{chatPrompt}, text, []byte{'_'}))
}

// sendChat sends the typed line (0x00419670), if there is one.
func (s *startScene) sendChat() {
	c := &s.view.chats
	text := resfmt.CP866ToUTF8(c.field.Text)
	if text != "" && s.view.chat != nil {
		s.view.chat(text, c.mode == chatTeam)
	}
	c.field.Text = nil
}

// drawLANChat draws the chat lines at the view's top left, 15 px apart
// (0x0041F043: x = view left + 10, y = view top + 15·k), and the typed
// line 17 px above the bag strip, or the view's bottom without one
// (0x0041F0D0); the cursor blinks (0x9EBA00 bit 2; its period is the
// port's).
func (s *startScene) drawLANChat(dst *ebiten.Image) {
	if s.font == nil || s.view == nil {
		return
	}
	c := &s.view.chats
	for k, l := range c.log.Lines {
		f := s.chatFont(l.All)
		f.drawLine(dst, l.Text, viewLeft+10, viewTop+15*k)
	}
	if c.mode == chatOff {
		return
	}
	y := viewBottom
	if s.inv.showStrip() {
		y = s.inv.strip.y
	}
	line := slices.Concat([]byte{chatPrompt}, c.field.Text)
	if s.frame/30%2 == 0 {
		line = append(line, '_')
	}
	s.chatFont(c.mode == chatAll).drawLine(dst, line, viewLeft+10, y-17)
}

// chatFont is the small font for a line to everyone (0x0078C1A0), the
// highlight font for a team line (0x0079295E).
func (s *startScene) chatFont(all bool) *gameFont {
	if !all && s.talk.hi != nil {
		return s.talk.hi
	}
	return s.font
}
