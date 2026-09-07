package ui

import (
	"fmt"
	"strings"
)

type GlueSceneModel struct {
	Path     string
	Position [3]float64
	Facing   float64
	Scale    float64
	Alpha    float64
	Sequence int
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
				models = append(models, GlueSceneModel{Path: child.modelFile, Position: child.modelPosition, Facing: child.modelFacing, Scale: child.modelScale, Alpha: child.alpha, Sequence: child.sequence})
				continue
			}
			visit(child)
		}
	}
	visit(scene)
	return models
}

func (eng *UIEngine) VisibleGlueSceneKey() string {
	models := eng.VisibleGlueSceneModels()
	if len(models) == 0 {
		return ""
	}
	var key strings.Builder
	for _, model := range models {
		fmt.Fprintf(&key, "%q|%.6f,%.6f,%.6f|%.6f|%.6f|%.6f|", model.Path, model.Position[0], model.Position[1], model.Position[2], model.Facing, model.Scale, model.Alpha)
		key.WriteString(fmt.Sprintf("%d;", model.Sequence))
	}
	return key.String()
}

func widgetVisible(w *widget) bool {
	for current := w; current != nil; current = current.parent {
		if !current.shown {
			return false
		}
	}
	return true
}
