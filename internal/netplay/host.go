package netplay

import (
	"fmt"
	"net"
	"strconv"
)

// Host starts a session in this process, the way the original LAN exe hosts
// (server mode [0x00A83398] with the host's own player, 0x00430803): a server
// on port (0 = any free port), discovery on the same UDP port, and the local
// player joined over loopback. Close the client, then the server.
func Host(port int, cfg ServerConfig, j JoinInfo, opts Options) (*Server, *Client, error) {
	s, err := Listen(":"+strconv.Itoa(port), cfg)
	if err != nil {
		return nil, nil, err
	}
	p := s.Addr().(*net.TCPAddr).Port
	if err := s.ServeDiscovery(":" + strconv.Itoa(p)); err != nil {
		s.Close()
		return nil, nil, err
	}
	c, err := Dial(net.JoinHostPort("127.0.0.1", strconv.Itoa(p)), j, opts)
	if err != nil {
		s.Close()
		return nil, nil, fmt.Errorf("netplay: host: join own server: %w", err)
	}
	return s, c, nil
}
