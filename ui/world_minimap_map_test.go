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
	if !engine.SetWorldMinimapMap("Azeroth") {
		t.Fatal("Azeroth minimap map was not selected")
	}
	mapWidget := engine.Rt.widgets["MinimapMap"]
	if mapWidget == nil || mapWidget.textureFile != `Interface\WorldMap\Azeroth\Azeroth1` {
		t.Fatalf("minimap map=%#v", mapWidget)
	}
	if errors := engine.Rt.ScriptErrors(); len(errors) != 0 {
		t.Fatalf("minimap map script errors=%v", errors)
	}
}
