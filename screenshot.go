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
	options, _ := config.Load("config.json")
	if options.InterfacePath == "" {
	    options.InterfacePath = `G:\Development\Rust\Warcraft\Research\mpq-extract`
	}
	engine, err := ui.LoadUIEngine(
		filepath.Join(options.InterfacePath, "GlueXML"),
		filepath.Join(options.InterfacePath, "FrameXML"),
		filepath.Join(options.InterfacePath, "Interface-tree"),
	)
	if err != nil {
		panic(err)
	}
	defer engine.Close()
	rgba := engine.Render(1920, 1080)
	f, _ := os.Create("test.png")
	defer f.Close()
	png.Encode(f, rgba)
}
