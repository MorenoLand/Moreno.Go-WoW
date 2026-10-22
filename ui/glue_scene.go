package ui

import (
	"math"
	"strconv"
)

type GlueSceneModel struct {
	Path         string
	Position     [3]float64
	Facing       float64
	Scale        float64
	Alpha        float64
	Sequence     int
	WidthSquish  float64
	HeightSquish float64
	Camera       int
	Light        [13]float64
	HasLight     bool
	// FFX is a ModelFFX frame. The placement rules below were traced for plain
	// Model frames only (Wow.exe FUN_0095fba0), so FFX models keep the old path.
	FFX bool
	// EffectiveScale is the frame's effective scale: its own scale times its
	// parents' (GetEffectiveScale). Model position and scale are multiplied by it.
	EffectiveScale float64
	// FrameWidth and FrameHeight are the model frame's size in UI units; the
	// original renders the scene inside that rectangle.
	FrameWidth, FrameHeight float64
	// Viewport is the frame's rectangle as the original sets it before drawing
	// the model (Wow.exe FUN_0095fc30 -> FUN_00681f60): minX, maxX, minY, maxY
	// as fractions of the screen, clamped to [0,1], origin bottom-left.
	// HasViewport is false until the frame has been laid out.
	Viewport    [4]float64
	HasViewport bool
	// Frame is the same rectangle without the clamp, so it can reach past the
	// screen when a frame is larger than the window (a scene frame that covers
	// a wide window and is cropped by it).
	Frame [4]float64
}

// withViewport fills in the frame's normalised, clamped rectangle from its
// last layout.
func (m GlueSceneModel) withViewport(eng *UIEngine, w *widget) GlueSceneModel {
	rect, ok := eng.rects[w]
	screenWidth, screenHeight := eng.screen.W(), eng.screen.H()
	if !ok || screenWidth <= 0 || screenHeight <= 0 || rect.W() <= 0 || rect.H() <= 0 {
		return m
	}
	clamp := func(v float64) float64 { return math.Max(0, math.Min(1, v)) }
	m.Frame = [4]float64{rect.X0 / screenWidth, rect.X1 / screenWidth, rect.Y0 / screenHeight, rect.Y1 / screenHeight}
	m.Viewport = [4]float64{clamp(m.Frame[0]), clamp(m.Frame[1]), clamp(m.Frame[2]), clamp(m.Frame[3])}
	m.HasViewport = m.Viewport[1] > m.Viewport[0] && m.Viewport[3] > m.Viewport[2]
	return m
}

// effectiveScale is the product of the widget's scale and its ancestors'.
func (w *widget) effectiveScale() float64 {
	scale := 1.0
	for current := w; current != nil; current = current.parent {
		if current.scale > 0 {
			scale *= current.scale
		}
	}
	return scale
}

func (eng *UIEngine) VisibleGlueSceneModels() []GlueSceneModel {
	if eng == nil || eng.Rt == nil {
		return nil
	}
	scene := eng.Rt.widgets["LoginScene"]
	if scene == nil || !widgetVisible(scene) {
		return nil
	}
	models := make([]GlueSceneModel, 0)
	var visit func(*widget)
	visit = func(parent *widget) {
		for _, child := range parent.children {
			if !widgetVisible(child) {
				continue
			}
			if (child.kind == kindModel || child.kind == kindModelFFX) && child.modelFile != "" {
				widthSquish, heightSquish := 1.0, 1.0
				if scene.width > 0 && child.width > 0 {
					widthSquish = scene.width / child.width
				}
				if scene.height > 0 && child.height > 0 {
					heightSquish = scene.height / child.height
				}
				models = append(models, GlueSceneModel{Path: child.modelFile, Position: child.modelPosition, Facing: child.modelFacing, Scale: child.modelScale, Alpha: child.alpha, Sequence: child.sequence, WidthSquish: widthSquish, HeightSquish: heightSquish, Camera: child.camera, Light: child.modelLight, HasLight: child.modelLightSet, FFX: child.kind == kindModelFFX, EffectiveScale: child.effectiveScale(), FrameWidth: child.width, FrameHeight: child.height}.withViewport(eng, child))
				continue
			}
			visit(child)
		}
	}
	visit(scene)
	return models
}

func (eng *UIEngine) VisibleGlueSceneKey() string {
	return GlueSceneKey(eng.VisibleGlueSceneModels())
}

// GlueSceneStructureKey identifies what forces a scene rebuild: the number of
// model frames and, per slot, the model file, camera and frame kind. Everything
// else a script animates (position, facing, scale, alpha, light, sequence,
// frame size) is applied to the loaded scene in place.
func GlueSceneStructureKey(models []GlueSceneModel) string {
	if len(models) == 0 {
		return ""
	}
	key := make([]byte, 0, 96*len(models))
	for _, model := range models {
		key = strconv.AppendQuote(key, model.Path)
		key = append(key, '|')
		key = strconv.AppendInt(key, int64(model.Camera), 10)
		if model.FFX {
			key = append(key, 'F')
		}
		key = append(key, ';')
	}
	return string(key)
}

// GlueSceneKey identifies a set of scene models. It runs every menu frame, so it
// is built with strconv rather than fmt; the text is identical to the former
// fmt.Fprintf("%q|%.6f,...|%d|") form.
func GlueSceneKey(models []GlueSceneModel) string {
	if len(models) == 0 {
		return ""
	}
	key := make([]byte, 0, 256*len(models))
	number := func(value float64) []byte { return strconv.AppendFloat(key, value, 'f', 6, 64) }
	for _, model := range models {
		key = strconv.AppendQuote(key, model.Path)
		key = append(key, '|')
		key = number(model.Position[0])
		key = append(key, ',')
		key = number(model.Position[1])
		key = append(key, ',')
		key = number(model.Position[2])
		for _, value := range []float64{model.Facing, model.Scale, model.Alpha, model.WidthSquish, model.HeightSquish} {
			key = append(key, '|')
			key = number(value)
		}
		key = append(key, '|')
		key = strconv.AppendInt(key, int64(model.Camera), 10)
		key = append(key, '|')
		if model.FFX {
			key = append(key, 'F')
		}
		for _, value := range []float64{model.EffectiveScale, model.FrameWidth, model.FrameHeight} {
			key = number(value)
			key = append(key, ',')
		}
		key = append(key, '|')
		if model.HasLight {
			for _, value := range model.Light {
				key = number(value)
				key = append(key, ',')
			}
		}
		key = append(key, ';')
		key = strconv.AppendInt(key, int64(model.Sequence), 10)
		key = append(key, ';')
	}
	return string(key)
}

func widgetVisible(w *widget) bool {
	for current := w; current != nil; current = current.parent {
		if !current.shown {
			return false
		}
	}
	return true
}
