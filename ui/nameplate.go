package ui

import (
	"image"
	"image/color"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// NameplateLine is one line of a name drawn over a unit, in its colour.
type NameplateLine struct {
	Text    string
	R, G, B uint8
}

// RenderNameplate draws the lines centred one under the other, each with a
// black outline, the way a unit's name is shown over its head. size is the
// font height in pixels.
func (eng *UIEngine) RenderNameplate(lines []NameplateLine, size float64) *image.RGBA {
	if eng == nil || eng.FontObj == nil || len(lines) == 0 || size < 4 {
		return nil
	}
	face := eng.cachedFace("__nameplate", eng.FontObj, size)
	if face == nil {
		return nil
	}
	metrics := face.Metrics()
	lineHeight := metrics.Height.Ceil()
	ascent := metrics.Ascent.Ceil()
	const outline = 1
	width := 0
	for _, line := range lines {
		if w := font.MeasureString(face, line.Text).Ceil(); w > width {
			width = w
		}
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width+2*outline+2, lineHeight*len(lines)+2*outline+2))
	for index, line := range lines {
		lineWidth := font.MeasureString(face, line.Text).Ceil()
		x := (canvas.Bounds().Dx() - lineWidth) / 2
		y := outline + 1 + index*lineHeight + ascent
		drawText := func(dx, dy int, c color.RGBA) {
			drawer := font.Drawer{Dst: canvas, Src: image.NewUniform(c), Face: face, Dot: fixed.P(x+dx, y+dy)}
			drawer.DrawString(line.Text)
		}
		black := color.RGBA{A: 255}
		for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-1, -1}, {1, 1}, {-1, 1}, {1, -1}} {
			drawText(d[0], d[1], black)
		}
		drawText(0, 0, color.RGBA{R: line.R, G: line.G, B: line.B, A: 255})
	}
	return canvas
}
