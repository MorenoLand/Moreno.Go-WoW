package render

import (
	"image"
	"time"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/renderer"
)

// worldIndoorMinimapSize is the side of the picture of the rooms the minimap shows.
const worldIndoorMinimapSize = 256

// worldIndoorMinimap draws the rooms around the player as seen from above, with
// what is over the player's head left out, for the minimap inside a building.
type worldIndoorMinimap struct {
	lastAt     time.Time
	lastX      float32
	lastY      float32
	lastZoom   int
	lastFacing float32
	cam        *camera.Camera
}

// worldIndoorRadius is how far the picture reaches from the player (yards) at
// each of the minimap's zoom levels.
func worldIndoorRadius(zoom int) float32 {
	levels := ui.MinimapZoomLevels()
	if zoom < 0 {
		zoom = 0
	}
	if zoom >= levels {
		zoom = levels - 1
	}
	return 60 - float32(zoom)*8
}

// update redraws the picture when the player has moved or the zoom changed and
// reports whether the minimap changed.
func (m *worldIndoorMinimap) update(now time.Time, r *renderer.Renderer, gl *gls.GLS, root *core.Node, engine *ui.UIEngine, position [3]float32, viewWidth, viewHeight int) bool {
	zoom := engine.MinimapZoom()
	moved := (position[0]-m.lastX)*(position[0]-m.lastX)+(position[1]-m.lastY)*(position[1]-m.lastY) > 0.25
	if engine.MinimapIndoor() && !moved && zoom == m.lastZoom && now.Sub(m.lastAt) < time.Second {
		return false
	}
	if now.Sub(m.lastAt) < 100*time.Millisecond && zoom == m.lastZoom {
		return false
	}
	m.lastAt, m.lastX, m.lastY, m.lastZoom = now, position[0], position[1], zoom
	view := m.render(r, gl, root, position, worldIndoorRadius(zoom))
	gl.Viewport(0, 0, int32(viewWidth), int32(viewHeight))
	if view == nil {
		return false
	}
	return engine.SetWorldMinimapIndoor(view)
}

func (m *worldIndoorMinimap) render(r *renderer.Renderer, gl *gls.GLS, root *core.Node, position [3]float32, radius float32) *image.RGBA {
	if m.cam == nil {
		m.cam = camera.NewOrthographic(1, 0.05, 200, 2*radius, camera.Vertical)
	}
	cam := m.cam
	cam.SetSize(2 * radius)
	// The view looks down from just above the player's head: what is higher than
	// that is behind the camera and is not drawn, so the ceiling does not hide
	// the room.
	cam.SetPosition(position[0], position[1], position[2]+2.6)
	cam.LookAt(math32.NewVector3(position[0], position[1], position[2]), math32.NewVector3(1, 0, 0))
	saved := worldAtmo
	defer func() { worldAtmo = saved }()
	worldAtmo.fogStart, worldAtmo.fogEnd = 0, 0
	gl.Viewport(0, 0, worldIndoorMinimapSize, worldIndoorMinimapSize)
	gl.ClearColor(0.05, 0.07, 0.1, 1)
	gl.Clear(gls.DEPTH_BUFFER_BIT | gls.STENCIL_BUFFER_BIT | gls.COLOR_BUFFER_BIT)
	if err := r.Render(root, cam); err != nil {
		return nil
	}
	pixels := gl.ReadPixels(0, 0, worldIndoorMinimapSize, worldIndoorMinimapSize, gls.RGBA, gls.UNSIGNED_BYTE)
	img := image.NewRGBA(image.Rect(0, 0, worldIndoorMinimapSize, worldIndoorMinimapSize))
	for y := 0; y < worldIndoorMinimapSize; y++ {
		copy(img.Pix[y*img.Stride:(y+1)*img.Stride], pixels[(worldIndoorMinimapSize-1-y)*worldIndoorMinimapSize*4:(worldIndoorMinimapSize-y)*worldIndoorMinimapSize*4])
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	return img
}
