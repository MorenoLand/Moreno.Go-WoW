package main

import (
	"image/png"
	"os"

	"github.com/MorenoLand/Moreno.WoW/config"
	"github.com/MorenoLand/Moreno.WoW/ui"
)

func main() {
	options, _ := config.Load("config.json")
	if options.InterfacePath == "" {
	    options.InterfacePath = `G:\Development\Rust\Warcraft\Research\mpq-extract`
	}
	rgba, err := ui.SoftwareRenderLogin(
		options.InterfacePath + `\GlueXML`,
		options.InterfacePath + `\FrameXML`,
		options.InterfacePath + `\Interface-tree`,
	)
	if err != nil {
		panic(err)
	}
	f, _ := os.Create("test.png")
	defer f.Close()
	png.Encode(f, rgba)
}
