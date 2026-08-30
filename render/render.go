package render

import (
	"fmt"
	"log"
	"os"
	"os/signal"

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

	"github.com/MorenoLand/Moreno.WoW/network"
	"github.com/MorenoLand/Moreno.WoW/world"
	"time"
)

func Run(config network.Config, dataPath string, debug bool) {
	a := app.App()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	done := make(chan struct{})
	defer func() {
		signal.Stop(signals)
		close(done)
	}()
	go func() {
		select {
		case <-signals:
			if debug {
				log.Print("shutdown: Ctrl+C received")
			}
			a.Exit()
		case <-done:
		}
	}()
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
	title := gui.NewLabel("MorenoWoW")
	title.SetFontSize(24)
	title.SetPosition(24, 24)
	scene.Add(title)
	status := gui.NewLabel("Preparing network session...")
	status.SetPosition(24, 66)
	scene.Add(status)
	realmLabel := gui.NewLabel("")
	realmLabel.SetPosition(24, 94)
	scene.Add(realmLabel)
	if dataPath == "" {
		dataPath = "not configured"
	}
	dataLabel := gui.NewLabel(fmt.Sprintf("MPQ data: %s", dataPath))
	dataLabel.SetPosition(24, 122)
	scene.Add(dataLabel)
	charactersLabel := gui.NewLabel("Characters")
	charactersLabel.SetFontSize(18)
	charactersLabel.SetPosition(24, 154)
	scene.Add(charactersLabel)
	resultChannel := make(chan *network.Session, 1)
	errorChannel := make(chan error, 1)
	entryChannel := make(chan world.WorldPosition, 1)
	entryErrorChannel := make(chan error, 1)
	if config.Account == "" || config.Password == "" {
		status.SetText("Set WOW_ACCOUNT and WOW_PASSWORD, then run again.")
	} else {
		status.SetText("Authenticating...")
		go func() {
			result, err := network.Login(config)
			if err != nil {
				errorChannel <- err
				return
			}
			resultChannel <- result
		}()
	}
	a.Run(func(r *renderer.Renderer, dt time.Duration) {
		select {
		case result := <-resultChannel:
			status.SetText(fmt.Sprintf("Connected to %s", result.Realm.Name))
			realmLabel.SetText(fmt.Sprintf("Realm: %s  Characters: %d", result.Realm.Name, len(result.Characters)))
			if len(result.Characters) == 0 {
				charactersLabel.SetText("Characters\nNo characters returned")
			} else {
				for i, character := range result.Characters {
					index := i
					row := gui.NewButton(fmt.Sprintf("%s  Level %d  %s %s", character.Name, character.Level, world.RaceName(character.Race), world.ClassName(character.Class)))
					row.SetPosition(40, float32(188+i*28))
					row.Subscribe(gui.OnClick, func(name string, ev interface{}) {
						status.SetText(fmt.Sprintf("Entering %s...", result.Characters[index].Name))
						if debug {
							log.Printf("world: entering character %q", result.Characters[index].Name)
						}
						go func() {
							position, err := result.EnterWorld(index)
							if err != nil {
								entryErrorChannel <- err
								return
							}
							entryChannel <- position
						}()
					})
					scene.Add(row)
				}
			}
		case err := <-errorChannel:
			status.SetText(fmt.Sprintf("Login failed: %s", err))
		case position := <-entryChannel:
			status.SetText(fmt.Sprintf("Entered map %d at %.2f, %.2f, %.2f", position.Map, position.X, position.Y, position.Z))
			if debug {
				log.Printf("world: entered map %d at %.2f, %.2f, %.2f", position.Map, position.X, position.Y, position.Z)
			}
		case err := <-entryErrorChannel:
			status.SetText(fmt.Sprintf("Character login failed: %s", err))
		default:
		}
		a.Gls().Clear(gls.DEPTH_BUFFER_BIT | gls.STENCIL_BUFFER_BIT | gls.COLOR_BUFFER_BIT)
		r.Render(scene, cam)
	})
}
