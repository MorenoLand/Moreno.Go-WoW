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
	frame := engine.RenderWorld(960, 640)
	centerX := int((mapWidget.renderRect.X0 + mapWidget.renderRect.X1) * engine.uiScale / 2)
	centerY := 640 - int((mapWidget.renderRect.Y0+mapWidget.renderRect.Y1)*engine.uiScale/2)
	if frame.RGBAAt(centerX, centerY).A == 0 {
		t.Fatal("minimap center is transparent")
	}
	cornerX := int((mapWidget.renderRect.X0+10)*engine.uiScale)
	cornerY := 640 - int((mapWidget.renderRect.Y1-10)*engine.uiScale)
	if frame.RGBAAt(cornerX, cornerY).A != 0 {
		t.Fatal("minimap map leaked outside circular mask")
	}
	if errors := engine.Rt.ScriptErrors(); len(errors) != 0 {
		t.Fatalf("minimap script errors=%v", errors)
	}
}
