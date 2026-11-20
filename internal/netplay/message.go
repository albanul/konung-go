// Package netplay is the port's own network layer for the LAN game (see
// docs/konung_lan.md). It is not wire-compatible with LAN_KONG.EXE (DirectPlay
// 4); it keeps the original's shape: one server owns the session, every
// client message goes to the server, and messages are a type plus a payload.
//
// The transport, message envelope and lobby handling are adapted from the
// author's go-tetris (github.com/albanul/go-tetris, pkg/network, MIT), with
// its bugs fixed: a connection closes once (no double-close panic), joining
// cannot deadlock, game messages are never dropped silently (a stalled peer
// is disconnected instead), idle connections stay open with pings, and the
// join is an explicit handshake. LAN discovery (UDP) is new.
//
// Wire format: one JSON object per line over TCP; discovery uses single UDP
// datagrams on the same port number.
package netplay

import (
	"encoding/json"
	"fmt"

	"konung1.5/internal/gamestate"
)

// DefaultPort is the TCP game port and the UDP discovery port. The original
// LAN game used 7777 for TCP/IP (DPAID_INetPort, LAN_KONG.EXE 0x0043021B).
const DefaultPort = 7777

// MaxMessageSize bounds one encoded message; longer lines close the
// connection.
const MaxMessageSize = 1 << 20

// ProtocolVersion is checked at join; the original showed an error table for
// a client of the wrong version.
const ProtocolVersion = 15

// MsgType identifies a message.
type MsgType int

const (
	MsgPing         MsgType = iota + 1 // keep-alive, handled by Conn
	MsgJoin                            // client → server, first message: JoinPayload
	MsgWelcome                         // server → client: WelcomePayload
	MsgError                           // server → client: ErrorPayload, then the server closes
	MsgPlayerJoined                    // server → all: PlayerInfo
	MsgPlayerLeft                      // server → all: PlayerLeftPayload
	MsgSession                         // server → all: SessionInfo after a change
	MsgChat                            // client → server → all: ChatPayload
	MsgGame                            // gameplay, defined with the traced protocol; routed to Server.OnGame
	MsgEnter                           // client → server at «Ok»: enter the world (the original's 0x4C)
	MsgEntered                         // server → client: EnterPayload, where the hero starts and its slot (0x8C)
	MsgClock                           // server → client: ClockPayload, the world clock (clock.go)
	MsgHero                            // server → client: Hero, the own hero after a change (kill experience, 0x5C)
	MsgOrder                           // client → server: OrderPayload, an order for the own hero (view.go; the original's 0x55)
	MsgView                            // server → client: ViewPayload, the characters of the own hero's map (0x19)
	MsgPlayer                          // server → client: PlayerPayload, a player's hero as it looks (0x5A)
	MsgEvents                          // server → client: EventsPayload, sounds on the own hero's map
	MsgMapChange                       // server → client: EnterPayload, the own hero took an exit to another map
	MsgInventory                       // server → client: InventoryPayload, the own party's items and hand (items.go)
	MsgPiles                           // server → client: PilesPayload, the piles on the own hero's map
	MsgPileWindow                      // server → client: PileWindowPayload, window 7 (the pile window or a trade) opened, changed or closed
	MsgDialogue                        // server → client: DialoguePayload, the dialogue window opened, changed or closed (talk.go)
	MsgParty                           // server → client: PartyPayload, the own hero's hirelings (party.go)
	MsgAmulets                         // server → client: AmuletsPayload, the amulet parts and the nations' counts (amulets.go; the original's 0x16)
	MsgPlayers                         // server → client: PlayersPayload, the players in the world and the own quest log (players.go; Q)
)

var msgNames = map[MsgType]string{
	MsgPing: "ping", MsgJoin: "join", MsgWelcome: "welcome", MsgError: "error",
	MsgPlayerJoined: "player-joined", MsgPlayerLeft: "player-left", MsgSession: "session",
	MsgChat: "chat", MsgGame: "game", MsgEnter: "enter", MsgEntered: "entered",
	MsgClock: "clock", MsgHero: "hero", MsgOrder: "order", MsgView: "view", MsgPlayer: "player",
	MsgEvents: "events", MsgMapChange: "map-change", MsgInventory: "inventory", MsgPiles: "piles",
	MsgPileWindow: "pile-window", MsgDialogue: "dialogue", MsgParty: "party", MsgAmulets: "amulets",
	MsgPlayers: "players",
}

func (t MsgType) String() string {
	if n, ok := msgNames[t]; ok {
		return n
	}
	return fmt.Sprintf("msg(%d)", int(t))
}

// Message is the envelope of every message. From is the sender's player ID,
// set by the server on everything it relays (clients cannot spoof it).
type Message struct {
	Type    MsgType         `json:"type"`
	From    string          `json:"from,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// NewMessage builds a message with payload encoded as JSON (nil = none).
func NewMessage(t MsgType, payload any) (*Message, error) {
	m := &Message{Type: t}
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode %v payload: %w", t, err)
		}
		m.Payload = b
	}
	return m, nil
}

// mustMessage is NewMessage for payload types that always encode.
func mustMessage(t MsgType, payload any) *Message {
	m, err := NewMessage(t, payload)
	if err != nil {
		panic(err)
	}
	return m
}

// Decode reads the payload into v.
func (m *Message) Decode(v any) error {
	if len(m.Payload) == 0 {
		return fmt.Errorf("%v message has no payload", m.Type)
	}
	if err := json.Unmarshal(m.Payload, v); err != nil {
		return fmt.Errorf("decode %v payload: %w", m.Type, err)
	}
	return nil
}

// Hero models, chosen with the login screen's portraits (LAN_KONG.EXE
// 0x0042F269: the three boxes select models 1, 2, 0).
const Models = 3

// JoinInfo is what a player logs in with, as the original's login message
// 0x47 (name 32 bytes, password 8) plus the clan and hero model. Text is
// UTF-8 on the wire.
type JoinInfo struct {
	Name     string `json:"name"`
	Password string `json:"password,omitempty"`
	// Clan names a new account's clan (CLANS.CFG starts with the three
	// nations); an existing account changes clan only with NewHero.
	Clan  string `json:"clan,omitempty"`
	Model int    `json:"model"` // the hero type for this session (start village and team)
	// NewHero rebuilds the account's hero from «Новый герой» choices (the
	// original sends the built hero with 0x4C after «Старый персонаж будет
	// удален!»); the server checks them with gamestate.LANHeroChoice.Build.
	// Progress (level, gold, free points, items) is reset; rating and
	// counters stay.
	NewHero *HeroChoice `json:"new_hero,omitempty"`
}

// HeroChoice is what «Новый герой» picks: class byte 2–5 (Купец, Охотник,
// Вождь, Воин), the combat skill (Меткость, else Фехтование), the peaceful
// skill (0 Идентификация, 1 Торговля, 2 Знахарство), the four base stats
// after trading (Хар, Сил, Лов, Вын) and the look 0–7 (one index for every
// hero type, as the original's [0xA6EB4C]).
type HeroChoice struct {
	Class   int    `json:"class"`
	Archery bool   `json:"archery"`
	Skill   int    `json:"skill"`
	Stats   [4]int `json:"stats"`
	Look    int    `json:"look"`
}

// JoinPayload is the client's first message.
type JoinPayload struct {
	Version int `json:"version"`
	JoinInfo
}

// PlayerInfo describes one player in the session.
type PlayerInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Clan  string `json:"clan,omitempty"`
	Model int    `json:"model"`
	// New marks an account created by this login («Новый игрок <name> клан
	// <clan>», 0x0044B1CE).
	New bool `json:"new,omitempty"`
}

// SessionInfo describes a session: what discovery reports and what a client
// gets on joining. Defaults follow the LAN version's SERVER file
// ("TEAM 3 180", up to 16 players).
type SessionInfo struct {
	Name       string       `json:"name"`
	Port       int          `json:"port"` // TCP game port
	MaxPlayers int          `json:"max_players"`
	Minutes    int          `json:"minutes"` // session length
	Players    []PlayerInfo `json:"players"`
	// TeamSafe: heroes of one team (hero type) cannot hurt each other
	// (ServerConfig.TeamSafe; the original lets them).
	TeamSafe bool `json:"team_safe,omitempty"`
}

// WelcomePayload answers a successful join. Hero is the account's hero on
// a server that keeps accounts (the original's login answer 0x87).
type WelcomePayload struct {
	PlayerID string      `json:"player_id"`
	Session  SessionInfo `json:"session"`
	Hero     *Hero       `json:"hero,omitempty"`
	// Clans are the server's clans for the «Новый герой» list (the
	// original's login answer 0x87 is followed by the clan list).
	Clans []string `json:"clans,omitempty"`
}

// Hero is an account's hero and counters, the LAN server's LANUSERS.DAT
// record (0x80 bytes, docs/konung_lan.md#accounts). Fencing and Archery are
// ×100, as stored.
type Hero struct {
	Level      int `json:"level"`
	Class      int `json:"class"`
	Model      int `json:"model"`
	Palette    int `json:"palette"`
	FreePoints int `json:"free_points"` // +0x38: free experience for stat raises (0x004475F8)
	Gold       int `json:"gold"`

	Charisma  int `json:"charisma"`
	Strength  int `json:"strength"`
	Dexterity int `json:"dexterity"`
	Endurance int `json:"endurance"`

	Fencing        int `json:"fencing"`
	Archery        int `json:"archery"`
	Identification int `json:"identification"`
	Trade          int `json:"trade"`
	Smithing       int `json:"smithing"`
	Carpentry      int `json:"carpentry"`
	Herbalism      int `json:"herbalism"`

	Rating         int `json:"rating"`
	KillsHeroes    int `json:"kills_heroes"`
	KillsHirelings int `json:"kills_hirelings"`
	KillsMonsters  int `json:"kills_monsters"`
	Amulets        int `json:"amulets"`

	// Look is the chosen look (0–7, gamestate.LANLooks), one index for
	// every hero type; Palette is its palette for Model, this session's type.
	Look int `json:"look"`

	// HP and Poison as the hero left the world (a port rule: the original
	// enters with HP 1600, 0x0042105C, so leaving and entering again
	// healed); 0 HP means full (gamestate.HPFull).
	HP     int `json:"hp,omitempty"`
	Poison int `json:"poison,omitempty"`
}

// ErrorPayload explains a refusal.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error codes.
const (
	ErrFull     = "full"
	ErrName     = "name"
	ErrModel    = "model"
	ErrPassword = "password" // message 7, «Неправильный пароль»
	ErrHero     = "hero"     // NewHero is not something «Новый герой» can produce
	ErrServer   = "server"   // the server cannot check the login
	ErrLeaving  = "leaving"  // message 8: the hero is still leaving the world (Start returned ErrHeroLeaving)
	ErrClan     = "clan"
	ErrVersion  = "version"
	ErrProtocol = "protocol"
)

// PlayerLeftPayload names a player who left.
type PlayerLeftPayload struct {
	ID string `json:"id"`
}

// Position is a hero's place in the world: a MAPS.RES map (1-based) and a
// fine-grid cell (row 0–319, col 0–159) with a facing (0–7).
type Position struct {
	Map    int `json:"map"`
	Row    int `json:"row"`
	Col    int `json:"col"`
	Facing int `json:"facing"`
}

// Valid reports whether p is inside the fine grid.
func (p Position) Valid() bool {
	return p.Map >= 1 && p.Map <= 255 && p.Row >= 0 && p.Row < 320 && p.Col >= 0 && p.Col < 160 && p.Facing >= 0 && p.Facing < 8
}

// EnterPayload is where the own hero is (entering, MsgEntered; a map
// change, MsgMapChange) and, on entering, its player slot: its entity and
// group in the server's world (0–15).
type EnterPayload struct {
	Position
	Slot int `json:"slot"`
}

// ChatPayload is a chat line.
type ChatPayload struct {
	Text string `json:"text"`
}

// Build checks c with the creation screen's rules (gamestate.LANHeroChoice)
// and returns the level-1 hero it makes, or why the screen could not have
// produced c.
func (c HeroChoice) Build() (gamestate.Entity, error) {
	if c.Look < 0 || c.Look > 7 {
		return gamestate.Entity{}, fmt.Errorf("look %d", c.Look)
	}
	if c.Class < 0 || c.Class > 255 {
		return gamestate.Entity{}, fmt.Errorf("class %d", c.Class)
	}
	return gamestate.LANHeroChoice{Class: byte(c.Class), Archery: c.Archery, Skill: c.Skill, Stats: c.Stats}.Build()
}

// EnterRequest is «Ok» on the login screen (the original's 0x4C carries the
// hero with its model): the hero type for this session, which picks the
// start village, the team and the look's palette.
type EnterRequest struct {
	Model int `json:"model"`
}

// WithModel is h as hero type model: the model and the look's palette.
func (h Hero) WithModel(model int) Hero {
	h.Model = model
	h.Palette = gamestate.LANLookPalette(model, h.Look)
	return h
}
