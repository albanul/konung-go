package main

import (
	"github.com/oakmound/oak/v4"
	"github.com/oakmound/oak/v4/event"
	"github.com/oakmound/oak/v4/key"
	"github.com/oakmound/oak/v4/render"
	"github.com/oakmound/oak/v4/render/mod"
	"github.com/oakmound/oak/v4/scene"
	"image"
	"konung1.5/internal/gopher"
	"log"
	"os"
)

// this stores all the rotations of the gopher
var cache = [360]*image.RGBA{}

const mainSceneName = "main"

func main() {
	err := oak.AddScene(mainSceneName, scene.Scene{
		Start: firstSceneStart,
	})
	if err != nil {
		panic(err)
	}

	render.SetDrawStack(render.NewCompositeR())

	err = oak.Init(mainSceneName, func(config oak.Config) (oak.Config, error) {
		//config.Fullscreen = true
		return config, nil
	})
	if err != nil {
		panic(err)
	}
}

func firstSceneStart(ctx *scene.Context) {
	_, err := render.Draw(render.NewDrawFPS(0.03, nil, 10, 10))
	if err != nil {
		log.Println("[error]: unable to draw fps", err)
	}
	_, err = render.Draw(render.NewLogicFPS(0.03, nil, 10, 20))
	if err != nil {
		log.Println("[error]: unable to draw logic fps", err)
	}

	layer := 0
	layerTxt := render.DefaultFont().Copy().NewIntText(&layer, 30, 20)
	layerTxt.SetLayer(1000000)
	_, err = render.Draw(layerTxt, 0)
	if err != nil {
		log.Println("[error]: unable to draw layer text", err)
	}

	event.GlobalBind(ctx, event.Enter, func(ev event.EnterPayload) event.Response {
		if oak.IsDown(key.K) {
			gopher.NewGopher(ctx, layer, &cache)
			layer++
		}

		if oak.IsDown(key.Escape) {
			os.Exit(0)
		}

		return 0
	})

	for i := 0; i < 360; i++ {
		//assetPath := filepath.Join("..", "..", "assets", "images", "gopher11.png")
		sprite, err := render.LoadSprite("/home/alba/projects/konung1.5/assets/images/gopher-logo.png")
		if err != nil {
			log.Fatalln("[fatal]: unable to load gopher sprite")
		}

		sprite.Modify(mod.Rotate(float32(i))).Modify(mod.Scale(1.0/30, 1.0/30))
		cache[i] = sprite.GetRGBA()
	}
}
