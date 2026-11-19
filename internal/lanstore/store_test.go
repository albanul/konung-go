package lanstore

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"konung1.5/internal/gamestate"
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

// «Новый герой» rebuilds the hero: the choice's values, level 1, the
// starting gold, no items; rating and counters stay; the clan changes only
// here. The session's hero type picks the look's palette.
func TestNewHero(t *testing.T) {
	s, _ := open(t)
	s.Login(netplay.JoinInfo{Name: "Volk", Password: "p", Clan: "Русские", Model: 0})
	h, _ := s.Hero("Volk")
	h.Level, h.Gold, h.Rating, h.KillsHeroes = 7, 5000, 1800, 4
	s.SaveHero("Volk", h)
	s.SaveItems("Volk", []Item{{Slot: SlotBag, Category: 9, Type: 2}})

	// A plain login does not change the clan.
	if _, _, err := s.Login(netplay.JoinInfo{Name: "Volk", Password: "p", Clan: "Викинги", Model: 0}); err != nil {
		t.Fatal(err)
	}
	if clanOf(t, s, "Volk") != "Русские" {
		t.Error("plain login changed the clan")
	}

	choice := &netplay.HeroChoice{Class: 4, Archery: true, Skill: 1, Stats: [4]int{26, 2, 3, 7}, Look: 7}
	h, created, err := s.Login(netplay.JoinInfo{Name: "Volk", Password: "p", Clan: "Викинги", Model: 2, NewHero: choice})
	if err != nil || created {
		t.Fatalf("rebuild: %v, created %v", err, created)
	}
	if h.Level != 1 || h.Gold != 1000 || h.Class != 4 || h.Charisma != 26 || h.Strength != 2 || h.Archery != 100 || h.Fencing != 0 ||
		h.Trade != 20 || h.FreePoints != 0 || h.Rating != 1800 || h.KillsHeroes != 4 || h.Look != 7 ||
		h.Model != 2 || h.Palette != 112 {
		t.Errorf("rebuilt hero = %+v", h)
	}
	if clanOf(t, s, "Volk") != "Викинги" {
		t.Error("«Новый герой» did not change the clan")
	}
	if items, _ := s.LoadItems("Volk"); len(items) != 0 {
		t.Errorf("items kept: %+v", items)
	}
	// Another session as Волк: the same look index on that type.
	if h, _, _ := s.Login(netplay.JoinInfo{Name: "Volk", Password: "p", Model: 0}); h.Model != 0 || h.Palette != 114 {
		t.Errorf("as model 0: model %d palette %d", h.Model, h.Palette)
	}
	// Entering as Драгомир (the portrait at «Ok») is kept.
	if err := s.SetModel("Volk", 1); err != nil {
		t.Fatal(err)
	}
	if h, _ := s.Hero("Volk"); h.Model != 1 || h.Palette != 110 {
		t.Errorf("after SetModel: model %d palette %d", h.Model, h.Palette)
	}
	// A wrong password does not rebuild; an impossible build is refused.
	if _, _, err := s.Login(netplay.JoinInfo{Name: "Volk", Password: "x", NewHero: choice}); !errors.Is(err, netplay.ErrWrongPassword) {
		t.Errorf("wrong password: %v", err)
	}
	bad := &netplay.HeroChoice{Class: 5, Stats: [4]int{30, 30, 30, 30}}
	if _, _, err := s.Login(netplay.JoinInfo{Name: "Volk", Password: "p", NewHero: bad}); !errors.Is(err, netplay.ErrBadHero) {
		t.Errorf("impossible build: %v", err)
	}
	if clans, _ := s.Clans(); len(clans) != 3 || clans[2] != "Викинги" {
		t.Errorf("clans = %v", clans)
	}
}

func clanOf(t *testing.T, s *Store, name string) string {
	t.Helper()
	var c string
	if err := s.db.QueryRow(`SELECT c.name FROM accounts a JOIN clans c ON c.id = a.clan_id WHERE a.name = ?`, name).Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

// A version-1 database (before the looks) opens with its accounts intact.
func TestMigrateV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "konung.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schema + `INSERT INTO clans VALUES (0, 'Византийцы');
		INSERT INTO accounts (name, password, palette, model, experience, created_at, last_login) VALUES ('Old', 'x', 28, 1, 42, '', '');
		PRAGMA user_version = 1;`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h, err := s.Hero("Old")
	if err != nil || h.FreePoints != 42 || h.Look != 0 || h.Palette != 28 {
		t.Errorf("migrated hero = %+v, %v", h, err)
	}
}

// A version-2 database (a look per type) keeps the look of the type last
// played.
func TestMigrateV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "konung.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schema + migrations[0] + `INSERT INTO clans VALUES (0, 'Византийцы');
		INSERT INTO accounts (name, password, palette, model, look1, created_at, last_login) VALUES ('Old', 'x', 33, 1, 5, '', '');
		PRAGMA user_version = 2;`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if h, err := s.Hero("Old"); err != nil || h.Look != 5 || h.Palette != 33 {
		t.Errorf("migrated hero = %+v, %v", h, err)
	}
}

// The original server's item rules in the store: the starting knife once
// for a new or rebuilt hero; a death keeps the worn items with the dead
// flag; entering again while dead empties them; a session's end revives.
func TestStarterAndDeath(t *testing.T) {
	s, _ := open(t)
	s.Login(netplay.JoinInfo{Name: "Volk", Password: "p"})
	if ok, err := s.TakeStarter("Volk"); err != nil || !ok {
		t.Fatalf("new account: starter %v, %v", ok, err)
	}
	if ok, _ := s.TakeStarter("Volk"); ok {
		t.Error("the starter twice")
	}
	sword := Item{Slot: SlotEquip, Index: 2, Category: 2, Enchantment: -1, Type: 3, Condition: 40, MaxCondition: 50}
	if err := s.Died("Volk", []Item{sword}); err != nil {
		t.Fatal(err)
	}
	if was, err := s.Revive("Volk"); err != nil || !was {
		t.Fatalf("revive: %v, %v", was, err)
	}
	if items, _ := s.LoadItems("Volk"); len(items) != 0 {
		t.Errorf("entered again after death with %+v", items)
	}
	if was, _ := s.Revive("Volk"); was {
		t.Error("still dead after entering again")
	}
	s.Died("Volk", []Item{sword})
	if err := s.EndSession(); err != nil {
		t.Fatal(err)
	}
	if was, _ := s.Revive("Volk"); was {
		t.Error("the session's end did not revive")
	}
	if items, _ := s.LoadItems("Volk"); len(items) != 1 || items[0] != sword {
		t.Errorf("after the session's end: %+v, want the sword", items)
	}
	s.Login(netplay.JoinInfo{Name: "Volk", Password: "p", NewHero: &netplay.HeroChoice{Class: 5, Stats: [4]int{3, 15, 3, 10}}})
	if ok, _ := s.TakeStarter("Volk"); !ok {
		t.Error("a rebuilt hero has no starter")
	}
}

// Version 3 → 4: accounts that never saved items get the knife once,
// accounts with items keep them.
func TestMigrateV3(t *testing.T) {
	path := filepath.Join(t.TempDir(), "konung.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schema + migrations[0] + migrations[1] + `INSERT INTO clans VALUES (0, 'Византийцы');
		INSERT INTO accounts (id, name, password, palette, model, created_at, last_login) VALUES (1, 'Empty', 'x', 28, 1, '', '');
		INSERT INTO accounts (id, name, password, palette, model, created_at, last_login) VALUES (2, 'Rich', 'x', 28, 1, '', '');
		INSERT INTO items VALUES (2, 'equip', 2, 2, -1, 3, 0, 40, 50, 0);
		PRAGMA user_version = 3;`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if ok, _ := s.TakeStarter("Empty"); !ok {
		t.Error("an account without items gets no knife")
	}
	if ok, _ := s.TakeStarter("Rich"); ok {
		t.Error("an account with items gets the knife")
	}
	if items, _ := s.LoadItems("Rich"); len(items) != 1 {
		t.Errorf("Rich's items %+v", items)
	}
}

// The health a hero left with is kept (a port rule against leaving to
// heal); re-entering after death and the session's end heal; Alive clears
// the dead flag (a revival in the world).
func TestHealthKept(t *testing.T) {
	s, _ := open(t)
	s.Login(netplay.JoinInfo{Name: "Volk", Password: "p", Model: 1})
	h, _ := s.Hero("Volk")
	if h.HP != gamestate.HPFull || h.Poison != 0 {
		t.Fatalf("a new hero: HP %d poison %d", h.HP, h.Poison)
	}
	h.HP, h.Poison = 700, 4
	if err := s.SaveProgress("Volk", h); err != nil {
		t.Fatal(err)
	}
	if h, _ = s.Hero("Volk"); h.HP != 700 || h.Poison != 4 {
		t.Errorf("kept HP %d poison %d, want 700 and 4", h.HP, h.Poison)
	}
	// Died, revived in the world: alive again, the items stay.
	s.Died("Volk", []Item{{Slot: SlotEquip, Index: 2, Category: 2, Type: 18}})
	if err := s.Alive("Volk"); err != nil {
		t.Fatal(err)
	}
	if dead, _ := s.Revive("Volk"); dead {
		t.Error("Alive did not clear the dead flag")
	}
	if items, _ := s.LoadItems("Volk"); len(items) != 1 {
		t.Errorf("a revived hero's items %+v", items)
	}
	// Died and entering again: naked and healed.
	s.Died("Volk", nil)
	if dead, _ := s.Revive("Volk"); !dead {
		t.Fatal("not dead")
	}
	if h, _ = s.Hero("Volk"); h.HP != gamestate.HPFull || h.Poison != 0 {
		t.Errorf("after death: HP %d poison %d", h.HP, h.Poison)
	}
	// The session's end heals everyone.
	h.HP = 300
	s.SaveProgress("Volk", h)
	s.EndSession()
	if h, _ = s.Hero("Volk"); h.HP != gamestate.HPFull {
		t.Errorf("after the session's end HP %d", h.HP)
	}
}

// A version-4 database (before the health) opens with full health.
func TestMigrateV4(t *testing.T) {
	path := filepath.Join(t.TempDir(), "konung.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schema + migrations[0] + migrations[1] + migrations[2] + `INSERT INTO clans VALUES (0, 'Византийцы');
		INSERT INTO accounts (id, name, password, palette, model, created_at, last_login) VALUES (1, 'Old', 'x', 28, 1, '', '');
		PRAGMA user_version = 4;`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if h, err := s.Hero("Old"); err != nil || h.HP != gamestate.HPFull {
		t.Errorf("migrated hero %+v, %v", h, err)
	}
}
