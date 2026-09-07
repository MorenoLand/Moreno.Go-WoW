package render

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/MorenoLand/Moreno.WoW/ui"
)

func TestTmpLoginTextureDiag(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	base := `INTERFACE\GLUES\MODELS\UI_MAINMENU\NORTHREND\`
	for _, path := range []string{base + "WOTLK_LOGIN_LANDING01.BLP", base + "WOTLK_LOGIN_LANDING02.BLP", base + "WOTLK_LOGIN_LANDING03.BLP", base + "WOTLK_LOGIN_LANDING04.BLP", base + "WOTLK_LOGIN_LANDING05.BLP", base + "WOTLK_LOGIN_LANDING06.BLP"} {
		data, readErr := loader.ReadAsset(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		decoded, decodeErr := ui.DecodeBLP(data)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		bounds := decoded.Bounds()
		result := image.NewRGBA(bounds)
		draw.Draw(result, bounds, &image.Uniform{C: color.RGBA{R: 24, G: 24, B: 24, A: 255}}, image.Point{}, draw.Src)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				pixel := color.NRGBAModel.Convert(decoded.At(x, y)).(color.NRGBA)
				result.SetRGBA(x, y, color.RGBA{R: 80 + pixel.A/3, G: 24 + pixel.A/2, B: 180 + pixel.A/3, A: 255})
			}
		}
		name := strings.TrimSuffix(strings.TrimPrefix(path, base), ".BLP")
		file, createErr := os.Create(`..\bin\tmp-` + name + `-alpha.png`)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if encodeErr := png.Encode(file, result); encodeErr != nil {
			file.Close()
			t.Fatal(encodeErr)
		}
		file.Close()
	}
}
