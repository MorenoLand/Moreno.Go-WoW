package ui

import (
	"os"
	"testing"

	"github.com/g3n/engine/window"
)

func TestLiveWorldMenuHotkeysOpenNativePanels(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.Rt.SetUnit("player", UnitInfo{Exists: true, Name: "Tester", Level: 21, Health: 1, HealthMax: 1, Power: 1, PowerMax: 1, PowerToken: "MANA", Connected: true, Player: true, Visible: true})
	if err := engine.LoadWorldUI(); err != nil {
		t.Fatal(err)
	}
	engine.RenderWorld(960, 640)
	for _, test := range []struct {
		key   window.Key
		frame string
	}{
		{window.KeyL, "QuestLogFrame"},
		{window.KeyC, "CharacterFrame"},
		{window.KeyP, "SpellBookFrame"},
	} {
		if engine.Rt.widgets[test.frame] == nil {
			t.Fatalf("missing %s", test.frame)
		}
		if !engine.HandleKey(test.key) || !engine.Rt.widgets[test.frame].shown {
			t.Fatalf("hotkey %v did not open %s; errors=%v", test.key, test.frame, engine.Rt.ScriptErrors())
		}
		if engine.Rt.widgets["GameMenuFrame"].shown {
			t.Fatalf("%s left GameMenuFrame open", test.frame)
		}
		if !engine.HandleKey(test.key) || engine.Rt.widgets[test.frame].shown {
			t.Fatalf("hotkey %v did not close %s; errors=%v", test.key, test.frame, engine.Rt.ScriptErrors())
		}
	}
	if errors := engine.Rt.ScriptErrors(); len(errors) != 0 {
		t.Fatalf("world hotkey script errors=%v", errors)
	}
}
