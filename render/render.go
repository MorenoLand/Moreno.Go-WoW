package render

import (
	"github.com/g3n/engine/app"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/light"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/renderer"
	"github.com/g3n/engine/window"
	"time"
)

func Run() {
	a := app.App()
	scene := core.NewNode()
	gui.Manager().Set(scene)
	cam := camera.New(1)
	cam.SetPosition(0, 0, 3)
	scene.Add(cam)
	geom := geometry.NewTorus(1, .4, 12, 32, math32.Pi*2)
	mat := material.NewStandard(math32.NewColor("DarkBlue"))
	mesh := graphic.NewMesh(geom, mat)
	scene.Add(mesh)
	scene.Add(light.NewAmbient(&math32.Color{R: 1, G: 1, B: 1}, .8))
	point := light.NewPoint(&math32.Color{R: 1, G: 1, B: 1}, 5)
	point.SetPosition(1, 0, 2)
	scene.Add(point)
	onResize := func(evname string, ev interface{}) {
		width, height := a.GetSize()
		a.Gls().Viewport(0, 0, int32(width), int32(height))
		cam.SetAspect(float32(width) / float32(height))
	}
	a.Subscribe(window.OnWindowSize, onResize)
	onResize("", nil)
	a.Gls().ClearColor(.5, .5, .5, 1)
	a.Run(func(r *renderer.Renderer, dt time.Duration) {
		a.Gls().Clear(gls.DEPTH_BUFFER_BIT | gls.STENCIL_BUFFER_BIT | gls.COLOR_BUFFER_BIT)
		r.Render(scene, cam)
	})
}
