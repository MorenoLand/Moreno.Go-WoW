package ui

import (
	"os"
	"testing"
)

func TestLiveWorldMainMenuBarLoads(t *testing.T) {
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
	for _, name := range []string{"MainMenuBar", "MainMenuBarArtFrame", "MainMenuExpBar"} {
		frame := engine.Rt.widgets[name]
		if frame == nil || !frame.shown {
			t.Fatalf("%s missing or hidden: %#v", name, frame)
		}
	}
	bar := engine.Rt.widgets["MainMenuExpBar"]
	if bar.kind != kindStatusBar || bar.statusBarTexture == nil || bar.statusBarTexture.textureFile == "" {
		t.Fatalf("MainMenuExpBar status state=%#v", bar)
	}
	if len(engine.Rt.ScriptErrors()) != 0 {
		t.Fatalf("world UI script errors=%v", engine.Rt.ScriptErrors())
	}
}
