package ui

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
)

func (eng *UIEngine) SetLoadingScreen(path string, progress float64) {
	eng.loading = true
	eng.loadingPath = path
	eng.loadingProgress = progress
	if eng.loadingProgress < 0 {
		eng.loadingProgress = 0
	}
	if eng.loadingProgress > 1 {
		eng.loadingProgress = 1
	}
}

func (eng *UIEngine) ClearLoadingScreen() {
	eng.loading = false
	eng.loadingPath = ""
	eng.loadingProgress = 0
}

func (eng *UIEngine) renderLoadingScreen(canvas *image.RGBA) {
	if source := eng.loadBLP(eng.loadingPath); source != nil {
		xdraw.BiLinear.Scale(canvas, canvas.Bounds(), source, source.Bounds(), xdraw.Src, nil)
	} else {
		draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
	}
	bar := loadingBarRect(canvas.Bounds().Dx(), canvas.Bounds().Dy())
	if fill := eng.loadBLP(`Interface\Glues\LoadingBar\Loading-BarFill`); fill != nil && eng.loadingProgress > 0 {
		fillRect := loadingBarFillRect(bar, eng.loadingProgress)
		if !fillRect.Empty() {
			xdraw.BiLinear.Scale(canvas, fillRect, fill, fill.Bounds(), xdraw.Over, nil)
		}
	}
	if border := eng.loadBLP(`Interface\Glues\LoadingBar\Loading-BarBorder`); border != nil {
		xdraw.BiLinear.Scale(canvas, bar, border, border.Bounds(), xdraw.Over, nil)
	}
}

func loadingBarRect(width, height int) image.Rectangle {
	if width <= 0 || height <= 0 {
		return image.Rectangle{}
	}
	return image.Rect(int(float64(width)*0.2), int(float64(height)*0.9), int(float64(width)*0.8), int(float64(height)*0.95))
}

func loadingBarFillRect(bar image.Rectangle, progress float64) image.Rectangle {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	insetX := int(float64(bar.Dx()) * 0.0625)
	insetY := int(float64(bar.Dy()) * 0.25)
	inner := image.Rect(bar.Min.X+insetX, bar.Min.Y+insetY, bar.Max.X-insetX, bar.Max.Y-insetY)
	return image.Rect(inner.Min.X, inner.Min.Y, inner.Min.X+int(math.Round(float64(inner.Dx())*progress)), inner.Max.Y).Intersect(inner)
}
