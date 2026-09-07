package ui

import (
	"os"
	"strings"
	"testing"
)

func TestTmpFrameXMLInventory(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	for _, path := range []string{`Interface\FrameXML\FrameXML.toc`, `Interface\FrameXML\FrameXML.xml`, `Interface\FrameXML\CombatLog.xml`, `Interface\FrameXML\MainMenuBar.xml`, `Interface\FrameXML\ActionBarFrame.xml`, `Interface\FrameXML\PlayerFrame.xml`, `Interface\FrameXML\UnitFrame.xml`, `Interface\FrameXML\TargetFrame.xml`, `Interface\FrameXML\PartyFrame.xml`, `Interface\FrameXML\Minimap.xml`, `Interface\FrameXML\WorldStateFrame.xml`, `Interface\FrameXML\ChatFrame.lua`, `Interface\FrameXML\FloatingChatFrame.lua`} {
		data, readErr := engine.AssetLoader.ReadFile(path)
		if readErr != nil {
			t.Logf("missing %s: %v", path, readErr)
			continue
		}
		t.Logf("asset %s bytes=%d", path, len(data))
		if strings.Contains(strings.ToLower(path), "combatlog.xml") {
			for index, line := range strings.Split(string(data), "\n") {
				t.Logf("combat %d %s", index+1, strings.TrimSpace(line))
			}
		}
		if strings.HasSuffix(strings.ToLower(path), ".toc") || strings.HasSuffix(strings.ToLower(path), ".xml") && strings.Contains(path, "FrameXML.xml") {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "##") && !strings.HasPrefix(line, "<!--") {
					t.Logf("entry %s", line)
				}
			}
		}
	}
}
