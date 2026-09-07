package render

import (
	"os"
	"testing"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

func TestLiveWorldAuraPacketUpdatesBuffFrame(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := ui.LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.Rt.SetUnit("player", ui.UnitInfo{Exists: true, Name: "Tester", Health: 1, HealthMax: 1, Power: 1, PowerMax: 1, PowerToken: "MANA", Connected: true, Player: true, Visible: true})
	if err := engine.LoadWorldUI(); err != nil {
		t.Fatal(err)
	}
	state := worldAuraState{}
	if !state.apply(engine.Rt, engine.AssetLoader, world.AuraUpdateData{GUID: 0x1234, Updates: []world.AuraUpdateEntry{{Slot: 0, Aura: world.AuraSlot{SpellID: 21562, Flags: 0x08, Charges: 1, MaxDuration: 60000, Duration: 30000}}}}, true, 0x1234) {
		t.Fatal("player aura update was not applied")
	}
	if !engine.Rt.Execute(`local name = UnitAura("player", 1, "HELPFUL"); assert(name, "unit aura")`, "@world-aura-unit-test.lua") {
		t.Fatalf("unit aura state errors=%v", engine.Rt.ScriptErrors())
	}
	engine.RenderWorld(960, 640)
	if !engine.Rt.Execute(`assert(BuffButton1 and BuffButton1:IsShown(), "button"); assert(BuffButton1Icon:GetTexture() and BuffButton1Icon:GetTexture() ~= "Interface\\Icons\\INV_Misc_QuestionMark", "icon")`, "@world-aura-test.lua") {
		t.Fatalf("buff packet state errors=%v", engine.Rt.ScriptErrors())
	}
	state.apply(engine.Rt, engine.AssetLoader, world.AuraUpdateData{GUID: 0x1234, Updates: []world.AuraUpdateEntry{{Slot: 0, Aura: world.AuraSlot{}}}}, false, 0x1234)
	if !engine.Rt.Execute(`assert(not BuffButton1:IsShown())`, "@world-aura-remove-test.lua") {
		t.Fatal("removed aura remained visible")
	}
	if errors := engine.Rt.ScriptErrors(); len(errors) != 0 {
		t.Fatalf("aura script errors=%v", errors)
	}
}
