package render

import (
	"os"
	"testing"

	"github.com/MorenoLand/Moreno.WoW/ui"
)

func TestLivePatch4LoginSceneLoadsCompositeModels(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := ui.LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.SetInitialCredentials("denveous", "", false)
	for index := 0; index < 5; index++ {
		engine.Update(1.0 / 60)
	}
	models := engine.VisibleGlueSceneModels()
	if len(models) < 40 {
		t.Fatalf("visible patch-4 models=%d", len(models))
	}
	scene, err := loadGlueScene(engine.AssetLoader, models)
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	if len(scene.Children()) < 40 {
		t.Fatalf("loaded patch-4 models=%d", len(scene.Children()))
	}
	info, ok := scene.UserData().(glueModelInfo)
	if !ok || info.stats.parts == 0 {
		t.Fatalf("composite scene metadata=%v parts=%d", ok, info.stats.parts)
	}
}

func TestGlueScenePositionUsesNativeModelAxes(t *testing.T) {
	if got, want := glueScenePosition([3]float64{3, 5, 7}), [3]float32{5, 7, -3}; got != want {
		t.Fatalf("scene position=%v want %v", got, want)
	}
}

func TestGlueSceneScaleUsesFrameSquish(t *testing.T) {
	if width, height, depth := glueSceneScale(3, ui.GlueSceneModel{Scale: 2, WidthSquish: 4, HeightSquish: 5}); width != 1.5 || height != 1.2 || depth != 6 {
		t.Fatalf("scene scale=%v,%v,%v", width, height, depth)
	}
}
