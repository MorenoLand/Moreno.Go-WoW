package ui

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const debugPanelTitleHeight = 40.0

type debugPanelState struct {
	visible     bool
	collapsed   bool
	lines       []string
	left        float64
	top         float64
	positioned  bool
	dragging    bool
	dragOffsetX float64
	dragOffsetY float64
}

func (eng *UIEngine) ToggleDebugPanel() bool {
	eng.debugPanel.visible = !eng.debugPanel.visible
	return eng.debugPanel.visible
}

func (eng *UIEngine) DebugPanelVisible() bool { return eng.debugPanel.visible }

func (eng *UIEngine) DebugPanelDragging() bool { return eng.debugPanel.dragging }

func (eng *UIEngine) SetDebugPanelLines(lines []string) {
	eng.debugPanel.lines = append(eng.debugPanel.lines[:0], lines...)
}

func (eng *UIEngine) debugPanelRect() Rect {
	top := eng.screen.Y1
	if top <= 0 {
		top = 768
	}
	width := 540.0
	height := debugPanelTitleHeight + float64(len(eng.debugPanel.lines))*20 + 16
	if eng.debugPanel.collapsed {
		width = 280
		height = debugPanelTitleHeight
	}
	maxHeight := top - 24
	if maxHeight > debugPanelTitleHeight && height > maxHeight {
		height = maxHeight
	}
	if !eng.debugPanel.positioned {
		eng.debugPanel.left = 16
		eng.debugPanel.top = 16
		eng.debugPanel.positioned = true
	}
	return Rect{X0: eng.debugPanel.left, Y0: top - eng.debugPanel.top - height, X1: eng.debugPanel.left + width, Y1: top - eng.debugPanel.top}
}

func (s *debugPanelState) contains(x, y float64, eng *UIEngine) bool {
	if !s.visible {
		return false
	}
	scale := eng.uiScale
	if scale <= 0 {
		scale = 1
	}
	screenHeight := float64(eng.screenHeight)
	if screenHeight <= 0 {
		screenHeight = 768
	}
	pointX, pointY := x/scale, (screenHeight-y)/scale
	panel := eng.debugPanelRect()
	return pointX >= panel.X0 && pointX < panel.X1 && pointY >= panel.Y0 && pointY < panel.Y1
}

func (s *debugPanelState) handleMouse(x, y float64, down bool, eng *UIEngine) bool {
	if s.dragging {
		if !down {
			s.dragging = false
		}
		return true
	}
	if !s.contains(x, y, eng) {
		return false
	}
	if down {
		scale := eng.uiScale
		if scale <= 0 {
			scale = 1
		}
		screenHeight := float64(eng.screenHeight)
		if screenHeight <= 0 {
			screenHeight = 768
		}
		pointX, pointY := x/scale, (screenHeight-y)/scale
		panel := eng.debugPanelRect()
		if pointY >= panel.Y1-debugPanelTitleHeight {
			if pointX < panel.X0+32 {
				s.collapsed = !s.collapsed
			} else {
				top := screenHeight/scale - panel.Y1
				s.dragging = true
				s.dragOffsetX = pointX - panel.X0
				s.dragOffsetY = (screenHeight/scale - pointY) - top
			}
		}
	}
	return true
}

func (s *debugPanelState) move(x, y float64, eng *UIEngine) bool {
	if !s.dragging {
		return false
	}
	scale := eng.uiScale
	if scale <= 0 {
		scale = 1
	}
	screenHeight := float64(eng.screenHeight)
	if screenHeight <= 0 {
		screenHeight = 768
	}
	screenTop := eng.screen.Y1
	if screenTop <= 0 {
		screenTop = 768
	}
	pointX, pointY := x/scale, (screenHeight-y)/scale
	s.left = pointX - s.dragOffsetX
	s.top = screenTop - pointY - s.dragOffsetY
	panel := eng.debugPanelRect()
	screenWidth := eng.screen.X1
	if screenWidth <= 0 {
		screenWidth = screenHeight * 1.5
	}
	if s.left < 8 {
		s.left = 8
	}
	if s.left+panel.W() > screenWidth-8 {
		s.left = screenWidth - panel.W() - 8
	}
	if s.top < 8 {
		s.top = 8
	}
	if s.top+panel.H() > screenTop-8 {
		s.top = screenTop - panel.H() - 8
	}
	return true
}

func (eng *UIEngine) drawDebugPanel(canvas *image.RGBA, face, titleFace font.Face) {
	panel := screenScaledRect(eng.debugPanelRect(), eng.uiScale)
	dst := ScreenRect(panel, float64(canvas.Bounds().Dy())).Intersect(canvas.Bounds())
	if dst.Dx() <= 0 || dst.Dy() <= 0 {
		return
	}
	draw.Draw(canvas, dst, &image.Uniform{C: color.RGBA{R: 10, G: 13, B: 16, A: 232}}, image.Point{}, draw.Over)
	drawBorder(canvas, dst, color.RGBA{R: 72, G: 78, B: 84, A: 255}, 1)
	titleHeight := int(debugPanelTitleHeight * eng.uiScale)
	if titleHeight < 1 {
		titleHeight = 1
	}
	titleMaxY := dst.Min.Y + titleHeight
	if titleMaxY > dst.Max.Y {
		titleMaxY = dst.Max.Y
	}
	title := image.Rect(dst.Min.X, dst.Min.Y, dst.Max.X, titleMaxY)
	draw.Draw(canvas, image.Rect(title.Min.X, title.Max.Y-1, title.Max.X, title.Max.Y), &image.Uniform{C: color.RGBA{R: 58, G: 63, B: 68, A: 255}}, image.Point{}, draw.Over)
	drawDebugChevron(canvas, dst.Min.X+16, title.Min.Y+title.Dy()/2, !eng.debugPanel.collapsed)
	titleMetrics := titleFace.Metrics()
	titleBaseline := title.Min.Y + (title.Dy()-titleMetrics.Ascent.Ceil()-titleMetrics.Descent.Ceil())/2 + titleMetrics.Ascent.Ceil()
	drawDebugText(canvas, titleFace, "MorenoWoW", dst.Min.X+36, titleBaseline, color.RGBA{R: 185, G: 185, B: 185, A: 255})
	if eng.debugPanel.collapsed {
		return
	}
	lineHeight := face.Metrics().Height.Ceil()
	if lineHeight < 1 {
		lineHeight = 1
	}
	baseline := title.Max.Y + face.Metrics().Ascent.Ceil() + int(6*eng.uiScale)
	for _, line := range eng.debugPanel.lines {
		if baseline > dst.Max.Y-face.Metrics().Descent.Ceil() {
			break
		}
		drawDebugText(canvas, face, line, dst.Min.X+10, baseline, color.RGBA{R: 218, G: 218, B: 218, A: 255})
		baseline += lineHeight
	}
}

func drawDebugText(canvas *image.RGBA, face font.Face, text string, x, baseline int, c color.Color) {
	drawer := &font.Drawer{Dst: canvas, Src: image.NewUniform(c), Face: face, Dot: fixed.P(x, baseline)}
	drawer.DrawString(text)
}

func drawDebugChevron(canvas *image.RGBA, x, y int, expanded bool) {
	fill := &image.Uniform{C: color.RGBA{R: 185, G: 185, B: 185, A: 255}}
	if expanded {
		for row := 0; row < 8; row++ {
			width := 15 - row*2
			draw.Draw(canvas, image.Rect(x-width/2, y-4+row, x+width/2+1, y-3+row), fill, image.Point{}, draw.Over)
		}
		return
	}
	for row := -7; row <= 7; row++ {
		length := 8 - absInt(row)
		draw.Draw(canvas, image.Rect(x, y+row, x+length, y+row+1), fill, image.Point{}, draw.Over)
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
