package ui

import (
	"os"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestLiveWorldChatTabAppearanceMatchesFrameXML(t *testing.T) {
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
	if errors := engine.Rt.ScriptErrors(); len(errors) != 0 {
		t.Fatalf("chat tab init script errors=%v", errors)
	}

	chat := engine.Rt.widgets["ChatFrame1"]
	if chat == nil {
		t.Fatal("ChatFrame1 missing")
	}
	name := chat.fields.RawGetString("name")
	if name.Type() != lua.LTString || name.String() == "" {
		t.Fatalf("ChatFrame1.name=%v (FCF_SetWindowName should run via UPDATE_CHAT_WINDOWS)", name)
	}

	tab := engine.Rt.widgets["ChatFrame1Tab"]
	if tab == nil {
		t.Fatal("ChatFrame1Tab missing")
	}
	if !tab.shown {
		t.Fatal("ChatFrame1Tab should be shown")
	}
	if tab.text == "" {
		t.Fatal("ChatFrame1Tab text empty after FCF_SetWindowName")
	}
	if tab.width < 48 || tab.height != 32 {
		t.Fatalf("ChatFrame1Tab size=%gx%g want width>=48 height=32", tab.width, tab.height)
	}

	tabText := engine.Rt.widgets["ChatFrame1TabText"]
	if tabText == nil {
		t.Fatal("ChatFrame1TabText missing")
	}
	if tabText.textWidth <= 0 && tabText.width <= 0 {
		t.Fatalf("ChatFrame1TabText zero width text=%q width=%g textWidth=%g auto=%v", tabText.text, tabText.width, tabText.textWidth, tabText.autoTextWidth)
	}

	middle := engine.Rt.widgets["ChatFrame1TabMiddle"]
	if middle == nil || middle.width <= 0 || middle.height != 32 {
		t.Fatalf("ChatFrame1TabMiddle=%v", middle)
	}
	left := engine.Rt.widgets["ChatFrame1TabLeft"]
	right := engine.Rt.widgets["ChatFrame1TabRight"]
	if left == nil || right == nil || left.width != 16 || right.width != 16 {
		t.Fatalf("tab sides left=%v right=%v", left, right)
	}
	expected := left.width + middle.width + right.width
	if tab.width < expected-0.5 || tab.width > expected+0.5 {
		t.Fatalf("ChatFrame1Tab width=%g != left+middle+right=%g", tab.width, expected)
	}

	for _, path := range []string{
		`Interface\ChatFrame\ChatFrameTab-BGLeft`,
		`Interface\ChatFrame\ChatFrameTab-BGMid`,
		`Interface\ChatFrame\ChatFrameTab-BGRight`,
		`Interface\ChatFrame\ChatFrameTab-SelectedLeft`,
		`Interface\ChatFrame\ChatFrameTab-SelectedMid`,
		`Interface\ChatFrame\ChatFrameTab-SelectedRight`,
		`Interface\ChatFrame\ChatFrameTab-HighlightLeft`,
		`Interface\ChatFrame\ChatFrameTab-HighlightMid`,
		`Interface\ChatFrame\ChatFrameTab-HighlightRight`,
	} {
		if engine.loadBLP(path) == nil {
			t.Fatalf("missing chat tab asset %s", path)
		}
	}

	selectedMid := engine.Rt.widgets["ChatFrame1TabSelectedMiddle"]
	if selectedMid == nil || !selectedMid.shown {
		t.Fatalf("selected middle should show for selected dock tab: %#v", selectedMid)
	}
	highlightMid := engine.Rt.widgets["ChatFrame1TabHighlightMiddle"]
	if highlightMid == nil || highlightMid.layerLevel != layerHighlight {
		t.Fatalf("highlight middle layer=%v", highlightMid)
	}

	combatTab := engine.Rt.widgets["ChatFrame2Tab"]
	if combatTab == nil || !combatTab.shown {
		t.Fatalf("ChatFrame2Tab should be shown docked: %#v", combatTab)
	}
	if combatTab.text == "" || combatTab.width < 48 {
		t.Fatalf("ChatFrame2Tab text=%q size=%gx%g", combatTab.text, combatTab.width, combatTab.height)
	}

	hidden := engine.Rt.widgets["ChatFrame3Tab"]
	if hidden == nil || hidden.shown {
		t.Fatalf("ChatFrame3Tab should exist hidden: %#v", hidden)
	}
	if hidden.width < 48 || hidden.height != 32 {
		t.Fatalf("ChatFrame3Tab size=%gx%g", hidden.width, hidden.height)
	}

	engine.RenderWorld(960, 640)
	if tab.renderRect.H() < 30 || tab.renderRect.W() < 48 {
		t.Fatalf("ChatFrame1Tab renderRect=%v", tab.renderRect)
	}
	if middle.renderRect.W() <= 0 {
		t.Fatalf("ChatFrame1TabMiddle renderRect=%v", middle.renderRect)
	}
	if !strings.EqualFold(left.textureFile, `Interface\ChatFrame\ChatFrameTab-BGLeft`) {
		t.Fatalf("left texture=%q", left.textureFile)
	}
}

func TestFontStringSetWidthZeroUsesNaturalWidth(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	ok := engine.Rt.Execute(`
local f = CreateFrame("Frame", "TabMeasureProbe", UIParent)
local fs = f:CreateFontString("TabMeasureProbeText", "ARTWORK", "GameFontNormalSmall")
fs:SetText("General")
fs:SetWidth(50)
_G.TabMeasureProbeFixed = fs:GetWidth()
fs:SetWidth(0)
_G.TabMeasureProbeNatural = fs:GetWidth()
`, "@tab-measure.lua")
	if !ok {
		t.Fatalf("script errors=%v", engine.Rt.ScriptErrors())
	}
	fixed, _ := engine.Rt.L.GetGlobal("TabMeasureProbeFixed").(lua.LNumber)
	natural, _ := engine.Rt.L.GetGlobal("TabMeasureProbeNatural").(lua.LNumber)
	if float64(fixed) != 50 {
		t.Fatalf("fixed GetWidth=%v want 50", fixed)
	}
	if float64(natural) <= 0 || float64(natural) == 50 {
		t.Fatalf("natural GetWidth after SetWidth(0)=%v want measured string width", natural)
	}
}
