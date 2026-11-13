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
// login screen «Новая игра» (5) with its message box and the wait for the
// server to start the game (0).
const (
	lanMenu = iota
	lanRating
	lanLogin
	lanWaiting
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

// lanUI runs the LAN screens. Network work (discovery, hosting, joining)
// runs in goroutines that hand their results back through results; Update
// applies them, so all UI state stays on the game goroutine.
type lanUI struct {
	exe      *gamestate.LANExe
	menuBG   *ebiten.Image
	menu     []*ebiten.Image // ME800R frames
	play     []*ebiten.Image // PLAY800R frames
	font     *gameFont       // text (the LAN exe's own fonts are not mapped; the save screen's pair stands in)
	focusFnt *gameFont       // the focused field
	msgText  [10][2][]byte   // message box lines by id

	state, hover int
	msg          int // message box id, 0 = none

	// Login fields; focus is the exe's [0x00A6EAE4]: 0 none, 2 name,
	// 3 password, 6 address.
	focus             int
	name, pass, addr  gamestate.TextField
	savePass          bool
	model             int // hero model of the selected portrait
	scroll            int
	found             []netplay.Found
	discovering, busy bool
	tick              int

	cfg     gamestate.LANConfig
	cfgPath string
	server  gamestate.ServerFile

	client   *netplay.Client
	host     *netplay.Server
	session  string // joined session's name
	players  int
	results  chan func()
	leaveLAN bool // «Выход»: back to the port's main menu
}

func newLANUI(gameDir string, assets *sceneAssets, mePath, playPath string) (*lanUI, error) {
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

	// SERVER names a hosted session (0x00431481); without it the exe's
	// default name.
	u.server = gamestate.ServerFile{Name: gamestate.DefaultServerName, Minutes: 180}
	if data, err := os.ReadFile(filepath.Join(gameDir, "SERVER")); err == nil {
		if f, err := gamestate.ParseServerFile(data); err == nil {
			u.server = f
		} else {
			log.Printf("SERVER: %v", err)
		}
	}
	data, _ := os.ReadFile(u.cfgPath)
	u.cfg = gamestate.ParseLANConfig(data)
	u.name = gamestate.TextField{Text: u.cfg.Name, Max: gamestate.FieldName}
	u.pass = gamestate.TextField{Text: u.cfg.Password, Max: gamestate.FieldPassword}
	u.addr = gamestate.TextField{Max: gamestate.FieldAddress}
	u.savePass = u.cfg.SavePassword
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
	case u.state == lanRating, u.state == lanLogin:
		u.toMenu()
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
	switch item {
	case lanItemRating:
		u.state = lanRating
	case lanItemNewGame: // 0x0042E05E → state 11 → login
		u.enterLogin()
	case lanItemExit:
		u.leaveLAN = true
	case lanItemSettings, lanItemNewHero:
		// «Установки» (state 8) and «Новый герой» (state 4) are not ported yet.
	}
}

func (u *lanUI) toMenu() {
	u.commitField()
	u.disconnect()
	u.addr.Text = nil
	u.state, u.msg = lanMenu, 0
}

// enterLogin is state 11 (0x0042F727): not hosting, the list refilled. The
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

// readSession follows the joined session's player count (the " [n]" after
// the address) and notices a lost connection.
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
			case netplay.MsgPlayerJoined:
				u.players++
			case netplay.MsgPlayerLeft:
				u.players--
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
	if u.host != nil {
		u.host.Close()
		u.host = nil
	}
	u.session, u.players = "", 0
}

// rows is the server list: «Новый сервер», then the sessions found by
// name, as the original lists them; sessions sharing a name also show the
// host's address so they can be told apart.
func (u *lanUI) rows() [][]byte {
	rows := [][]byte{u.exe.NewServer}
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
	}
}

// pickRow hosts (row 0, «Новый сервер») or joins a session; both need a
// name (message 4).
func (u *lanUI) pickRow(row int) {
	rows := u.rows()
	if row >= len(rows) || u.busy {
		return
	}
	if len(u.name.Text) == 0 {
		u.msg = gamestate.MsgNoName
		return
	}
	u.disconnect()
	u.saveConfig()
	join := netplay.JoinInfo{
		Name:     resfmt.CP866ToUTF8(u.name.Text),
		Password: resfmt.CP866ToUTF8(u.pass.Text),
		Clan:     resfmt.CP866ToUTF8(u.cfg.Clan),
		Model:    u.model,
	}
	u.busy = true
	if row == 0 {
		cfg := netplay.ServerConfig{Name: resfmt.CP866ToUTF8(u.server.Name), Minutes: u.server.Minutes}
		go func() {
			s, c, err := netplay.Host(netplay.DefaultPort, cfg, join, netplay.Options{})
			u.results <- func() { u.joined(s, c, err) }
		}()
		return
	}
	addr := u.found[row-1].Addr
	go func() {
		c, err := netplay.Dial(addr, join, netplay.Options{})
		u.results <- func() { u.joined(nil, c, err) }
	}()
}

// joined applies a host or join result; refusals show the exe's messages
// (the login answer 0x0042062C: 2 version, 3 too many players; 5 when the
// server cannot be reached).
func (u *lanUI) joined(s *netplay.Server, c *netplay.Client, err error) {
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
			}
		}
		return
	}
	if u.state != lanLogin { // left the screen meanwhile
		c.Close()
		if s != nil {
			s.Close()
		}
		return
	}
	u.host, u.client = s, c
	u.session, u.players = c.Session.Name, len(c.Session.Players)
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
		for i, f := range lanMenuFrames {
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

	cursor := []byte{}
	if u.tick/15%2 == 0 { // the '_' blinks with the tick counter
		cursor = []byte{'_'}
	}
	text := func(focus int, t []byte, r gamestate.Rect) {
		f := u.font
		if u.focus == focus {
			f, t = u.focusFnt, append(append([]byte(nil), t...), cursor...)
		}
		f.drawLine(dst, clip(f, t, r.X2-r.X1-10), r.X1+5, r.Y1+5)
	}
	text(2, u.name.Text, R[gamestate.LoginName])
	text(3, u.pass.Text, R[gamestate.LoginPassword])
	addr := u.addr.Text
	switch {
	case u.focus == 6:
	case u.busy:
		addr = cp866("...")
	case u.session != "":
		addr = cp866(fmt.Sprintf("%s [%d]", u.session, u.players))
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
	r := u.exe.MessageRects[4]
	drawAt(dst, u.play[playOk], r.X1, r.Y1)
}
