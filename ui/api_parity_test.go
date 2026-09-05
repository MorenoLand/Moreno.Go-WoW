package ui

import "testing"

func TestGlueStateAndCreateFrameArguments(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	rt.Glue.Characters = []CharacterEntry{{Name: "one"}, {Name: "two"}}
	if !rt.Execute("SelectCharacter(99); assert(GetSelectBackgroundModel(1) == 'CharacterSelect'); local f = CreateFrame('Frame', 'TestFrame', nil, nil, 17); assert(f:GetID() == 17); SetCurrentScreen('charselect')", "@api-parity.lua") {
		t.Fatalf("script failed: %v", rt.ScriptErrors())
	}
	if rt.Glue.SelectedCharacter != 2 {
		t.Fatalf("selected character=%d", rt.Glue.SelectedCharacter)
	}
	if screen, ok := rt.GetCVar("currentGlueScreen"); !ok || screen != "charselect" {
		t.Fatalf("current screen=%q present=%v", screen, ok)
	}
}

func TestCreateFramePreservesCaseInsensitiveObjectTypes(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	if !rt.Execute(`
button = CreateFrame("BUTTON", "UpperButton")
slider = CreateFrame("SLIDER", "UpperSlider")
scroll = CreateFrame("SCROLLFRAME", "UpperScroll")
assert(button:GetObjectType() == "Button")
assert(slider:GetObjectType() == "Slider")
assert(scroll:GetObjectType() == "ScrollFrame")
`, "@create-frame-type-test.lua") {
		t.Fatalf("CreateFrame type mapping failed: %v", rt.ScriptErrors())
	}
}

func TestGlueXMLHandlerParameters(t *testing.T) {
	for _, test := range []struct{ handler, want string }{{"OnSizeChanged", "self, width, height"}, {"OnAttributeChanged", "self, name, value"}, {"OnEnable", "self"}, {"OnDisable", "self"}} {
		if got := scriptParams(test.handler); got != test.want {
			t.Fatalf("%s parameters=%q want=%q", test.handler, got, test.want)
		}
	}
}
