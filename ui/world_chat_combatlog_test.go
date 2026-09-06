package ui

import (
	"os"
	"testing"

	"github.com/g3n/engine/window"
	lua "github.com/yuin/gopher-lua"
)

func TestChatFrame2DockedHiddenAndSetAllPointsToPrimary(t *testing.T) {
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

	cf1 := engine.Rt.widgets["ChatFrame1"]
	cf2 := engine.Rt.widgets["ChatFrame2"]
	if cf1 == nil || cf2 == nil {
		t.Fatal("ChatFrame1/2 missing")
	}
	if !cf1.shown {
		t.Fatal("ChatFrame1 should be shown (selected dock)")
	}
	if cf2.shown {
		t.Fatal("ChatFrame2 Combat Log must stay hidden while General is selected")
	}
	tab2 := engine.Rt.widgets["ChatFrame2Tab"]
	if tab2 == nil || !tab2.shown {
		t.Fatalf("Combat Log tab should be shown: %#v", tab2)
	}
	if tab2.text == "" {
		t.Fatal("Combat Log tab text empty")
	}
	for _, name := range []string{"CombatLogUpButton", "CombatLogDownButton", "CombatLogBottomButton", "CombatLogQuickButtonFrame_Custom"} {
		if engine.Rt.widgets[name] == nil {
			t.Fatalf("Combat Log widget %s missing", name)
		}
	}

	// SetAllPoints(primary) must retain relativeTo, not fill UIParent.
	if !engine.Rt.Execute(`
ChatFrame2:SetAllPoints(ChatFrame1)
local p1, rel, p2 = ChatFrame2:GetPoint(1)
_G.DiagRel = rel and rel:GetName() or ""
`, "@setallpoints-diag.lua") {
		t.Fatalf("SetAllPoints script failed: %v", engine.Rt.ScriptErrors())
	}
	rel := engine.Rt.L.GetGlobal("DiagRel")
	if rel.Type() != lua.LTString || rel.String() != "ChatFrame1" {
		t.Fatalf("SetAllPoints relativeTo=%v want ChatFrame1", rel)
	}
	if len(cf2.points) != 2 || cf2.points[0].relativeTo != "ChatFrame1" || cf2.points[1].relativeTo != "ChatFrame1" {
		t.Fatalf("ChatFrame2 points=%v want relativeTo ChatFrame1", cf2.points)
	}

	engine.RenderWorld(1920, 1080)
	if cf2.renderRect == (Rect{0, 0, engine.screen.X1, engine.screen.Y1}) {
		t.Fatalf("ChatFrame2 still filling screen: %v", cf2.renderRect)
	}
	// After SetAllPoints(ChatFrame1) + layout, rects should match primary.
	if cf2.renderRect != cf1.renderRect {
		// Allow if still hidden and not laid out identically; force show for geometry check.
		cf2.shown = true
		engine.RenderWorld(1920, 1080)
		if cf2.renderRect != cf1.renderRect {
			t.Fatalf("ChatFrame2 render=%v want ChatFrame1=%v", cf2.renderRect, cf1.renderRect)
		}
	}
}

func TestEnterActivatesWorldChatWithoutPriorFocus(t *testing.T) {
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
	edit := engine.Rt.widgets["ChatFrame1EditBox"]
	if edit == nil {
		t.Fatal("ChatFrame1EditBox missing")
	}
	if engine.Rt.focused != nil {
		t.Fatalf("world chat started focused on %s", engine.Rt.focused.name)
	}
	if !engine.HandleKey(window.KeyEnter) {
		t.Fatal("Enter did not activate world chat")
	}
	if engine.Rt.focused != edit || !edit.shown {
		t.Fatalf("chat activation focus=%v shown=%v", engine.Rt.focused, edit.shown)
	}
}

func TestGetChatWindowMessagesDefaultsByID(t *testing.T) {
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
local n1 = select("#", GetChatWindowMessages(1))
local n2 = select("#", GetChatWindowMessages(2))
_G.DiagN1, _G.DiagN2 = n1, n2
assert(IsCombatLog(ChatFrame2))
assert(IsAddOnLoaded("Blizzard_CombatLog"))
`, "@chat-msg-defaults.lua") {
		t.Fatalf("script failed: %v", engine.Rt.ScriptErrors())
	}
	n1 := float64(engine.Rt.L.GetGlobal("DiagN1").(lua.LNumber))
	n2 := float64(engine.Rt.L.GetGlobal("DiagN2").(lua.LNumber))
	if n1 < 10 {
		t.Fatalf("GetChatWindowMessages(1) count=%v want general defaults", n1)
	}
	if n2 != 0 {
		t.Fatalf("GetChatWindowMessages(2) count=%v want 0 (Combat Log)", n2)
	}
}

func TestChatFontNormalFaceNearNativePixelHeight(t *testing.T) {
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
	engine.RenderWorld(1920, 1080)
	chat := engine.Rt.widgets["ChatFrame1"]
	var fontAttr *widget
	for _, c := range chat.children {
		if c.kind == kindFontString {
			fontAttr = c
			break
		}
	}
	if fontAttr == nil {
		t.Fatal("font attr missing")
	}
	face := engine.faceFor(fontAttr, nil, nil)
	if face == nil {
		t.Fatal("face nil")
	}
	got := face.Metrics().Height.Ceil()
	want := engine.Rt.fonts["ChatFontNormal"].Height * engine.uiScale
	// Allow small metric padding above em size; fail on old DPI-96 inflation (~33%+).
	if float64(got) > want*1.25+2 {
		t.Fatalf("chat face height=%d want around %.1f (uiScale=%g)", got, want, engine.uiScale)
	}
}
