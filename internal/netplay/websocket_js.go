//go:build js

package netplay

import "net"

// The browser build only connects (dial_js.go); a server there takes the
// JSON-line protocol alone.

type webState struct{}

func (s *Server) sniff(c net.Conn) {
	defer s.wg.Done()
	s.accept(c)
}

func (s *Server) closeWeb() {}
