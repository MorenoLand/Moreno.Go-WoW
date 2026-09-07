package ui

import (
	"os"
	"strings"
	"testing"
)

func TestTmpHUDLoad(t *testing.T) {
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
	for _, path := range []string{
		`Interface\FrameXML\WorldFrame.xml`,
		`Interface\FrameXML\AnimTimerFrame.xml`,
		`Interface\FrameXML\TextStatusBar.lua`,
		`Interface\FrameXML\TextStatusBar.xml`,
		`Interface\FrameXML\UIErrorsFrame.xml`,
		`Interface\FrameXML\Sound.lua`,
		`Interface\FrameXML\AlertFrames.xml`,
		`Interface\FrameXML\MirrorTimer.xml`,
		`Interface\FrameXML\CoinPickupFrame.xml`,
		`Interface\FrameXML\StackSplitFrame.xml`,
		`Interface\FrameXML\FadingFrame.xml`,
		`Interface\FrameXML\ZoneText.xml`,
		`Interface\FrameXML\BattlefieldFrame.xml`,
		`Interface\FrameXML\MainMenuBar.xml`,
		`Interface\FrameXML\MainMenuBarMicroButtons.xml`,
		`Interface\FrameXML\Minimap.xml`,
		`Interface\FrameXML\GameTime.xml`,
		`Interface\FrameXML\Cooldown.xml`,
		`Interface\FrameXML\ActionButtonTemplate.xml`,
		`Interface\FrameXML\ActionBarFrame.xml`,
		`Interface\FrameXML\MultiActionBars.xml`,
		`Interface\FrameXML\BuffFrame.xml`,
		`Interface\FrameXML\CombatFeedback.xml`,
		`Interface\FrameXML\CastingBarFrame.xml`,
		`Interface\FrameXML\UnitPopup.xml`,
		`Interface\FrameXML\UnitFrame.xml`,
		`Interface\FrameXML\PlayerFrame.xml`,
		`Interface\FrameXML\PartyFrame.xml`,
		`Interface\FrameXML\TargetFrame.xml`,
		`Interface\FrameXML\PetFrame.xml`,
		`Interface\FrameXML\WorldStateFrame.xml`,
	} {
		before := len(engine.Rt.ScriptErrors())
		loadErr := engine.AssetLoader.LoadInterfaceFile(path)
		errors := engine.Rt.ScriptErrors()[before:]
		t.Logf("load=%s err=%v errors=%d widgets=%d", path, loadErr, len(errors), len(engine.Rt.widgets))
		for index, scriptError := range errors {
			if index >= 3 {
				break
			}
			t.Logf("error=%s:%s", scriptError.Source, strings.Split(scriptError.Message, "\n")[0])
		}
	}
	for _, name := range []string{"WorldFrame", "MainMenuBar", "MainMenuBarArtFrame", "ActionButton1", "ActionButton12", "MinimapCluster", "PlayerFrame", "TargetFrame", "PetFrame", "PartyMemberFrame1", "WorldStateAlwaysUpFrame"} {
		widget := engine.Rt.widgets[name]
		t.Logf("final=%s exists=%t shown=%t", name, widget != nil, widget != nil && widget.shown)
	}
	for _, source := range []struct {
		path       string
		start, end int
	}{
		{`Interface\FrameXML\SecureTemplates.lua`, 65, 82},
		{`Interface\FrameXML\MainMenuBar.lua`, 1, 15},
		{`Interface\FrameXML\MainMenuBar.lua`, 172, 195},
		{`Interface\FrameXML\MainMenuBarMicroButtons.lua`, 1, 15},
		{`Interface\FrameXML\MainMenuBarMicroButtons.lua`, 55, 82},
		{`Interface\FrameXML\MainMenuBarMicroButtons.lua`, 35, 160},
		{`Interface\FrameXML\MainMenuBarMicroButtons.lua`, 160, 300},
		{`Interface\FrameXML\MainMenuBarMicroButtons.xml`, 1, 220},
		{`Interface\FrameXML\MainMenuBarMicroButtons.xml`, 70, 100},
		{`Interface\FrameXML\MainMenuBarMicroButtons.xml`, 1, 70},
		{`Interface\FrameXML\MainMenuBarMicroButtons.xml`, 220, 450},
		{`Interface\FrameXML\MainMenuBar.xml`, 1, 300},
		{`Interface\FrameXML\MainMenuBar.xml`, 300, 500},
		{`Interface\FrameXML\QuestLogFrame.lua`, 280, 350},
		{`Interface\FrameXML\QuestLogFrame.lua`, 510, 530},
		{`Interface\FrameXML\CharacterFrame.lua`, 80, 120},
		{`Interface\FrameXML\SpellBookFrame.lua`, 80, 110},
		{`Interface\FrameXML\BuffFrame.lua`, 1, 260},
		{`Interface\FrameXML\BuffFrame.lua`, 260, 440},
		{`Interface\FrameXML\Minimap.xml`, 1, 260},
		{`Interface\FrameXML\BuffFrame.xml`, 1, 220},
		{`Interface\FrameXML\Minimap.lua`, 20, 36},
		{`Interface\FrameXML\GameTime.lua`, 1, 24},
		{`Interface\FrameXML\ActionButton.lua`, 120, 145},
		{`Interface\FrameXML\UnitFrame.lua`, 70, 100},
		{`Interface\FrameXML\UnitFrame.lua`, 155, 175},
		{`Interface\FrameXML\UnitFrame.lua`, 267, 285},
		{`Interface\FrameXML\UnitPopup.lua`, 445, 465},
		{`Interface\FrameXML\WorldStateFrame.lua`, 105, 120},
	} {
		data, readErr := engine.AssetLoader.ReadFile(source.path)
		if readErr != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		t.Logf("SOURCE %s", source.path)
		for index := source.start - 1; index < source.end && index < len(lines); index++ {
			if index >= 0 {
				t.Logf("%d %s", index+1, strings.TrimSpace(lines[index]))
			}
		}
	}
}
