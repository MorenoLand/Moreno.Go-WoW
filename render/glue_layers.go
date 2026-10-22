package render

import (
	"hash/maphash"
	"image"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/texture"
)

// glueLayerStack puts the glue UI around the 3D scene the way the WoW frame
// stack does. The back panel is opaque and drawn before the scene's opaque
// geometry without touching depth, so models draw over it; the front planes
// are drawn after the scene with the blend their textures asked for (normal,
// additive or multiply).
type glueLayerStack struct {
	backSum uint64
	backBuf *image.RGBA // the pixels the back texture was given
	scene   *core.Node
	front   *core.Node // holds the front planes; drawn on its own after the models in pass mode
	back    *gui.Image
	backTex *texture.Texture2D
	planes  []glueLayerPlane
	shown   int
}

type glueLayerPlane struct {
	image *gui.Image
	tex   *texture.Texture2D
	sum   uint64
	valid bool // tex holds the pixels summed in sum
	// buf is the pixel buffer the texture was handed. G3N uploads from it when
	// the texture is next drawn, not when it is set, and the UI recycles its
	// plane buffers between repaints (in a different order each time), so the
	// texture must own its pixels or a pending upload would read another
	// plane's.
	buf *image.RGBA
}

// copyLayer copies src into *buf (reallocating when the size changed) and
// returns it, so a texture can be pointed at pixels nothing else rewrites.
func copyLayer(buf **image.RGBA, src *image.RGBA) *image.RGBA {
	if *buf == nil || (*buf).Rect != src.Rect {
		*buf = image.NewRGBA(src.Rect)
	}
	copy((*buf).Pix, src.Pix)
	return *buf
}

var glueLayerSeed = maphash.MakeSeed()

// glueLayerSum is a fast checksum of a layer's pixels, used to skip uploading a
// full-window texture that did not change since the last repaint.
func glueLayerSum(img *image.RGBA) uint64 {
	return maphash.Bytes(glueLayerSeed, img.Pix)
}

func newGlueLayerStack(scene *core.Node) *glueLayerStack {
	front := core.NewNode()
	scene.Add(front)
	return &glueLayerStack{scene: scene, front: front}
}

func newGlueLayerImage() (*gui.Image, *texture.Texture2D) {
	tex := texture.NewTexture2DFromRGBA(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	img := gui.NewImageFromTex(tex)
	img.SetColor4(&math32.Color4{R: 1, G: 1, B: 1, A: 0})
	img.SetPosition(0, 0)
	return img, tex
}

func glueBlending(blend ui.GlueBlend) material.Blending {
	switch blend {
	case ui.GlueBlendAdd:
		return material.BlendAdditive
	case ui.GlueBlendMod:
		return material.BlendMultiply
	}
	return material.BlendNormal
}

// show uploads the layers and makes exactly them visible.
func (s *glueLayerStack) show(back *image.RGBA, planes []ui.GluePlane, width, height int) {
	if back == nil {
		if s.back != nil {
			s.back.SetVisible(false)
		}
	} else {
		if s.back == nil {
			s.back, s.backTex = newGlueLayerImage()
			// Opaque list, and no depth interaction: it is drawn before the
			// scene's opaque objects and they cover it where they exist.
			s.back.Material().SetTransparent(false)
			s.back.Material().SetDepthTest(false)
			s.back.Material().SetDepthMask(false)
			s.scene.Add(s.back)
		}
		if sum := glueLayerSum(back); sum != s.backSum || !s.back.Visible() {
			s.backTex.SetFromRGBA(copyLayer(&s.backBuf, back))
			s.backSum = sum
		}
		s.back.SetSize(float32(width), float32(height))
		s.back.SetVisible(true)
	}
	for len(s.planes) < len(planes) {
		img, tex := newGlueLayerImage()
		s.front.Add(img)
		s.planes = append(s.planes, glueLayerPlane{image: img, tex: tex})
	}
	for index := range s.planes {
		plane := &s.planes[index]
		if index >= len(planes) {
			plane.image.SetVisible(false)
			plane.valid = false
			continue
		}
		plane.image.Material().SetBlending(glueBlending(planes[index].Blend))
		if sum := glueLayerSum(planes[index].Image); !plane.valid || sum != plane.sum || !plane.image.Visible() {
			plane.tex.SetFromRGBA(copyLayer(&plane.buf, planes[index].Image))
			plane.sum, plane.valid = sum, true
		}
		plane.image.SetSize(float32(width), float32(height))
		plane.image.SetVisible(true)
	}
	s.shown = len(planes)
}

func (s *glueLayerStack) hide() {
	if s == nil || (s.shown == 0 && (s.back == nil || !s.back.Visible())) {
		return
	}
	if s.back != nil {
		s.back.SetVisible(false)
	}
	for _, plane := range s.planes {
		plane.image.SetVisible(false)
	}
	s.shown = 0
}
