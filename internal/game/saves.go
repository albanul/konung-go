package game

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/gamestate"
	"konung1.5/internal/mapdata"
	"konung1.5/internal/resfmt"
)

// Saving and loading (docs/konung_save_format.md). The files sit in the
// game directory next to KONUNG.N, as in the original (0x0083DE58):
// KONUNG.SAV with the slot names and KONUNG.SA0 … KONUNG.SA9.

// saveSlots800 is the 800x600 half of the slot table 0x0046203C (32 bytes
// per slot: 640x480 x1,y1,x2,y2, then 800x600). The screen hit-tests it and
// draws each name with its top-left at (x1, y1) (0x004314E0).
var saveSlots800 = [gamestate.SaveSlots][4]int{
	{175, 86, 638, 125}, {175, 125, 638, 162}, {175, 164, 638, 200}, {175, 203, 638, 238},
	{175, 241, 638, 275}, {175, 280, 638, 313}, {175, 319, 638, 350}, {175, 358, 638, 388},
	{175, 396, 638, 425}, {175, 435, 638, 463},
}

const (
	screenLoad = 0 // state 4, SAVE800R.RES frame 0 «Загрузка игры» (slot 0x1FE)
	screenSave = 1 // state 5, frame 1 «Сохранение игры» (slot 0x1FF)
)

// saveScreen is the load/save screen (draw 0x004314E0, loops 0x00438966 and
// 0x004389F4). The slot under the mouse is selected (0x00A3E7AC) and drawn
// in the large font; when saving it shows the new save's name instead of
// its own. There is no name editing.
type saveScreen struct {
	bg       [2]*ebiten.Image
	selFont  *gameFont // 0x00592CFC
	font     *gameFont // 0x00593040
	dir      string
	names    [gamestate.SaveSlots][]byte // 0x00674640
	empty    []byte                      // «Пустая строка»
	mode     int
	selected int
	name     []byte // 0x0083DF5C, the new save's name
}

func newSaveScreen(resPath, dir string, t *resfmt.ExeTables, large *gameFont) (*saveScreen, error) {
	frames, err := resfmt.LoadType5File(resPath)
	if err != nil {
		return nil, fmt.Errorf("load %q: %w", resPath, err)
	}
	if len(frames) < 2 {
		return nil, fmt.Errorf("%s has %d frames; need 2", resPath, len(frames))
	}
	f, err := t.Font(resfmt.FontSlotVA)
	if err != nil {
		return nil, err
	}
	s := &saveScreen{
		bg:      [2]*ebiten.Image{ebiten.NewImageFromImage(frames[0]), ebiten.NewImageFromImage(frames[1])},
		selFont: large, font: newGameFont(f), dir: dir, empty: t.CString(gamestate.EmptySlotVA),
		selected: -1,
	}
	// 0x0043A969: read the names at startup; without the file every slot is empty.
	data, _ := os.ReadFile(filepath.Join(dir, gamestate.SlotNamesFile))
	s.names = gamestate.SlotNames(data, s.empty)
	return s, nil
}

// open shows the screen; name is the new save's name (save mode only).
func (s *saveScreen) open(mode int, name []byte) {
	s.mode, s.name, s.selected = mode, name, -1
}

// hit returns the slot under (x, y), or -1 (0x004398C4 on 0x0046203C).
func (s *saveScreen) hit(x, y int) int {
	for i, r := range saveSlots800 {
		if x >= r[0] && x < r[2] && y >= r[1] && y < r[3] {
			return i
		}
	}
	return -1
}

func (s *saveScreen) draw(dst *ebiten.Image) {
	dst.DrawImage(s.bg[s.mode], nil)
	for i, r := range saveSlots800 {
		font, name := s.font, s.names[i]
		if i == s.selected {
			font = s.selFont
			if s.mode == screenSave {
				name = s.name
			}
		}
		font.drawLine(dst, name, r[0], r[1])
	}
}

// save writes slot i (0x00438A2F): the slot takes the new name, then
// KONUNG.SAV and KONUNG.SA<i> are written (0x00424E34).
func (s *saveScreen) save(i int, w *startScene) error {
	data, err := w.game.MarshalSave(w.saveState())
	if err != nil {
		return err
	}
	s.names[i] = append([]byte(nil), s.name...)
	if err := os.WriteFile(filepath.Join(s.dir, gamestate.SlotNamesFile), gamestate.MarshalSlotNames(s.names), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, gamestate.SlotFile(i)), data, 0o644)
}

// saveName is the new save's name for the current map (0x00437A25).
func (w *startScene) saveName(now time.Time) []byte {
	return gamestate.SaveName(w.tables.CString(gamestate.LocationName(w.mapNumber)), now)
}

// saveState writes the scene back into the game (as for a map change) and
// returns what the slot holds besides it: the clock, the arrows in flight
// and the map's active piles with their image positions.
func (w *startScene) saveState() gamestate.SaveState {
	w.storeWorld()
	st := gamestate.SaveState{Time: w.gameTime(), Arrows: w.arrows}
	if len(st.Arrows) > 100 {
		st.Arrows = st.Arrows[:100]
	}
	for c, i := range w.pileAt {
		p := w.game.Piles[i]
		ap := gamestate.ActivePile{Pile: p, Image: w.game.PileImage(w.tables, p)}
		ap.X, ap.Y = mapdata.CellCenter(c)
		if img := w.hud.slot(ap.Image); img != nil {
			ap.X -= img.Bounds().Dx() / 2
			ap.Y -= img.Bounds().Dy() / 2
		}
		st.Active = append(st.Active, ap)
	}
	slices.SortFunc(st.Active, func(a, b gamestate.ActivePile) int {
		return (a.Row*1000 + a.Col) - (b.Row*1000 + b.Col)
	})
	if len(st.Active) > 200 {
		st.Active = st.Active[:200]
	}
	return st
}

// loadSaved reads slot i (0x004249AC): the hero's model picks GAME.<model>
// for the map exits, then the save replaces the rest, and the scene is
// built for group 0's map in restore mode.
func (a *sceneAssets) loadSaved(dir string, i int) (*startScene, error) {
	save, err := os.ReadFile(filepath.Join(dir, gamestate.SlotFile(i)))
	if err != nil {
		return nil, err
	}
	model, err := gamestate.SaveHeroModel(save)
	if err != nil {
		return nil, err
	}
	gameN, err := os.ReadFile(filepath.Join(a.mapDir, fmt.Sprintf("GAME.%d", model)))
	if err != nil {
		return nil, err
	}
	g, st, err := gamestate.ParseSave(save, gameN)
	if err != nil {
		return nil, err
	}
	return a.buildScene(g, g.Groups[0].Map, nil, &st)
}

// restoreSaved applies a save's clock, arrows and active piles to a freshly
// built scene. Record piles take the active copy's contents; runtime piles
// are added to the game.
func (w *startScene) restoreSaved(st gamestate.SaveState) {
	w.tick = st.Time * walkTicksPerStep
	w.arrows = append([]gamestate.Arrow(nil), st.Arrows...)
	for _, ap := range st.Active {
		p := ap.Pile
		p.Map = w.mapNumber
		idx := p.Index
		if idx >= 0 && idx < len(w.game.Piles) && w.game.Piles[idx].Index == idx {
			w.game.Piles[idx] = p
		} else {
			p.Index = -1
			w.game.Piles = append(w.game.Piles, p)
			idx = len(w.game.Piles) - 1
		}
		w.pileAt[mapdata.Cell{Row: p.Row, Col: p.Col}] = idx
	}
}

// readGameFile reads name from dir, matching the name case-insensitively
// (the game opens QUESTR.RES; the installed file is questr.RES).
func readGameFile(dir, name string) ([]byte, error) {
	if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
		return data, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) {
			return os.ReadFile(filepath.Join(dir, e.Name()))
		}
	}
	return nil, fmt.Errorf("%s not found in %s", name, dir)
}
