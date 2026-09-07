package ui

import (
	"os"
	"testing"
)

func TestLiveWorldBuffFrameRendersUnitAuras(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.Rt.SetUnit("player", UnitInfo{Exists: true, Name: "Tester", Health: 1, HealthMax: 1, Power: 1, PowerMax: 1, PowerToken: "MANA", Connected: true, Player: true, Visible: true, Auras: []AuraInfo{{Name: "Test Blessing", Texture: `Interface\Icons\Spell_Holy_WordFortitude`, Count: 2, Duration: 60, ExpirationTime: 60}}})
	if err := engine.LoadWorldUI(); err != nil {
		t.Fatal(err)
	}
	engine.RenderWorld(960, 640)
	buff := engine.Rt.widgets["BuffButton1"]
	icon := engine.Rt.widgets["BuffButton1Icon"]
	count := engine.Rt.widgets["BuffButton1Count"]
	if buff == nil || !buff.shown || icon == nil || icon.textureFile == "" || count == nil || count.text != "2" {
		t.Fatalf("buff state button=%#v icon=%#v count=%#v", buff, icon, count)
	}
	if buff.renderRect != icon.renderRect {
		t.Fatalf("buff icon rect=%v button rect=%v", icon.renderRect, buff.renderRect)
	}
	if errors := engine.Rt.ScriptErrors(); len(errors) != 0 {
		t.Fatalf("buff script errors=%v", errors)
	}
}
