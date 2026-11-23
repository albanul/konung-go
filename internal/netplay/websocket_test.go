package netplay

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The browsers' transport on the game port: GET /info answers the session
// (readable from any page), a client joins over WebSocket and plays with a
// TCP client on the same server, and DiscoverHTTP finds the server.
func TestWebSocketAndInfo(t *testing.T) {
	s := startServer(t, ServerConfig{Name: "Тест"})
	addr := s.Addr().String()

	resp, err := http.Get("http://" + addr + "/info")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("/info: %d, CORS %q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	found := DiscoverHTTP(ctx, []string{addr, "127.0.0.1:1"})
	if len(found) != 1 || found[0].Addr != addr || found[0].Session.Name != "Тест" {
		t.Fatalf("DiscoverHTTP = %+v", found)
	}

	web, err := Dial("ws://"+addr+"/", JoinInfo{Name: "Ivar", Model: 1}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer web.Close()
	tcp := dial(t, s, "Olaf", 1)
	if err := web.ChatTeam("за мной"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*Client{web, tcp} {
		var chat ChatPayload
		if waitFor(t, c.Conn, MsgChat).Decode(&chat); chat.Text != "за мной" || chat.Name != "Ivar" {
			t.Errorf("chat = %+v", chat)
		}
	}
	if n := len(s.Session().Players); n != 2 {
		t.Errorf("%d players, want the WebSocket and the TCP one", n)
	}
	// Leaving over WebSocket is seen.
	web.Close()
	var left PlayerLeftPayload
	if waitFor(t, tcp.Conn, MsgPlayerLeft).Decode(&left); left.ID != web.PlayerID {
		t.Errorf("player-left = %q", left.ID)
	}
}

// Close ends WebSocket players too.
func TestWebSocketServerClose(t *testing.T) {
	s, err := Listen("127.0.0.1:0", ServerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	web, err := Dial("ws://"+s.Addr().String()+"/", JoinInfo{Name: "Ivar", Model: 1}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	go func() { s.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(wait):
		t.Fatal("Close hung")
	}
	waitClosed(t, web.Conn)
}

// With the browser build served (ServeFiles), the game port gives the
// page and its files (.wasm as application/wasm, which browsers require),
// /info, and the WebSocket session, all at once.
func TestServeFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<title>Князь</title>"), 0o644)
	os.WriteFile(filepath.Join(dir, "konung.wasm"), []byte("\x00asm"), 0o644)
	s := startServer(t, ServerConfig{Name: "Тест"})
	s.ServeFiles(http.FileServer(http.Dir(dir)))
	addr := s.Addr().String()
	get := func(path string) (*http.Response, string) {
		t.Helper()
		resp, err := http.Get("http://" + addr + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	if resp, body := get("/"); resp.StatusCode != http.StatusOK || !strings.Contains(body, "Князь") {
		t.Errorf("/: %d %q", resp.StatusCode, body)
	}
	if resp, _ := get("/konung.wasm"); resp.Header.Get("Content-Type") != "application/wasm" {
		t.Errorf("konung.wasm served as %q", resp.Header.Get("Content-Type"))
	}
	if _, body := get("/info"); !strings.Contains(body, "Тест") {
		t.Errorf("/info: %q", body)
	}
	web, err := Dial("ws://"+addr+"/", JoinInfo{Name: "Ivar", Model: 1}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	web.Close()
}
