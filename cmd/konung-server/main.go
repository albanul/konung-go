// Command konung-server is the dedicated multiplayer server: it keeps the
// players' accounts and progress (SQLite, internal/lanstore) like the LAN
// version's server kept LANUSERS.DAT and SAVITEMS.DAT, and hosts the session
// players join from the game's «Сетевая игра» screens.
//
//	konung-server [-data DIR] [-config FILE] [-port 7777] [-max 16] [-name NAME] [-minutes 180] [-speed 0] [-team-damage=true]
//
// Settings come from DIR/server.conf (or -config FILE; every setting, see
// konung-server.conf.example), and the flags given override it.
//
// DIR holds konung.db, the LAN version's GAME.RES (the world: players start
// on their hero model's village map; without it nobody can enter),
// optionally its SERVER file (session name and length; the flags override
// it), and for fights between players konung.exe and GAME.0, GAME.1, GAME.2
// (the strike rules and the heroes' gear; without them heroes cannot hurt
// each other). TCP on the port carries the game;
// UDP on the same port answers LAN discovery. The server owns the world
// clock (time of day, world speed) for every client. See Containerfile for
// running it in a container.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/lanfight"
	"konung1.5/internal/lanstore"
	"konung1.5/internal/netplay"
)

func main() {
	dataDir := flag.String("data", ".", "folder for konung.db, the game files and server.conf")
	configPath := flag.String("config", "", "settings file (default: DATA/server.conf when it exists; see konung-server.conf.example)")
	d := defaultSettings()
	port := flag.Int("port", d.Port, "TCP game port; UDP on the same port answers LAN discovery")
	maxPlayers := flag.Int("max", d.MaxPlayers, "players at once (the original allowed 16)")
	name := flag.String("name", "", "session name (default: config, SERVER file line 2, else «Стандартный Сервер с Командной игрой»)")
	minutes := flag.Int("minutes", d.Minutes, "session length in minutes")
	speed := flag.Int("speed", d.Speed, "world speed for everyone: 0 normal, 1 double, 2 triple")
	teamDamage := flag.Bool("team-damage", d.TeamDamage, "heroes of one team (hero type) can hurt each other, as the original; false protects them")
	flag.Parse()

	st, err := loadSettings(*dataDir, *configPath)
	if err != nil {
		log.Fatal(err)
	}
	// Flags given on the command line override the files.
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "port":
			st.Port = *port
		case "max":
			st.MaxPlayers = *maxPlayers
		case "name":
			if *name != "" {
				st.Name = *name
			}
		case "minutes":
			st.Minutes = *minutes
		case "speed":
			st.Speed = *speed
		case "team-damage":
			st.TeamDamage = *teamDamage
		}
	})
	if err := run(*dataDir, st); err != nil {
		log.Fatal(err)
	}
}

// loadSettings is the defaults, then DIR/SERVER (the original's session
// name and length), then the config file (configPath, else DIR/server.conf
// when it exists).
func loadSettings(dataDir, configPath string) (settings, error) {
	st := defaultSettings()
	if data, err := os.ReadFile(filepath.Join(dataDir, "SERVER")); err == nil {
		if err := st.applyServerFile(data); err != nil {
			return st, fmt.Errorf("SERVER: %w", err)
		}
	}
	path := configPath
	if path == "" {
		path = filepath.Join(dataDir, "server.conf")
	}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := st.applyConfig(data); err != nil {
			return st, fmt.Errorf("%s: %w", path, err)
		}
		log.Printf("settings from %s", path)
	case configPath != "": // asked for but missing
		return st, err
	}
	return st, nil
}

func run(dataDir string, st settings) error {
	port, maxPlayers, speed, teamDamage := st.Port, st.MaxPlayers, st.Speed, st.TeamDamage
	sessionName, sessionMinutes := st.Name, st.Minutes

	// GAME.RES places entering heroes (0x0044ADB4).
	var start func(netplay.PlayerInfo) (netplay.EnterPayload, error)
	if data, err := os.ReadFile(filepath.Join(dataDir, "GAME.RES")); err != nil {
		log.Printf("no GAME.RES in %s: players can log in but not enter the world", dataDir)
	} else if start, err = netplay.StartFromGameRes(data); err != nil {
		return err
	}

	// Fights between players: decided here with the game's rules.
	var combat netplay.Combat
	if w, err := loadFights(dataDir); err != nil {
		log.Printf("fights between players off: %v", err)
	} else {
		combat = w
	}

	store, err := lanstore.Open(filepath.Join(dataDir, "konung.db"))
	if err != nil {
		return err
	}
	defer store.Close()

	addr := ":" + strconv.Itoa(port)
	srv, err := netplay.Listen(addr, netplay.ServerConfig{
		Name: sessionName, MaxPlayers: maxPlayers, Minutes: sessionMinutes,
		Accounts: store,
		Start:    start,
		Speed:    speed,
		TeamSafe: !teamDamage,
		Combat:   combat,
		OnKill: func(killer, victim netplay.PlayerInfo, killerHero *netplay.Hero, killerLevel, victimLevel int) {
			points := gamestate.HeroKillPoints(victimLevel, killerLevel, false)
			log.Printf("%s killed %s: %d points", killer.Name, victim.Name, points)
			if err := store.Score(killer.Name, lanstore.KillHero, points); err != nil {
				log.Printf("score: %v", err)
			}
			if killerHero != nil {
				if err := store.SaveProgress(killer.Name, killerHero); err != nil {
					log.Printf("save %s: %v", killer.Name, err)
				}
			}
		},
		OnJoin: func(p netplay.PlayerInfo) {
			if p.New { // as the original server prints (0x0044B1CE)
				log.Printf("Новый игрок %s клан %s", p.Name, p.Clan)
			}
			log.Printf("joined: %s", p.Name)
		},
		OnEnter: func(p netplay.PlayerInfo) {
			log.Printf("entered the world: %s", p.Name)
			// A participant of this session (0x0044B0EC), so its end rates it.
			if err := store.JoinSession(p.Name); err != nil {
				log.Printf("session: %v", err)
			}
		},
		OnLeave: func(p netplay.PlayerInfo) { log.Printf("left: %s", p.Name) },
	})
	if err != nil {
		return err
	}
	defer srv.Close()
	if err := srv.ServeDiscovery(addr); err != nil {
		log.Printf("LAN discovery off: %v", err) // the game still joins by typed address
	}
	log.Printf("«%s»: TCP %s, %d players, %d-minute sessions, speed %d, team damage %v, data in %s",
		sessionName, srv.Addr(), maxPlayers, sessionMinutes, srv.Clock().Speed, teamDamage, dataDir)

	// The session clock: at each end the participants' ratings move
	// (0x00444150) and a new session starts. Players see the time left.
	length := time.Duration(sessionMinutes) * time.Minute
	sessions := time.NewTicker(length)
	defer sessions.Stop()
	srv.SetSessionEnd(time.Now().Add(length))
	// The world clock is resent now and then, so clients stay in step.
	sync := time.NewTicker(30 * time.Second)
	defer sync.Stop()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	for {
		select {
		case <-sync.C:
			srv.SyncClock()
		case <-sessions.C:
			srv.SetSessionEnd(time.Now().Add(length))
			if err := store.EndSession(); err != nil {
				log.Printf("session end: %v", err)
			} else {
				log.Print("session over: ratings updated")
			}
			for _, p := range srv.Session().Players {
				store.JoinSession(p.Name) // still online: in the next session too
			}
		case s := <-stop:
			log.Printf("%v: stopping", s)
			return nil
		}
	}
}

// loadFights reads konung.exe and GAME.0–GAME.2 from dir for lanfight.
func loadFights(dir string) (*lanfight.World, error) {
	exe, err := os.ReadFile(filepath.Join(dir, "konung.exe"))
	if err != nil {
		return nil, err
	}
	var games [netplay.Models][]byte
	for m := range games {
		if games[m], err = os.ReadFile(filepath.Join(dir, "GAME."+strconv.Itoa(m))); err != nil {
			return nil, err
		}
	}
	return lanfight.New(exe, games)
}
