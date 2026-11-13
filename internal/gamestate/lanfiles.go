package gamestate

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// LAN version client settings, LAN.CFG in the game folder (LAN_KONG.EXE:
// read 0x00431176, written 0x00432898 after every login-screen edit). Layout,
// little-endian, 486 bytes:
//
//	+0x000  7 dwords: [0] resolution index [0xA6EB2C], bit 31 set when
//	        «Сохранить пароль» is on ([0xA6EB0C]); [1..6] settings
//	        0xA6EAC4, 0xA6EAD8, 0xA6EAEC, 0xA6EAF4, 0xA6EB10, 0xA6EB18
//	+0x01C  name, 33 bytes (CP866, NUL-terminated)
//	+0x03D  password, 9 bytes; zero unless «Сохранить пароль» is on
//	+0x046  clan, 32 bytes
//	+0x066  TCP/IP address, modem name, phone number, 128 bytes each
const (
	LANConfigFile   = "LAN.CFG"
	LANConfigSize   = 7*4 + 33 + 9 + 32 + 3*128
	lanSavePassword = 0x80000000
)

// LANConfig is LAN.CFG. Settings keeps the dwords 1..6 unchanged.
type LANConfig struct {
	Resolution   uint32
	SavePassword bool
	Settings     [6]uint32
	Name         []byte // CP866, at most 32 bytes
	Password     []byte // at most 8 bytes
	Clan         []byte // at most 31 bytes
	Address      []byte // TCP/IP address typed on the login screen
	Modem, Phone []byte
}

// ParseLANConfig reads LAN.CFG; nil or short data gives an empty config
// (the exe keeps its defaults when the file is missing).
func ParseLANConfig(data []byte) LANConfig {
	var c LANConfig
	if len(data) < LANConfigSize {
		return c
	}
	d0 := binary.LittleEndian.Uint32(data)
	c.Resolution, c.SavePassword = d0&^lanSavePassword, d0&lanSavePassword != 0
	for i := range c.Settings {
		c.Settings[i] = binary.LittleEndian.Uint32(data[4+i*4:])
	}
	o := 28
	field := func(n int) []byte {
		b := cString(data[o : o+n])
		o += n
		return b
	}
	c.Name, c.Password, c.Clan = field(33), field(9), field(32)
	c.Address, c.Modem, c.Phone = field(128), field(128), field(128)
	if !c.SavePassword {
		c.Password = nil
	}
	return c
}

// Marshal writes LAN.CFG; the password is stored only with SavePassword.
func (c LANConfig) Marshal() []byte {
	out := make([]byte, LANConfigSize)
	d0 := c.Resolution &^ lanSavePassword
	if c.SavePassword {
		d0 |= lanSavePassword
	}
	binary.LittleEndian.PutUint32(out, d0)
	for i, v := range c.Settings {
		binary.LittleEndian.PutUint32(out[4+i*4:], v)
	}
	o := 28
	put := func(b []byte, n int) {
		copy(out[o:o+n-1], b) // keep the terminating NUL
		o += n
	}
	put(c.Name, 33)
	if c.SavePassword {
		put(c.Password, 9)
	} else {
		o += 9
	}
	put(c.Clan, 32)
	put(c.Address, 128)
	put(c.Modem, 128)
	put(c.Phone, 128)
	return out
}

func cString(b []byte) []byte {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return append([]byte(nil), b...)
}

// ServerFile is the LAN server's settings file SERVER (CP866 text, parsed by
// 0x00431481): "<mode> <teams> <minutes>" (mode TEAM: team play), the
// session name, then "map x y" lines up to a line of '=' (the shipped file
// has six; their meaning is not traced), then comments.
type ServerFile struct {
	Mode           string
	Teams, Minutes int
	Name           []byte // CP866
	Points         [][3]int
}

// DefaultServerName is the session name without a SERVER file:
// «Стандартный Сервер с Командной игрой» (0x00470719).
var DefaultServerName = []byte("\x91\xe2\xa0\xad\xa4\xa0\xe0\xe2\xad\xeb\xa9 \x91\xa5\xe0\xa2\xa5\xe0 \xe1 \x8a\xae\xac\xa0\xad\xa4\xad\xae\xa9 \xa8\xa3\xe0\xae\xa9")

// ParseServerFile reads SERVER.
func ParseServerFile(data []byte) (ServerFile, error) {
	var f ServerFile
	sc := bufio.NewScanner(bytes.NewReader(data))
	line := func() (string, bool) {
		if !sc.Scan() {
			return "", false
		}
		return strings.TrimRight(sc.Text(), " \r"), true
	}
	first, ok := line()
	if !ok {
		return f, fmt.Errorf("SERVER: empty")
	}
	if _, err := fmt.Sscanf(first, "%s %d %d", &f.Mode, &f.Teams, &f.Minutes); err != nil {
		return f, fmt.Errorf("SERVER: first line %q: %w", first, err)
	}
	name, ok := line()
	if !ok {
		return f, fmt.Errorf("SERVER: no session name")
	}
	f.Name = []byte(name)
	for {
		l, ok := line()
		if !ok || strings.HasPrefix(l, "=") {
			break
		}
		fs := strings.Fields(l)
		if len(fs) != 3 {
			return f, fmt.Errorf("SERVER: point line %q", l)
		}
		var p [3]int
		for i, s := range fs {
			v, err := strconv.Atoi(s)
			if err != nil {
				return f, fmt.Errorf("SERVER: point line %q: %w", l, err)
			}
			p[i] = v
		}
		f.Points = append(f.Points, p)
	}
	return f, nil
}
