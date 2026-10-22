package ui

import (
	"image"
	"image/draw"
	"sync"

	xdraw "golang.org/x/image/draw"
)

// The software UI repaints often (hover changes, scene changes) and scaled the
// same textures to the same on-screen sizes with bilinear filtering every time.
// Textures decoded from assets (eng.loadBLP / eng.tintTextureImage) never change
// after they are cached, so their scaled result for a given crop and size can be
// reused. Dynamic images (minimap, movie frames, portraits) are never
// registered, so they always take the uncached path.

const scaledTextureCacheLimit = 128 << 20

// The key is relative to the destination rectangle, so a texture drawn at the
// same size with the same crop hits the cache wherever it sits on the canvas.
type scaledTextureKey struct {
	img           image.Image
	tc            [4]float64
	width, height int // destination size before clipping
	offsetX, offY int // clipped region origin inside the destination
	clipW, clipH  int // clipped region size
}

var textureScaleCache = struct {
	sync.Mutex
	static map[image.Image]struct{}
	scaled map[scaledTextureKey]*image.RGBA
	opaque map[*image.RGBA]bool // scaled copies with no transparent pixel
	// additive holds each scaled copy's precomputed additive contribution.
	additive map[*image.RGBA]*image.RGBA
	bytes    int
}{static: make(map[image.Image]struct{}), scaled: make(map[scaledTextureKey]*image.RGBA), opaque: make(map[*image.RGBA]bool), additive: make(map[*image.RGBA]*image.RGBA)}

func rgbaIsOpaque(img *image.RGBA) bool {
	for index := 3; index < len(img.Pix); index += 4 {
		if img.Pix[index] != 255 {
			return false
		}
	}
	return true
}

// registerStaticTexture marks img as immutable so scaled copies may be cached.
func registerStaticTexture(img image.Image) {
	if img == nil {
		return
	}
	textureScaleCache.Lock()
	textureScaleCache.static[img] = struct{}{}
	textureScaleCache.Unlock()
}

// scaledStaticTexture returns img cropped by tc and bilinear-scaled to the
// destination dst, restricted to clipped (Src, sized to clipped), or nil when
// img is not a registered static texture.
func scaledStaticTexture(img image.Image, tc [4]float64, dst, clipped image.Rectangle) *image.RGBA {
	textureScaleCache.Lock()
	_, static := textureScaleCache.static[img]
	key := scaledTextureKey{img: img, tc: tc, width: dst.Dx(), height: dst.Dy(),
		offsetX: clipped.Min.X - dst.Min.X, offY: clipped.Min.Y - dst.Min.Y, clipW: clipped.Dx(), clipH: clipped.Dy()}
	cached := textureScaleCache.scaled[key]
	textureScaleCache.Unlock()
	if !static {
		return nil
	}
	if cached != nil {
		return cached
	}
	resolved, ok := resolveTextureSubRGBA(img, tc)
	if !ok {
		return nil
	}
	source := clipTextureSource(resolved, dst, clipped)
	scaled := image.NewRGBA(image.Rect(0, 0, clipped.Dx(), clipped.Dy()))
	xdraw.BiLinear.Scale(scaled, scaled.Bounds(), source, source.Bounds(), xdraw.Src, nil)
	size := len(scaled.Pix)
	opaque := rgbaIsOpaque(scaled)
	textureScaleCache.Lock()
	if textureScaleCache.bytes+size > scaledTextureCacheLimit {
		textureScaleCache.scaled = make(map[scaledTextureKey]*image.RGBA)
		textureScaleCache.opaque = make(map[*image.RGBA]bool)
		textureScaleCache.additive = make(map[*image.RGBA]*image.RGBA)
		textureScaleCache.bytes = 0
	}
	textureScaleCache.scaled[key] = scaled
	textureScaleCache.opaque[scaled] = opaque
	textureScaleCache.bytes += size
	textureScaleCache.Unlock()
	return scaled
}

// drawCachedTexture draws a cached scaled static texture into clipped, the part
// of dst that lies on the canvas. It reports whether it drew.
func drawCachedTexture(canvas *image.RGBA, img image.Image, dst, clipped image.Rectangle, tc [4]float64, additive bool) bool {
	scaled := scaledStaticTexture(img, tc, dst, clipped)
	if scaled == nil {
		return false
	}
	if !additive {
		textureScaleCache.Lock()
		opaque := textureScaleCache.opaque[scaled]
		textureScaleCache.Unlock()
		op := draw.Over
		if opaque && !legacyFullCanvasPaint {
			op = draw.Src // nothing shows through, so a plain copy is the same pixels
		}
		draw.Draw(canvas, clipped, scaled, scaled.Bounds().Min, op)
		return true
	}
	if legacyFullCanvasPaint {
		drawAdditiveLegacy(canvas, scaled, clipped)
		return true
	}
	// Each texel's additive contribution does not depend on what is underneath,
	// so it is computed once per cached texture; a repaint is then a saturating
	// add of three bytes per pixel.
	contribution := additiveContributionOf(scaled)
	width := clipped.Dx() * 4
	for y := 0; y < clipped.Dy(); y++ {
		source := contribution.Pix[contribution.PixOffset(0, y):][:width]
		target := canvas.Pix[canvas.PixOffset(clipped.Min.X, clipped.Min.Y+y):][:width]
		for index := 0; index+3 < len(source); index += 4 {
			if source[index+3] == 0 {
				continue
			}
			for channel := 0; channel < 3; channel++ {
				sum := int(target[index+channel]) + int(source[index+channel])
				if sum > 255 {
					sum = 255
				}
				target[index+channel] = uint8(sum)
			}
			if source[index+3] > target[index+3] {
				target[index+3] = source[index+3]
			}
		}
	}
	return true
}

// additiveContributionOf returns, for every texel of a scaled texture, the
// colour it adds to the canvas (zero alpha where it adds nothing: transparent
// texels and the near-black texels that Glue textures use as "no light"). The
// alpha channel is the texel's alpha, which the canvas alpha is raised to.
func additiveContributionOf(scaled *image.RGBA) *image.RGBA {
	textureScaleCache.Lock()
	cached := textureScaleCache.additive[scaled]
	textureScaleCache.Unlock()
	if cached != nil {
		return cached
	}
	out := image.NewRGBA(scaled.Rect)
	for index := 0; index+3 < len(scaled.Pix); index += 4 {
		r8, g8, b8, a8 := scaled.Pix[index], scaled.Pix[index+1], scaled.Pix[index+2], scaled.Pix[index+3]
		if a8 == 0 || isAdditiveKeyColor(r8, g8, b8) {
			continue
		}
		alpha := uint32(a8) * 0x101
		out.Pix[index] = uint8(scaleChannel(uint32(r8)*0x101, alpha) >> 8)
		out.Pix[index+1] = uint8(scaleChannel(uint32(g8)*0x101, alpha) >> 8)
		out.Pix[index+2] = uint8(scaleChannel(uint32(b8)*0x101, alpha) >> 8)
		out.Pix[index+3] = a8
	}
	textureScaleCache.Lock()
	textureScaleCache.additive[scaled] = out
	textureScaleCache.Unlock()
	return out
}

// drawAdditiveLegacy is the per-pixel additive blend the cached path replaced;
// tests use it to check the two give the same pixels.
func drawAdditiveLegacy(canvas, scaled *image.RGBA, clipped image.Rectangle) {
	for y := 0; y < clipped.Dy(); y++ {
		dstOff := canvas.PixOffset(clipped.Min.X, clipped.Min.Y+y)
		srcOff := scaled.PixOffset(0, y)
		for x := 0; x < clipped.Dx(); x++ {
			sr8, sg8, sb8, sa8 := scaled.Pix[srcOff], scaled.Pix[srcOff+1], scaled.Pix[srcOff+2], scaled.Pix[srcOff+3]
			if sa8 != 0 && !isAdditiveKeyColor(sr8, sg8, sb8) {
				sa := uint32(sa8) * 0x101
				sr := uint32(sr8) * 0x101
				sg := uint32(sg8) * 0x101
				sb := uint32(sb8) * 0x101
				dr := uint32(canvas.Pix[dstOff]) * 0x101
				dg := uint32(canvas.Pix[dstOff+1]) * 0x101
				db := uint32(canvas.Pix[dstOff+2]) * 0x101
				da := uint32(canvas.Pix[dstOff+3]) * 0x101
				canvas.Pix[dstOff] = addChannel(dr, scaleChannel(sr, sa))
				canvas.Pix[dstOff+1] = addChannel(dg, scaleChannel(sg, sa))
				canvas.Pix[dstOff+2] = addChannel(db, scaleChannel(sb, sa))
				canvas.Pix[dstOff+3] = maxChannel(da, sa)
			}
			dstOff += 4
			srcOff += 4
		}
	}
}
