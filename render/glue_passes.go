package render

import (
	"math"

	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/renderer"
)

// The original draws each plain Model frame inside the frame's own rectangle:
// it sets a viewport from the clamped, normalised frame rect, clears depth,
// draws the model with a camera whose aspect is that rectangle's, then restores
// the full viewport (Wow.exe FUN_0095fc30 -> FUN_00681f60 / FUN_006813b0).
// Frames with a small rectangle (a squished frame) therefore only show inside
// it, and the scene never spills outside the frame into the surrounding bars.
// Depth is cleared for every frame, so frames layer strictly in creation
// order (a later frame draws over an earlier one wherever they overlap)
// instead of being depth-sorted against each other.

type glueViewportPass struct {
	viewport [4]float64 // minX, maxX, minY, maxY as fractions of the window; may reach past [0,1]
	unit     *glueSceneUnit
}

// usesViewports reports whether every unit is a plain Model frame with a laid
// out rectangle, which is when the scene is drawn in viewport passes.
func (layout *glueSceneLayout) usesViewports() bool {
	if layout == nil || len(layout.units) == 0 {
		return false
	}
	for _, unit := range layout.units {
		if unit.applied.FFX || !unit.applied.HasViewport {
			return false
		}
	}
	return true
}

// viewportPasses is one pass per Model frame, in frame order.
func (layout *glueSceneLayout) viewportPasses() []glueViewportPass {
	passes := make([]glueViewportPass, 0, len(layout.units))
	for _, unit := range layout.units {
		passes = append(passes, glueViewportPass{viewport: unit.applied.Frame, unit: unit})
	}
	return passes
}

// renderGlueScene draws a frame that has a viewport-pass scene: the back layer,
// then the models pass by pass, then the front planes. It reports false when
// the scene is not drawable that way, leaving the caller to render normally.
func (s *glueLayerStack) renderGlueScene(r *renderer.Renderer, gl *gls.GLS, scene *core.Node, cam *camera.Camera, root *core.Node, diagFOV float32, width, height int) (bool, error) {
	layout := glueSceneLayoutOf(root)
	if s == nil || layout == nil || !layout.usesViewports() || width < 1 || height < 1 || diagFOV <= 0 {
		return false, nil
	}
	windowAspect := cam.Aspect()
	visible := make([]bool, len(layout.units))
	visibleBefore := make(map[*glueSceneUnit]bool, len(layout.units))
	for index, unit := range layout.units {
		visible[index] = unit.node.Visible()
		visibleBefore[unit] = visible[index]
	}
	rootVisible, frontVisible := root.Visible(), s.front.Visible()
	restore := func() {
		for index, unit := range layout.units {
			unit.node.SetVisible(visible[index])
		}
		root.SetVisible(rootVisible)
		s.front.SetVisible(frontVisible)
		gl.Viewport(0, 0, int32(width), int32(height))
		cam.SetAspect(windowAspect)
	}
	defer restore()

	// Back layer and everything else in the scene, without the models or the
	// front planes.
	root.SetVisible(false)
	s.front.SetVisible(false)
	if err := r.Render(scene, cam); err != nil {
		return true, err
	}

	root.SetVisible(true)
	for _, pass := range layout.viewportPasses() {
		if !visibleBefore[pass.unit] {
			continue
		}
		x0 := int32(math.Round(pass.viewport[0] * float64(width)))
		y0 := int32(math.Round(pass.viewport[2] * float64(height)))
		w := int32(math.Round((pass.viewport[1] - pass.viewport[0]) * float64(width)))
		h := int32(math.Round((pass.viewport[3] - pass.viewport[2]) * float64(height)))
		if w <= 0 || h <= 0 {
			continue
		}
		for index, unit := range layout.units {
			unit.node.SetVisible(unit == pass.unit && visible[index])
		}
		gl.Viewport(x0, y0, w, h)
		cam.SetAspect(float32(w) / float32(h))
		cam.SetFov(glueFrameFOV(diagFOV, float32(w)/float32(h)))
		// Colour is kept; only depth restarts, so frames layer in order.
		gl.Clear(gls.DEPTH_BUFFER_BIT)
		if err := r.Render(root, cam); err != nil {
			return true, err
		}
	}

	// Front planes over the finished scene, full viewport.
	root.SetVisible(false)
	s.front.SetVisible(true)
	gl.Viewport(0, 0, int32(width), int32(height))
	cam.SetAspect(windowAspect)
	setSceneCameraFOV(cam, diagFOV, 1)
	gl.Clear(gls.DEPTH_BUFFER_BIT)
	return true, r.Render(s.front, cam)
}

// glueFrameFOV is the vertical FOV in degrees for a Model frame: the camera's
// FOV divided by sqrt(1 + a^2), a being the aspect of the frame rectangle being
// drawn into (Wow.exe FUN_004becf0 -> FUN_006bfe00). It does not depend on the
// window, so the 16:9 login frame is framed the same in every window shape.
func glueFrameFOV(diagFOV, frameAspect float32) float32 {
	if frameAspect <= 0 {
		frameAspect = 1
	}
	return float32(float64(diagFOV) / math.Sqrt(float64(1+frameAspect*frameAspect)) * (180 / math.Pi))
}
