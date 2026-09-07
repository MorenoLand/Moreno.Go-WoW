package ui

import (
	"os"
	"testing"
)

func TestLiveWorldMinimapHasMapContent(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if err := engine.LoadWorldUI(); err != nil {
		t.Fatal(err)
	}
	engine.RenderWorld(960, 640)
	mapWidget := engine.Rt.widgets["MinimapMap"]
	if mapWidget == nil || !mapWidget.shown || mapWidget.textureFile == "" || engine.loadBLP(mapWidget.textureFile) == nil {
		t.Fatalf("minimap map content=%#v", mapWidget)
	}
	if mapWidget.renderRect.W() < 139 || mapWidget.renderRect.H() < 139 {
		t.Fatalf("minimap map rect=%v", mapWidget.renderRect)
	}
	if errors := engine.Rt.ScriptErrors(); len(errors) != 0 {
		t.Fatalf("minimap script errors=%v", errors)
	}
}
