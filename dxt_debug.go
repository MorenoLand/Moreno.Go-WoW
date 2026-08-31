//go:build ignore

package main

import (
	"fmt"
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
	data, err := loader.ReadAsset("Interface\\Glues\\Common\\Glue-Panel-Button-Up-Blue")
	if err != nil {
		panic(err)
	}
	img, err := ui.DecodeBLP(data)
	if err != nil {
		panic(err)
	}
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			fmt.Printf("%02x ", a>>8)
		}
		fmt.Println()
	}
}
