//go:build js

package netplay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall/js"
	"time"

	"github.com/coder/websocket"
)

// dialServer opens the connection to a server from the browser: always a
// WebSocket (websocket.go on the server), "ws://host:port/", or "wss://"
// when the page itself came over HTTPS (browsers refuse ws:// there).
func dialServer(addr string) (net.Conn, error) {
	u := addr
	if !strings.Contains(u, "://") {
		u = pageScheme("ws") + "://" + addr + "/"
	}
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, u, nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(MaxMessageSize + 1)
	return websocket.NetConn(context.Background(), ws, websocket.MessageText), nil
}

// dialTimeout bounds connecting.
const dialTimeout = 5 * time.Second

// pageScheme is base ("ws" or "http") with an "s" when the page came over
// HTTPS.
func pageScheme(base string) string {
	if loc := js.Global().Get("location"); loc.Truthy() && loc.Get("protocol").String() == "https:" {
		return base + "s"
	}
	return base
}

// PageHost is the host the page came from ("" if none): the browser's
// default server, the dedicated server serving its own page or one beside
// the page's host.
func PageHost() string {
	if loc := js.Global().Get("location"); loc.Truthy() {
		return loc.Get("hostname").String()
	}
	return "" // not in a page (Node)
}

// infoURL is target's session info address (websocket.go on the server).
func infoURL(target string) string { return pageScheme("http") + "://" + target + "/info" }

// getInfo fetches a session info document with the browser's fetch (Go's
// net/http would also use it in a browser, but not under Node, where the
// browser build is tested; and it keeps net/http out of the WASM build).
func getInfo(ctx context.Context, u string) ([]byte, error) {
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	var then, fail js.Func
	release := func() { then.Release(); fail.Release() }
	fail = js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "fetch failed"
		if len(args) > 0 {
			msg = args[0].Call("toString").String()
		}
		done <- result{err: errors.New(msg)}
		return nil
	})
	then = js.FuncOf(func(_ js.Value, args []js.Value) any {
		resp := args[0]
		if !resp.Get("ok").Bool() {
			done <- result{err: fmt.Errorf("%s: status %d", u, resp.Get("status").Int())}
			return nil
		}
		var gotText js.Func
		gotText = js.FuncOf(func(_ js.Value, a []js.Value) any {
			done <- result{text: a[0].String()}
			gotText.Release()
			return nil
		})
		resp.Call("text").Call("then", gotText, fail)
		return nil
	})
	js.Global().Call("fetch", u).Call("then", then, fail)
	select {
	case r := <-done:
		go func() { release() }()
		return []byte(r.text), r.err
	case <-ctx.Done():
		return nil, ctx.Err() // the callbacks stay until the fetch settles
	}
}
