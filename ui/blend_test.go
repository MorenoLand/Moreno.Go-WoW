package ui

import (
	"image"
	"image/color"
	"testing"
)

func TestAdditiveTextureKeysBlackBackground(t *testing.T) {
	canvas := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			canvas.SetRGBA(x, y, color.RGBA{R: 20, G: 20, B: 20, A: 255})
		}
	}
	source := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if x == 0 || y == 0 || x == 3 || y == 3 {
				source.SetRGBA(x, y, color.RGBA{R: 16, G: 16, B: 16, A: 255})
			} else {
				source.SetRGBA(x, y, color.RGBA{R: 0, G: 64, B: 192, A: 255})
			}
		}
	}
	drawSubMode(canvas, source, Rect{X0: 0, Y0: 0, X1: 4, Y1: 4}, 4, [4]float64{0, 1, 0, 1}, true)
	if got := canvas.RGBAAt(0, 0); got != (color.RGBA{R: 20, G: 20, B: 20, A: 255}) {
		t.Fatalf("near-black additive pixel changed background: %+v", got)
	}
	if got := canvas.RGBAAt(2, 2); got.B <= 20 {
		t.Fatalf("blue additive pixel did not brighten background: %+v", got)
	}
}

func TestTextureCoordinatesPreserveHorizontalFlip(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		source.SetRGBA(0, y, color.RGBA{R: 255, A: 255})
		source.SetRGBA(1, y, color.RGBA{B: 255, A: 255})
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 2))
	drawSubModeFilter(canvas, source, Rect{X0: 0, Y0: 0, X1: 2, Y1: 2}, 2, [4]float64{1, 0, 0, 1}, false)
	if canvas.RGBAAt(0, 0).B < 240 || canvas.RGBAAt(1, 0).R < 240 {
		t.Fatalf("horizontal flip was discarded: left=%+v right=%+v", canvas.RGBAAt(0, 0), canvas.RGBAAt(1, 0))
	}
}
