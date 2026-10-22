package ui

import (
	"math"
	"strings"
)

// widgetVisual is the part of a widget's state that a software UI repaint
// shows and that scripts animate.
type widgetVisual struct {
	alpha                  float64
	shown                  bool
	width, height, scale   float64
	text                   string
	textureFile            string
	vertexColor, textColor rgba
	texCoords              [4]float64
	value                  float64
	checked, highlighted   bool
	buttonState            string
	points                 int
	pointsSum              float64
}

func (w *widget) visual() widgetVisual {
	// Alpha is compared in 1/255 steps: a smaller change cannot show.
	v := widgetVisual{alpha: math.Round(w.alpha * 255), shown: w.shown, width: w.width, height: w.height, scale: w.scale,
		text: w.text, textureFile: w.textureFile, vertexColor: w.vertexColor, textColor: w.textColor,
		texCoords: [4]float64{w.texCoordL, w.texCoordR, w.texCoordT, w.texCoordB}, value: w.value,
		checked: w.checked, highlighted: w.highlighted, buttonState: w.buttonState, points: len(w.points)}
	for _, point := range w.points {
		v.pointsSum += point.x*3 + point.y*7
	}
	return v
}

// isReadOnlyMethodName reports methods that only read state, so their calls
// are never compared.
func isReadOnlyMethodName(name string) bool {
	for _, prefix := range []string{"Get", "Is", "Has", "Can", "Num"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
