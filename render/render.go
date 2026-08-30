package render

import (
	"log"
	"path/filepath"

	"github.com/MorenoLand/Moreno.WoW/network"
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/renderer"
	"github.com/g3n/engine/texture"
	"github.com/g3n/engine/window"
)

func Run(clientConfig network.Config, dataPath, interfacePath, lastCharacter, configPath string, debug bool) {
	if err := window.Init(960, 640, "MorenoWoW"); err != nil {
		log.Printf("window: %v", err)
		return
	}
	win, ok := window.Get().(*window.GlfwWindow)
	if !ok {
		log.Print("window: unsupported window implementation")
		return
	}
	defer win.Destroy()
	gl := win.Gls()
	r := renderer.NewRenderer(gl)
	if err := r.AddDefaultShaders(); err != nil {
		log.Printf("renderer: %v", err)
		return
	}
	scene := core.NewNode()
	gui.Manager().Set(scene)
	
	var uiEngine *ui.UIEngine
	var uiImage *gui.Image
	if interfacePath != "" {
		glue := filepath.Join(interfacePath, "GlueXML")
		frame := filepath.Join(interfacePath, "FrameXML")
		assets := filepath.Join(interfacePath, "Interface-tree")
		eng, err := ui.LoadUIEngine(glue, frame, assets)
		if err == nil {
			uiEngine = eng
			uiImage = gui.NewImageFromRGBA(eng.Render(960, 640))
			uiImage.SetPosition(0, 0)
			scene.Add(uiImage)
		} else {
			log.Printf("ui render error: %v", err)
		}
	}
	
	cam := camera.New(1)
	cam.SetPosition(0, 0, 3)
	scene.Add(cam)
	
	onResize := func(name string, event interface{}) {
		width, height := win.GetSize()
		gl.Viewport(0, 0, int32(width), int32(height))
		if height > 0 {
			cam.SetAspect(float32(width) / float32(height))
		}
		if uiImage != nil && uiEngine != nil {
			rgba := uiEngine.Render(width, height)
			tex := texture.NewTexture2DFromRGBA(rgba)
			uiImage.SetTexture(tex)
			uiImage.SetSize(float32(width), float32(height))
		}
	}
	win.Subscribe(window.OnWindowSize, onResize)
	onResize("", nil)
	gl.ClearColor(.04, .06, .1, 1)

	for !win.ShouldClose() {
		gl.Clear(gls.DEPTH_BUFFER_BIT | gls.STENCIL_BUFFER_BIT | gls.COLOR_BUFFER_BIT)
		_ = r.Render(scene, cam)
		win.SwapBuffers()
		win.PollEvents()
	}
}
