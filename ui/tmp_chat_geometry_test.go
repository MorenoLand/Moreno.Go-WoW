package ui

import (
	"os"
	"testing"
)

func TestTmpChatGeometry(t *testing.T) {
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
	for _, state := range []struct{ name, script string }{
		{"general", "ChatFrame1Tab:Click()"},
		{"combat", "ChatFrame2Tab:Click()"},
		{"general-again", "ChatFrame1Tab:Click()"},
	} {
		if !engine.Rt.Execute(state.script, "@tmp-chat-geometry.lua") {
			t.Fatal(engine.Rt.ScriptErrors())
		}
		engine.RenderWorld(2560, 888)
		t.Logf("STATE %s", state.name)
		for _, name := range []string{"ChatFrame1", "ChatFrame1ButtonFrame", "ChatFrame2", "ChatFrame2ButtonFrame", "CombatLogButtons", "CombatLogQuickButtonFrame_Custom"} {
			widget := engine.Rt.widgets[name]
			if widget == nil {
				t.Logf("%s missing", name)
				continue
			}
			parentShown := true
			for parent := widget.parent; parent != nil; parent = parent.parent {
				parentShown = parentShown && parent.shown
			}
			t.Logf("%s shown=%t parentShown=%t rect=%v parent=%s", name, widget.shown, parentShown, widget.renderRect, widgetParentForTmp(widget))
		}
	}
}
