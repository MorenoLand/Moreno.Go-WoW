package ui

import (
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

func blpFixture(encoding, alphaDepth, alphaType byte, width, height int, payload []byte) []byte {
	data := make([]byte, blpHeaderSize+len(payload))
	copy(data[:4], "BLP2")
	binary.LittleEndian.PutUint32(data[4:8], 1)
	data[8], data[9], data[10] = encoding, alphaDepth, alphaType
	binary.LittleEndian.PutUint32(data[12:16], uint32(width))
	binary.LittleEndian.PutUint32(data[16:20], uint32(height))
	binary.LittleEndian.PutUint32(data[20:24], uint32(blpHeaderSize))
	binary.LittleEndian.PutUint32(data[84:88], uint32(len(payload)))
	copy(data[blpHeaderSize:], payload)
	return data
}

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

func TestDecodeBLPUsesAlphaTypeForDXT3(t *testing.T) {
	payload := make([]byte, 16)
	payload[0] = 0x0f
	payload[1] = 0xf0
	payload[8] = 0xff
	payload[10] = 0
	data := blpFixture(blpEncodingDXT, 4, 1, 4, 4, payload)
	img, err := DecodeBLP(data)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, alpha := img.At(0, 0).RGBA()
	if alpha>>8 != 255 {
		t.Fatalf("DXT3 alpha=%d", alpha>>8)
	}
	_, _, _, alpha = img.At(1, 0).RGBA()
	if alpha>>8 != 0 {
		t.Fatalf("DXT3 alpha=%d", alpha>>8)
	}
}

func TestDecodeBLPRawSwapsBGRA(t *testing.T) {
	data := blpFixture(blpEncodingUncomp, 8, 0, 1, 1, []byte{3, 2, 1, 4})
	img, err := DecodeBLP(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(img.At(0, 0)); got != (color.NRGBA{R: 1, G: 2, B: 3, A: 4}) {
		t.Fatalf("raw pixel=%v", got)
	}
}

func TestDecodeBLPPaletteUsesFourBitAlpha(t *testing.T) {
	payload := []byte{0, 1, 0x2f}
	data := blpFixture(blpEncodingAlpha, 4, 0, 2, 1, payload)
	data[148+2] = 0xff
	data[148+1] = 0xff
	data[148] = 0
	img, err := DecodeBLP(data)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, alpha := img.At(0, 0).RGBA()
	if alpha>>8 != 255 {
		t.Fatalf("palette low nibble alpha=%d", alpha>>8)
	}
	_, _, _, alpha = img.At(1, 0).RGBA()
	if alpha>>8 != 34 {
		t.Fatalf("palette high nibble alpha=%d", alpha>>8)
	}
}
