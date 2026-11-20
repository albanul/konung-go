package netplay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

// LAN discovery replaces DirectPlay's EnumSessions (LAN_KONG.EXE 0x004300DF,
// the «Доступные сервера» list): a client sends discoveryQuery to UDP
// broadcast addresses; each server answers with its SessionInfo as JSON.
var discoveryQuery = []byte("KONUNG-DISCOVER 1")

// Found is a server answering discovery.
type Found struct {
	Addr    string // TCP "host:port" to Dial
	Session SessionInfo
}

type announcer struct {
	pc   net.PacketConn
	info func() SessionInfo
	wg   sync.WaitGroup
}

func listenAnnouncer(addr string, info func() SessionInfo) (*announcer, error) {
	pc, err := net.ListenPacket("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("netplay: discovery: %w", err)
	}
	a := &announcer{pc: pc, info: info}
	a.wg.Add(1)
	go a.loop()
	return a, nil
}

func (a *announcer) loop() {
	defer a.wg.Done()
	buf := make([]byte, 512)
	for {
		n, from, err := a.pc.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		if !bytes.Equal(buf[:n], discoveryQuery) {
			continue
		}
		reply, err := json.Marshal(a.info())
		if err != nil {
			continue
		}
		a.pc.WriteTo(reply, from)
	}
}

func (a *announcer) close() {
	a.pc.Close()
	a.wg.Wait()
}

// BroadcastAddrs lists the IPv4 broadcast address of every active,
// non-loopback interface, plus 255.255.255.255 (adapted from go-tetris
// GetLocalIP, which stopped at the first interface).
func BroadcastAddrs() []string {
	out := []string{"255.255.255.255"}
	seen := map[string]bool{out[0]: true}
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipn.IP.To4()
			if ip == nil || len(ipn.Mask) != net.IPv4len {
				continue
			}
			b := make(net.IP, net.IPv4len)
			for i := range b {
				b[i] = ip[i] | ^ipn.Mask[i]
			}
			if s := b.String(); !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// Discover asks every target ("host:port" UDP addresses; nil = the
// BroadcastAddrs on DefaultPort) for servers and collects the answers until
// ctx ends (give it a deadline, e.g. one second). Each server appears once.
func Discover(ctx context.Context, targets []string) ([]Found, error) {
	if targets == nil {
		for _, b := range BroadcastAddrs() {
			targets = append(targets, net.JoinHostPort(b, strconv.Itoa(DefaultPort)))
		}
	}
	pc, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, fmt.Errorf("netplay: discover: %w", err)
	}
	defer pc.Close()
	sent := 0
	for _, t := range targets {
		ua, err := net.ResolveUDPAddr("udp4", t)
		if err != nil {
			continue
		}
		if _, err := pc.WriteTo(discoveryQuery, ua); err == nil {
			sent++
		}
	}
	if sent == 0 {
		return nil, errors.New("netplay: discover: no target reachable")
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Second)
	}
	pc.SetReadDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { pc.SetReadDeadline(time.Now()) })
	defer stop()

	var found []Found
	seen := map[string]bool{}
	buf := make([]byte, 64*1024)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return found, nil
			}
			return found, fmt.Errorf("netplay: discover: %w", err)
		}
		var info SessionInfo
		if json.Unmarshal(buf[:n], &info) != nil || info.Port <= 0 {
			continue
		}
		host, _, _ := net.SplitHostPort(from.String())
		addr := net.JoinHostPort(host, strconv.Itoa(info.Port))
		if seen[addr] {
			continue
		}
		seen[addr] = true
		found = append(found, Found{Addr: addr, Session: info})
	}
}
