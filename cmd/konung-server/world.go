package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"konung1.5/internal/lanserver"
	"konung1.5/internal/netplay"
)

// serverWorld runs the LAN world (internal/lanserver) on the server's
// clock: every map steps once per game tick, while anybody is in the world
// (the clock stands still otherwise). Players entering get a hero in it, in
// the first free player slot (0–15), and see their hero's map
// (lanserver.Host, docs/konung_lan.md#server-world).
type serverWorld struct {
	mu   sync.Mutex // the summary counters
	w    *lanserver.World
	host *lanserver.Host

	steps    int           // ticks run since the last summary
	stepTime time.Duration // time spent in them
	skipped  int           // ticks jumped over (the server fell behind)
	since    time.Time     // the last summary
}

// worldFiles are the files the world needs in the data folder.
var worldFiles = []string{"GAME.RES", "MAPS.RES", "GRAPH.RES", "OBJECTS.RES", "konung.exe", "GAME.0", "GAME.1", "GAME.2"}

// loadWorld builds the world from dir; an error names a missing file.
func loadWorld(dir string) (*serverWorld, error) {
	data := map[string][]byte{}
	for _, name := range worldFiles {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		data[name] = b
	}
	f := lanserver.Files{GameRes: data["GAME.RES"], MapsRes: data["MAPS.RES"], Graph: data["GRAPH.RES"],
		Objects: data["OBJECTS.RES"], Exe: data["konung.exe"]}
	for m := range f.Games {
		f.Games[m] = data["GAME."+strconv.Itoa(m)]
	}
	// QUESTR.RES (the installed name is questr.RES): the dialogues and
	// trade; without it nobody talks.
	for _, name := range []string{"QUESTR.RES", "questr.RES"} {
		if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			f.Quests = b
			break
		}
	}
	if f.Quests == nil {
		log.Printf("QUESTR.RES not found in %s: no dialogues or trade in the world", dir)
	}
	w, err := lanserver.Load(f)
	if err != nil {
		return nil, err
	}
	return &serverWorld{w: w, since: time.Now()}, nil
}

// maxCatchUp is the most ticks one wake-up runs; further behind, the world
// jumps to the clock (and counts the skipped ticks).
const maxCatchUp = 8

// run steps the world with clock until stop closes.
func (s *serverWorld) run(clock func() netplay.ClockPayload, stop <-chan struct{}) {
	t := time.NewTicker(netplay.TickDuration / 4)
	defer t.Stop()
	summary := time.NewTicker(summaryEvery)
	defer summary.Stop()
	for {
		select {
		case <-stop:
			return
		case <-summary.C:
			s.logSummary()
		case <-t.C:
			target := int(clock().Tick)
			skipped := s.host.Jump(target, maxCatchUp*4, maxCatchUp)
			t0 := time.Now()
			n := s.host.Advance(target, maxCatchUp)
			s.mu.Lock()
			s.skipped += skipped
			s.stepTime += time.Since(t0)
			s.steps += n
			s.mu.Unlock()
		}
	}
}

// logSummary logs a line: characters alive per map, ticks per second and
// the mean tick time.
func (s *serverWorld) logSummary() {
	s.mu.Lock()
	defer s.mu.Unlock()
	var maps []string
	snaps, tick := s.host.Snapshots()
	for n := 1; n <= lanserver.Maps; n++ {
		alive, heroes := 0, 0
		for _, c := range snaps[n].Chars {
			if !c.Dead {
				alive++
				if c.Player {
					heroes++
				}
			}
		}
		if heroes > 0 {
			maps = append(maps, fmt.Sprintf("%d:%d+%dh", n, alive-heroes, heroes))
		} else {
			maps = append(maps, fmt.Sprintf("%d:%d", n, alive))
		}
	}
	mean := time.Duration(0)
	if s.steps > 0 {
		mean = s.stepTime / time.Duration(s.steps)
	}
	elapsed := time.Since(s.since).Seconds()
	log.Printf("world: tick %d, %.1f ticks/s, %v per tick, %d skipped; alive by map %s",
		tick, float64(s.steps)/max(elapsed, 1e-9), mean, s.skipped, strings.Join(maps, " "))
	s.steps, s.stepTime, s.skipped, s.since = 0, 0, 0, time.Now()
}

// summaryEvery is how often the world logs its summary line.
var summaryEvery = time.Minute
