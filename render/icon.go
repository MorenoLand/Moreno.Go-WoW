package render

import (
	"bytes"
	"image"
	"image/png"
	"log"

	"github.com/MorenoLand/Moreno.WoW/assets"
	"github.com/g3n/engine/window"
)

func installAppIcon(win *window.GlfwWindow) {
	icon, err := png.Decode(bytes.NewReader(assets.MorenoWoWPNG))
	if err != nil {
		log.Printf("icon: %v", err)
		return
	}
	win.Window.SetIcon([]image.Image{icon})
}
