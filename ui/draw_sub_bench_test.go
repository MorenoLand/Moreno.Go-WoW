package ui

import (
	"image"
	"image/color"
	"testing"
)

func BenchmarkDrawSubModeFilterOver(b *testing.B) {
	canvas := image.NewRGBA(image.Rect(0, 0, 256, 256))
	source := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			source.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 64, A: 255})
		}
	}
	tc := [4]float64{0.25, 0.75, 0.25, 0.75}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		drawSubModeFilter(canvas, source, Rect{X0: 0, Y0: 0, X1: 128, Y1: 128}, 256, tc, false)
	}
}

func BenchmarkDrawSubModeFilterAdditive(b *testing.B) {
	canvas := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			canvas.SetRGBA(x, y, color.RGBA{R: 20, G: 20, B: 20, A: 255})
		}
	}
	source := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if x == 0 || y == 0 || x == 31 || y == 31 {
				source.SetRGBA(x, y, color.RGBA{A: 255})
			} else {
				source.SetRGBA(x, y, color.RGBA{R: 0, G: 64, B: 192, A: 255})
			}
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		drawSubModeFilter(canvas, source, Rect{X0: 0, Y0: 0, X1: 64, Y1: 64}, 64, [4]float64{0, 1, 0, 1}, true)
	}
}
