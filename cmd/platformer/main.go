package main

import (
	"image/color"
	"log"
	"os"

	"github.com/oakmound/oak/v4"
	"github.com/oakmound/oak/v4/alg/floatgeom"
	"github.com/oakmound/oak/v4/collision"
	"github.com/oakmound/oak/v4/entities"
	"github.com/oakmound/oak/v4/event"
	"github.com/oakmound/oak/v4/key"
	"github.com/oakmound/oak/v4/render"
	"github.com/oakmound/oak/v4/scene"
)

const mainSceneName = "main"

const Ground collision.Label = 1

func main() {
	err := oak.AddScene(mainSceneName, scene.Scene{
		Start: mainSceneStart,
	})
	if err != nil {
		panic(err)
	}

	render.SetDrawStack(render.NewCompositeR())

	err = oak.Init(mainSceneName, func(config oak.Config) (oak.Config, error) {
		config.Fullscreen = true
		return config, nil
	})
	if err != nil {
		panic(err)
	}
}

func mainSceneStart(ctx *scene.Context) {
	_, err := render.Draw(render.NewDrawFPS(0.03, nil, 10, 10))
	if err != nil {
		log.Println("[error]: unable to draw fps", err)
	}

	event.GlobalBind(ctx, event.Enter, func(ev event.EnterPayload) event.Response {
		if oak.IsDown(key.Escape) {
			os.Exit(0)
		}

		return 0
	})

	// s, err := render.LoadSprite("/home/alba/projects/konung1.5/assets/images/gopher-logo.png")
	// if err != nil {
	// log.Fatalln("[fatal]: can't load gopher sprite", err)
	// }

	gopher := entities.New(
		ctx,
		// entities.WithRenderable(s.Modify(mod.Scale(1./40, 1./40))),
		// entities.WithPosition(floatgeom.Point2{100, 100}),
		entities.WithRect(floatgeom.NewRect2WH(100, 100, 16, 32)),
		entities.WithColor(color.RGBA{R: 255, A: 255}),
		entities.WithSpeed(floatgeom.Point2{3, 7}),
	)

	event.Bind(
		ctx,
		event.Enter,
		gopher,
		onGopherEnter,
	)

	platforms := []floatgeom.Rect2{
		floatgeom.NewRect2WH(0, 400, 300, 20),
		floatgeom.NewRect2WH(100, 250, 30, 20),
		floatgeom.NewRect2WH(340, 300, 100, 20),
	}

	for _, p := range platforms {
		entities.New(ctx,
			entities.WithRect(p),
			entities.WithColor(color.RGBA{B: 255, A: 255}),
			entities.WithLabel(Ground),
		)
	}
}

func onGopherEnter(entity *entities.Entity, eventPayload event.EnterPayload) event.Response {

	// moving left and right
	if oak.IsDown(key.RightArrow) {
		entity.Delta[0] = entity.Speed.X()
	} else if oak.IsDown(key.LeftArrow) {
		entity.Delta[0] = -entity.Speed.X()
	} else {
		entity.Delta[0] = 0
	}

	// gravity part
	fallSpeed := .2
	hit := collision.HitLabel(entity.Space, Ground)

	if hit == nil {
		entity.Delta[1] += fallSpeed
	} else {
		entity.Delta[1] = 0
	}

	jumping := false

	if oak.IsDown(key.UpArrow) && entity.Delta[1] == 0 {
		jumping = true
	} else {
		jumping = false
	}

	if jumping {
		entity.Delta[1] = -entity.Speed.Y()
	}

	entity.ShiftDelta()
	return 0
}
