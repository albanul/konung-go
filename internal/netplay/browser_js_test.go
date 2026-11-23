//go:build js

package netplay

import (
	"context"
	"os"
	"testing"
	"time"
)

// The browser build's transport against a running server (KONUNG_TEST_SERVER
// = its "host:port"; skipped without it): run under Node, which has the
// browser's WebSocket and fetch:
//
//	go run ./cmd/konung-server -data /tmp/konung-data &   (any data folder: the session runs without the world's files)
//	PATH=$PATH:$(go env GOROOT)/lib/wasm KONUNG_TEST_SERVER=127.0.0.1:7777 GOOS=js GOARCH=wasm go test -run Browser ./internal/netplay
func TestBrowserTransport(t *testing.T) {
	addr := os.Getenv("KONUNG_TEST_SERVER")
	if addr == "" {
		t.Skip("KONUNG_TEST_SERVER not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	found, err := Discover(ctx, []string{addr})
	if err != nil || len(found) != 1 || found[0].Addr != addr {
		t.Fatalf("Discover = %+v, %v", found, err)
	}
	c, err := Dial(addr, JoinInfo{Name: "Ivar", Model: 1}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Chat("привет из браузера"); err != nil {
		t.Fatal(err)
	}
	timeout := time.After(3 * time.Second)
	for {
		select {
		case m, ok := <-c.Incoming():
			if !ok {
				t.Fatal("connection closed")
			}
			var chat ChatPayload
			if m.Type == MsgChat && m.Decode(&chat) == nil {
				if chat.Text != "привет из браузера" || chat.Name != "Ivar" {
					t.Errorf("chat = %+v", chat)
				}
				return
			}
		case <-timeout:
			t.Fatal("no chat back")
		}
	}
}
