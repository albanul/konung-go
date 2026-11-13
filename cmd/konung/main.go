// Command konung is the game.
//
//	konung [MENU800R.RES]
package main

import (
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/game"
)

func main() {
	var resourcePath string
	if len(os.Args) > 1 {
		resourcePath = os.Args[1]
	} else {
		var err error
		resourcePath, err = game.FindGameResource("assets/Konung/MENU800R.RES")
		if err != nil {
			log.Fatal(err)
		}
	}
	graphPath, err := game.FindGameResource("assets/Konung/GRAPH.RES")
	if err != nil {
		log.Fatal(err)
	}
	g, err := game.New(resourcePath, graphPath)
	if err != nil {
		log.Fatal(err)
	}

	ebiten.SetFullscreen(true)
	ebiten.SetWindowSize(game.ScreenWidth, game.ScreenHeight)
	ebiten.SetWindowTitle("Князь: Легенды лесной страны")
	ebiten.SetCursorMode(ebiten.CursorModeHidden)
	if err := ebiten.RunGame(g); err != nil && err != ebiten.Termination {
		log.Fatal(err)
	}
}
