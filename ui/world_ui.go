package ui

import (
	"errors"
	"fmt"
	"io/fs"
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
	`Interface\FrameXML\GameTooltip.xml`,
	`Interface\FrameXML\AutoComplete.xml`,
	`Interface\FrameXML\MoneyFrame.lua`,
	`Interface\FrameXML\MoneyFrame.xml`,
	`Interface\FrameXML\MoneyInputFrame.lua`,
	`Interface\FrameXML\MoneyInputFrame.xml`,
	`Interface\FrameXML\ItemButtonTemplate.xml`,
	`Interface\FrameXML\ClassTrainerFrameTemplates.xml`,
	`Interface\FrameXML\StaticPopup.lua`,
	`Interface\FrameXML\StaticPopup.xml`,
	`Interface\FrameXML\VideoOptionsFrame.xml`,
	`Interface\FrameXML\AudioOptionsFrame.xml`,
	`Interface\FrameXML\InterfaceOptionsFrame.xml`,
	`Interface\FrameXML\InterfaceOptionsPanels.lua`,
	`Interface\FrameXML\InterfaceOptionsPanels.xml`,
	`Interface\FrameXML\GameMenuFrame.xml`,
	`Interface\FrameXML\TextStatusBar.lua`,
	`Interface\FrameXML\TextStatusBar.xml`,
	`Interface\FrameXML\MainMenuBar.xml`,
}

func (eng *UIEngine) LoadWorldUI() error {
	if eng.worldUIReady {
		return nil
	}
	if eng.AssetLoader == nil || eng.Rt == nil {
		return fmt.Errorf("world UI has no asset loader")
	}
	if eng.Rt.Host != nil {
		if _, height := eng.Rt.Host.ScreenSize(); height > 0 {
			eng.Rt.SetCVar("uiScale", fmt.Sprintf("%.6f", height/768))
		}
	}
	for _, path := range worldUIFiles {
		if (path == `Interface\FrameXML\VideoOptionsFrame.xml` && eng.Rt.widgets["VideoOptionsFrame"] != nil) || (path == `Interface\FrameXML\AudioOptionsFrame.xml` && eng.Rt.widgets["AudioOptionsFrame"] != nil) {
			continue
		}
		if path == `Interface\FrameXML\FloatingChatFrame.xml` {
			if err := eng.loadCombatLogBase(); err != nil {
				return err
			}
		}
		if path == `Interface\FrameXML\GameTooltip.xml` && eng.Rt.L.GetGlobal("GameTooltip") == lua.LNil {
			if !eng.Rt.Execute(`GameTooltip = CreateFrame("Frame", "GameTooltip", UIParent); GameTooltip:Hide();`, "@world-ui-tooltip.lua") {
				return fmt.Errorf("initialize GameTooltip: %v", eng.Rt.ScriptErrors())
			}
		}
		if err := eng.AssetLoader.LoadInterfaceFile(path); err != nil {
			return fmt.Errorf("load world UI %s: %w", path, err)
		}
	}
	if err := eng.loadCombatLogAddon(); err != nil {
		return err
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
    if DEFAULT_CHATFRAME_COLOR then
        FCF_SetWindowColor(ChatFrame1, DEFAULT_CHATFRAME_COLOR.r, DEFAULT_CHATFRAME_COLOR.g, DEFAULT_CHATFRAME_COLOR.b);
    end
    if DEFAULT_CHATFRAME_ALPHA then
        FCF_SetWindowAlpha(ChatFrame1, DEFAULT_CHATFRAME_ALPHA);
    end
end`, "@world-ui-init.lua") {
		return fmt.Errorf("initialize world chat UI: %v", eng.Rt.ScriptErrors())
	}
	// ChatFrame1 waits for UPDATE_CHAT_WINDOWS before FloatingChatFrame_Update
	// runs FCF_SetWindowName / PanelTemplates_TabResize. Fire the same event
	// the original client emits after chat saved-variables load.
	eng.Rt.FireEvent("UPDATE_CHAT_WINDOWS")
	eng.Rt.FireEvent("UPDATE_FLOATING_CHAT_WINDOWS")
	// ChatFrame_ConfigEventHandler re-Shows every saved "shown" window after
	// FloatingChatFrame_Update docks them. Force a dock tab pass so Combat Log
	// stays hidden while General is selected.
	if !eng.Rt.Execute(`
if FCF_DockUpdate then
    FCF_DockUpdate();
end
if ChatFrame1 then
    FCF_SetButtonSide(ChatFrame1, "left", true);
end`, "@world-ui-button-side.lua") {
		return fmt.Errorf("set world chat button side: %v", eng.Rt.ScriptErrors())
	}
	if !eng.Rt.Execute(`
if not StaticPopup_Visible then
    function StaticPopup_Visible()
        return nil;
    end
end
if not UpdateMicroButtons then
    function UpdateMicroButtons()
    end
end
if not Disable_BagButtons then
    function Disable_BagButtons()
    end
end
if not Enable_BagButtons then
    function Enable_BagButtons()
    end
end
if not VoiceChat_Toggle then
    function VoiceChat_Toggle()
    end
end
for _, name in ipairs({
    "GameTooltip",
    "Minimap",
    "MinimapCluster",
    "HelpFrame",
    "VideoOptionsFrame",
    "AudioOptionsFrame",
    "InterfaceOptionsFrame",
    "MultiCastFlyoutFrame",
    "OpacityFrame",
    "CharacterFrame",
    "SpellBookFrame",
    "QuestLogFrame",
    "PVPParentFrame",
    "FriendsFrame",
    "LFDParentFrame",
    "PlayerTalentFrame",
    "AchievementFrame",
    "KeyRingButton",
    "ChannelFrameAutoJoin",
    "VoiceChatTalkers",
}) do
    if not _G[name] then
        local frame = CreateFrame("Frame", name, UIParent);
        frame:Hide();
    end
end
`, "@world-ui-game-menu-stubs.lua") {
		return fmt.Errorf("initialize game menu stubs: %v", eng.Rt.ScriptErrors())
	}
	for _, addon := range []string{"Blizzard_BindingUI", "Blizzard_MacroUI"} {
		path := `Interface\AddOns\` + addon + `\` + addon + `.toc`
		if _, err := eng.AssetLoader.ReadFile(path); err == nil {
			if loaded, reason := eng.loadAddOn(addon); !loaded {
				return fmt.Errorf("load world UI %s: %s", path, reason)
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("probe world UI %s: %w", path, err)
		}
	}
	// Drop glue-screen keyboard focus (e.g. AccountLoginAccountEdit) so the
	// first world ESC runs TOGGLEGAMEMENU instead of only clearing focus.
	eng.Rt.setFocus(nil)
	eng.syncCombatLogButtons()
	eng.worldUIReady = true
	return nil
}

func (eng *UIEngine) syncCombatLogButtons() {
	if eng == nil || eng.Rt == nil {
		return
	}
	buttons := eng.Rt.widgets["CombatLogButtons"]
	combat := eng.Rt.widgets["ChatFrame2"]
	if buttons != nil && combat != nil {
		buttons.shown = combat.shown
	}
}

func (eng *UIEngine) loadCombatLogBase() error {
	path := `Interface\FrameXML\CombatLog.xml`
	if _, err := eng.AssetLoader.ReadFile(path); err == nil {
		if err := eng.AssetLoader.LoadInterfaceFile(path); err != nil {
			return fmt.Errorf("load world UI %s: %w", path, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("probe world UI %s: %w", path, err)
	}
	return nil
}

func (eng *UIEngine) loadCombatLogAddon() error {
	combatLogTOC := `Interface\AddOns\Blizzard_CombatLog\Blizzard_CombatLog.toc`
	if _, err := eng.AssetLoader.ReadFile(combatLogTOC); err == nil {
		if loaded, reason := eng.loadAddOn("Blizzard_CombatLog"); !loaded {
			return fmt.Errorf("load world UI %s: %s", combatLogTOC, reason)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("probe world UI %s: %w", combatLogTOC, err)
	}
	return nil
}

func (eng *UIEngine) loadAddOn(name string) (bool, string) {
	if eng == nil || eng.AssetLoader == nil || eng.Rt == nil {
		return false, "ADDON_NOT_FOUND"
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if eng.Rt.loadedAddOns[key] {
		return true, ""
	}
	tocs := map[string]string{
		"blizzard_bindingui": `Interface\AddOns\Blizzard_BindingUI\Blizzard_BindingUI.toc`,
		"blizzard_combatlog": `Interface\AddOns\Blizzard_CombatLog\Blizzard_CombatLog.toc`,
		"blizzard_macroui":   `Interface\AddOns\Blizzard_MacroUI\Blizzard_MacroUI.toc`,
	}
	toc, ok := tocs[key]
	if !ok {
		return false, "ADDON_NOT_FOUND"
	}
	if _, err := eng.AssetLoader.ReadFile(toc); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, "ADDON_NOT_FOUND"
		}
		return false, "ADDON_LOAD_FAILED"
	}
	if err := eng.AssetLoader.LoadTOC(toc, nil); err != nil {
		return false, "ADDON_LOAD_FAILED"
	}
	eng.Rt.loadedAddOns[key] = true
	eng.Rt.FireEvent("ADDON_LOADED", lua.LString(name))
	return true, ""
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

const logoutCampSeconds = 20

func (rt *Runtime) beginLogout(quit bool) {
	if rt == nil {
		return
	}
	rt.logoutPending = !quit
	rt.quitPending = quit
	rt.logoutRemaining = logoutCampSeconds
	if quit {
		rt.FireEvent("PLAYER_QUITING")
	} else {
		rt.FireEvent("PLAYER_CAMPING")
	}
}

func (rt *Runtime) cancelLogout() {
	if rt == nil {
		return
	}
	if !rt.logoutPending && !rt.quitPending {
		return
	}
	rt.logoutPending = false
	rt.quitPending = false
	rt.logoutRemaining = 0
	rt.FireEvent("LOGOUT_CANCEL")
}

func (rt *Runtime) completeLogout() {
	if rt == nil {
		return
	}
	rt.logoutPending = false
	rt.quitPending = false
	rt.logoutRemaining = 0
	rt.hideLogoutPopup("CAMP")
	if host, ok := rt.Host.(LogoutHost); ok {
		host.Logout()
	}
}

func (rt *Runtime) completeQuit() {
	if rt == nil {
		return
	}
	rt.logoutPending = false
	rt.quitPending = false
	rt.logoutRemaining = 0
	rt.hideLogoutPopup("QUIT")
	if rt.Host != nil {
		rt.Host.Quit(false)
	}
}

func (rt *Runtime) hideLogoutPopup(which string) {
	if rt == nil || rt.L == nil {
		return
	}
	fn := rt.L.GetGlobal("StaticPopup_Hide")
	if fn.Type() != lua.LTFunction {
		return
	}
	top := rt.L.GetTop()
	defer rt.L.SetTop(top)
	rt.L.Push(fn)
	rt.L.Push(lua.LString(which))
	if err := rt.L.PCall(1, 0, nil); err != nil {
		rt.recordScriptError("@logout-popup-hide.lua", err.Error())
	}
}

func (rt *Runtime) tickLogout(elapsed float64) bool {
	if rt == nil || (!rt.logoutPending && !rt.quitPending) {
		return false
	}
	rt.logoutRemaining -= elapsed
	if rt.logoutRemaining > 0 {
		return false
	}
	if rt.quitPending {
		rt.completeQuit()
	} else {
		rt.completeLogout()
	}
	return true
}

// ToggleGameMenu runs the live FrameXML ToggleGameMenu binding used by ESC.
func (eng *UIEngine) ToggleGameMenu() bool {
	if eng == nil || eng.Rt == nil || !eng.worldUIReady {
		return false
	}
	return eng.Rt.Execute(`ToggleGameMenu();`, "@bindings-TOGGLEGAMEMENU.lua")
}

func (eng *UIEngine) GameMenuShown() bool {
	if eng == nil || eng.Rt == nil {
		return false
	}
	menu := eng.Rt.widgets["GameMenuFrame"]
	if menu == nil || !menu.shown {
		return false
	}
	for p := menu.parent; p != nil; p = p.parent {
		if !p.shown {
			return false
		}
	}
	return true
}
