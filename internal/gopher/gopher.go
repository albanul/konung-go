package gopher

import (
	"github.com/oakmound/oak/v4/event"
	"github.com/oakmound/oak/v4/render"
	"github.com/oakmound/oak/v4/scene"
	"image"
	"log"
	"math/rand/v2"
)

const (
	minX = 0
	minY = 0
	maxX = 578
	maxY = 416
)

type Gopher struct {
	*render.Switch
	event.CallerID
	deltaX, deltaY float64
	rotation       int
	Cache          *[360]*image.RGBA
}

func NewGopher(ctx *scene.Context, layer int, cache *[360]*image.RGBA) {
	gopher := &Gopher{}

	gopher.Cache = cache

	modifiableMap := map[string]render.Modifiable{
		"goph": render.EmptyRenderable(),
	}

	gopher.Switch = render.NewSwitch("goph", modifiableMap)
	gopher.Switch.SetLayer(layer)
	gopher.Switch.SetPos(
		rand.Float64()*maxX,
		rand.Float64()*maxY,
	)

	gopher.CallerID = ctx.Register(gopher)
	event.Bind(ctx, event.Enter, gopher, onGopherEnter)
	gopher.deltaX = 4 * float64(rand.IntN(2)*2-1)
	gopher.deltaY = 4 * float64(rand.IntN(2)*2-1)
	gopher.rotation = rand.IntN(360)

	// todo ALBA: should layer be 0
	_, err := render.Draw(gopher.Switch, 0)
	if err != nil {
		log.Printf("[error]: can't render gopher(%v) on layer %v\n", gopher.CallerID, layer)
	}
}

func onGopherEnter(gopher *Gopher, event event.EnterPayload) event.Response {
	err := gopher.Add("goph", render.NewSprite(0, 0, gopher.Cache[gopher.rotation]))
	if err != nil {
		// todo
	}
	if gopher.X() < minX || gopher.X() > maxX {
		gopher.deltaX *= -1
	}
	if gopher.Y() < minY || gopher.Y() > maxY {
		gopher.deltaY *= -1
	}
	gopher.SetPos(gopher.deltaX+gopher.X(), gopher.deltaY+gopher.Y())
	gopher.rotation = (gopher.rotation + 1) % 360
	return 0
}
