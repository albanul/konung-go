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
)

// DefaultPort is the TCP game port and the UDP discovery port. The original
// LAN game used 7777 for TCP/IP (DPAID_INetPort, LAN_KONG.EXE 0x0043021B).
const DefaultPort = 7777

// MaxMessageSize bounds one encoded message; longer lines close the
// connection.
const MaxMessageSize = 1 << 20

// ProtocolVersion is checked at join; the original showed an error table for
// a client of the wrong version.
const ProtocolVersion = 1

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
)

var msgNames = map[MsgType]string{
	MsgPing: "ping", MsgJoin: "join", MsgWelcome: "welcome", MsgError: "error",
	MsgPlayerJoined: "player-joined", MsgPlayerLeft: "player-left", MsgSession: "session",
	MsgChat: "chat", MsgGame: "game",
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
	Password string `json:"password,omitempty"` // accounts are not ported; the server ignores it
	Clan     string `json:"clan,omitempty"`     // CLANS.CFG starts with the three nations
	Model    int    `json:"model"`
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
}

// WelcomePayload answers a successful join.
type WelcomePayload struct {
	PlayerID string      `json:"player_id"`
	Session  SessionInfo `json:"session"`
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
	ErrClan     = "clan"
	ErrVersion  = "version"
	ErrProtocol = "protocol"
)

// PlayerLeftPayload names a player who left.
type PlayerLeftPayload struct {
	ID string `json:"id"`
}

// ChatPayload is a chat line.
type ChatPayload struct {
	Text string `json:"text"`
}
