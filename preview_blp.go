//go:build ignore

package main

import (
	"image/png"
	"os"
	"path/filepath"

	"github.com/MorenoLand/Moreno.WoW/config"
	"github.com/MorenoLand/Moreno.WoW/ui"
)

func main() {
	options, _ := config.Load("")
	if options.InterfacePath == "" {
		options.InterfacePath = `G:\Development\Rust\Warcraft\Research\mpq-extract`
	}
	assets := filepath.Join(options.InterfacePath, "Interface-tree")
	rt := ui.NewRuntime(nil)
	loader := ui.NewLoader(assets, rt)

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
