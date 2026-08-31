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
	rt := ui.NewRuntime(nil)
	loader, err := ui.NewMPQLoader(options.DataPath, options.Locale, rt)
	if err != nil {
		panic(err)
	}
	defer loader.Close()

	name := "Interface/Glues/MODELS/UI_MainMenu_Northrend/Login_SkyBowlA"
	data, err := loader.ReadAsset(name)
	if err != nil {
		panic(err)
	}
	img, err := ui.DecodeBLP(data)
	if err != nil {
		panic(err)
	}
	f, _ := os.Create("preview_sky.png")
	defer f.Close()
	png.Encode(f, img)
}
