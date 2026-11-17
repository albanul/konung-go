package lanstore

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"konung1.5/internal/netplay"
)

func open(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "konung.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func TestLoginCreatesThenChecks(t *testing.T) {
	s, _ := open(t)
	h, created, err := s.Login(netplay.JoinInfo{Name: "Волк", Password: "1234567", Model: 1})
	if err != nil || !created {
		t.Fatalf("first login: %v, created %v", err, created)
	}
	// The original's defaults (0x00432E2B), the chosen model and its first dye.
	if h.Level != 1 || h.Gold != 1000 || h.Rating != 1000 || h.Class != 5 || h.Fencing != 100 || h.Strength != 15 || h.Model != 1 || h.Palette != 28 {
		t.Errorf("new hero = %+v", h)
	}
	if _, created, err := s.Login(netplay.JoinInfo{Name: "Волк", Password: "1234567"}); err != nil || created {
		t.Errorf("second login: %v, created %v", err, created)
	}
	if _, _, err := s.Login(netplay.JoinInfo{Name: "Волк", Password: "wrong"}); !errors.Is(err, netplay.ErrWrongPassword) {
		t.Errorf("wrong password: %v", err)
	}
	// Names compare case-sensitively (strcmp): another account.
	if _, created, err := s.Login(netplay.JoinInfo{Name: "волк", Password: "x"}); err != nil || !created {
		t.Errorf("other case: %v, created %v", err, created)
	}
}

func TestPasswordIsHashed(t *testing.T) {
	s, _ := open(t)
	s.Login(netplay.JoinInfo{Name: "Volk", Password: "секрет7"})
	var stored string
	if err := s.db.QueryRow(`SELECT password FROM accounts WHERE name = 'Volk'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "секрет7") || !strings.HasPrefix(stored, "pbkdf2-sha256$") {
		t.Errorf("stored password %q", stored)
	}
}

func TestClans(t *testing.T) {
	s, _ := open(t)
	s.Login(netplay.JoinInfo{Name: "A", Password: "p", Clan: "Викинги"})
	s.Login(netplay.JoinInfo{Name: "B", Password: "p", Clan: "Варяги"})
	rows, err := s.Ratings(10)
	if err != nil {
		t.Fatal(err)
	}
	clans := map[string]string{}
	for _, r := range rows {
		clans[r.Name] = r.Clan
	}
	if clans["A"] != "Викинги" || clans["B"] != "Варяги" {
		t.Errorf("clans = %v", clans)
	}
	var id int
	s.db.QueryRow(`SELECT id FROM clans WHERE name = 'Варяги'`).Scan(&id)
	if id != 3 { // appended after the three defaults
		t.Errorf("new clan id %d, want 3", id)
	}
}

func TestHeroScoreAndSession(t *testing.T) {
	s, path := open(t)
	for _, n := range []string{"A", "B", "C"} {
		s.Login(netplay.JoinInfo{Name: n, Password: "p"})
	}
	h, _ := s.Hero("A")
	h.Level, h.Gold = 4, 250
	if err := s.SaveHero("A", h); err != nil {
		t.Fatal(err)
	}
	s.JoinSession("A")
	s.JoinSession("B")
	s.Score("A", KillHero, 25)
	s.Score("A", Amulet, 25)
	s.Score("A", KillMonster, 0)
	if err := s.EndSession(); err != nil {
		t.Fatal(err)
	}
	// A: 50 points, B: −1 (none), C not in the session. R̄ = 1000, P = 50,
	// p̄ = 25: A gains 500, B loses 500, C unchanged.
	rows, _ := s.Ratings(10)
	got := map[string]RatingRow{}
	for _, r := range rows {
		got[r.Name] = r
	}
	if got["A"].Rating != 1500 || got["B"].Rating != 500 || got["C"].Rating != 1000 || got["A"].Rank != 1 {
		t.Errorf("ratings = %+v", rows)
	}
	if got["A"].KillsHeroes != 1 || got["A"].Amulets != 1 || got["A"].KillsMonsters != 1 {
		t.Errorf("counters = %+v", got["A"])
	}
	// The session points are cleared and everything survives a reopen.
	s.Close()
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	h2, _ := s2.Hero("A")
	if h2.Level != 4 || h2.Gold != 250 || h2.Rating != 1500 {
		t.Errorf("after reopen = %+v", h2)
	}
	var sp int
	s2.db.QueryRow(`SELECT session_points FROM accounts WHERE name = 'A'`).Scan(&sp)
	if sp != 0 {
		t.Errorf("session points %d after the session's end", sp)
	}
}

func TestItems(t *testing.T) {
	s, _ := open(t)
	s.Login(netplay.JoinInfo{Name: "Volk", Password: "p"})
	items := []Item{
		{Slot: SlotEquip, Index: 0, Category: 0, Enchantment: -1, Type: 3, Condition: 40, MaxCondition: 50},
		{Slot: SlotArrows, Category: 12, Type: 1, Count: 30},
		{Slot: SlotBag, Index: 0, Category: 9, Type: 2, Condition: 1, MaxCondition: 1},
		{Slot: SlotBag, Index: 1, Category: 11, Type: 12}, // an amulet part: dropped on load
	}
	if err := s.SaveItems("Volk", items); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadItems("Volk")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("loaded %d items: %+v", len(got), got)
	}
	for _, it := range got {
		if it.Category == 11 {
			t.Error("amulet part survived the load")
		}
		if it.Slot == SlotArrows && it.Count != 30 {
			t.Errorf("arrows = %+v", it)
		}
	}
	if err := s.SaveItems("Volk", []Item{{Slot: SlotBag, Index: 32}}); err == nil {
		t.Error("bag index 32 accepted")
	}
}

// Concurrent first logins of one name create exactly one account; the
// others check against the password the first one set.
func TestConcurrentLogins(t *testing.T) {
	s, _ := open(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	creates, wrong := 0, 0
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pw := "same"
			if i == 7 {
				pw = "other"
			}
			_, created, err := s.Login(netplay.JoinInfo{Name: "Volk", Password: pw})
			mu.Lock()
			defer mu.Unlock()
			if created {
				creates++
			}
			if errors.Is(err, netplay.ErrWrongPassword) {
				wrong++
			} else if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var n int
	s.db.QueryRow(`SELECT count(*) FROM accounts`).Scan(&n)
	// Whoever came first set the password: either the odd one out is
	// refused (1) or, if it came first, all seven others are (7).
	if n != 1 || creates != 1 || (wrong != 1 && wrong != 7) {
		t.Errorf("accounts %d, creates %d, wrong %d", n, creates, wrong)
	}
}

// The store plugs into the server: the first join creates the account and
// returns the hero, a wrong password is refused with the code the client
// shows as «Неправильный пароль».
func TestServerWithStore(t *testing.T) {
	s, _ := open(t)
	srv, err := netplay.Listen("127.0.0.1:0", netplay.ServerConfig{Accounts: s})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	c, err := netplay.Dial(srv.Addr().String(), netplay.JoinInfo{Name: "Volk", Password: "pw", Clan: "Русские", Model: 0}, netplay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Hero == nil || c.Hero.Palette != 70 || !c.Session.Players[0].New {
		t.Errorf("welcome = %+v / %+v", c.Hero, c.Session.Players)
	}
	c.Close()
	var je *netplay.JoinError
	if _, err := netplay.Dial(srv.Addr().String(), netplay.JoinInfo{Name: "Volk", Password: "bad"}, netplay.Options{}); !errors.As(err, &je) || je.Code != netplay.ErrPassword {
		t.Errorf("wrong password: %v", err)
	}
}
