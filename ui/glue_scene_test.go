package ui

import (
	"os"
	"testing"
)

func TestLivePatch4LoginSceneExposesVisibleModels(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.SetInitialCredentials("denveous", "", false)
	for index := 0; index < 5; index++ {
		engine.Update(1.0 / 60)
	}
	models := engine.VisibleGlueSceneModels()
	if len(models) < 40 || engine.VisibleGlueSceneKey() == "" {
		t.Fatalf("visible patch4 models=%d key=%q", len(models), engine.VisibleGlueSceneKey())
	}
	if models[0].Path == "" || models[0].Scale <= 0 {
		t.Fatalf("first patch4 model=%+v", models[0])
	}
	lightModels, cameraModels := 0, 0
	for _, model := range models {
		if model.HasLight {
			lightModels++
		}
		if model.Camera == 1 {
			cameraModels++
		}
	}
	if lightModels != len(models) || cameraModels != len(models) {
		t.Fatalf("patch4 scene metadata lights=%d cameras=%d models=%d", lightModels, cameraModels, len(models))
	}
	if errors := engine.Rt.ScriptErrors(); len(errors) != 0 {
		t.Fatalf("patch4 scene script errors=%v", errors)
	}
}
