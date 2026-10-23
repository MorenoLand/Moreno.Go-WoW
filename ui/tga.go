package ui

import (
	"fmt"
	"image"
	"image/color"
)

// decodeTGA reads an uncompressed or run-length true-colour Targa file, the
// kind the interface textures use (8-bit grey, 24 and 32 bits per pixel).
func decodeTGA(data []byte) (image.Image, error) {
	if len(data) < 18 {
		return nil, fmt.Errorf("tga: short header")
	}
	idLength := int(data[0])
	colourMapType := data[1]
	imageType := data[2]
	colourMapLength := int(data[5]) | int(data[6])<<8
	colourMapBits := int(data[7])
	width := int(data[12]) | int(data[13])<<8
	height := int(data[14]) | int(data[15])<<8
	bits := int(data[16])
	descriptor := data[17]
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("tga: empty image")
	}
	offset := 18 + idLength
	var palette []color.NRGBA
	if colourMapType == 1 {
		entry := (colourMapBits + 7) / 8
		if offset+colourMapLength*entry > len(data) {
			return nil, fmt.Errorf("tga: short colour map")
		}
		for index := 0; index < colourMapLength; index++ {
			p := data[offset+index*entry : offset+(index+1)*entry]
			switch entry {
			case 3:
				palette = append(palette, color.NRGBA{R: p[2], G: p[1], B: p[0], A: 255})
			case 4:
				palette = append(palette, color.NRGBA{R: p[2], G: p[1], B: p[0], A: p[3]})
			default:
				palette = append(palette, color.NRGBA{A: 255})
			}
		}
		offset += colourMapLength * entry
	}
	pixelBytes := (bits + 7) / 8
	if pixelBytes < 1 || pixelBytes > 4 {
		return nil, fmt.Errorf("tga: %d bits per pixel", bits)
	}
	compressed := imageType >= 9
	imageType &^= 8
	if imageType != 1 && imageType != 2 && imageType != 3 {
		return nil, fmt.Errorf("tga: type %d", imageType)
	}
	raw := make([]byte, width*height*pixelBytes)
	if compressed {
		written := 0
		for written < len(raw) && offset < len(data) {
			header := data[offset]
			offset++
			count := int(header&0x7f) + 1
			if header&0x80 != 0 {
				if offset+pixelBytes > len(data) {
					return nil, fmt.Errorf("tga: short run")
				}
				for index := 0; index < count && written < len(raw); index++ {
					copy(raw[written:written+pixelBytes], data[offset:offset+pixelBytes])
					written += pixelBytes
				}
				offset += pixelBytes
			} else {
				size := count * pixelBytes
				if offset+size > len(data) {
					return nil, fmt.Errorf("tga: short packet")
				}
				if written+size > len(raw) {
					size = len(raw) - written
				}
				copy(raw[written:], data[offset:offset+size])
				written += size
				offset += count * pixelBytes
			}
		}
	} else {
		if offset+len(raw) > len(data) {
			return nil, fmt.Errorf("tga: short pixel data")
		}
		copy(raw, data[offset:offset+len(raw)])
	}
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		row := y
		if descriptor&0x20 == 0 {
			row = height - 1 - y
		}
		for x := 0; x < width; x++ {
			p := raw[(y*width+x)*pixelBytes:]
			var pixel color.NRGBA
			switch {
			case imageType == 1:
				if int(p[0]) < len(palette) {
					pixel = palette[p[0]]
				}
			case imageType == 3:
				pixel = color.NRGBA{R: p[0], G: p[0], B: p[0], A: 255}
			case pixelBytes == 4:
				pixel = color.NRGBA{R: p[2], G: p[1], B: p[0], A: p[3]}
			case pixelBytes == 3:
				pixel = color.NRGBA{R: p[2], G: p[1], B: p[0], A: 255}
			default:
				pixel = color.NRGBA{A: 255}
			}
			result.SetNRGBA(x, row, pixel)
		}
	}
	return result, nil
}
