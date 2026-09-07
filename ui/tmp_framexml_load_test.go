package ui

import (
	"os"
	"strings"
	"testing"
)

func TestTmpFrameXMLLoad(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	before := len(engine.Rt.ScriptErrors())
	loadErr := engine.AssetLoader.LoadTOC(`Interface\FrameXML\FrameXML.toc`, nil)
	newErrors := engine.Rt.ScriptErrors()[before:]
	t.Logf("load err=%v new error count=%d widgets=%d", loadErr, len(newErrors), len(engine.Rt.widgets))
	for index, scriptError := range newErrors {
		if index >= 80 {
			break
		}
		message := strings.Split(scriptError.Message, "\n")[0]
		t.Logf("error %s: %s", scriptError.Source, message)
	}
	for _, name := range []string{"MainMenuBar", "ActionButton1", "Minimap", "PlayerFrame", "PartyMemberFrame1", "TargetFrame", "PetFrame", "WorldStateFrame", "SpellBookFrame", "QuestLogFrame", "CharacterFrame"} {
		widget := engine.Rt.widgets[name]
		t.Logf("widget %s exists=%t shown=%t", name, widget != nil, widget != nil && widget.shown)
	}
	for _, scriptError := range engine.Rt.ScriptErrors()[before:] {
		if strings.Contains(scriptError.Message, "attempt to call") || strings.Contains(scriptError.Message, "attempt to index") || strings.Contains(scriptError.Message, "missing") {
			t.Logf("important error %s: %s", scriptError.Source, scriptError.Message)
		}
	}
}
