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
	"konung1.5/internal/lanserver"
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

	// The whole world (docs/konung_lan.md#server-world): every map runs
	// here; players see their hero's map. Without its files players can
	// log in but not enter (the server answers «Ok» with an error).
	sw, err := loadWorld(dataDir, lanserver.AmuletRules{Points: st.AmuletPoints, Limit: st.Amulets})
	if err != nil {
		log.Printf("world not running, players cannot enter: %v", err)
		sw = nil
	}

	store, err := lanstore.Open(filepath.Join(dataDir, "konung.db"))
	if err != nil {
		return err
	}
	defer store.Close()

	won := make(chan int, 1) // a nation reached the amulet limit: the session ends now (0x00434438)
	addr := ":" + strconv.Itoa(port)
	var srv *netplay.Server
	cfg := netplay.ServerConfig{
		Name: sessionName, MaxPlayers: maxPlayers, Minutes: sessionMinutes,
		Accounts: store,
		Speed:    speed,
		TeamSafe: !teamDamage,
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
		OnLeave: func(p netplay.PlayerInfo) {
			log.Printf("left: %s", p.Name)
			if sw != nil {
				sw.host.Leave(p.ID)
			}
		},
	}
	if sw != nil {
		sw.w.TeamSafe = !teamDamage
		sw.host = lanserver.NewHost(sw.w, func(id string, m *netplay.Message) error { return srv.Send(id, m) })
		sw.host.OnKill = func(k lanserver.Kill) { scoreKill(store, k) }
		sw.host.OnAmulet = func(p netplay.PlayerInfo, nation int) { scoreAmulet(store, p, nation) }
		sw.host.OnWin = func(nation int) {
			log.Printf("%s", gamestate.NationWins[nation])
			select {
			case won <- nation:
			default:
			}
		}
		sw.host.Load = func(p netplay.PlayerInfo) ([]lanserver.Item, bool) { return loadItems(store, p.Name) }
		sw.host.Save = func(p netplay.PlayerInfo, hero *netplay.Hero, items []lanserver.Item) {
			saveHero(store, p.Name, hero, items)
		}
		sw.host.Died = func(p netplay.PlayerInfo, hero *netplay.Hero, worn []lanserver.Item) {
			log.Printf("%s died", p.Name)
			heroDied(store, p.Name, hero, worn)
		}
		cfg.Start = func(p netplay.PlayerInfo, hero *netplay.Hero) (netplay.EnterPayload, error) {
			at, err := sw.host.Start(p, hero)
			if err == nil {
				log.Printf("world: %s is hero %d on map %d (%d, %d)", p.Name, at.Slot, at.Map, at.Row, at.Col)
			}
			return at, err
		}
		cfg.OnOrder = sw.host.Order
	}
	srv, err = netplay.Listen(addr, cfg)
	if err != nil {
		return err
	}
	defer srv.Close()
	if err := srv.ServeDiscovery(addr); err != nil {
		log.Printf("LAN discovery off: %v", err) // the game still joins by typed address
	}
	log.Printf("«%s»: TCP %s, %d players, %d-minute sessions, speed %d, team damage %v, data in %s",
		sessionName, srv.Addr(), maxPlayers, sessionMinutes, srv.Clock().Speed, teamDamage, dataDir)
	if sw != nil {
		stopWorld := make(chan struct{})
		defer close(stopWorld)
		go sw.run(srv.Clock, stopWorld)
		log.Printf("world: %d maps running on the server clock", lanserver.Maps)
	}

	// The session clock: at each end the participants' ratings move
	// (0x00444150) and a new session starts. Players see the time left.
	length := time.Duration(sessionMinutes) * time.Minute
	sessions := time.NewTicker(length)
	defer sessions.Stop()
	srv.SetSessionEnd(time.Now().Add(length))
	// The world clock is resent now and then, so clients stay in step.
	sync := time.NewTicker(30 * time.Second)
	defer sync.Stop()
	// endSession is a session's end: the world anew, the ratings, a new
	// session.
	endSession := func() {
		srv.SetSessionEnd(time.Now().Add(length))
		if sw != nil {
			// The world anew from GAME.RES, every hero back at its
			// start, alive and healed, then saved (0x0044687C,
			// 0x00448BB8).
			if err := sw.host.EndSession(); err != nil {
				log.Printf("session end: the world: %v", err)
			}
		}
		if err := store.EndSession(); err != nil {
			log.Printf("session end: %v", err)
		} else {
			log.Print("session over: ratings updated")
		}
		for _, p := range srv.Session().Players {
			store.JoinSession(p.Name) // still online: in the next session too
		}
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	for {
		select {
		case <-sync.C:
			srv.SyncClock()
		case <-sessions.C:
			endSession()
		case n := <-won:
			// A nation delivered the limit's amulets: the session ends
			// now (0x00434438) and the next one gets its full length.
			log.Printf("session ends early: %s", gamestate.NationWins[n])
			sessions.Reset(length)
			endSession()
		case s := <-stop:
			log.Printf("%v: stopping", s)
			if sw != nil {
				sw.host.SaveAll()
			}
			return nil
		}
	}
}

// scoreAmulet scores a delivered amulet (0x00437B69): 25 points and the
// account's amulet counter.
func scoreAmulet(store *lanstore.Store, p netplay.PlayerInfo, nation int) {
	log.Printf("%s delivered the amulet for %s: %d points", p.Name, gamestate.NationNames[nation], gamestate.AmuletPoints)
	if err := store.Score(p.Name, lanstore.Amulet, gamestate.AmuletPoints); err != nil {
		log.Printf("score: %v", err)
	}
}

// scoreKill scores a player's kill (0x0043B67C): a player victim gives
// HeroKillPoints, a creature or villager is counted with 0 points; the
// killer's experience is saved (0x00448970).
func scoreKill(store *lanstore.Store, k lanserver.Kill) {
	if k.VictimPlayer {
		points := gamestate.HeroKillPoints(k.VictimLevel, k.KillerLevel, k.VictimAmulet)
		log.Printf("%s killed %s: %d points", k.Killer.Name, k.Victim.Name, points)
		if err := store.Score(k.Killer.Name, lanstore.KillHero, points); err != nil {
			log.Printf("score: %v", err)
		}
	} else if k.VictimHireling {
		// A player's hireling: its own counter, 0 points (0x0043B67C).
		if err := store.Score(k.Killer.Name, lanstore.KillHireling, 0); err != nil {
			log.Printf("score: %v", err)
		}
	} else if err := store.Score(k.Killer.Name, lanstore.KillMonster, 0); err != nil {
		log.Printf("score: %v", err)
	}
	if k.KillerHero != nil {
		if err := store.SaveProgress(k.Killer.Name, k.KillerHero); err != nil {
			log.Printf("save %s: %v", k.Killer.Name, err)
		}
	}
}

// loadItems is an entering hero's items (the original server's rules,
// docs/konung_lan.md#items): a new or rebuilt hero gets its starting knife
// (starter); one that died comes naked; else its saved items (lanstore,
// after SAVITEMS.DAT's drop rules), none when it has none.
func loadItems(store *lanstore.Store, name string) ([]lanserver.Item, bool) {
	if dead, err := store.Revive(name); err != nil {
		log.Printf("items of %s: %v", name, err)
	} else if dead {
		log.Printf("%s enters again after death: naked", name)
	}
	if starter, err := store.TakeStarter(name); err != nil {
		log.Printf("items of %s: %v", name, err)
	} else if starter {
		return nil, true
	}
	saved, err := store.LoadItems(name)
	if err != nil {
		log.Printf("items of %s: %v", name, err)
		return nil, false
	}
	out := make([]lanserver.Item, len(saved))
	for i, it := range saved {
		out[i] = lanserver.Item(it)
	}
	return out, false
}

// heroDied keeps a hero that died: its values and the worn items, with the
// dead flag (lanstore.Died).
func heroDied(store *lanstore.Store, name string, hero *netplay.Hero, worn []lanserver.Item) {
	if hero != nil {
		if err := store.SaveProgress(name, hero); err != nil {
			log.Printf("save %s: %v", name, err)
		}
	}
	saved := make([]lanstore.Item, len(worn))
	for i, it := range worn {
		saved[i] = lanstore.Item(it)
	}
	if err := store.Died(name, saved); err != nil {
		log.Printf("death of %s: %v", name, err)
	}
}

// saveHero keeps a living player's hero (level, gold, stats, skills,
// experience, health) and items (logout, the session's end, a revival):
// a hero saved alive is not dead any more (a revived one, lanstore.Alive).
func saveHero(store *lanstore.Store, name string, hero *netplay.Hero, items []lanserver.Item) {
	if hero != nil {
		if err := store.SaveProgress(name, hero); err != nil {
			log.Printf("save %s: %v", name, err)
		}
	}
	if err := store.Alive(name); err != nil {
		log.Printf("save %s: %v", name, err)
	}
	saved := make([]lanstore.Item, len(items))
	for i, it := range items {
		saved[i] = lanstore.Item(it)
	}
	if err := store.SaveItems(name, saved); err != nil {
		log.Printf("save items of %s: %v", name, err)
	}
}
