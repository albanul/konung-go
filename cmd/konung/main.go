package main

import (
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	screenWidth  = 800
	screenHeight = 600
	menuItems    = 6
)

func main() {
	resourcePath, err := findGameResource("assets/Konung/MENU800R.RES")
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 {
		resourcePath = os.Args[1]
	}
	graphPath, err := findGameResource("assets/Konung/GRAPH.RES")
	if err != nil {
		log.Fatal(err)
	}
	g, err := loadMenu(resourcePath, graphPath)
	if err != nil {
		log.Fatal(err)
	}

	ebiten.SetFullscreen(true)
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Князь: Легенды лесной страны")
	ebiten.SetCursorMode(ebiten.CursorModeHidden)
	if err := ebiten.RunGame(g); err != nil && err != ebiten.Termination {
		log.Fatal(err)
	}
}
