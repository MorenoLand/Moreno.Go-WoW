//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/MorenoLand/Moreno.WoW/ui"
)

func main() {
	data, err := os.ReadFile(`G:\Development\Rust\Warcraft\Research\mpq-extract/Interface-tree\Interface\Glues\Common\Glue-Panel-Button-Up-Blue.blp`)
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
