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
	engine.Rt.SetUnit("player", UnitInfo{Exists: true, Name: "Tester", Level: 21, RaceID: 4, RaceFile: "NightElf", ClassID: 2, ClassFile: "PALADIN", Health: 1, HealthMax: 1, Power: 1, PowerMax: 1, PowerToken: "MANA", Sex: 2, Connected: true, Player: true, Visible: true})
	if err := engine.LoadWorldUI(); err != nil {
		t.Fatal(err)
	}
	engine.RenderWorld(960, 640)
	for _, name := range []string{"MainMenuBar", "MainMenuBarArtFrame", "MainMenuExpBar", "PlayerFrame", "PlayerFrameHealthBar", "PlayerFrameManaBar"} {
		frame := engine.Rt.widgets[name]
		if frame == nil || !frame.shown {
			t.Fatalf("%s missing or hidden: %#v", name, frame)
		}
	}
	if engine.Rt.widgets["TargetFrame"] == nil {
		t.Fatal("TargetFrame missing")
	}
	bar := engine.Rt.widgets["MainMenuExpBar"]
	if bar.kind != kindStatusBar || bar.statusBarTexture == nil || bar.statusBarTexture.textureFile == "" {
		t.Fatalf("MainMenuExpBar status state=%#v", bar)
	}
	if name := engine.Rt.widgets["PlayerName"]; name == nil || name.text != "Tester" {
		t.Fatalf("PlayerName=%#v", name)
	}
	if len(engine.Rt.ScriptErrors()) != 0 {
		t.Fatalf("world UI script errors=%v", engine.Rt.ScriptErrors())
	}
}
