package ui

import (
	"image/png"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestTmpHUDCapture(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.Rt.SetUnit("player", UnitInfo{Exists: true, Name: "Tester", Health: 1, HealthMax: 1, Power: 1, PowerMax: 1, PowerToken: "MANA", Connected: true, Player: true, Visible: true, Auras: []AuraInfo{{Name: "Fortitude", Texture: `Interface\Icons\Spell_Holy_PrayerOfFortitude`, Count: 1}, {Name: "Test", Texture: `Interface\Icons\Spell_Nature_WispSplode`, Count: 2}}})
	if err := engine.LoadWorldUI(); err != nil {
		t.Fatal(err)
	}
	engine.SetWorldMinimapPosition("Azeroth", 0, 0)
	engine.Update(1.0 / 60)
	frame := engine.RenderWorld(960, 640)
	for _, name := range []string{"MinimapCluster", "Minimap", "MinimapMap", "MinimapBackdrop", "MinimapBorder", "MinimapZoomIn", "MinimapZoomOut"} {
		widget := engine.Rt.widgets[name]
		if widget != nil {
			t.Logf("minimap layer %s level=%d layer=%d rect=%v parent=%s points=%v texture=%q backdrop=%+v", name, widget.frameLevel, widget.layerLevel, widget.renderRect, widgetParentForTmp(widget), widget.points, widget.textureFile, widget.backdrop)
		}
	}
	names := make([]string, 0)
	for name, widget := range engine.Rt.widgets {
		lower := strings.ToLower(name)
		if widget.shown && (strings.Contains(lower, "buff") || strings.Contains(lower, "enchant")) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		widget := engine.Rt.widgets[name]
		t.Logf("visible aura widget %s kind=%s rect=%v parent=%s texture=%q", name, widget.kind.objectType(), widget.renderRect, widgetParentForTmp(widget), widget.textureFile)
	}
	file, err := os.Create("../bin/tmp-hud.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, frame); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()
	for _, name := range []string{"MainMenuBar", "MainMenuBarArtFrame", "ActionButton1", "ActionButton12", "ActionBarUpButton"} {
		widget := engine.Rt.widgets[name]
		t.Logf("%s shown=%t rect=%v", name, widget != nil && widget.shown, func() Rect { if widget == nil { return Rect{} }; return widget.renderRect }())
	}
}
