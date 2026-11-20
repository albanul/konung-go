package game

import (
	"bytes"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
)

// Chat lines from the server: «name : text», a server's text alone; a
// team line in the highlight font. The typed line is sent as UTF-8 with
// its team flag; while typing, the game's keys are off. Drawing does not
// fail with the line open.
func TestLANChatView(t *testing.T) {
	s := viewScene(t)
	s.applyChat(netplay.ChatPayload{Name: "Olaf", Text: "вперед"})
	s.applyChat(netplay.ChatPayload{Name: "Ivar", Text: "за мной", Team: true})
	s.applyChat(netplay.ChatPayload{Text: "ИГРА ОКОНЧЕНА! ", System: true})
	lines := s.view.chats.log.Lines
	want := []gamestate.LANChatLine{
		{Text: cp866("Olaf : вперед"), All: true},
		{Text: cp866("Ivar : за мной"), All: false},
		{Text: cp866("ИГРА ОКОНЧЕНА! "), All: true},
	}
	if len(lines) != len(want) {
		t.Fatalf("%d lines", len(lines))
	}
	for k := range want {
		if !bytes.Equal(lines[k].Text, want[k].Text) || lines[k].All != want[k].All {
			t.Errorf("line %d: %q all %v", k, lines[k].Text, lines[k].All)
		}
	}
	if s.chatFont(false) == s.chatFont(true) {
		t.Error("a team line in the small font")
	}

	var sent []string
	var team []bool
	s.view.chat = func(text string, tm bool) { sent, team = append(sent, text), append(team, tm) }
	if s.chatTyping() || s.keyJust(ebiten.KeyA) != false {
		t.Fatal("typing before Enter")
	}
	s.view.chats.mode = chatTeam
	s.view.chats.field = gamestate.TextField{Max: gamestate.FieldChat, Text: cp866("держитесь")}
	if !s.chatTyping() || s.keyHeld(ebiten.KeyArrowLeft) {
		t.Fatal("the game's keys are on while typing")
	}
	img := ebiten.NewImage(ScreenWidth, ScreenHeight)
	s.drawLANStatus(img)
	s.sendChat()
	if len(sent) != 1 || sent[0] != "держитесь" || !team[0] || len(s.view.chats.field.Text) != 0 {
		t.Fatalf("sent %q team %v", sent, team)
	}
	// A window opening (a click on the map button) closes the line.
	s.view.chats.mode = chatAll
	s.wmap.open = true
	s.updateChat()
	if s.chatTyping() {
		t.Fatal("the chat line stays open under the map window")
	}
	s.wmap.open = false
	s.sendChat() // nothing typed: nothing sent
	if len(sent) != 1 {
		t.Fatal("an empty line was sent")
	}
	// The width limit leaves the prompt, the cursor and the margins inside
	// the view.
	f := gamestate.TextField{Max: gamestate.FieldChat}
	for f.Type('W', s.chatWidth, viewRight-viewLeft-10) {
	}
	if w := s.chatWidth(f.Text); w+15 >= viewRight-viewLeft-10 || len(f.Text)+1 > gamestate.FieldChat {
		t.Errorf("full line: %d bytes, %d px", len(f.Text), w)
	}
}

// Chat lines reach the world's queue once the world is entered, not
// before (the world's chat starts empty); the own line comes back too.
func TestLANChatQueue(t *testing.T) {
	srv, err := netplay.Listen("127.0.0.1:0", netplay.ServerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	c, err := netplay.Dial(srv.Addr().String(), netplay.JoinInfo{Name: "Ivar", Model: 1}, netplay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	u := &lanUI{client: c, state: lanWaiting}
	chats := func() (n int) {
		for _, m := range u.worldMsgs {
			if m.Type == netplay.MsgChat {
				n++
			}
		}
		return n
	}
	read := func(want int) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for chats() < want && time.Now().Before(deadline) {
			u.readSession()
			time.Sleep(5 * time.Millisecond)
		}
		u.readSession()
	}
	c.Chat("в лобби")
	time.Sleep(50 * time.Millisecond)
	read(0)
	if chats() != 0 {
		t.Fatal("a chat line before the world was queued")
	}
	u.start = &netplay.EnterPayload{}
	if at, _, _ := u.takeStart(); at == nil || !u.inWorld {
		t.Fatal("takeStart did not enter the world")
	}
	c.Chat("в мире")
	read(1)
	if chats() != 1 {
		t.Fatalf("%d chat lines queued in the world, want the own line back", chats())
	}
}

// The session's end line: the chat holds it alone, the typed line and the
// client's windows close; the start's clear empties the chat.
func TestLANSessionEndChat(t *testing.T) {
	s := viewScene(t)
	s.applyChat(netplay.ChatPayload{Name: "Olaf", Text: "вперед"})
	s.view.chats.mode = chatAll
	s.inv.open, s.wmap.open = true, true
	end := "ИГРА ОКОНЧЕНА! Победа викингов!"
	s.applyChat(netplay.ChatPayload{Text: end, System: true, End: true})
	lines := s.view.chats.log.Lines
	if len(lines) != 1 || !bytes.Equal(lines[0].Text, cp866(end)) || !lines[0].All {
		t.Fatalf("lines after the end: %+v", lines)
	}
	if s.chatTyping() || s.inv.open || s.wmap.open {
		t.Error("the typed line or a window stayed open")
	}
	s.applyChat(netplay.ChatPayload{System: true, Clear: true})
	if len(s.view.chats.log.Lines) != 0 {
		t.Error("the session's start did not clear the chat")
	}
}
