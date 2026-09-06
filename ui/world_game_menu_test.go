package ui

import (
	"os"
	"testing"

	"github.com/g3n/engine/window"
	lua "github.com/yuin/gopher-lua"
)

type logoutProbeHost struct {
	hostScreen
	logoutCount int
	quitCount   int
}

func (h *logoutProbeHost) Logout()   { h.logoutCount++ }
func (h *logoutProbeHost) Quit(bool) { h.quitCount++ }

func TestUIParentXMLAttributesSeedPanelOffsets(t *testing.T) {
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
	if !engine.Rt.Execute(`
local v = UIParent:GetAttribute("DEFAULT_FRAME_WIDTH")
assert(type(v) == "number", "DEFAULT_FRAME_WIDTH type=" .. type(v))
assert(v == 384, "DEFAULT_FRAME_WIDTH=" .. tostring(v))
assert(UIParent:GetAttribute("LEFT_OFFSET") == 0)
assert(UIParent:GetAttribute("CENTER_OFFSET") == 384)
assert(UIParent:GetAttribute("RIGHT_OFFSET") == 768)
`, "@attr-check.lua") {
		t.Fatalf("attribute check errors=%v", engine.Rt.ScriptErrors())
	}
}

func TestLiveGameMenuOpenSelectCancelClose(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	host := &logoutProbeHost{hostScreen: hostScreen{w: 960, h: 640}}
	engine.Rt.Host = host
	if err := engine.LoadWorldUI(); err != nil {
		t.Fatal(err)
	}
	engine.RenderWorld(960, 640)
	baseErrors := len(engine.Rt.ScriptErrors())

	if engine.GameMenuShown() {
		t.Fatal("GameMenuFrame should start hidden")
	}
	if !engine.HandleKey(window.KeyEscape) {
		t.Fatal("ESC should run ToggleGameMenu")
	}
	if !engine.GameMenuShown() {
		t.Fatalf("GameMenuFrame not shown after ESC; errors=%v", engine.Rt.ScriptErrors()[baseErrors:])
	}
	if newErrors := engine.Rt.ScriptErrors()[baseErrors:]; len(newErrors) != 0 {
		t.Fatalf("open script errors=%v", newErrors)
	}
	if engine.Rt.widgets["GameMenuButtonContinue"] == nil {
		t.Fatal("GameMenuButtonContinue missing")
	}
	engine.Rt.Execute(`GameMenuButtonContinue:Click()`, "@game-menu-continue.lua")
	if engine.GameMenuShown() {
		t.Fatal("Continue should hide GameMenuFrame")
	}

	if !engine.ToggleGameMenu() || !engine.GameMenuShown() {
		t.Fatal("ToggleGameMenu should reopen")
	}
	if !engine.HandleKey(window.KeyEscape) || engine.GameMenuShown() {
		t.Fatal("ESC should close open GameMenuFrame")
	}

	if !engine.ToggleGameMenu() || !engine.GameMenuShown() {
		t.Fatal("reopen before logout")
	}
	logoutErrors := len(engine.Rt.ScriptErrors())
	if !engine.Rt.Execute(`GameMenuButtonLogout:Click(); _G.CampVisible = StaticPopup_Visible("CAMP")`, "@game-menu-logout.lua") {
		t.Fatalf("logout click failed: %v", engine.Rt.ScriptErrors()[logoutErrors:])
	}
	if newErrors := engine.Rt.ScriptErrors()[logoutErrors:]; len(newErrors) != 0 {
		t.Fatalf("logout script errors=%v", newErrors)
	}
	if engine.GameMenuShown() {
		t.Fatal("Logout click should hide GameMenuFrame")
	}
	if !engine.Rt.logoutPending || engine.Rt.quitPending {
		t.Fatalf("logoutPending=%v quitPending=%v", engine.Rt.logoutPending, engine.Rt.quitPending)
	}
	if engine.Rt.L.GetGlobal("CampVisible") == lua.LNil {
		t.Fatal("Logout click should show CAMP popup")
	}
	engine.Rt.Execute(`CancelLogout()`, "@game-menu-cancel-logout.lua")
	if engine.Rt.logoutPending || engine.Rt.quitPending {
		t.Fatal("CancelLogout should clear pending logout")
	}

	if !engine.ToggleGameMenu() {
		t.Fatal("reopen before quit")
	}
	quitErrors := len(engine.Rt.ScriptErrors())
	if !engine.Rt.Execute(`GameMenuButtonQuit:Click(); _G.QuitVisible = StaticPopup_Visible("QUIT")`, "@game-menu-quit.lua") {
		t.Fatalf("quit click failed: %v", engine.Rt.ScriptErrors()[quitErrors:])
	}
	if newErrors := engine.Rt.ScriptErrors()[quitErrors:]; len(newErrors) != 0 {
		t.Fatalf("quit script errors=%v", newErrors)
	}
	if !engine.Rt.quitPending || engine.Rt.logoutPending {
		t.Fatalf("quitPending=%v logoutPending=%v", engine.Rt.quitPending, engine.Rt.logoutPending)
	}
	if engine.Rt.L.GetGlobal("QuitVisible") == lua.LNil {
		t.Fatal("Quit click should show QUIT popup")
	}
	engine.Update(logoutCampSeconds + 1)
	if host.quitCount != 1 {
		t.Fatalf("timer quit count=%d", host.quitCount)
	}

	engine.Rt.beginLogout(false)
	engine.Update(logoutCampSeconds + 1)
	if host.logoutCount != 1 {
		t.Fatalf("logout host count=%d", host.logoutCount)
	}
}

func TestLiveGameMenuActionButtons(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	for _, action := range []struct{ button, panel string }{
		{"GameMenuButtonOptions", "VideoOptionsFrame"},
		{"GameMenuButtonSoundOptions", "AudioOptionsFrame"},
		{"GameMenuButtonUIOptions", "InterfaceOptionsFrame"},
		{"GameMenuButtonKeybindings", "KeyBindingFrame"},
		{"GameMenuButtonMacros", "MacroFrame"},
	} {
		t.Run(action.button, func(t *testing.T) {
			engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()
			engine.Rt.Host = &logoutProbeHost{hostScreen: hostScreen{w: 960, h: 640}}
			if err := engine.LoadWorldUI(); err != nil {
				t.Fatal(err)
			}
			engine.RenderWorld(960, 640)
			before := len(engine.Rt.ScriptErrors())
			if !engine.ToggleGameMenu() || !engine.GameMenuShown() {
				t.Fatalf("menu did not open for %s", action.button)
			}
			if !engine.Rt.Execute(action.button+":Click()", "@game-menu-action.lua") {
				t.Fatalf("%s click failed: %v", action.button, engine.Rt.ScriptErrors()[before:])
			}
			if newErrors := engine.Rt.ScriptErrors()[before:]; len(newErrors) != 0 {
				t.Fatalf("%s script errors=%v", action.button, newErrors)
			}
			if frame := engine.Rt.widgets[action.panel]; frame == nil || !frame.shown {
				t.Fatalf("%s did not show %s", action.button, action.panel)
			}
		})
	}
}
