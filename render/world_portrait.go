package render

import (
	"image"
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/renderer"
)

// worldPortraitSize is the side of a rendered portrait, the size of
// Interface\CharacterFrame\TempPortraitAlphaMask.blp.
const worldPortraitSize = 128

// worldPortraits renders the portrait of a creature display: the model seen
// through its own portrait camera (the first camera of its M2) over black, with
// its alpha taken from TempPortraitAlphaMask as PortraitButton.cpp does
// (FUN_00619580).
type worldPortraits struct {
	images  map[uint32]*image.RGBA
	queued  map[uint32]bool
	pending []uint32
	mask    []uint8
}

func newWorldPortraits() *worldPortraits {
	return &worldPortraits{images: make(map[uint32]*image.RGBA), queued: make(map[uint32]bool)}
}

// source is the portrait of a display, nil until it has been rendered; asking
// queues it.
func (p *worldPortraits) source(displayID uint32) image.Image {
	if p == nil || displayID == 0 {
		return nil
	}
	if img := p.images[displayID]; img != nil {
		return img
	}
	if !p.queued[displayID] {
		p.queued[displayID] = true
		p.pending = append(p.pending, displayID)
	}
	return nil
}

func (p *worldPortraits) loadMask(loader *ui.Loader) {
	if p.mask != nil {
		return
	}
	p.mask = make([]uint8, worldPortraitSize*worldPortraitSize)
	for i := range p.mask {
		p.mask[i] = 255
	}
	data, err := loader.ReadAsset(`Interface\CharacterFrame\TempPortraitAlphaMask.blp`)
	if err != nil {
		return
	}
	img, err := ui.DecodeBLP(data)
	if err != nil {
		return
	}
	bounds := img.Bounds()
	for y := 0; y < worldPortraitSize; y++ {
		for x := 0; x < worldPortraitSize; x++ {
			_, _, _, a := img.At(bounds.Min.X+x*bounds.Dx()/worldPortraitSize, bounds.Min.Y+y*bounds.Dy()/worldPortraitSize).RGBA()
			p.mask[y*worldPortraitSize+x] = uint8(a >> 8)
		}
	}
}

// process renders one queued portrait, then tells the interface which units
// show it.
func (p *worldPortraits) process(rt *ui.Runtime, loader *ui.Loader, tables *worldCreatureTables, r *renderer.Renderer, gl *gls.GLS, width, height int) {
	if p == nil || len(p.pending) == 0 || rt == nil || loader == nil || tables == nil {
		return
	}
	displayID := p.pending[0]
	p.pending = p.pending[1:]
	img := p.render(loader, tables, r, gl, displayID)
	gl.Viewport(0, 0, int32(width), int32(height))
	gl.ClearColor(0, 0, 0, 1)
	if img == nil {
		return
	}
	p.images[displayID] = img
	rt.PortraitReady(displayID)
}

func (p *worldPortraits) render(loader *ui.Loader, tables *worldCreatureTables, r *renderer.Renderer, gl *gls.GLS, displayID uint32) *image.RGBA {
	definition, err := tables.definition(loader, displayID, 0)
	if err != nil {
		return nil
	}
	model, err := buildWorldCreatureModel(loader, definition)
	if err != nil || model == nil {
		return nil
	}
	info, ok := model.UserData().(glueModelInfo)
	if !ok || !info.hasPortrait {
		return nil
	}
	root := wrapWorldModel(model, world.WorldPosition{}, 1)
	defer root.Dispose()
	if info.animation != nil {
		info.animation.Update(0.01)
	}
	offset := model.Position()
	position := math32.NewVector3(info.portrait.position[0]+offset.X, info.portrait.position[1]+offset.Y, info.portrait.position[2]+offset.Z)
	target := math32.NewVector3(info.portrait.target[0]+offset.X, info.portrait.target[1]+offset.Y, info.portrait.target[2]+offset.Z)
	scene := core.NewNode()
	scene.Add(root)
	cam := camera.New(1)
	cam.SetAspect(1)
	fov := info.portrait.fov
	if fov <= 0 {
		fov = 0.95
	}
	// The camera's field of view is the diagonal of its picture.
	cam.SetFov(math32.RadToDeg(fov / float32(math.Sqrt2)))
	near, far := info.portrait.nearClip, info.portrait.farClip
	if near <= 0 {
		near = 0.05
	}
	if far <= near {
		far = 100
	}
	cam.SetNear(near)
	cam.SetFar(far)
	cam.SetPosition(position.X, position.Y, position.Z)
	cam.LookAt(target, math32.NewVector3(0, 0, 1))
	// A portrait is lit from the front, not by the sky of the place it is taken.
	saved := worldAtmo
	defer func() { worldAtmo = saved }()
	worldAtmo = worldAtmosphere{valid: true, ambient: [3]float32{0.55, 0.55, 0.55}, diffuse: [3]float32{0.6, 0.6, 0.6}, direction: [3]float32{0.7, 0.3, 0.65}}
	gl.Viewport(0, 0, worldPortraitSize, worldPortraitSize)
	gl.ClearColor(0, 0, 0, 1)
	gl.Clear(gls.DEPTH_BUFFER_BIT | gls.STENCIL_BUFFER_BIT | gls.COLOR_BUFFER_BIT)
	if err := r.Render(scene, cam); err != nil {
		return nil
	}
	pixels := gl.ReadPixels(0, 0, worldPortraitSize, worldPortraitSize, gls.RGBA, gls.UNSIGNED_BYTE)
	p.loadMask(loader)
	img := image.NewRGBA(image.Rect(0, 0, worldPortraitSize, worldPortraitSize))
	for y := 0; y < worldPortraitSize; y++ {
		for x := 0; x < worldPortraitSize; x++ {
			from := ((worldPortraitSize-1-y)*worldPortraitSize + x) * 4
			to := (y*worldPortraitSize + x) * 4
			alpha := uint32(p.mask[y*worldPortraitSize+x])
			img.Pix[to] = uint8(uint32(pixels[from]) * alpha / 255)
			img.Pix[to+1] = uint8(uint32(pixels[from+1]) * alpha / 255)
			img.Pix[to+2] = uint8(uint32(pixels[from+2]) * alpha / 255)
			img.Pix[to+3] = uint8(alpha)
		}
	}
	return img
}
