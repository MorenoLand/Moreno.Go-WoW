package ui

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
)

// BLP2 texture decoding for interface assets. Format reference: the
// original client's texture loader (BLP magic, encoding 0 JPEG /
// 1 palette-alpha / 2+ DXT and uncompressed).
//
// Only the decodings the interface data uses are implemented: palette
// images (with alpha or alpha-bitmap) and DXT1/3/5. JPEG-encoded mips are
// rejected with an explicit error rather than guessed at.

const (
	blpEncodingJPEG   = 0
	blpEncodingAlpha  = 1
	blpEncodingDXT    = 2
	blpEncodingUncomp = 3
)

// DecodeBLP decodes the first (largest) mip level of a BLP2 texture.
func DecodeBLP(data []byte) (image.Image, error) {
	if len(data) < 148 {
		return nil, fmt.Errorf("blp: truncated header (%d bytes)", len(data))
	}
	if string(data[0:4]) != "BLP2" {
		return nil, fmt.Errorf("blp: bad magic %q", data[0:4])
	}
	// data[4:8] is the format type. 1 is BLP2.
	// data[8] is the encoding (1=Palette, 2=DXT, 3=ARGB8888)
	// data[9] is the alpha depth (0, 1, 4, 8)
	encoding := data[8]
	alphaDepth := uint32(data[9])
	width := int(binary.LittleEndian.Uint32(data[12:16]))
	height := int(binary.LittleEndian.Uint32(data[16:20]))
	if width <= 0 || height <= 0 || width > 4096 || height > 4096 {
		return nil, fmt.Errorf("blp: bad dimensions %dx%d", width, height)
	}

	// Mip levels 0..15: offsets at 20..84, sizes at 84..148.
	mipOffset := int(binary.LittleEndian.Uint32(data[20:24]))
	mipSize := int(binary.LittleEndian.Uint32(data[84:88]))
	if mipOffset == 0 || mipSize <= 0 || mipOffset+mipSize > len(data) {
		return nil, fmt.Errorf("blp: mip 0 out of range (offset %d size %d, file %d)", mipOffset, mipSize, len(data))
	}
	mip := data[mipOffset : mipOffset+mipSize]

	switch encoding {
	case blpEncodingAlpha:
		var palette [256]color.RGBA
		if len(data) >= 148+1024 {
			palData := data[148 : 148+1024]
			for i := 0; i < 256; i++ {
				palette[i] = color.RGBA{R: palData[i*4+2], G: palData[i*4+1], B: palData[i*4+0], A: 255}
			}
		}
		return decodeBLPPalette(mip, width, height, alphaDepth, palette)
	case blpEncodingDXT, blpEncodingUncomp:
		return decodeBLPDXT(mip, width, height, alphaDepth)
	}
	return nil, fmt.Errorf("blp: unknown encoding %d", encoding)
}

func decodeBLPPalette(mip []byte, width, height int, alphaDepth uint32, palette [256]color.RGBA) (image.Image, error) {
	pixels := mip
	if len(pixels) < width*height {
		return nil, fmt.Errorf("blp: palette pixel data short (%d < %d)", len(pixels), width*height)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	if alphaDepth == 8 {
		alpha := pixels[width*height:]
		if len(alpha) < width*height {
			return nil, fmt.Errorf("blp: alpha data short")
		}
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				c := palette[pixels[y*width+x]]
				c.A = alpha[y*width+x]
				img.Set(x, y, color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A})
			}
		}
	} else if alphaDepth == 1 {
		alphaBits := pixels[width*height:]
		need := (width*height + 7) / 8
		if len(alphaBits) < need {
			return nil, fmt.Errorf("blp: alpha bits short")
		}
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				c := palette[pixels[y*width+x]]
				bit := uint(alphaBits[(y*width+x)/8]) >> (7 - uint((y*width+x)%8))
				if bit&1 == 0 {
					c.A = 0
				}
				img.Set(x, y, color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A})
			}
		}
	} else {
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				c := palette[pixels[y*width+x]]
				img.Set(x, y, color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A})
			}
		}
	}
	return img, nil
}

func decodeBLPDXT(mip []byte, width, height int, alphaDepth uint32) (image.Image, error) {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	switch alphaDepth {
	case 0: // DXT1
		need := ((width + 3) / 4) * ((height + 3) / 4) * 8
		if len(mip) < need {
			return nil, fmt.Errorf("blp: dxt1 data short (%d < %d)", len(mip), need)
		}
		for by := 0; by < height; by += 4 {
			for bx := 0; bx < width; bx += 4 {
				off := ((bx / 4) + (by/4)*((width+3)/4)) * 8
				decodeDXT1Block(mip[off:off+8], img, bx, by)
			}
		}
	case 1: // DXT1 with 1-bit alpha (same layout)
		need := ((width + 3) / 4) * ((height + 3) / 4) * 8
		if len(mip) < need {
			return nil, fmt.Errorf("blp: dxt1a data short")
		}
		for by := 0; by < height; by += 4 {
			for bx := 0; bx < width; bx += 4 {
				off := ((bx / 4) + (by/4)*((width+3)/4)) * 8
				decodeDXT1Block(mip[off:off+8], img, bx, by)
			}
		}
	case 4: // DXT3
		need := ((width + 3) / 4) * ((height + 3) / 4) * 16
		if len(mip) < need {
			return nil, fmt.Errorf("blp: dxt3 data short")
		}
		for by := 0; by < height; by += 4 {
			for bx := 0; bx < width; bx += 4 {
				off := ((bx / 4) + (by/4)*((width+3)/4)) * 16
				decodeDXT3Block(mip[off:off+16], img, bx, by)
			}
		}
	case 8: // DXT5
		need := ((width + 3) / 4) * ((height + 3) / 4) * 16
		if len(mip) < need {
			return nil, fmt.Errorf("blp: dxt5 data short")
		}
		for by := 0; by < height; by += 4 {
			for bx := 0; bx < width; bx += 4 {
				off := ((bx / 4) + (by/4)*((width+3)/4)) * 16
				decodeDXT5Block(mip[off:off+16], img, bx, by)
			}
		}
	default:
		return nil, fmt.Errorf("blp: unsupported alpha depth %d", alphaDepth)
	}
	return img, nil
}

func decodeDXT1Block(block []byte, img *image.RGBA, bx, by int) {
	c0 := binary.LittleEndian.Uint16(block[0:2])
	c1 := binary.LittleEndian.Uint16(block[2:4])
	r0, g0, b0 := rgb565(c0)
	r1, g1, b1 := rgb565(c1)
	var palette [4][3]uint8
	palette[0] = [3]uint8{r0, g0, b0}
	palette[1] = [3]uint8{r1, g1, b1}
	if c0 > c1 {
		palette[2] = [3]uint8{(2*r0 + r1) / 3, (2*g0 + g1) / 3, (2*b0 + b1) / 3}
		palette[3] = [3]uint8{(r0 + 2*r1) / 3, (g0 + 2*g1) / 3, (b0 + 2*b1) / 3}
	} else {
		palette[2] = [3]uint8{(r0 + r1) / 2, (g0 + g1) / 2, (b0 + b1) / 2}
		palette[3] = [3]uint8{0, 0, 0}
	}
	bits := binary.LittleEndian.Uint32(block[4:8])
	for py := 0; py < 4; py++ {
		for px := 0; px < 4; px++ {
			x, y := bx+px, by+py
			if x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
				continue
			}
			code := uint8(bits >> (2 * uint(py*4+px)) & 3)
			c := color.RGBA{R: palette[code][0], G: palette[code][1], B: palette[code][2], A: 255}
			if c0 <= c1 && code == 3 {
				c.A = 0
			}
			img.Set(x, y, color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A})
		}
	}
}

func decodeDXT3Block(block []byte, img *image.RGBA, bx, by int) {
	c0 := binary.LittleEndian.Uint16(block[8:10])
	c1 := binary.LittleEndian.Uint16(block[10:12])
	r0, g0, b0 := rgb565(c0)
	r1, g1, b1 := rgb565(c1)
	palette := [4][3]uint8{
		{r0, g0, b0}, {r1, g1, b1},
		{(2*r0 + r1) / 3, (2*g0 + g1) / 3, (2*b0 + b1) / 3},
		{(r0 + 2*r1) / 3, (g0 + 2*g1) / 3, (b0 + 2*b1) / 3},
	}
	bits := binary.LittleEndian.Uint32(block[12:16])
	for py := 0; py < 4; py++ {
		for px := 0; px < 4; px++ {
			x, y := bx+px, by+py
			if x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
				continue
			}
			code := uint8(bits >> (2 * uint(py*4+px)) & 3)
			idx := py*4 + px
			aByte := block[idx/2]
			var a uint8
			if idx%2 == 0 {
				a = aByte & 0x0F
			} else {
				a = aByte >> 4
			}
			img.Set(x, y, color.NRGBA{R: palette[code][0], G: palette[code][1], B: palette[code][2], A: a | a<<4})
		}
	}
}

func decodeDXT5Block(block []byte, img *image.RGBA, bx, by int) {
	a0, a1 := block[0], block[1]
	var alpha [8]uint8
	alpha[0], alpha[1] = a0, a1
	if a0 > a1 {
		for i := 0; i < 6; i++ {
			alpha[2+i] = uint8((uint16(6-i)*uint16(a0) + uint16(i+1)*uint16(a1)) / 7)
		}
	} else {
		for i := 0; i < 4; i++ {
			alpha[2+i] = uint8((uint16(4-i)*uint16(a0) + uint16(i+1)*uint16(a1)) / 5)
		}
		alpha[6], alpha[7] = 0, 255
	}
	// 48 bits of 3-bit alpha codes follow the two anchors.
	var alphaBits uint64
	for i := 0; i < 6; i++ {
		alphaBits |= uint64(block[2+i]) << (8 * uint(i))
	}
	c0 := binary.LittleEndian.Uint16(block[8:10])
	c1 := binary.LittleEndian.Uint16(block[10:12])
	r0, g0, b0 := rgb565(c0)
	r1, g1, b1 := rgb565(c1)
	palette := [4][3]uint8{
		{r0, g0, b0}, {r1, g1, b1},
		{(2*r0 + r1) / 3, (2*g0 + g1) / 3, (2*b0 + b1) / 3},
		{(r0 + 2*r1) / 3, (g0 + 2*g1) / 3, (b0 + 2*b1) / 3},
	}
	bits := binary.LittleEndian.Uint32(block[12:16])
	for py := 0; py < 4; py++ {
		for px := 0; px < 4; px++ {
			x, y := bx+px, by+py
			if x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
				continue
			}
			code := uint8(bits >> (2 * uint(py*4+px)) & 3)
			aIdx := uint8(alphaBits >> (3 * uint(py*4+px)) & 7)
			img.Set(x, y, color.NRGBA{R: palette[code][0], G: palette[code][1], B: palette[code][2], A: alpha[aIdx]})
		}
	}
}

func rgb565(v uint16) (uint8, uint8, uint8) {
	r := uint8(float64((v>>11)&0x1F) / 31 * 255)
	g := uint8(float64((v>>5)&0x3F) / 63 * 255)
	b := uint8(float64(v&0x1F) / 31 * 255)
	return r, g, b
}
