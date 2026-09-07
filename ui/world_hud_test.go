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
	engine.Rt.SetUnit("pet", UnitInfo{Exists: true, Name: "Companion", Level: 20, Health: 1, HealthMax: 1, Power: 1, PowerMax: 1, PowerToken: "MANA", Connected: true, Visible: true})
	if err := engine.LoadWorldUI(); err != nil {
		t.Fatal(err)
	}
	if engine.Rt.widgets["TempEnchant1"].shown || engine.Rt.widgets["TempEnchant2"].shown {
		t.Fatal("empty temporary enchant buttons were visible after world UI load")
	}
	engine.Update(1.0 / 60)
	engine.RenderWorld(960, 640)
	for _, name := range []string{"MainMenuBar", "MainMenuBarArtFrame", "MainMenuExpBar", "PlayerFrame", "PlayerFrameHealthBar", "PlayerFrameManaBar"} {
		frame := engine.Rt.widgets[name]
		if frame == nil || !frame.shown {
			t.Fatalf("%s missing or hidden: %#v", name, frame)
		}
	}
	for _, name := range []string{"TargetFrame", "PetFrame", "PetFrameHealthBar", "PetFrameManaBar"} {
		if engine.Rt.widgets[name] == nil {
			t.Fatalf("%s missing", name)
		}
	}
	for _, name := range []string{"CharacterFrame", "PaperDollFrame"} {
		if engine.Rt.widgets[name] == nil {
			t.Fatalf("%s missing", name)
		}
	}
	bar := engine.Rt.widgets["MainMenuExpBar"]
	if bar.kind != kindStatusBar || bar.statusBarTexture == nil || bar.statusBarTexture.textureFile == "" {
		 t.Fatalf("MainMenuExpBar status state=%#v", bar)
	}
	health := engine.Rt.widgets["PlayerFrameHealthBar"]
	mana := engine.Rt.widgets["PlayerFrameManaBar"]
	if health.statusBarColor != (rgba{r: 0, g: 1, b: 0, a: 1}) || mana.statusBarColor != (rgba{r: 0, g: 0, b: 1, a: 1}) {
		t.Fatalf("unit bar colors health=%v mana=%v", health.statusBarColor, mana.statusBarColor)
	}
	if name := engine.Rt.widgets["PlayerName"]; name == nil || name.text != "Tester" {
		t.Fatalf("PlayerName=%#v", name)
	}
	if len(engine.Rt.ScriptErrors()) != 0 {
		t.Fatalf("world UI script errors=%v", engine.Rt.ScriptErrors())
	}
}
