//go:build !js

package netplay

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// dialServer opens the connection to a server: TCP to "host:port", or a
// WebSocket for a "ws://" or "wss://" URL (the browser build's transport,
// dial_js.go; natively for tests and tools).
func dialServer(addr string) (net.Conn, error) {
	if strings.HasPrefix(addr, "ws://") || strings.HasPrefix(addr, "wss://") {
		return dialWebSocket(addr)
	}
	return net.DialTimeout("tcp", addr, dialTimeout)
}

// dialTimeout bounds connecting.
const dialTimeout = 5 * time.Second

// dialWebSocket connects to the WebSocket URL u; the connection carries
// the JSON lines as text messages.
func dialWebSocket(u string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, u, nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(MaxMessageSize + 1)
	return websocket.NetConn(context.Background(), ws, websocket.MessageText), nil
}

// infoURL is target's session info address (websocket.go).
func infoURL(target string) string { return "http://" + target + "/info" }

// PageHost is "" natively (dial_js.go: the page's host in the browser).
func PageHost() string { return "" }

// getInfo fetches a session info document (GET, status 200).
func getInfo(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", u, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<16))
}
