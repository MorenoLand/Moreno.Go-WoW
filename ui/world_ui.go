package ui

import (
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

var worldUIFiles = []string{
	`Interface\FrameXML\GlobalStrings.lua`,
	`Interface\FrameXML\Constants.lua`,
	`Interface\FrameXML\Fonts.xml`,
	`Interface\FrameXML\FontStyles.xml`,
	`Interface\FrameXML\Localization.xml`,
	`Interface\FrameXML\BasicControls.xml`,
	`Interface\FrameXML\UIPanelTemplates.lua`,
	`Interface\FrameXML\UIPanelTemplates.xml`,
	`Interface\FrameXML\UIMenu.xml`,
	`Interface\FrameXML\UIDropDownMenu.xml`,
	`Interface\FrameXML\AutoComplete.lua`,
	`Interface\FrameXML\UIParent.xml`,
	`Interface\FrameXML\ChatFrame.xml`,
	`Interface\FrameXML\FloatingChatFrame.xml`,
}

func (eng *UIEngine) LoadWorldUI() error {
	if eng.worldUIReady {
		return nil
	}
	if eng.AssetLoader == nil || eng.Rt == nil {
		return fmt.Errorf("world UI has no asset loader")
	}
	for _, path := range worldUIFiles {
		if err := eng.AssetLoader.LoadInterfaceFile(path); err != nil {
			return fmt.Errorf("load world UI %s: %w", path, err)
		}
	}
	eng.worldRoot = eng.Rt.widgets["UIParent"]
	if eng.worldRoot == nil {
		return fmt.Errorf("world UI did not create UIParent")
	}
	if !eng.Rt.Execute(`
if ChatFrame1 then
    ChatFrame1:Show();
    ChatFrame_RegisterForMessages(ChatFrame1, "SAY", "YELL", "PARTY", "RAID", "GUILD", "OFFICER", "WHISPER", "EMOTE", "TEXT_EMOTE", "CHANNEL");
    ChatEdit_SetLastActiveWindow(ChatFrame1.editBox);
    FCF_SetButtonSide(ChatFrame1, "left");
end`, "@world-ui-init.lua") {
		return fmt.Errorf("initialize world chat UI")
	}
	eng.worldUIReady = true
	return nil
}

func (eng *UIEngine) SetWorldLoading(loading bool) {
	eng.worldLoading = loading
	if button := eng.Rt.widgets["CharSelectEnterWorldButton"]; button != nil {
		if loading {
			button.enabled = false
			button.buttonState = "DISABLED"
		} else {
			button.enabled = true
			button.buttonState = "NORMAL"
		}
	}
}

func (eng *UIEngine) SelectedCharacterIndex() int {
	if eng == nil || eng.Rt == nil {
		return -1
	}
	return eng.Rt.Glue.SelectedCharacter - 1
}

func (eng *UIEngine) FireWorldChat(event string, args ...lua.LValue) int {
	if eng == nil || eng.Rt == nil || !strings.HasPrefix(event, "CHAT_MSG_") {
		return 0
	}
	return eng.Rt.FireEvent(event, args...)
}
