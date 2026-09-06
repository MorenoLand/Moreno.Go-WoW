package render

import (
	"math"
	"os"
	"testing"

	"github.com/MorenoLand/Moreno.WoW/ui"
)

func TestM2PartTintHonorsColorAndWeightTracks(t *testing.T) {
	colorTrack := m2TrackVec3{sequences: []m2Vec3Keys{{times: []uint32{0}, values: [][3]float32{{0.25, 0.5, 0.75}}}}}
	alphaTrack := m2TrackScalar{sequences: []m2ScalarKeys{{times: []uint32{0}, values: []float32{0.5}}}}
	weightTrack := m2TrackScalar{sequences: []m2ScalarKeys{{times: []uint32{0}, values: []float32{0.25}}}}
	model := parsedM2{
		colors:         []m2Color{{colorTrack: colorTrack, alphaTrack: alphaTrack}},
		textureWeights: []m2TextureWeight{{weightTrack: weightTrack}},
	}
	color, alpha := m2PartTintAt(&model, 0, 0, 0, 0, 0)
	if color != [3]float32{0.25, 0.5, 0.75} || math.Abs(float64(alpha-0.125)) > 0.00001 {
		t.Fatalf("tint=%v alpha=%v", color, alpha)
	}
	zeroAlpha := m2TrackScalar{sequences: []m2ScalarKeys{{times: []uint32{0}, values: []float32{0}}}}
	_, alpha = m2PartTintAt(&parsedM2{colors: []m2Color{{alphaTrack: zeroAlpha}}}, 0, -1, 0, 0, 0)
	if alpha != 1 {
		t.Fatalf("zero color alpha=%v want default opaque", alpha)
	}
}

func TestLiveMainMenuM2MaterialTracks(t *testing.T) {
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
	path := `Interface\Glues\Models\UI_MainMenu_Northrend\UI_MainMenu_Northrend.m2`
	data, err := loader.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	model, err := parseM2(data)
	if err != nil {
		t.Fatal(err)
	}
	loadM2AnimationTracks(loader, path, &model)
	if len(model.colors) != 24 || len(model.textureWeights) != 10 || len(model.textureWeightCombos) == 0 {
		t.Fatalf("colors=%d textureWeights=%d weightCombos=%d", len(model.colors), len(model.textureWeights), len(model.textureWeightCombos))
	}
	color, alpha := m2PartTintAt(&model, 13, -1, 0, 0, 0)
	if math.Abs(float64(color[0]-0.5647059)) > 0.0001 || math.Abs(float64(color[1]-0.85098046)) > 0.0001 || math.Abs(float64(color[2]-0.40784317)) > 0.0001 || math.Abs(float64(alpha-0.50001526)) > 0.0001 {
		t.Fatalf("color13=%v alpha=%v", color, alpha)
	}
}
