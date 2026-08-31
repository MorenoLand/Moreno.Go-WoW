package ui

import (
	"encoding/binary"
	"image"
	"testing"
)

func TestDXT5ColorInterpolationDoesNotOverflow(t *testing.T) {
	block := []byte{255, 255, 0, 0, 0, 0, 0, 0, 0x70, 0x43, 0x30, 0x3b, 0xeb, 0xfa, 0xff, 0xff}
	if got := binary.LittleEndian.Uint16(block[8:10]); got != 0x4370 {
		t.Fatal(got)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	decodeDXT5Block(block, img, 0, 0)
	r, g, b, a := img.At(0, 0).RGBA()
	if r>>8 < 50 || g>>8 < 90 || b>>8 < 110 || a>>8 != 255 {
		t.Fatalf("decoded pixel=%d,%d,%d,%d", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestDXT1ColorModeKeepsIndexThreeOpaque(t *testing.T) {
	block := []byte{0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}
	colorMode := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	decodeDXT1Block(block, colorMode, 0, 0, false)
	_, _, _, alpha := colorMode.At(0, 0).RGBA()
	if alpha>>8 != 255 {
		t.Fatalf("DXT1 color-mode alpha=%d", alpha>>8)
	}
	alphaMode := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	decodeDXT1Block(block, alphaMode, 0, 0, true)
	_, _, _, alpha = alphaMode.At(0, 0).RGBA()
	if alpha != 0 {
		t.Fatalf("DXT1 alpha-mode alpha=%d", alpha>>8)
	}
}
