//go:build ignore

package main

import (
	"image/png"
	"os"

	"github.com/MorenoLand/Moreno.WoW/config"
	"github.com/MorenoLand/Moreno.WoW/ui"
)

func main() {
	configPath, err := config.Path()
	if err != nil {
		panic(err)
	}
	options, err := config.Load(configPath)
	if err != nil {
		panic(err)
	}
	engine, err := ui.LoadUIEngineFromMPQ(options.DataPath, options.Locale, options.BackgroundPath)
	if err != nil {
		panic(err)
	}
	defer engine.Close()
	rgba := engine.Render(1920, 1080)
	f, _ := os.Create("test.png")
	defer f.Close()
	png.Encode(f, rgba)
}
