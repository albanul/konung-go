package main

import (
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/konungmenu"
)

func main() {
	var resourcePath string
	if len(os.Args) > 1 {
		resourcePath = os.Args[1]
	} else {
		var err error
		resourcePath, err = konungmenu.FindGameResource("assets/Konung/MENU800R.RES")
		if err != nil {
			log.Fatal(err)
		}
	}
	graphPath, err := konungmenu.FindGameResource("assets/Konung/GRAPH.RES")
	if err != nil {
		log.Fatal(err)
	}
	g, err := konungmenu.New(resourcePath, graphPath)
	if err != nil {
		log.Fatal(err)
	}

	ebiten.SetFullscreen(true)
	ebiten.SetWindowSize(konungmenu.ScreenWidth, konungmenu.ScreenHeight)
	ebiten.SetWindowTitle("Князь: Легенды лесной страны")
	ebiten.SetCursorMode(ebiten.CursorModeHidden)
	if err := ebiten.RunGame(g); err != nil && err != ebiten.Termination {
		log.Fatal(err)
	}
}
