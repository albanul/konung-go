package netplay

import (
	"errors"
	"fmt"
	"net"
	"time"
)

// JoinError is a server's refusal of a join.
type JoinError struct {
	Code, Message string
}

func (e *JoinError) Error() string {
	return fmt.Sprintf("netplay: join refused (%s): %s", e.Code, e.Message)
}

// Client is a joined connection to a server.
type Client struct {
	*Conn
	PlayerID string
	Session  SessionInfo // as of joining; later changes arrive as messages
	Hero     *Hero       // the account's hero; nil on a server without accounts
	Clans    []string    // the server's clans (nil on a server without accounts)
}

// Dial connects to a server at addr ("host:port") and joins as j. It fails
// with a *JoinError when the server refuses.
func Dial(addr string, j JoinInfo, opts Options) (*Client, error) {
	opts = opts.withDefaults()
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("netplay: connect %s: %w", addr, err)
	}
	conn := NewConn(c, opts)
	join := mustMessage(MsgJoin, JoinPayload{Version: ProtocolVersion, JoinInfo: j})
	if err := conn.Send(join); err != nil {
		conn.Close()
		return nil, err
	}
	timer := time.NewTimer(opts.IdleTimeout)
	defer timer.Stop()
	select {
	case m, ok := <-conn.Incoming():
		if !ok {
			err := conn.Err()
			if err == nil {
				err = ErrClosed
			}
			return nil, fmt.Errorf("netplay: join: %w", err)
		}
		switch m.Type {
		case MsgWelcome:
			var w WelcomePayload
			if err := m.Decode(&w); err != nil {
				conn.Close()
				return nil, err
			}
			return &Client{Conn: conn, PlayerID: w.PlayerID, Session: w.Session, Hero: w.Hero, Clans: w.Clans}, nil
		case MsgError:
			var e ErrorPayload
			if err := m.Decode(&e); err != nil {
				conn.Close()
				return nil, err
			}
			conn.Close()
			return nil, &JoinError{Code: e.Code, Message: e.Message}
		default:
			conn.Close()
			return nil, fmt.Errorf("netplay: join: unexpected %v", m.Type)
		}
	case <-timer.C:
		conn.Close()
		return nil, errors.New("netplay: join: no answer from the server")
	}
}

// Enter asks to enter the world as hero type model (the login portrait at
// «Ok»); the server answers with MsgEntered (or MsgError) on Incoming.
func (c *Client) Enter(model int) error {
	return c.Send(mustMessage(MsgEnter, EnterRequest{Model: model}))
}

// Move reports the own hero's position.
func (c *Client) Move(p Position) error { return c.Send(mustMessage(MsgMove, p)) }

// Look reports the own hero's look (stance, weapon, equipment).
func (c *Client) Look(l Look) error { return c.Send(mustMessage(MsgLook, LookPayload{Look: l})) }

// Attack reports a blow (or an arrow, ranged) of the own hero that reached
// target's hero; the server answers everyone with MsgHit.
func (c *Client) Attack(target string, ranged bool) error {
	return c.Send(mustMessage(MsgAttack, AttackPayload{Target: target, Ranged: ranged}))
}

// Chat sends a chat line to everyone in the session.
func (c *Client) Chat(text string) error {
	return c.Send(mustMessage(MsgChat, ChatPayload{Text: text}))
}

// SendGame sends a gameplay message (payload encoded as JSON) to the server.
func (c *Client) SendGame(payload any) error {
	m, err := NewMessage(MsgGame, payload)
	if err != nil {
		return err
	}
	return c.Send(m)
}
