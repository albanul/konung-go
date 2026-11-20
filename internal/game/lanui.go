package game

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/netplay"
	"konung1.5/internal/resfmt"
)

// The LAN version's screens (LAN_KONG.EXE, docs/konung_lan.md#screens),
// rebuilt on the port's own network layer (internal/netplay). States follow
// the exe's [0x009EB9EC]: the LAN menu (3), the rating screen (2) and the
// login screen «Новая игра» (5) with its message box, the wait for the
// server to start the game (0) and «Новый герой» (4, lannewhero.go).
const (
	lanMenu = iota
	lanRating
	lanLogin
	lanWaiting
	lanNewHero
)

// LAN menu items (hotspots 0x00479884, actions 0x0042E358) and the
// ME800R.RES label frames drawn for them without a game; the hover frame is
// the next one.
const (
	lanItemRating = iota
	lanItemSettings
	lanItemNewGame
	lanItemNewHero
	lanItemExit
)

var lanMenuFrames = [5]int{5, 11, 7, 9, 13}

// lanGameMenuFrames are the labels while a LAN game runs: «Вернуться в
// игру» and «Закончить игру» in place of «Рейтинг» and «Установки».
var lanGameMenuFrames = [5]int{3, 15, 7, 9, 13}

const lanMenuLabelX, lanMenuLabelY = 209, 132

// PLAY800R.RES frames.
const (
	playLogin    = 0
	playRating   = 1
	playPortrait = 2  // 2/3, 4/5, 6/7: selected (colour) / not (grey)
	playCancel   = 8  // 8 normal, 9 pressed
	playOk       = 10 // 10 normal, 11 pressed
	playMsgPanel = 12
	playCheck    = 13
)

// msgNameTaken is a port message: the original identifies players by
// account (LANUSERS.DAT), which is not ported, so two players may not share
// a name in a session.
const msgNameTaken = 9

// lanUI runs the LAN screens. Network work (discovery, joining)
// runs in goroutines that hand their results back through results; Update
// applies them, so all UI state stays on the game goroutine.
type lanUI struct {
	exe      *gamestate.LANExe
	menuBG   *ebiten.Image
	menu     []*ebiten.Image // ME800R frames
	play     []*ebiten.Image // PLAY800R frames
	font     *gameFont       // text (the LAN exe's own fonts are not mapped; the save screen's pair stands in)
	focusFnt *gameFont       // the focused field
	msgText  [11][2][]byte   // message box lines by id

	state, hover int
	msg          int // message box id, 0 = none

	// Login fields; focus is the exe's [0x00A6EAE4]: 0 none, 1 clan
	// («Новый герой»), 2 name, 3 password, 6 address.
	focus                  int
	name, pass, addr, clan gamestate.TextField
	savePass               bool
	model                  int // hero model of the selected portrait
	scroll                 int
	found                  []netplay.Found
	discovering, busy      bool
	tick                   int

	cfg     gamestate.LANConfig
	cfgPath string

	client   *netplay.Client
	hero     *netplay.Hero // the account's hero from a server with accounts
	session  string        // joined session's name
	player   string        // the own name as the server knows it (UTF-8)
	results  chan func()
	leaveLAN bool // «Выход»: back to the port's main menu
	// inGame: a LAN world runs (the menu was opened by Escape in it).
	// resume and endGame ask the menu to go back to it or to end it.
	inGame, Resume, endGame bool
	// start is set when the server answers «Ok» (MsgEntered): the menu
	// takes it and builds the LAN world (takeStart).
	start *netplay.EnterPayload
	// gameDir holds MAPS.RES.
	gameDir string
	// worldMsgs are the server's world messages (views, players' heroes,
	// sounds, map changes, the own hero's values) for the LAN viewer,
	// queued from the wait on; the menu applies them (applyLAN).
	worldMsgs []*netplay.Message
	// clock is the server's world clock (MsgClock), nil until it arrives.
	clock *serverClock

	// «Новый герой» (lannewhero.go); newFrames nil = NEW800R.RES missing.
	newFrames []*ebiten.Image
	graph     []byte
	tables    *resfmt.ExeTables
	items     *gamestate.Game // empty: the LAN template has no equipment
	nh        gamestate.Entity
	Look      int // the look index 0–7, one for every type ([0xA6EB4C])
	// previewModel is the type «Новый герой»'s preview shows (Михаил;
	// a right click steps it, for demonstration only).
	previewModel int
	clans        [][]byte // CP866: CLANS.CFG, typed ones, the server's
	clanScroll   int
	preview      heroPreview
	// pending is the build «Ok» kept for the next join; pendingRow is the
	// server row waiting for «Старый персонаж будет удален!».
	pending    *netplay.HeroChoice
	pendingRow int
}

func newLANUI(gameDir string, assets *sceneAssets, mePath, playPath, newPath string) (*lanUI, error) {
	exeData, err := os.ReadFile(filepath.Join(gameDir, "LAN_KONG.EXE"))
	if err != nil {
		return nil, err
	}
	exe, err := gamestate.ParseLANExe(exeData)
	if err != nil {
		return nil, err
	}
	me, err := resfmt.LoadType5File(mePath)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", mePath, err)
	}
	play, err := resfmt.LoadType5File(playPath)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", playPath, err)
	}
	if len(me) < 15 || len(play) <= playCheck {
		return nil, fmt.Errorf("LAN screens: %d ME and %d PLAY frames", len(me), len(play))
	}
	slot, err := assets.tables.Font(resfmt.FontSlotVA)
	if err != nil {
		return nil, err
	}
	u := &lanUI{
		exe: exe, font: newGameFont(slot), focusFnt: assets.largeFont,
		cfgPath: filepath.Join(gameDir, gamestate.LANConfigFile),
		results: make(chan func(), 16), hover: -1, model: gamestate.LoginModels[0],
	}
	for _, f := range me {
		u.menu = append(u.menu, toEbiten(f))
	}
	u.menuBG = u.menu[0]
	for _, f := range play {
		u.play = append(u.play, toEbiten(f))
	}
	copy(u.msgText[:], exe.Messages[:])
	u.msgText[msgNameTaken] = [2][]byte{cp866("Ошибка: Игрок с таким именем уже в игре"), nil}
	u.msgText[msgBadHero] = [2][]byte{cp866("Ошибка: Сервер не принял нового героя"), nil}

	data, _ := os.ReadFile(u.cfgPath)
	u.cfg = gamestate.ParseLANConfig(data)
	u.name = gamestate.TextField{Text: u.cfg.Name, Max: gamestate.FieldName}
	u.pass = gamestate.TextField{Text: u.cfg.Password, Max: gamestate.FieldPassword}
	u.addr = gamestate.TextField{Max: gamestate.FieldAddress}
	u.clan = gamestate.TextField{Text: u.cfg.Clan, Max: gamestate.FieldClan}
	u.savePass = u.cfg.SavePassword
	u.gameDir = gameDir
	if newPath != "" {
		if err := u.loadNewHero(newPath, assets); err != nil {
			log.Printf("«Новый герой» unavailable: %v", err)
		}
	}
	return u, nil
}

// cp866 encodes s for the game fonts, dropping characters they lack.
func cp866(s string) []byte {
	var out []byte
	for _, r := range s {
		if c, ok := resfmt.RuneToCP866(r); ok {
			out = append(out, c)
		}
	}
	return out
}

// open shows the LAN menu.
func (u *lanUI) open() {
	u.state, u.msg, u.focus, u.leaveLAN = lanMenu, 0, 0, false
}

// openInGame is Escape in a LAN world: the LAN menu with «Вернуться в
// игру» and «Закончить игру»; the hero stays online.
func (u *lanUI) openInGame() {
	u.open()
	u.inGame, u.hover = true, -1
}

// escape handles Escape and reports whether the LAN screens are left.
// Escape (0x0042D90D): a text field only ends the edit; the message box and
// the wait close; the rating screen returns to the menu. In the LAN menu the
// original does nothing; the port goes back to its main menu.
func (u *lanUI) escape() bool {
	switch {
	case u.focus != 0:
		u.commitField()
	case u.msg != 0:
		u.msg = 0
	case u.state == lanWaiting:
		u.disconnect()
		u.state = lanLogin
	case u.state == lanRating, u.state == lanLogin, u.state == lanNewHero:
		u.toMenu()
	case u.inGame:
		u.Resume = true // back to the game
	default:
		u.disconnect()
		return true
	}
	return false
}

// update runs one frame and reports whether the LAN screens are left.
func (u *lanUI) update() bool {
	u.tick++
	u.applyResults()
	u.readSession()
	mx, my := ebiten.CursorPosition()
	click := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	switch u.state {
	case lanMenu:
		u.hover = gamestate.HitRects(u.exe.MenuRects, mx, my)
		if click && u.hover >= 0 {
			sfx.play(gamestate.SoundClick, 0, 0)
			u.menuItem(u.hover)
		}
	case lanRating:
		if click {
			u.state = lanMenu // any click (0x0042E384)
		}
	case lanLogin:
		u.updateLogin(mx, my, click)
	case lanNewHero:
		u.updateNewHero(mx, my, click)
	case lanWaiting:
		// The original waits here for the server to start the game
		// (state 0 → 10); the networked game is not ported yet.
	}
	if u.leaveLAN {
		u.disconnect()
	}
	return u.leaveLAN
}

func (u *lanUI) menuItem(item int) {
	if u.inGame {
		switch item {
		case lanItemRating: // «Вернуться в игру»
			u.Resume = true
			return
		case lanItemSettings: // «Закончить игру»: logout (0x44)
			u.endGame = true
			return
		}
		// «Новая игра», «Новый герой» and «Выход» end the game first.
		u.endGame = true
		u.disconnect()
	}
	switch item {
	case lanItemRating:
		u.state = lanRating
	case lanItemNewGame: // 0x0042E05E → state 11 → login
		u.enterLogin()
	case lanItemExit:
		u.leaveLAN = true
	case lanItemNewHero:
		u.openNewHero()
	case lanItemSettings:
		// «Установки» (state 8) is not ported yet.
	}
}

func (u *lanUI) toMenu() {
	u.commitField()
	u.disconnect()
	u.addr.Text = nil
	u.state, u.msg = lanMenu, 0
}

// enterLogin is state 11 (0x0042F727): the list refilled. The
// original first lists DirectPlay's connection types; the port has one (its
// own TCP/IP), so it searches the LAN at once.
func (u *lanUI) enterLogin() {
	u.state, u.msg, u.focus, u.scroll = lanLogin, 0, 0, 0
	u.found = nil
	u.discover("")
}

// discover searches the LAN, plus address when one is typed (Enter in the
// address field re-enumerates with it, 0x004300DC(connection, 1)).
func (u *lanUI) discover(address string) {
	if u.discovering {
		return
	}
	u.discovering = true
	go func() {
		var targets []string
		for _, b := range netplay.BroadcastAddrs() {
			targets = append(targets, net.JoinHostPort(b, strconv.Itoa(netplay.DefaultPort)))
		}
		if address != "" {
			if _, _, err := net.SplitHostPort(address); err != nil {
				address = net.JoinHostPort(address, strconv.Itoa(netplay.DefaultPort))
			}
			targets = append(targets, address)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		found, err := netplay.Discover(ctx, targets)
		if err != nil {
			log.Printf("LAN discovery: %v", err)
		}
		u.results <- func() {
			u.discovering = false
			u.found = found
		}
	}()
}

func (u *lanUI) applyResults() {
	for {
		select {
		case f := <-u.results:
			f()
		default:
			return
		}
	}
}

// readSession queues the world messages and notices a lost connection.
func (u *lanUI) readSession() {
	if u.client == nil {
		return
	}
	for {
		select {
		case m, ok := <-u.client.Incoming():
			if !ok {
				u.disconnect()
				if u.state == lanLogin || u.state == lanWaiting {
					u.state, u.msg = lanLogin, gamestate.MsgUnavailable
				}
				return
			}
			switch m.Type {
			case netplay.MsgView, netplay.MsgPlayer, netplay.MsgEvents, netplay.MsgMapChange, netplay.MsgHero,
				netplay.MsgInventory, netplay.MsgPiles, netplay.MsgPileWindow, netplay.MsgDialogue, netplay.MsgParty,
				netplay.MsgAmulets, netplay.MsgPlayers:
				u.worldMsgs = append(u.worldMsgs, m)
			case netplay.MsgClock:
				var c netplay.ClockPayload
				if m.Decode(&c) == nil {
					u.clock = &serverClock{c: c, at: time.Now()}
				}
			case netplay.MsgEntered:
				var at netplay.EnterPayload
				if m.Decode(&at) == nil && u.state == lanWaiting {
					u.start = &at
				}
			case netplay.MsgError:
				// The server cannot place the hero (e.g. no GAME.RES).
				var e netplay.ErrorPayload
				m.Decode(&e)
				log.Printf("LAN: %s", e.Message)
				if u.state == lanWaiting {
					u.state, u.msg = lanLogin, gamestate.MsgUnavailable
					if e.Code == netplay.ErrLeaving {
						u.msg = gamestate.MsgLeaving // «На сервере идет выход персонажа из игры»
					}
				}
			}
		default:
			return
		}
	}
}

func (u *lanUI) disconnect() {
	if u.client != nil {
		u.client.Close()
		u.client = nil
	}
	u.session, u.hero = "", nil
	u.worldMsgs, u.clock = nil, nil
}

// rows is the server list: the sessions found, by name as the original
// lists them; sessions sharing a name also show the host's address so they
// can be told apart. The original's row 0 «Новый сервер» (host from the
// game) is left out: sessions run on the dedicated server (cmd/konung-server).
func (u *lanUI) rows() [][]byte {
	var rows [][]byte
	count := map[string]int{}
	for _, f := range u.found {
		count[f.Session.Name]++
	}
	for _, f := range u.found {
		name := f.Session.Name
		if count[name] > 1 {
			host, _, _ := net.SplitHostPort(f.Addr)
			name += " " + host
		}
		rows = append(rows, cp866(name))
	}
	return rows
}

// clip shortens text to fit width pixels in font f.
func clip(f *gameFont, text []byte, width int) []byte {
	for len(text) > 0 && f.font.Width(text) > width {
		text = text[:len(text)-1]
	}
	return text
}

func (u *lanUI) updateLogin(mx, my int, click bool) {
	if u.focus != 0 {
		u.typeText()
	}
	if u.msg == gamestate.MsgOldHero {
		// «Старый персонаж будет удален!»: Ok (hotspot 2, 0x0042DBDB) joins
		// with the build; Отмена (3, 0x0042D9D8) drops it for the LAN menu.
		if !click {
			return
		}
		switch gamestate.HitRects(u.exe.MessageRects[2:4], mx, my) {
		case 0:
			sfx.play(gamestate.SoundClick, 0, 0)
			u.msg = 0
			u.join(u.pendingRow, u.pending)
		case 1:
			sfx.play(gamestate.SoundClick, 0, 0)
			u.pending = nil
			u.toMenu()
		}
		return
	}
	if u.msg != 0 {
		// One «Ok» closes the message box (hotspot 4 at (347,347)).
		if click && gamestate.HitRects(u.exe.MessageRects[4:5], mx, my) == 0 {
			sfx.play(gamestate.SoundClick, 0, 0)
			u.msg = 0
		}
		return
	}
	if !click {
		return
	}
	// A click first ends the edit (0x00423CA4).
	u.commitField()
	hit := gamestate.HitRects(u.exe.LoginRects, mx, my)
	if hit >= 0 {
		sfx.play(gamestate.SoundClick, 0, 0)
	}
	switch {
	case hit >= gamestate.LoginPortrait0 && hit < gamestate.LoginPortrait0+3:
		u.model = gamestate.LoginModels[hit-gamestate.LoginPortrait0]
	case hit == gamestate.LoginOk:
		u.ok()
	case hit == gamestate.LoginCancel: // menu, address cleared
		u.toMenu()
	case hit == gamestate.LoginName:
		u.focus = 2
	case hit == gamestate.LoginPassword:
		u.focus = 3
	case hit == gamestate.LoginAddress:
		u.focus = 6
		u.addr.Text = nil // the field shows the typed address while editing
	case hit == gamestate.LoginScrollUp:
		u.scroll = max(u.scroll-1, 0)
	case hit == gamestate.LoginScrollDown:
		if u.scroll+gamestate.LoginRows < len(u.rows()) {
			u.scroll++
		}
	case hit >= gamestate.LoginRow0 && hit < gamestate.LoginRow0+gamestate.LoginRows:
		u.pickRow(hit - gamestate.LoginRow0 + u.scroll)
	case hit == gamestate.LoginSavePass:
		u.savePass = !u.savePass
		u.saveConfig()
	}
}

// ok is «Ok» (hotspot 3): a name and a joined server are needed; then the
// client waits for the server to start the game.
func (u *lanUI) ok() {
	switch {
	case len(u.name.Text) == 0:
		u.msg = gamestate.MsgNoName
	case u.client == nil:
		u.msg = gamestate.MsgNoServer
	default:
		u.saveConfig()
		u.state = lanWaiting
		// The portrait picked now is the session's hero type (0x4C carries
		// the model); the server answers with the start (0x8C, readSession).
		if u.hero != nil {
			h := u.hero.WithModel(u.model)
			u.hero = &h
		}
		if err := u.client.Enter(u.model); err != nil {
			u.state, u.msg = lanLogin, gamestate.MsgUnavailable
		}
	}
}

// takeStart returns the server's start once («Ok» answered) and the
// account's hero (nil on a server without accounts).
func (u *lanUI) takeStart() (*netplay.EnterPayload, *netplay.Hero, string) {
	at := u.start
	u.start = nil
	return at, u.hero, u.player
}

// pickRow joins a listed session; it needs a name (message 4).
func (u *lanUI) pickRow(row int) {
	rows := u.rows()
	if row >= len(rows) || u.busy {
		return
	}
	if len(u.name.Text) == 0 {
		u.msg = gamestate.MsgNoName
		return
	}
	if u.pending != nil {
		// A build from «Новый герой» replaces the account's hero: warn
		// first (message 1, 0x0042ED8F).
		u.pendingRow, u.msg = row, gamestate.MsgOldHero
		return
	}
	u.join(row, nil)
}

// join logs in to server row, with a «Новый герой» build or without.
func (u *lanUI) join(row int, build *netplay.HeroChoice) {
	if row >= len(u.found) || u.busy {
		return
	}
	u.disconnect()
	u.saveConfig()
	join := netplay.JoinInfo{
		Name:     resfmt.CP866ToUTF8(u.name.Text),
		Password: resfmt.CP866ToUTF8(u.pass.Text),
		Clan:     resfmt.CP866ToUTF8(u.clan.Text),
		Model:    u.model,
		NewHero:  build,
	}
	u.busy = true
	addr := u.found[row].Addr
	go func() {
		c, err := netplay.Dial(addr, join, netplay.Options{})
		u.results <- func() { u.joined(c, err, build != nil) }
	}()
}

// joined applies a join result; refusals show the exe's messages (the login
// answer 0x0042062C: 2 version, 3 too many players, 7 wrong password; 5 when
// the server cannot be reached).
func (u *lanUI) joined(c *netplay.Client, err error, rebuilt bool) {
	u.busy = false
	if err != nil {
		log.Printf("LAN: %v", err)
		u.msg = gamestate.MsgUnavailable
		var je *netplay.JoinError
		if errors.As(err, &je) {
			switch je.Code {
			case netplay.ErrFull:
				u.msg = gamestate.MsgTooMany
			case netplay.ErrVersion:
				u.msg = gamestate.MsgVersion
			case netplay.ErrName:
				u.msg = msgNameTaken
			case netplay.ErrPassword:
				u.msg = gamestate.MsgWrongPass
			case netplay.ErrHero:
				u.msg, u.pending = msgBadHero, nil
			}
		}
		return
	}
	if u.state != lanLogin { // left the screen meanwhile
		c.Close()
		return
	}
	u.client = c
	u.session, u.hero = c.Session.Name, c.Hero
	if rebuilt {
		u.pending = nil // the server rebuilt the hero
	}
	if c.Hero != nil {
		u.Look = c.Hero.Look
	}
	for _, name := range c.Clans {
		u.addClan(cp866(name))
	}
	u.player = ""
	for _, p := range c.Session.Players {
		if p.ID == c.PlayerID {
			u.player = p.Name
		}
	}
}

// typeText edits the focused field (0x0042CB94): typed characters, Tab
// between name and password, Backspace, Enter to finish (Enter in the
// address field searches that address).
func (u *lanUI) typeText() {
	f, w := u.field()
	for _, r := range ebiten.AppendInputChars(nil) {
		if c, ok := resfmt.RuneToCP866(r); ok {
			f.Type(c, u.focusFnt.font.Width, w)
		}
	}
	if repeatKey(ebiten.KeyBackspace) {
		f.Backspace()
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyTab):
		if u.focus == 2 {
			u.focus = 3
		} else if u.focus == 3 {
			u.focus = 2
		}
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter), inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter):
		focus := u.focus
		u.commitField()
		if focus == 6 {
			u.disconnect()
			u.discover(string(u.addr.Text))
		}
	}
}

// repeatKey is a key press with auto-repeat.
func repeatKey(k ebiten.Key) bool {
	d := inpututil.KeyPressDuration(k)
	return d == 1 || d > 30 && d%4 == 0
}

func (u *lanUI) field() (*gamestate.TextField, int) {
	var f *gamestate.TextField
	var r gamestate.Rect
	switch u.focus {
	case 1:
		f, r = &u.clan, u.exe.NewHeroRects[gamestate.LANNewClan]
	case 2:
		f, r = &u.name, u.exe.LoginRects[gamestate.LoginName]
	case 3:
		f, r = &u.pass, u.exe.LoginRects[gamestate.LoginPassword]
	default:
		f, r = &u.addr, u.exe.LoginRects[gamestate.LoginAddress]
	}
	return f, r.X2 - r.X1
}

// commitField ends the edit and saves LAN.CFG (0x00423CA4).
func (u *lanUI) commitField() {
	if u.focus == 0 {
		return
	}
	u.focus = 0
	u.saveConfig()
}

func (u *lanUI) saveConfig() {
	u.cfg.Name, u.cfg.Password, u.cfg.SavePassword = u.name.Text, u.pass.Text, u.savePass
	u.cfg.Clan = u.clan.Text
	if u.focus != 6 && len(u.addr.Text) > 0 && u.session == "" {
		u.cfg.Address = u.addr.Text
	}
	if err := os.WriteFile(u.cfgPath, u.cfg.Marshal(), 0o644); err != nil {
		log.Printf("write %s: %v", u.cfgPath, err)
	}
}

func (u *lanUI) draw(dst *ebiten.Image) {
	switch u.state {
	case lanMenu:
		dst.DrawImage(u.menuBG, nil)
		frames := lanMenuFrames
		if u.inGame {
			frames = lanGameMenuFrames
		}
		for i, f := range frames {
			if i == u.hover {
				f++
			}
			drawAt(dst, u.menu[f], lanMenuLabelX, lanMenuLabelY)
		}
	case lanRating:
		// The rating rows come from the server (not ported): the frame only.
		dst.DrawImage(u.play[playRating], nil)
	case lanLogin, lanWaiting:
		u.drawLogin(dst)
	case lanNewHero:
		u.drawNewHero(dst)
	}
}

// drawLogin is 0x0042959C: the frame, portraits, buttons, texts at each
// hotspot's top-left +5,+5 (the password unmasked, as in the original),
// the list, the checkbox and the message box.
func (u *lanUI) drawLogin(dst *ebiten.Image) {
	R := u.exe.LoginRects
	dst.DrawImage(u.play[playLogin], nil)
	for i, m := range gamestate.LoginModels {
		f := playPortrait + 2*i + 1
		if m == u.model {
			f--
		}
		drawAt(dst, u.play[f], R[gamestate.LoginPortrait0+i].X1, R[gamestate.LoginPortrait0+i].Y1)
	}
	ok := playOk
	if u.state == lanWaiting { // «Ok» stays pressed while waiting
		ok++
	}
	drawAt(dst, u.play[ok], R[gamestate.LoginOk].X1, R[gamestate.LoginOk].Y1)
	drawAt(dst, u.play[playCancel], R[gamestate.LoginCancel].X1, R[gamestate.LoginCancel].Y1)

	text := func(focus int, t []byte, r gamestate.Rect) { u.fieldText(dst, focus, t, r) }
	text(2, u.name.Text, R[gamestate.LoginName])
	text(3, u.pass.Text, R[gamestate.LoginPassword])
	addr := u.addr.Text
	switch {
	case u.focus == 6:
	case u.busy:
		addr = cp866("...")
	case u.session != "":
		addr = cp866(u.session)
	}
	text(6, addr, R[gamestate.LoginAddress])
	rows := u.rows()
	for i := range gamestate.LoginRows {
		if k := u.scroll + i; k < len(rows) {
			r := R[gamestate.LoginRow0+i]
			u.font.drawLine(dst, clip(u.font, rows[k], r.X2-r.X1-10), r.X1+5, r.Y1+5)
		}
	}
	if u.discovering {
		r := R[gamestate.LoginRow0+min(len(rows)-u.scroll, gamestate.LoginRows-1)]
		u.font.drawLine(dst, cp866("Поиск серверов..."), r.X1+5, r.Y1+5)
	}
	if u.hero != nil {
		for i, v := range heroValues(u.hero) {
			if v != "" {
				// The anchors are points (zero-size hotspots); the text
				// starts at the point, inside the value box.
				r := R[gamestate.LoginParams+i]
				u.font.drawLine(dst, cp866(v), r.X1, r.Y1)
			}
		}
	}
	if u.savePass {
		r := R[gamestate.LoginSavePass]
		c := u.play[playCheck].Bounds()
		drawAt(dst, u.play[playCheck], (r.X1+r.X2+1-c.Dx())/2, (r.Y1+r.Y2+1-c.Dy())/2)
	}
	if u.msg != 0 {
		u.drawMessage(dst)
	}
}

// drawMessage is 0x00424DE4: PLAY frame 12 at (144,193), the text centred
// on x = 400 from y = 233, one «Ok» at (347,347).
func (u *lanUI) drawMessage(dst *ebiten.Image) {
	drawAt(dst, u.play[playMsgPanel], gamestate.MessageBoxX, gamestate.MessageBoxY)
	y := gamestate.MessageTextY
	for _, line := range u.msgText[u.msg] {
		if len(line) > 0 {
			u.focusFnt.drawLine(dst, line, 400-u.focusFnt.font.Width(line)/2, y)
			y += 2 * resfmt.LineHeight
		}
	}
	if u.msg == gamestate.MsgOldHero {
		r := u.exe.MessageRects[2]
		drawAt(dst, u.play[playOk], r.X1, r.Y1)
		r = u.exe.MessageRects[3]
		drawAt(dst, u.play[playCancel], r.X1, r.Y1)
		return
	}
	r := u.exe.MessageRects[4]
	drawAt(dst, u.play[playOk], r.X1, r.Y1)
}

// fieldText draws a text field's text at its hotspot's top-left +5,+5;
// the field being edited in the focus font with the blinking '_'.
func (u *lanUI) fieldText(dst *ebiten.Image, focus int, t []byte, r gamestate.Rect) {
	f := u.font
	if u.focus == focus {
		t = append([]byte(nil), t...)
		if u.tick/15%2 == 0 { // the '_' blinks with the tick counter
			t = append(t, '_')
		}
		f = u.focusFnt
	}
	f.drawLine(dst, clip(f, t, r.X2-r.X1-10), r.X1+5, r.Y1+5)
}

// heroValues are the login screen's stat texts after login (0x0042959C),
// at the 14 anchors: «Параметры» Харизма, Сила, Ловкость, Выносливость,
// Золото, Макс. груз, Макс. отряд; «Навыки» Фехтование, Меткость,
// Идентификация, Торговля, Кузнечное дело, Плотницкое дело, Знахарство.
// Фехтование and Меткость are stored ×100. «Макс. груз» and «Макс. отряд»
// are derived from Вын and Хар with the LAN tables (gamestate/lanhero.go).
func heroValues(h *netplay.Hero) [14]string {
	n := strconv.Itoa
	weight := fmt.Sprintf("%.1f", float64(gamestate.LANMaxWeight(h.Endurance))*0.001)
	return [14]string{
		n(h.Charisma), n(h.Strength), n(h.Dexterity), n(h.Endurance), n(h.Gold), weight, n(gamestate.LANMaxParty(byte(h.Class), h.Charisma)),
		n(h.Fencing / 100), n(h.Archery / 100), n(h.Identification), n(h.Trade), n(h.Smithing), n(h.Carpentry), n(h.Herbalism),
	}
}
