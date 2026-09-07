package ui

import (
	"fmt"
	"strings"
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
				models = append(models, GlueSceneModel{Path: child.modelFile, Position: child.modelPosition, Facing: child.modelFacing, Scale: child.modelScale, Alpha: child.alpha, Sequence: child.sequence, WidthSquish: widthSquish, HeightSquish: heightSquish})
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
		fmt.Fprintf(&key, "%q|%.6f,%.6f,%.6f|%.6f|%.6f|%.6f|%.6f|%.6f|", model.Path, model.Position[0], model.Position[1], model.Position[2], model.Facing, model.Scale, model.Alpha, model.WidthSquish, model.HeightSquish)
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
