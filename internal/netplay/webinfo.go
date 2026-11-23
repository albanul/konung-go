package netplay

import (
	"context"
	"encoding/json"
	"net"
	"runtime"
	"strconv"
	"sync"
)

// DiscoverHTTP asks every target ("host:port", the game port) for its
// session over HTTP (GET /info, websocket.go): the browser's discovery,
// where UDP is impossible. Servers that answer before ctx ends are
// returned, each target once, in target order.
func DiscoverHTTP(ctx context.Context, targets []string) []Found {
	found := make([]*Found, len(targets))
	var wg sync.WaitGroup
	for k, t := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, err := getInfo(ctx, infoURL(t))
			var info SessionInfo
			if err != nil || json.Unmarshal(body, &info) != nil {
				return
			}
			found[k] = &Found{Addr: t, Session: info}
		}()
	}
	wg.Wait()
	var out []Found
	seen := map[string]bool{}
	for _, f := range found {
		if f != nil && !seen[f.Addr] {
			seen[f.Addr] = true
			out = append(out, *f)
		}
	}
	return out
}

// browser reports the browser build, which discovers over HTTP.
const browser = runtime.GOOS == "js"

// defaultTargets are the browser's discovery targets without a typed
// address: the page's host on the game port.
func defaultTargets() []string {
	if h := PageHost(); h != "" {
		return []string{net.JoinHostPort(h, strconv.Itoa(DefaultPort))}
	}
	return nil
}
