package ui

import (
	"image"
	"strings"
)

// GlueBlend is how a front plane combines with what is already on screen.
type GlueBlend int

const (
	// GlueBlendNormal is ordinary alpha-over.
	GlueBlendNormal GlueBlend = iota
	// GlueBlendAdd adds the plane's colour to the scene (ADD textures).
	GlueBlendAdd
	// GlueBlendMod multiplies the scene by the plane's colour (MOD textures).
	GlueBlendMod
)

// GluePlane is one full-screen layer of the glue UI that is drawn over the 3D
// scene with the given blend.
type GluePlane struct {
	Image *image.RGBA
	Blend GlueBlend
}

// legacyFullCanvasPaint makes the painter take the slower full-canvas paths
// (fading and compositing the whole window for each faded texture, and always
// blending cached textures over). It exists so tests can check that the faster
// paths produce the same pixels.
var legacyFullCanvasPaint bool

// maxGluePlanes bounds how many front planes one frame may open; past it,
// widgets keep painting into the last plane.
const maxGluePlanes = 6

type glueLayerPlane struct {
	img   *image.RGBA
	blend GlueBlend
}

// fillBytes sets every byte of buf to value by doubling copies, which is much
// faster than a byte loop on a full-window canvas.
func fillBytes(buf []byte, value byte) {
	if len(buf) == 0 {
		return
	}
	buf[0] = value
	for filled := 1; filled < len(buf); filled *= 2 {
		copy(buf[filled:], buf[:filled])
	}
}

func (eng *UIEngine) isPlaneImage(img *image.RGBA) bool {
	for _, plane := range eng.planes {
		if plane.img == img {
			return true
		}
	}
	return false
}

// planeFor returns the plane widgets of the given blend class paint into,
// opening a new one when the class changes so paint order is preserved.
func (eng *UIEngine) planeFor(blend GlueBlend, bounds image.Rectangle) *glueLayerPlane {
	if eng.curPlane != nil && eng.curPlane.blend == blend {
		return eng.curPlane
	}
	if eng.curPlane != nil && len(eng.planes) >= maxGluePlanes {
		return eng.curPlane
	}
	var img *image.RGBA
	for len(eng.planePool) > 0 && img == nil {
		last := len(eng.planePool) - 1
		candidate := eng.planePool[last]
		eng.planePool = eng.planePool[:last]
		if candidate.Bounds().Eq(bounds) {
			img = candidate
		}
	}
	if img == nil {
		img = image.NewRGBA(bounds)
	}
	if blend == GlueBlendMod {
		// A multiply plane must be neutral (white) where nothing is painted.
		fillBytes(img.Pix, 255)
	} else {
		clear(img.Pix)
	}
	plane := &glueLayerPlane{img: img, blend: blend}
	eng.planes = append(eng.planes, plane)
	eng.curPlane = plane
	return plane
}

// frontBlend classifies a widget painted after the scene. Only plain texture
// regions of a frame (screen overlays such as the login scene's colour
// correction and light textures) get their own blended plane; widgets inside
// buttons and edit boxes keep painting with the software blend.
func frontBlend(w *widget) GlueBlend {
	if w.kind != kindTexture || w.parent == nil {
		return GlueBlendNormal
	}
	// A button's highlight texture (the character list's selection glow) adds
	// to the scene like any ADD texture; blending it over would show its dark
	// rim as an opaque box.
	if w.parent.highlightTexture != w && w.parent.kind != kindFrame {
		return GlueBlendNormal
	}
	switch {
	case strings.EqualFold(w.blendMode, "ADD") || strings.EqualFold(w.alphaMode, "ADD"):
		return GlueBlendAdd
	case strings.EqualFold(w.blendMode, "MOD") || strings.EqualFold(w.alphaMode, "MOD"):
		return GlueBlendMod
	}
	return GlueBlendNormal
}

// glueLayerPlaneEmpty reports whether nothing was painted into the plane.
func glueLayerPlaneEmpty(plane *glueLayerPlane) bool {
	pix := plane.img.Pix
	if plane.blend == GlueBlendMod {
		for _, value := range pix {
			if value != 255 {
				return false
			}
		}
		return true
	}
	if plane.blend == GlueBlendAdd {
		// Alpha was forced to 255; look at the colour.
		for index := 0; index+3 < len(pix); index += 4 {
			if pix[index] != 0 || pix[index+1] != 0 || pix[index+2] != 0 {
				return false
			}
		}
		return true
	}
	// Alpha 1 is the transparent-pixel fixup applied for scene backgrounds.
	for index := 3; index < len(pix); index += 4 {
		if pix[index] > 1 {
			return false
		}
	}
	return true
}
