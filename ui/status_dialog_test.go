package ui

import (
	"image"
	"os"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestLiveGlueStatusDialogContract(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	dialog := engine.Rt.widgets["GlueDialog"]
	if dialog == nil {
		t.Fatal("GlueDialog is missing")
	}
	for _, event := range []string{"OPEN_STATUS_DIALOG", "UPDATE_STATUS_DIALOG", "CLOSE_STATUS_DIALOG"} {
		if !dialog.events[event] {
			t.Fatalf("GlueDialog is not registered for %s", event)
		}
	}
	engine.SetStatusKey("AUTH_FAILED")
	if !dialog.shown {
		t.Fatal("OPEN_STATUS_DIALOG did not show GlueDialog")
	}
	if !engine.Rt.Execute("assert(GlueDialog.which == 'OKAY')", "@status-dialog-test.lua") {
		t.Fatalf("dialog type after failure: %v", engine.Rt.ScriptErrors())
	}
	button := engine.Rt.widgets["GlueDialogButton1"]
	if button == nil {
		t.Fatal("GlueDialogButton1 is missing")
	}
	before := button.normalTexture
	if before == nil || before.textureFile == "" || engine.loadBLP(before.textureFile) == nil {
		t.Fatalf("auth button normal texture unavailable: %+v", before)
	}
	beforeCoords := [4]float64{before.texCoordL, before.texCoordR, before.texCoordT, before.texCoordB}
	engine.Update(1.0 / 60)
	upBlue := "Interface\\Glues\\Common\\Glue-Panel-Button-Up-Blue"
	downBlue := "Interface\\Glues\\Common\\Glue-Panel-Button-Down-Blue"
	hiBlue := "Interface\\Glues\\Common\\Glue-Panel-Button-Highlight-Blue"
	if button.normalTexture == nil || button.pushedTexture == nil || button.highlightTexture == nil || button.normalTexture.textureFile != upBlue || button.pushedTexture.textureFile != downBlue || button.highlightTexture.textureFile != hiBlue {
		t.Fatalf("auth button artwork normal=%v pushed=%v highlight=%v", button.normalTexture, button.pushedTexture, button.highlightTexture)
	}
	if button.normalTexture != before {
		t.Fatal("GlueDialog_OnUpdate replaced NormalTexture instead of updating the existing region")
	}
	if button.normalTexture.texCoordL != beforeCoords[0] || button.normalTexture.texCoordR != beforeCoords[1] || button.normalTexture.texCoordT != beforeCoords[2] || button.normalTexture.texCoordB != beforeCoords[3] {
		t.Fatalf("OnUpdate dropped TexCoords: got (%v,%v,%v,%v) want %v", button.normalTexture.texCoordL, button.normalTexture.texCoordR, button.normalTexture.texCoordT, button.normalTexture.texCoordB, beforeCoords)
	}
	if len(button.normalTexture.points) < 2 {
		t.Fatalf("button normal texture lost fill anchors: %+v", button.normalTexture.points)
	}
	bg := engine.Rt.widgets["GlueDialogBackground"]
	text := engine.Rt.widgets["GlueDialogText"]
	if bg == nil || text == nil {
		t.Fatal("GlueDialogBackground/Text missing")
	}
	if text.height <= 0 {
		t.Fatalf("status text height not measured after SetText: %v", text.height)
	}
	if bg.height < text.height+button.height {
		t.Fatalf("status dialog background too short: bg=%v text=%v button=%v", bg.height, text.height, button.height)
	}

	engine.SetStatusKey("GAME_SERVER_LOGIN")
	if text = engine.Rt.widgets["GlueDialogText"]; text == nil || text.text != engine.resolveText("GAME_SERVER_LOGIN") {
		t.Fatalf("dialog text=%v", text)
	}
	if !engine.Rt.Execute("assert(GlueDialog.which == 'CANCEL')", "@status-dialog-test.lua") {
		t.Fatalf("dialog type during login: %v", engine.Rt.ScriptErrors())
	}
	engine.Update(1.0 / 60)
	if button.normalTexture != before || len(button.normalTexture.points) < 2 || button.normalTexture.textureFile != upBlue {
		t.Fatalf("connecting button texture unstable after OnUpdate: %+v", button.normalTexture)
	}
	img := engine.Render(960, 640)
	buttonRect := engine.layoutRect(button, engine.layoutRect(bg, engine.glueParentRect()))
	if buttonRect.W() < 100 || buttonRect.H() < 20 {
		t.Fatalf("cancel button layout collapsed: %+v", buttonRect)
	}
	texRect := engine.layoutRect(button.normalTexture, buttonRect)
	if texRect.W() < buttonRect.W()*0.9 || texRect.H() < buttonRect.H()*0.9 {
		t.Fatalf("cancel button texture rect collapsed: tex=%+v button=%+v", texRect, buttonRect)
	}
	if !buttonBackgroundOpaque(img, buttonRect, engine.uiScale) {
		t.Fatalf("connecting popup button background not visible in render (button=%+v)", buttonRect)
	}
	engine.SetStatusText("raw error")
	if text = engine.Rt.widgets["GlueDialogText"]; text == nil || text.text != "raw error" {
		t.Fatalf("raw dialog text=%v", text)
	}
	engine.SetStatusText("")
	if dialog.shown {
		t.Fatal("CLOSE_STATUS_DIALOG did not hide GlueDialog")
	}
}

func TestLiveCharacterDeleteDialogSizing(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.SetGlueState(GlueState{
		Connected:         true,
		SelectedCharacter: 1,
		Characters: []CharacterEntry{{
			Name:  "Testchar",
			Race:  "RACE_HUMAN",
			Class: "WARRIOR",
			Level: 80,
		}},
	})
	if !engine.Rt.Execute("SetGlueScreen('charselect'); CharacterSelect.selectedIndex = 1; CharacterDeleteDialog_OnShow()", "@delete-dialog-test.lua") {
		t.Fatalf("CharacterDeleteDialog_OnShow failed: %v", engine.Rt.ScriptErrors())
	}
	bg := engine.Rt.widgets["CharacterDeleteBackground"]
	text1 := engine.Rt.widgets["CharacterDeleteText1"]
	text2 := engine.Rt.widgets["CharacterDeleteText2"]
	edit := engine.Rt.widgets["CharacterDeleteEditBox"]
	button := engine.Rt.widgets["CharacterDeleteButton1"]
	if bg == nil || text1 == nil || text2 == nil || edit == nil || button == nil {
		t.Fatal("CharacterDeleteDialog pieces missing")
	}
	if text1.height <= 0 || text2.height <= 0 {
		t.Fatalf("delete dialog text heights unmeasured: text1=%v text2=%v", text1.height, text2.height)
	}
	want := 16 + text1.height + text2.height + 23 + edit.height + 8 + button.height + 16
	if bg.height < want-1 || bg.height > want+1 {
		t.Fatalf("CharacterDeleteBackground height=%v want ~%v", bg.height, want)
	}
	engine.Update(1.0 / 60)
	if button.normalTexture == nil || len(button.normalTexture.points) < 2 || engine.loadBLP(button.normalTexture.textureFile) == nil {
		t.Fatalf("delete button normal texture invalid: %+v", button.normalTexture)
	}
}

func TestButtonTextureArgPreservesRegion(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	button := newWidget(kindButton, "ProbeButton")
	existing := newWidget(kindTexture, "")
	existing.parent = button
	existing.textureFile = "Interface\\Glues\\Common\\Glue-Panel-Button-Up"
	existing.texCoordL, existing.texCoordR = 0, 0.578125
	existing.texCoordT, existing.texCoordB = 0, 0.75
	existing.points = []anchorPoint{
		{point: "TOPLEFT", relativePoint: "TOPLEFT"},
		{point: "BOTTOMRIGHT", relativePoint: "BOTTOMRIGHT"},
	}
	button.normalTexture = existing
	rt.L.Push(button.luaValue(rt.L))
	rt.L.Push(lua.LString("Interface\\Glues\\Common\\Glue-Panel-Button-Up-Blue"))
	got := rt.buttonTextureArg(button, existing, rt.L, 2)
	if got != existing {
		t.Fatal("buttonTextureArg allocated a replacement texture")
	}
	if got.textureFile != "Interface\\Glues\\Common\\Glue-Panel-Button-Up-Blue" {
		t.Fatalf("texture file=%q", got.textureFile)
	}
	if got.texCoordR != 0.578125 || len(got.points) != 2 {
		t.Fatalf("lost region state: coords=%v points=%v", got.texCoordR, got.points)
	}
}

func buttonBackgroundOpaque(img *image.RGBA, button Rect, uiScale float64) bool {
	if img == nil || uiScale <= 0 {
		return false
	}
	screenH := float64(img.Bounds().Dy())
	x := int(((button.X0 + button.X1) / 2) * uiScale)
	yUI := (button.Y0 + button.Y1) / 2
	y := int(screenH - yUI*uiScale)
	if x < img.Bounds().Min.X || x >= img.Bounds().Max.X || y < img.Bounds().Min.Y || y >= img.Bounds().Max.Y {
		return false
	}
	_, _, _, a := img.At(x, y).RGBA()
	return a > 0x8000
}
