// Package lanstore keeps a dedicated LAN server's accounts in SQLite: what
// the LAN version's server kept in LANUSERS.DAT (accounts, hero, kill and
// amulet counters, rating), SAVITEMS.DAT (each account's equipment, bag,
// arrows and jewellery between sessions) and the clan list
// (docs/konung_lan.md#accounts). It implements netplay.AccountStore.
//
// Differences from the original, on purpose: passwords are stored as salted
// PBKDF2 hashes (the original kept them in plain text), and records are
// table rows instead of fixed 0x80 / 0x200-byte slots. Names compare
// case-sensitively, as the original's strcmp.
package lanstore

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"

	_ "modernc.org/sqlite" // pure Go: no C compiler needed
)

// Limits of the original tables.
const (
	MaxAccounts = 10000 // LANUSERS.DAT records
	MaxClans    = 256   // clan names in the LANUSERS.DAT header
)

// DefaultClans start the clan list (written when LANUSERS.DAT is created,
// 0x0043167B; CLANS.CFG has the same three).
var DefaultClans = []string{"Византийцы", "Русские", "Викинги"}

// defaultPalette is each hero model's first dye (tables 0x004821CC,
// 0x004821EC, 0x0048220C, entries ÷ 512); a new account starts with it.
var defaultPalette = [netplay.Models]int{70, 28, 34}

const schema = `
CREATE TABLE clans (
	id   INTEGER PRIMARY KEY,           -- index in the clan table (+0x68)
	name TEXT NOT NULL UNIQUE
);
CREATE TABLE accounts (                  -- LANUSERS.DAT record, 0x80 bytes
	id              INTEGER PRIMARY KEY, -- also selects the SAVITEMS.DAT record
	name            TEXT NOT NULL UNIQUE,-- +0x00, case-sensitive
	password        TEXT NOT NULL,       -- +0x20 in plain text there; here a PBKDF2 hash
	kills_heroes    INTEGER NOT NULL DEFAULT 0,    -- +0x28
	kills_hirelings INTEGER NOT NULL DEFAULT 0,    -- +0x2C
	kills_monsters  INTEGER NOT NULL DEFAULT 0,    -- +0x30
	amulets         INTEGER NOT NULL DEFAULT 0,    -- +0x34
	experience      INTEGER NOT NULL DEFAULT 0,    -- +0x38 (meaning a guess)
	gold            INTEGER NOT NULL DEFAULT 1000, -- +0x3C
	palette         INTEGER NOT NULL,              -- +0x40
	rating          INTEGER NOT NULL DEFAULT 1000, -- +0x44
	session_points  INTEGER NOT NULL DEFAULT 0,    -- +0x48: 0 not in the session, -1 joined, > 0 points
	unknown_4c      INTEGER NOT NULL DEFAULT 0,    -- +0x4C (entity +0x16)
	fencing         INTEGER NOT NULL DEFAULT 100,  -- +0x50, ×100
	trade           INTEGER NOT NULL DEFAULT 10,   -- +0x52
	archery         INTEGER NOT NULL DEFAULT 0,    -- +0x54, ×100
	carpentry       INTEGER NOT NULL DEFAULT 1,    -- +0x56
	herbalism       INTEGER NOT NULL DEFAULT 50,   -- +0x58
	identification  INTEGER NOT NULL DEFAULT 20,   -- +0x5A
	smithing        INTEGER NOT NULL DEFAULT 10,   -- +0x5C
	dexterity       INTEGER NOT NULL DEFAULT 3,    -- +0x5E
	charisma        INTEGER NOT NULL DEFAULT 3,    -- +0x60
	unknown_62      INTEGER NOT NULL DEFAULT 10,   -- +0x62 (entity +0x28)
	strength        INTEGER NOT NULL DEFAULT 15,   -- +0x64
	endurance       INTEGER NOT NULL DEFAULT 10,   -- +0x66
	clan_id         INTEGER NOT NULL DEFAULT 0 REFERENCES clans(id), -- +0x68
	level           INTEGER NOT NULL DEFAULT 1,    -- +0x69
	model           INTEGER NOT NULL DEFAULT 2,    -- +0x6A (a guess in the original)
	class           INTEGER NOT NULL DEFAULT 5,    -- +0x6B
	created_at      TEXT NOT NULL,
	last_login      TEXT NOT NULL
);
CREATE TABLE items (                     -- SAVITEMS.DAT record, 0x200 bytes
	account_id    INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	slot          TEXT NOT NULL CHECK (slot IN ('equip', 'jewel', 'bag', 'arrows')),
	idx           INTEGER NOT NULL,      -- equip 0–5, jewel 0–4, bag 0–31, arrows 0
	category      INTEGER NOT NULL,      -- item +0
	enchantment   INTEGER NOT NULL,      -- item +1 (int8, -1 none)
	type          INTEGER NOT NULL,      -- item +3
	flag          INTEGER NOT NULL,      -- 0x80 bit of the stored category (identified, a guess)
	condition     REAL NOT NULL,         -- item +4 (high word kept there)
	max_condition REAL NOT NULL,         -- item +8
	count         INTEGER NOT NULL,      -- arrows
	PRIMARY KEY (account_id, slot, idx)
);
`

// Store is an open account database.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open opens or creates the database at path.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection: SQLite has one writer, and Login's read-then-write
	// must not interleave with another login of the same name.
	db.SetMaxOpenConns(1)
	s := &Store{db: db, now: time.Now}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("lanstore: %s: %w", path, err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v >= 1 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(schema); err != nil {
		return err
	}
	for i, c := range DefaultClans {
		if _, err := tx.Exec(`INSERT INTO clans (id, name) VALUES (?, ?)`, i, c); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`PRAGMA user_version = 1`); err != nil {
		return err
	}
	return tx.Commit()
}

// Login implements netplay.AccountStore: the first login with a name creates
// the account with the original's defaults (0x00432E2B) and the chosen hero
// model; later logins must give the same password. A clan named at login is
// registered (new names appended, up to MaxClans; empty keeps the
// account's clan) and becomes the account's clan.
func (s *Store) Login(j netplay.JoinInfo) (*netplay.Hero, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	now := s.now().UTC().Format(time.RFC3339)

	var id int64
	var stored string
	err = tx.QueryRow(`SELECT id, password FROM accounts WHERE name = ?`, j.Name).Scan(&id, &stored)
	created := errors.Is(err, sql.ErrNoRows)
	switch {
	case created:
		var n int
		if err := tx.QueryRow(`SELECT count(*) FROM accounts`).Scan(&n); err != nil {
			return nil, false, err
		}
		if n >= MaxAccounts {
			// The original answers «Неправильный пароль» when its table is full.
			return nil, false, netplay.ErrWrongPassword
		}
		hash, err := hashPassword(j.Password)
		if err != nil {
			return nil, false, err
		}
		model := min(max(j.Model, 0), netplay.Models-1)
		res, err := tx.Exec(`INSERT INTO accounts (name, password, palette, model, created_at, last_login) VALUES (?, ?, ?, ?, ?, ?)`,
			j.Name, hash, defaultPalette[model], model, now, now)
		if err != nil {
			return nil, false, err
		}
		if id, err = res.LastInsertId(); err != nil {
			return nil, false, err
		}
	case err != nil:
		return nil, false, err
	default:
		if !checkPassword(stored, j.Password) {
			return nil, false, netplay.ErrWrongPassword
		}
		if _, err := tx.Exec(`UPDATE accounts SET last_login = ? WHERE id = ?`, now, id); err != nil {
			return nil, false, err
		}
	}
	if clan := strings.TrimSpace(j.Clan); clan != "" {
		cid, err := registerClan(tx, clan)
		if err != nil {
			return nil, false, err
		}
		if cid >= 0 {
			if _, err := tx.Exec(`UPDATE accounts SET clan_id = ? WHERE id = ?`, cid, id); err != nil {
				return nil, false, err
			}
		}
	}
	h, err := hero(tx, id)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return h, created, nil
}

// registerClan returns the clan's id, appending it when new; -1 when the
// clan table is full.
func registerClan(tx *sql.Tx, name string) (int, error) {
	var id int
	err := tx.QueryRow(`SELECT id FROM clans WHERE name = ?`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM clans`).Scan(&n); err != nil {
		return 0, err
	}
	if n >= MaxClans {
		return -1, nil
	}
	_, err = tx.Exec(`INSERT INTO clans (id, name) VALUES (?, ?)`, n, name)
	return n, err
}

type querier interface {
	QueryRow(query string, args ...any) *sql.Row
}

func hero(q querier, id int64) (*netplay.Hero, error) {
	h := &netplay.Hero{}
	err := q.QueryRow(`SELECT level, class, model, palette, experience, gold,
		charisma, strength, dexterity, endurance,
		fencing, archery, identification, trade, smithing, carpentry, herbalism,
		rating, kills_heroes, kills_hirelings, kills_monsters, amulets
		FROM accounts WHERE id = ?`, id).Scan(
		&h.Level, &h.Class, &h.Model, &h.Palette, &h.Experience, &h.Gold,
		&h.Charisma, &h.Strength, &h.Dexterity, &h.Endurance,
		&h.Fencing, &h.Archery, &h.Identification, &h.Trade, &h.Smithing, &h.Carpentry, &h.Herbalism,
		&h.Rating, &h.KillsHeroes, &h.KillsHirelings, &h.KillsMonsters, &h.Amulets)
	return h, err
}

// Hero returns an account's hero by name.
func (s *Store) Hero(name string) (*netplay.Hero, error) {
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM accounts WHERE name = ?`, name).Scan(&id); err != nil {
		return nil, err
	}
	return hero(s.db, id)
}

// SaveHero stores the hero and counters (0x004475F8, called on logout, quit,
// every score change and the session's end).
func (s *Store) SaveHero(name string, h *netplay.Hero) error {
	res, err := s.db.Exec(`UPDATE accounts SET level = ?, class = ?, model = ?, palette = ?, experience = ?, gold = ?,
		charisma = ?, strength = ?, dexterity = ?, endurance = ?,
		fencing = ?, archery = ?, identification = ?, trade = ?, smithing = ?, carpentry = ?, herbalism = ?,
		rating = ?, kills_heroes = ?, kills_hirelings = ?, kills_monsters = ?, amulets = ?
		WHERE name = ?`,
		h.Level, h.Class, h.Model, h.Palette, h.Experience, h.Gold,
		h.Charisma, h.Strength, h.Dexterity, h.Endurance,
		h.Fencing, h.Archery, h.Identification, h.Trade, h.Smithing, h.Carpentry, h.Herbalism,
		h.Rating, h.KillsHeroes, h.KillsHirelings, h.KillsMonsters, h.Amulets, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("lanstore: no account %q", name)
	}
	return nil
}

// Kill kinds counted by Score (0x0043B67C).
const (
	KillHero = iota
	KillHireling
	KillMonster
	Amulet
)

// Score counts a kill or an amulet delivery for an account and adds points
// to its session points (−1 = joined without points becomes the points).
func (s *Store) Score(name string, kind, points int) error {
	col := [...]string{"kills_heroes", "kills_hirelings", "kills_monsters", "amulets"}
	if kind < 0 || kind >= len(col) {
		return fmt.Errorf("lanstore: score kind %d", kind)
	}
	_, err := s.db.Exec(`UPDATE accounts SET `+col[kind]+` = `+col[kind]+` + 1,
		session_points = CASE WHEN session_points < 0 THEN ? ELSE session_points + ? END WHERE name = ?`,
		points, points, name)
	return err
}

// JoinSession marks an account as a participant without points (−1), as
// entering the game does (0x0044B0EC).
func (s *Store) JoinSession(name string) error {
	_, err := s.db.Exec(`UPDATE accounts SET session_points = -1 WHERE name = ? AND session_points = 0`, name)
	return err
}

// EndSession applies the rating update to the session's participants and
// clears their session points (0x00444150).
func (s *Store) EndSession() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id, rating, session_points FROM accounts WHERE session_points != 0 ORDER BY id`)
	if err != nil {
		return err
	}
	var ids []int64
	var ratings, points []int
	for rows.Next() {
		var id int64
		var r, p int
		if err := rows.Scan(&id, &r, &p); err != nil {
			rows.Close()
			return err
		}
		ids, ratings, points = append(ids, id), append(ratings, r), append(points, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for i, r := range gamestate.SessionRatings(ratings, points) {
		if _, err := tx.Exec(`UPDATE accounts SET rating = ?, session_points = 0 WHERE id = ?`, r, ids[i]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RatingRow is one row of the «Рейтинг» table (0x00446FA0).
type RatingRow struct {
	Rank                                 int
	Name, Clan                           string
	Amulets, KillsHeroes, KillsHirelings int
	KillsMonsters, Rating, ClanRating    int
	ClanRank                             int
}

// Ratings returns the accounts by rating, best first, with each clan's
// rating (the members' mean, a guess from the code shape) and rank.
func (s *Store) Ratings(limit int) ([]RatingRow, error) {
	rows, err := s.db.Query(`
		WITH clan AS (
			SELECT clan_id, avg(rating) AS r FROM accounts GROUP BY clan_id
		), clanrank AS (
			SELECT clan_id, r, rank() OVER (ORDER BY r DESC) AS k FROM clan
		)
		SELECT rank() OVER (ORDER BY a.rating DESC, a.id), a.name, c.name, a.amulets, a.kills_heroes,
			a.kills_hirelings, a.kills_monsters, a.rating, CAST(cr.r AS INTEGER), cr.k
		FROM accounts a JOIN clans c ON c.id = a.clan_id JOIN clanrank cr ON cr.clan_id = a.clan_id
		ORDER BY a.rating DESC, a.id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RatingRow
	for rows.Next() {
		var r RatingRow
		if err := rows.Scan(&r.Rank, &r.Name, &r.Clan, &r.Amulets, &r.KillsHeroes, &r.KillsHirelings,
			&r.KillsMonsters, &r.Rating, &r.ClanRating, &r.ClanRank); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Password hashing: PBKDF2-SHA256, stored as "pbkdf2-sha256$iter$salt$key".
const pbkdf2Iter = 200_000

func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iter, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return "pbkdf2-sha256$" + strconv.Itoa(pbkdf2Iter) + "$" + enc.EncodeToString(salt) + "$" + enc.EncodeToString(key), nil
}

func checkPassword(stored, pw string) bool {
	f := strings.Split(stored, "$")
	if len(f) != 4 || f[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(f[1])
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(f[2])
	want, err2 := enc.DecodeString(f[3])
	if err != nil || err1 != nil || err2 != nil || iter <= 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}
