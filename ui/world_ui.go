package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

func (eng *UIEngine) LoadWorldUI() error {
	eng.Rt.onMinimapZoom = eng.applyMinimapZoom
	if eng.worldUIReady {
		return nil
	}
	if eng.AssetLoader == nil || eng.Rt == nil {
		return fmt.Errorf("world UI has no asset loader")
	}
	eng.Rt.suppressSound = true
	defer func() { eng.Rt.suppressSound = false }()
	if eng.Rt.Host != nil {
		if _, height := eng.Rt.Host.ScreenSize(); height > 0 {
			eng.Rt.SetCVar("uiScale", fmt.Sprintf("%.6f", height/768))
		}
	}
	// The world UI always has a player unit; FrameXML reads its race and class
	// while it loads.
	if info := eng.Rt.unitInfo("player"); info == nil || !info.Exists {
		if !eng.Rt.SeedPlayerUnitFromGlue() {
			eng.Rt.SetUnit("player", UnitInfo{Exists: true, Name: "Player", Level: 1, RaceID: 1, RaceFile: "Human", RaceName: "Human", ClassID: 1, ClassFile: "WARRIOR", ClassName: "Warrior", Sex: 2, Health: 1, HealthMax: 1, Power: 1, PowerMax: 1, PowerType: 1, PowerToken: "RAGE", Connected: true, Player: true, Visible: true})
		}
	}
	if err := eng.AssetLoader.LoadTOCFiltered(`Interface\FrameXML\FrameXML.toc`, nil, func(path string) bool {
		// The glue screens already created the options frames; the world UI reuses them.
		return (path == `Interface\FrameXML\VideoOptionsFrame.xml` && eng.Rt.widgets["VideoOptionsFrame"] != nil) ||
			(path == `Interface\FrameXML\AudioOptionsFrame.xml` && eng.Rt.widgets["AudioOptionsFrame"] != nil)
	}); err != nil {
		return fmt.Errorf("load world UI: %w", err)
	}
	eng.loadBindings()
	// The frames read the keys when they load, before the table exists; the client
	// announces the bindings once they are in.
	eng.Rt.FireEvent("UPDATE_BINDINGS")
	if err := eng.loadCombatLogAddon(); err != nil {
		return err
	}
	if err := eng.loadStartupAddOns(); err != nil {
		return err
	}
	eng.loadUserAddOns()
	eng.worldRoot = eng.Rt.widgets["UIParent"]
	if eng.worldRoot == nil {
		return fmt.Errorf("world UI did not create UIParent")
	}
	if !eng.Rt.Execute(`
if ChatFrame1 then
    ChatFrame1:Show();
    ChatEdit_SetLastActiveWindow(ChatFrame1.editBox);
    ChatFrame1.editBox:SetJustifyH("LEFT");
    FCF_SetButtonSide(ChatFrame1, "left");
    if ChatFrame2 then
        FCF_SetButtonSide(ChatFrame2, "left", true);
    end
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
	eng.Rt.SendChatColors()
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
if Minimap and not MinimapMap then
    MinimapMap = MinimapBackdrop:CreateTexture("MinimapMap", "BACKGROUND")
    -- The map fills the Minimap frame, whatever size a script gives it.
    MinimapMap:SetAllPoints(Minimap)
end
if Minimap_Update then
    Minimap_Update()
end
if TemporaryEnchantFrame_Hide then
    TemporaryEnchantFrame_Hide()
end
`, "@world-ui-minimap.lua") {
		return fmt.Errorf("initialize minimap: %v", eng.Rt.ScriptErrors())

	}
	eng.Rt.FireEvent("VARIABLES_LOADED")
	eng.Rt.loggedIn = true
	eng.Rt.FireEvent("PLAYER_LOGIN")
	eng.Rt.FireEvent("PLAYER_ENTERING_WORLD")
	eng.Rt.FireEvent("UNIT_DISPLAYPOWER", lua.LString("player"))
	// Drop glue-screen keyboard focus (e.g. AccountLoginAccountEdit) so the
	// first world ESC runs TOGGLEGAMEMENU instead of only clearing focus.
	eng.Rt.setFocus(nil)
	eng.Rt.FireEvent("UNIT_AURA", lua.LString("player"))
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
	if buttons != nil {
		buttons.shown = false
	}
	if combat != nil && combat.shown && eng.Rt.L.GetGlobal("FCF_SetButtonSide").Type() == lua.LTFunction {
		buttonSide := ""
		if combat.fields != nil {
			buttonSide = combat.fields.RawGetString("buttonSide").String()
		}
		if buttonSide != "left" {
			eng.Rt.Execute(`FCF_SetButtonSide(ChatFrame2, "left", true)`, "@world-ui-chat-side.lua")
		}
	}
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
	if (!strings.HasPrefix(key, "blizzard_") && !eng.isUserAddOn(key)) || strings.ContainsAny(key, `\/.`) {
		return false, "ADDON_NOT_FOUND"
	}
	toc := `Interface\AddOns\` + strings.TrimSpace(name) + `\` + strings.TrimSpace(name) + `.toc`
	data, err := eng.AssetLoader.ReadFile(toc)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, "ADDON_NOT_FOUND"
		}
		return false, "ADDON_LOAD_FAILED"
	}
	if eng.Rt.addonTOCs == nil {
		eng.Rt.addonTOCs = make(map[string][]byte)
	}
	eng.Rt.addonTOCs[key] = data
	for _, dependency := range addonDependencies(data) {
		if loaded, reason := eng.loadAddOn(dependency); !loaded {
			return false, "ADDON_DEP_" + strings.ToUpper(reason)
		}
	}
	if err := eng.AssetLoader.LoadTOC(toc, nil); err != nil {
	log.Printf("add-on %s: %v", name, err)
	return false, "ADDON_LOAD_FAILED"
	}
	eng.Rt.loadedAddOns[key] = true
	eng.Rt.FireEvent("ADDON_LOADED", lua.LString(name))
	return true, ""
}

func (eng *UIEngine) isUserAddOn(key string) bool {
	for _, name := range eng.UserAddOns {
		if strings.EqualFold(strings.TrimSpace(name), key) {
			return true
		}
	}
	return false
}

// loadUserAddOns loads the configured user add-ons; one that is missing or
// fails does not stop the others or the interface.
func (eng *UIEngine) loadUserAddOns() {
	// The secure handler functions need the restricted environment, which this
	// client has none of; user add-ons that call them get functions that do nothing.
	if len(eng.UserAddOns) > 0 {
		registerStateDriverAPI(eng.Rt)
		eng.Rt.Execute(`
for _, name in ipairs({"SecureHandlerWrapScript", "SecureHandlerUnwrapScript"}) do
	_G[name] = function() end
end
`, "@secure-handlers.lua")
	}
	for _, name := range eng.UserAddOns {
		if loaded, reason := eng.loadAddOn(name); !loaded {
			log.Printf("add-on %s: %s", name, reason)
		}
	}
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
	if rt.RequestLogout != nil {
		// The server decides: SMSG_LOGOUT_RESPONSE says whether it is allowed and
		// whether it is at once (a city or an inn), and only then does the camp
		// timer and its prompt start.
		if rt.logoutAwaiting || rt.logoutPending || rt.quitPending {
			return
		}
		rt.logoutAwaiting = true
		rt.quitPending = quit
		rt.RequestLogout(quit)
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
	if !rt.logoutPending && !rt.quitPending && !rt.logoutAwaiting {
		return
	}
	if rt.RequestLogoutCancel != nil {
		rt.RequestLogoutCancel()
	}
	rt.logoutAwaiting = false
	rt.logoutPending = false
	rt.quitPending = false
	rt.logoutRemaining = 0
	rt.FireEvent("LOGOUT_CANCEL")
}

// LogoutResponse is the server's answer to a logout request: a reason that is
// not zero refuses it, and an instant one needs no wait.
func (rt *Runtime) LogoutResponse(reason uint32, instant bool) {
	if rt == nil {
		return
	}
	quit := rt.quitPending
	rt.logoutAwaiting = false
	if reason != 0 {
		rt.logoutPending = false
		rt.quitPending = false
		rt.FireEvent("UI_ERROR_MESSAGE", lua.LString(rt.globalString("ERR_CANT_LOGOUT", "You can't logout now.")))
		return
	}
	rt.logoutPending = !quit
	rt.quitPending = quit
	if instant {
		// SMSG_LOGOUT_COMPLETE follows at once; no prompt.
		rt.logoutRemaining = logoutCampSeconds
		return
	}
	rt.logoutRemaining = logoutCampSeconds
	if quit {
		rt.FireEvent("PLAYER_QUITING")
	} else {
		rt.FireEvent("PLAYER_CAMPING")
	}
}

// LogoutComplete is SMSG_LOGOUT_COMPLETE: the character is out of the world.
func (rt *Runtime) LogoutComplete() {
	if rt == nil {
		return
	}
	quit := rt.quitPending
	rt.logoutAwaiting = false
	if quit {
		rt.completeQuit()
	} else {
		rt.completeLogout()
	}
}

// LogoutCancelled is SMSG_LOGOUT_CANCEL_ACK.
func (rt *Runtime) LogoutCancelled() {
	if rt == nil {
		return
	}
	rt.logoutAwaiting = false
	rt.logoutPending = false
	rt.quitPending = false
	rt.logoutRemaining = 0
	rt.hideLogoutPopup("CAMP")
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
	if rt.RequestLogout != nil {
		// The server completes it; a long silence ends it here.
		if rt.logoutRemaining > -10 {
			return false
		}
	} else if rt.logoutRemaining > 0 {
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

// blizzardAddOns are the interface add-ons that ship with the client.
var blizzardAddOns = []string{
	"Blizzard_AchievementUI", "Blizzard_ArenaUI", "Blizzard_AuctionUI", "Blizzard_BarberShopUI", "Blizzard_BattlefieldMinimap",
	"Blizzard_BindingUI", "Blizzard_Calendar", "Blizzard_CombatLog", "Blizzard_CombatText", "Blizzard_FeedbackUI", "Blizzard_GMChatUI",
	"Blizzard_GMSurveyUI", "Blizzard_GlyphUI", "Blizzard_GuildBankUI", "Blizzard_InspectUI", "Blizzard_ItemSocketingUI", "Blizzard_MacroUI",
	"Blizzard_RaidUI", "Blizzard_TalentUI", "Blizzard_TimeManager", "Blizzard_TokenUI", "Blizzard_TradeSkillUI", "Blizzard_TrainerUI",
}

// addonTOCField reads a "## Key: value" line of a TOC.
func addonTOCField(toc []byte, key string) string {
	for _, line := range strings.Split(string(toc), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, bomPrefix))
		if !strings.HasPrefix(line, "##") {
			continue
		}
		separator := strings.IndexByte(line, ':')
		if separator < 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(line[2:separator]), key) {
			return strings.TrimSpace(line[separator+1:])
		}
	}
	return ""
}

func addonDependencies(toc []byte) []string {
	value := addonTOCField(toc, "Dependencies")
	if value == "" {
		value = addonTOCField(toc, "RequiredDeps")
	}
	var names []string
	for _, name := range strings.Split(value, ",") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// loadStartupAddOns loads the interface add-ons that are not load-on-demand,
// as the client does before the world UI is first shown.
func (eng *UIEngine) loadStartupAddOns() error {
	for _, name := range blizzardAddOns {
		toc, err := eng.AssetLoader.ReadFile(`Interface\AddOns\` + name + `\` + name + `.toc`)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return fmt.Errorf("probe add-on %s: %w", name, err)
		}
		if lod := addonTOCField(toc, "LoadOnDemand"); lod == "1" {
			continue
		}
		if loaded, reason := eng.loadAddOn(name); !loaded {
			return fmt.Errorf("load add-on %s: %s", name, reason)
		}
	}
	return nil
}

const bomPrefix = "\xEF\xBB\xBF"
