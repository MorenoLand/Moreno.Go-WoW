package ui

import (
	"os"
	"testing"
)

func TestLiveWorldMinimapFollowsMapName(t *testing.T) {
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
	if !engine.SetWorldMinimapPosition("Azeroth", 0, 0) {
		t.Fatal("Azeroth minimap tiles were not selected")
	}
	mapWidget := engine.Rt.widgets["MinimapMap"]
	if mapWidget == nil || engine.minimapMapName != "Azeroth" || mapWidget.texCoordL >= mapWidget.texCoordR || mapWidget.texCoordT >= mapWidget.texCoordB {
		t.Fatalf("minimap map=%#v", mapWidget)
	}
	if errors := engine.Rt.ScriptErrors(); len(errors) != 0 {
		t.Fatalf("minimap map script errors=%v", errors)
	}
}
