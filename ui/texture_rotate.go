package ui

import (
	"image"
	"math"
)

// drawRotatedTexture draws a texture region turned counter-clockwise by
// radians about the centre of its rectangle (Texture:SetRotation).
func drawRotatedTexture(canvas *image.RGBA, img image.Image, r Rect, screenHeight float64, tc [4]float64, radians float64) {
	dst := ScreenRect(r, screenHeight)
	if dst.Dx() <= 0 || dst.Dy() <= 0 {
		return
	}
	source, ok := resolveTextureSubRGBA(img, tc)
	if !ok {
		return
	}
	bounds := source.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return
	}
	centerX := float64(dst.Min.X+dst.Max.X) / 2
	centerY := float64(dst.Min.Y+dst.Max.Y) / 2
	halfWidth, halfHeight := float64(dst.Dx())/2, float64(dst.Dy())/2
	sin, cos := math.Sincos(radians)
	// The rotated rectangle fits in the circle through its corners.
	radius := int(math.Ceil(math.Hypot(halfWidth, halfHeight)))
	area := image.Rect(int(centerX)-radius, int(centerY)-radius, int(centerX)+radius+1, int(centerY)+radius+1).Intersect(canvas.Bounds())
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			dx, dy := float64(x)+0.5-centerX, float64(y)+0.5-centerY
			// Screen y points down, so a counter-clockwise turn is the inverse
			// of a clockwise rotation of the sampling point.
			u := dx*cos - dy*sin
			v := dx*sin + dy*cos
			if u < -halfWidth || u >= halfWidth || v < -halfHeight || v >= halfHeight {
				continue
			}
			sx := (u/halfWidth + 1) / 2 * float64(bounds.Dx())
			sy := (v/halfHeight + 1) / 2 * float64(bounds.Dy())
			pixel := sampleBilinear(source, sx-0.5, sy-0.5)
			if pixel[3] == 0 {
				continue
			}
			offset := canvas.PixOffset(x, y)
			alpha := uint32(pixel[3])
			for channel := 0; channel < 3; channel++ {
				canvas.Pix[offset+channel] = uint8(min(255, uint32(pixel[channel])+uint32(canvas.Pix[offset+channel])*(255-alpha)/255))
			}
			canvas.Pix[offset+3] = uint8(alpha + uint32(canvas.Pix[offset+3])*(255-alpha)/255)
		}
	}
}

// sampleBilinear reads a premultiplied RGBA pixel at a fractional position.
func sampleBilinear(img *image.RGBA, x, y float64) [4]uint8 {
	bounds := img.Bounds()
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	var out [4]float64
	for _, corner := range [4]struct {
		dx, dy int
		weight float64
	}{{0, 0, (1 - fx) * (1 - fy)}, {1, 0, fx * (1 - fy)}, {0, 1, (1 - fx) * fy}, {1, 1, fx * fy}} {
		px, py := x0+corner.dx, y0+corner.dy
		px = min(max(px, 0), bounds.Dx()-1)
		py = min(max(py, 0), bounds.Dy()-1)
		offset := img.PixOffset(bounds.Min.X+px, bounds.Min.Y+py)
		for channel := 0; channel < 4; channel++ {
			out[channel] += float64(img.Pix[offset+channel]) * corner.weight
		}
	}
	return [4]uint8{uint8(out[0] + 0.5), uint8(out[1] + 0.5), uint8(out[2] + 0.5), uint8(out[3] + 0.5)}
}
