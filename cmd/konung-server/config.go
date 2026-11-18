package main

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
)

// settings are the server's settings. They come, each later source
// overriding the earlier, from the defaults, the LAN version's SERVER file
// (name and session length), the config file (server.conf: every setting,
// see konung-server.conf.example) and the command-line flags.
type settings struct {
	Name       string // session name, shown in the game's server list
	Port       int    // TCP game port; UDP on the same port answers LAN discovery
	MaxPlayers int    // players at once
	Minutes    int    // rating session length
	Speed      int    // world speed: 0 normal, 1 double, 2 triple
	TeamDamage bool   // heroes of one team (hero type) can hurt each other
}

func defaultSettings() settings {
	return settings{
		Name:       resfmt.CP866ToUTF8(gamestate.DefaultServerName),
		Port:       netplay.DefaultPort,
		MaxPlayers: 16, // the original's limit
		Minutes:    180,
		TeamDamage: true, // the original has no team rule
	}
}

// applyServerFile takes the session name and length from a SERVER file
// (0x00431481).
func (s *settings) applyServerFile(data []byte) error {
	f, err := gamestate.ParseServerFile(data)
	if err != nil {
		return err
	}
	s.Name, s.Minutes = resfmt.CP866ToUTF8(f.Name), f.Minutes
	return nil
}

// configKeys are the config file's keys and how each is set.
var configKeys = map[string]func(s *settings, v string) error{
	"name": func(s *settings, v string) error {
		if v == "" {
			return fmt.Errorf("empty")
		}
		s.Name = v
		return nil
	},
	"port":        intSetting(func(s *settings) *int { return &s.Port }, 1, 65535),
	"max_players": intSetting(func(s *settings) *int { return &s.MaxPlayers }, 1, 16),
	"minutes":     intSetting(func(s *settings) *int { return &s.Minutes }, 1, 100000),
	"speed":       intSetting(func(s *settings) *int { return &s.Speed }, 0, netplay.MaxSpeed),
	"team_damage": func(s *settings, v string) error {
		switch strings.ToLower(v) {
		case "true", "yes", "on", "1":
			s.TeamDamage = true
		case "false", "no", "off", "0":
			s.TeamDamage = false
		default:
			return fmt.Errorf("%q is not true or false", v)
		}
		return nil
	},
}

func intSetting(field func(*settings) *int, lo, hi int) func(*settings, string) error {
	return func(s *settings, v string) error {
		n, err := strconv.Atoi(v)
		if err != nil || n < lo || n > hi {
			return fmt.Errorf("%q is not a number from %d to %d", v, lo, hi)
		}
		*field(s) = n
		return nil
	}
}

// applyConfig reads a config file: "key = value" lines (UTF-8), blank lines
// and "#" comments; the value may be quoted. An unknown key or a bad value
// is an error, so a typo does not go unnoticed.
func (s *settings) applyConfig(data []byte) error {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("line %d: want key = value", n)
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if u, err := strconv.Unquote(value); err == nil {
			value = u
		}
		set, ok := configKeys[key]
		if !ok {
			return fmt.Errorf("line %d: unknown setting %q", n, key)
		}
		if err := set(s, value); err != nil {
			return fmt.Errorf("line %d: %s: %v", n, key, err)
		}
	}
	return sc.Err()
}
