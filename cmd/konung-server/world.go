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
	"konung1.5/internal/world"
)

// serverWorld runs the LAN world (internal/lanserver) on the server's
// clock: every map steps once per game tick, while anybody is in the world
// (the clock stands still otherwise). Players entering get a hero in it, in
// the first free player slot (0–15); clients do not see it yet (they still
// run their own worlds, step 4 of docs/konung_lan.md#server-world).
type serverWorld struct {
	mu    sync.Mutex
	w     *lanserver.World
	slots map[string]int // player ID → slot

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
	w, err := lanserver.Load(f)
	if err != nil {
		return nil, err
	}
	return &serverWorld{w: w, slots: map[string]int{}, since: time.Now()}, nil
}

// enter gives player p a hero (hero nil: the type's default) in a free
// slot; a full world (16 heroes) leaves it out.
func (s *serverWorld) enter(p netplay.PlayerInfo, hero *netplay.Hero) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, ok := s.slots[p.ID]
	if !ok {
		used := map[int]bool{}
		for _, k := range s.slots {
			used[k] = true
		}
		slot = -1
		for k := range world.LANPlayers {
			if !used[k] {
				slot = k
				break
			}
		}
		if slot < 0 {
			log.Printf("world: no free slot for %s", p.Name)
			return
		}
		s.slots[p.ID] = slot
	}
	m, c, err := s.w.AddPlayer(slot, p.Model, hero)
	if err != nil {
		log.Printf("world: %s: %v", p.Name, err)
		return
	}
	log.Printf("world: %s is hero %d on map %d (%d, %d)", p.Name, slot, m, c.Row, c.Col)
}

// leave takes player id's hero out of the world.
func (s *serverWorld) leave(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slot, ok := s.slots[id]; ok {
		s.w.RemovePlayer(slot)
		delete(s.slots, id)
	}
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
			s.mu.Lock()
			if behind := target - s.w.Tick; behind > maxCatchUp*4 {
				s.skipped += behind - maxCatchUp
				s.w.Tick = target - maxCatchUp
			}
			for n := 0; s.w.Tick < target && n < maxCatchUp; n++ {
				t0 := time.Now()
				s.w.Step()
				s.stepTime += time.Since(t0)
				s.steps++
			}
			s.w.TakeEvents() // no clients for them yet (step 4)
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
	for n := 1; n <= lanserver.Maps; n++ {
		alive, heroes := 0, 0
		for _, c := range s.w.Snapshot(n).Chars {
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
		s.w.Tick, float64(s.steps)/max(elapsed, 1e-9), mean, s.skipped, strings.Join(maps, " "))
	s.steps, s.stepTime, s.skipped, s.since = 0, 0, 0, time.Now()
}

// summaryEvery is how often the world logs its summary line.
var summaryEvery = time.Minute
