package ui

import (
	"image"
	"image/png"
	"os"
	"strings"
	"testing"
)

func TestTmpWorldInventory(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	for _, path := range []string{`Interface\FrameXML\UIParent.xml`, `Interface\FrameXML\MainMenuBar.xml`, `Interface\FrameXML\ActionBarFrame.xml`, `Interface\FrameXML\UnitFrame.xml`, `Interface\FrameXML\Minimap.xml`, `Interface\FrameXML\PartyFrame.xml`, `Interface\FrameXML\TargetFrame.xml`, `Interface\FrameXML\WorldStateFrame.xml`, `Interface\FrameXML\ChatFrame.xml`, `Interface\FrameXML\FloatingChatFrame.xml`} {
		data, readErr := engine.AssetLoader.ReadFile(path)
		if readErr != nil {
			t.Logf("asset missing %s: %v", path, readErr)
			continue
		}
		t.Logf("asset %s bytes=%d", path, len(data))
		if path == `Interface\FrameXML\UIParent.xml` {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Contains(line, "Include") || strings.Contains(line, "FrameXML") || strings.Contains(line, "MainMenu") || strings.Contains(line, "ActionBar") || strings.Contains(line, "UnitFrame") {
					t.Logf("uiparent %s", strings.TrimSpace(line))
				}
			}
		}
	}
	if err := engine.LoadWorldUI(); err != nil {
		t.Fatal(err)
	}
	if !engine.Rt.Execute(`ChatFrame1:AddMessage("general before transition", 1, 1, 0); ChatFrame2Tab:Click()`, "@tmp-chat-combat.lua") {
		t.Fatal(engine.Rt.ScriptErrors())
	}
	if output := os.Getenv("WOW_TEST_CHAT_RENDER_DIR"); output != "" {
		saveTmpChatFrame(t, output+"\\combat.png", engine.RenderWorld(2560, 888))
	}
	if !engine.Rt.Execute(`ChatFrame1Tab:Click()`, "@tmp-chat-general.lua") {
		t.Fatal(engine.Rt.ScriptErrors())
	}
	if output := os.Getenv("WOW_TEST_CHAT_RENDER_DIR"); output != "" {
		saveTmpChatFrame(t, output+"\\general.png", engine.RenderWorld(2560, 888))
	}
	for _, name := range []string{"ChatFrame1", "ChatFrame1EditBox", "ChatFrame1ButtonFrame", "ChatFrame2", "CombatLogButtons", "CombatLogQuickButtonFrame_Custom"} {
		widget := engine.Rt.widgets[name]
		if widget == nil {
			t.Logf("chat state=%s missing", name)
			continue
		}
		parent := ""
		if widget.parent != nil {
			parent = widget.parent.name
		}
		t.Logf("chat state=%s shown=%t parent=%s parentShown=%t rect=%v messages=%d", name, widget.shown, parent, widget.parent == nil || widget.parent.shown, widget.renderRect, len(widget.messages))
	}
	for name, widget := range engine.Rt.widgets {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "action") || strings.Contains(lower, "mainmenu") || strings.Contains(lower, "player") || strings.Contains(lower, "target") || strings.Contains(lower, "minimap") || strings.Contains(lower, "party") || strings.Contains(lower, "pet") || strings.Contains(lower, "chat") {
			t.Logf("widget %s kind=%s shown=%t parent=%s", name, widget.kind.objectType(), widget.shown, widgetParentForTmp(widget))
		}
	}
	t.Logf("world root=%s children=%d errors=%v", engine.worldRoot.name, len(engine.worldRoot.children), engine.Rt.ScriptErrors())
}

func saveTmpChatFrame(t *testing.T, path string, frame image.Image) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, frame); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()
}

func widgetParentForTmp(w *widget) string {
	if w == nil || w.parent == nil {
		return ""
	}
	return w.parent.name
}
