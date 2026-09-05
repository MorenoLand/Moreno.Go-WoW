package ui

import "testing"

func TestGlueAPIMatchesOptionalAmbienceAndAddonSecurity(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	if !rt.Execute("PlayGlueAmbience()", "@api-test.lua") {
		t.Fatalf("optional ambience failed: %v", rt.ScriptErrors())
	}
	rt.Glue.AddOns = []AddonInfo{{Name: "Example"}}
	if !rt.Execute("local _, _, _, _, _, _, security = GetAddOnInfo(1); addonSecurity = security", "@api-test.lua") {
		t.Fatalf("addon info failed: %v", rt.ScriptErrors())
	}
	if got := rt.L.GetGlobal("addonSecurity").String(); got != "INSECURE" {
		t.Fatalf("security=%q", got)
	}
}

func TestCreateLocalizedValueUsesNativeNamesWhenGlobalsAreAbsent(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	for key, want := range map[string]string{"RACE_HUMAN": "Human", "RACE_NIGHTELF": "Night Elf", "RACE_SCOURGE": "Undead", "DEATHKNIGHT": "Death Knight"} {
		if got := createLocalizedValue(rt.L, key); got != want {
			t.Fatalf("%s=%q, want %q", key, got, want)
		}
	}
}

func TestScrollingMessageFrameUsesNativeMetadataAndOffset(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	chat := newWidget(kindScrollingMessageFrame, "Chat")
	rt.register(chat)
	if !rt.Execute(`
Chat:AddMessage("colored", 1, 0.5, 0.25, 7, 8, 11, "extra")
Chat:AddMessage("plain", 12, 0, 22, 0)
assert(Chat:GetNumMessages() == 2)
assert(Chat:GetNumMessages(11) == 1)
local text, accessID, typeID, extraData = Chat:GetMessageInfo(1)
assert(text == "colored" and accessID == 11 and typeID == 7 and extraData == "extra")
Chat:ScrollUp()
assert(Chat:GetCurrentScroll() == 1 and not Chat:AtBottom())
Chat:ScrollDown()
assert(Chat:GetCurrentScroll() == 0 and Chat:AtBottom())
Chat:ScrollToTop()
assert(Chat:GetCurrentScroll() == 1)
Chat:ScrollToBottom()
assert(Chat:GetCurrentScroll() == 0)
Chat:RemoveMessagesByAccessID(11)
assert(Chat:GetNumMessages() == 1)
`, "@scroll-api-test.lua") {
		t.Fatalf("scroll API failed: %v", rt.ScriptErrors())
	}
}

func TestWidgetDynamicRegionConstructors(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	root := newWidget(kindFrame, "Root")
	rt.register(root)
	if !rt.Execute(`
local texture = Root:CreateTexture("Icon", "OVERLAY")
local fontString = Root:CreateFontString("Label", "ARTWORK")
assert(texture:GetName() == "Icon" and texture:GetParent() == Root and texture:GetObjectType() == "Texture")
assert(fontString:GetName() == "Label" and fontString:GetParent() == Root and fontString:GetObjectType() == "FontString")
`, "@dynamic-region-test.lua") {
		t.Fatalf("dynamic region constructors failed: %v", rt.ScriptErrors())
	}
}

func TestAuthoredInsetsAndWidgetScaleArePreserved(t *testing.T) {
	edit := newWidget(kindEditBox, "Name")
	edit.width, edit.height = 200, 37
	edit.textInsetL = 15
	edit.textInsetsSet = true
	textRect := (&UIEngine{}).editTextRect(Rect{X0: 10, Y0: 20, X1: 210, Y1: 57}, edit)
	if textRect != (Rect{X0: 25, Y0: 20, X1: 210, Y1: 57}) {
		t.Fatalf("explicit text insets=%v", textRect)
	}
	edit.scale = 2
	if got := ResolveRect(edit, Rect{X0: 0, Y0: 0, X1: 300, Y1: 100}); got != (Rect{X0: -50, Y0: 13, X1: 350, Y1: 87}) {
		t.Fatalf("scaled edit rect=%v", got)
	}
}

func TestHitRectInsetsAreScriptConfigurable(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	button := newWidget(kindButton, "HitButton")
	rt.register(button)
	if !rt.Execute(`HitButton:SetHitRectInsets(1, -100, 3, 4)`, "@hit-insets-test.lua") {
		t.Fatalf("SetHitRectInsets failed: %v", rt.ScriptErrors())
	}
	if button.hitInsetL != 1 || button.hitInsetR != -100 || button.hitInsetT != 3 || button.hitInsetB != 4 {
		t.Fatalf("hit insets=%v,%v,%v,%v", button.hitInsetL, button.hitInsetR, button.hitInsetT, button.hitInsetB)
	}
}

func TestTextInsetsRoundTripThroughLua(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	edit := newWidget(kindEditBox, "InsetEdit")
	rt.register(edit)
	if !rt.Execute(`InsetEdit:SetTextInsets(15, 2, 3, 4); left, right, top, bottom = InsetEdit:GetTextInsets(); assert(left == 15 and right == 2 and top == 3 and bottom == 4)`, "@text-insets-test.lua") {
		t.Fatalf("text inset round trip failed: %v", rt.ScriptErrors())
	}
}
