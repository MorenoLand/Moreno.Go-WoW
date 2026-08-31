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
