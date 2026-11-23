package game

import (
	"bytes"
	_ "embed"
	"image"
	"image/draw"
	"image/png"
)

// The multiplayer entry is a port addition: the single-player konung.exe has
// no network game and MENU800R.RES no button for it. It opens the LAN
// version's screens (lanui.go), traced from LAN_KONG.EXE.

// The «Сетевая игра» label: two pictures in the menu's style (bronze, and
// bright copper under the mouse, like MENU800R.RES's pairs), supplied for
// the port and made to match the painted labels: cut out of their
// backgrounds with only a thin dark edge left of their rim (the painted
// letters are cut into the stone), their letters recoloured to the painted
// labels' mean and contrast per channel (normal to MENU800R.RES's normal
// frames, hover to the hover frames), turned 5.5° clockwise to +3°
// (between «Сохранить игру», +2.1°, in that place and «Загрузить игру»,
// +3.6°, below: the labels fan out downwards) and scaled to 171×51. They are
// embedded, so the browser build needs no extra file.
var (
	//go:embed art/network.png
	networkPNG []byte
	//go:embed art/network_hover.png
	networkHoverPNG []byte
)

// The label sits at the top of the menu stone, where «Сохранить игру» is
// drawn while a game runs (the item is shown only without a game), centred
// over «Загрузить игру» (x 265–488) and the stone's face.
const networkLabelX, networkLabelY = 293, 198

// networkButton returns the normal and selected «Сетевая игра» images.
func networkButton() ([2]*image.RGBA, error) {
	var out [2]*image.RGBA
	for k, data := range [][]byte{networkPNG, networkHoverPNG} {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return out, err
		}
		rgba := image.NewRGBA(img.Bounds())
		draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
		out[k] = rgba
	}
	return out, nil
}
