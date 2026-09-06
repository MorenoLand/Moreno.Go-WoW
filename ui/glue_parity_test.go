package ui

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g3n/engine/window"
	lua "github.com/yuin/gopher-lua"
)

func TestCharacterCreateRaceOrderMatchesNative(t *testing.T) {
	expected := []createRaceInfo{{1, "RACE_HUMAN", "Human", "Human", "Alliance"}, {3, "RACE_DWARF", "Dwarf", "Dwarf", "Alliance"}, {4, "RACE_NIGHTELF", "NightElf", "NightElf", "Alliance"}, {7, "RACE_GNOME", "Gnome", "Dwarf", "Alliance"}, {11, "RACE_DRAENEI", "Draenei", "Draenei", "Alliance"}, {2, "RACE_ORC", "Orc", "Orc", "Horde"}, {5, "RACE_SCOURGE", "Scourge", "Scourge", "Horde"}, {6, "RACE_TAUREN", "Tauren", "Tauren", "Horde"}, {8, "RACE_TROLL", "Troll", "Orc", "Horde"}, {10, "RACE_BLOODELF", "BloodElf", "BloodElf", "Horde"}}
	if len(createRaces) != len(expected) {
		t.Fatalf("race count=%d want=%d", len(createRaces), len(expected))
	}
	for index, want := range expected {
		if createRaces[index] != want {
			t.Fatalf("race index=%d got=%+v want=%+v", index+1, createRaces[index], want)
		}
	}
}

func TestLiveGlueInputGeometryMatchesMPQ(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.SetInitialCredentials("", "", false)
	if background := os.Getenv("WOW_TEST_LOGIN_BACKGROUND"); background != "" {
		engine.BgImagePath = background
	}
	account := engine.Rt.widgets["AccountLoginAccountEdit"]
	password := engine.Rt.widgets["AccountLoginPasswordEdit"]
	if account == nil || password == nil {
		t.Fatal("login edit boxes missing")
	}
	for _, edit := range []*widget{account, password} {
		if edit.width != 200 || edit.height != 37 {
			t.Fatalf("edit box %s size=%gx%g", edit.name, edit.width, edit.height)
		}
		if edit.backdrop == nil || edit.backdrop.bgFile != `Interface\Tooltips\UI-Tooltip-Background` || edit.backdrop.edgeFile != `Interface\Glues\Common\Glue-Tooltip-Border` || !edit.backdrop.tile {
			t.Fatalf("edit box %s backdrop=%+v", edit.name, edit.backdrop)
		}
	}
	outputDir := os.Getenv("WOW_TEST_LOGIN_RENDER_DIR")
	for _, size := range [][2]int{{1024, 768}, {1280, 720}, {1920, 1080}, {2560, 1392}} {
		frame := engine.Render(size[0], size[1])
		if math.Abs((account.renderRect.Y0-password.renderRect.Y0)-40) > 0.01 {
			t.Fatalf("authored vertical spacing at %dx%d account=%v password=%v", size[0], size[1], account.renderRect, password.renderRect)
		}
		t.Logf("input %dx%d account=%v screen=%v password=%v screen=%v", size[0], size[1], account.renderRect, ScreenRect(account.renderRect, float64(size[1])), password.renderRect, ScreenRect(password.renderRect, float64(size[1])))
		if outputDir != "" {
			file, createErr := os.Create(filepath.Join(outputDir, fmt.Sprintf("login-%dx%d.png", size[0], size[1])))
			if createErr != nil {
				t.Fatal(createErr)
			}
			if encodeErr := png.Encode(file, frame); encodeErr != nil {
				file.Close()
				t.Fatal(encodeErr)
			}
			file.Close()
		}
	}
}

func TestLiveLoginVideoOptionsPanelsDoNotOverlap(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.SetInitialCredentials("", "", false)
	before := len(engine.Rt.ScriptErrors())
	if !engine.Rt.Execute("OptionsButton:Click(); OptionsSelectFrameBackgroundContainerVideoOptionsButton:Click()", "@login-video-options-test.lua") {
		t.Fatalf("open video options failed: %v", engine.Rt.ScriptErrors()[before:])
	}
	video := engine.Rt.widgets["VideoOptionsFrame"]
	resolution := engine.Rt.widgets["VideoOptionsResolutionPanel"]
	effects := engine.Rt.widgets["VideoOptionsEffectsPanel"]
	if video == nil || !video.shown || resolution == nil || !resolution.shown || effects == nil || effects.shown {
		t.Fatalf("initial video panels video=%v resolution=%v effects=%v", video != nil && video.shown, resolution != nil && resolution.shown, effects != nil && effects.shown)
	}
	if !engine.Rt.Execute("VideoOptionsFrameCategoryFrameButton2:Click()", "@login-video-effects-test.lua") {
		t.Fatalf("select effects failed: %v", engine.Rt.ScriptErrors()[before:])
	}
	if resolution.shown || !effects.shown {
		t.Fatalf("effects selection overlap resolution=%v effects=%v", resolution.shown, effects.shown)
	}
	if !engine.Rt.Execute("VideoOptionsFrameCategoryFrameButton1:Click()", "@login-video-resolution-test.lua") {
		t.Fatalf("select resolution failed: %v", engine.Rt.ScriptErrors()[before:])
	}
	if !resolution.shown || effects.shown {
		t.Fatalf("resolution selection overlap resolution=%v effects=%v", resolution.shown, effects.shown)
	}
	if newErrors := engine.Rt.ScriptErrors()[before:]; len(newErrors) != 0 {
		t.Fatalf("video options script errors=%v", newErrors)
	}
}

func TestLiveGlueAccountLoginPlaceholder(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	account := engine.Rt.widgets["AccountLoginAccountEdit"]
	if account == nil {
		t.Fatal("AccountLoginAccountEdit missing")
	}
	fill := engine.Rt.widgets["AccountLoginAccountEditFill"]
	if fill == nil {
		t.Fatal("AccountLoginAccountEditFill missing")
	}
	t.Logf("fill: kind=%s shown=%v text=%q resolved=%q parent=%v", fill.kind.objectType(), fill.shown, fill.text, engine.resolveText(fill.text), fill.parent != nil)
	t.Logf("account: text=%q shown=%v", account.text, account.shown)
	for _, ch := range account.children {
		t.Logf("account child: name=%q kind=%s shown=%v text=%q", ch.name, ch.kind.objectType(), ch.shown, ch.text)
	}
	engine.SetSceneBackground(true)
	frame := engine.Render(1024, 768)
	t.Logf("rendered frame bounds=%v", frame.Bounds())
	rect := account.renderRect
	sr := ScreenRect(rect, 768)
	midY := (sr.Min.Y + sr.Max.Y) / 2
	fillrect := fill.renderRect
	fsr := ScreenRect(fillrect, 768)
	t.Logf("fill bounds = %v", fsr)
	bgPointX := sr.Min.X + 15
	bgPointY := midY
	centerColor := frame.At(bgPointX, bgPointY)
	t.Logf("account edit backdrop pixel at (%d,%d) = %v", bgPointX, bgPointY, centerColor)
	_, _, _, a := centerColor.RGBA()
	alpha8 := uint8(a >> 8)
	if alpha8 == 255 {
		t.Fatalf("edit box center is completely opaque (alpha 255), expected inner transparency")
	}
	if alpha8 == 0 {
		t.Fatalf("edit box center is completely empty (alpha 0)")
	}
	t.Logf("edit box center has authentic inner transparency: alpha=%d (~%.1f%% opacity)", alpha8, float64(alpha8)/255.0*100.0)

	// Verify placeholder hiding when text is entered
	engine.Rt.setText(account, "testuser")
	if fill.shown {
		t.Fatalf("placeholder should be hidden after setting text")
	}

	// Verify placeholder showing when text is cleared
	engine.Rt.setText(account, "")
	if !fill.shown {
		t.Fatalf("placeholder should be shown after clearing text")
	}
}

func TestLiveOptionsCategoryButtonsAreClickable(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if !engine.Rt.Execute("VideoOptionsFrame:Show()", "@options-click-test.lua") {
		t.Fatal(engine.Rt.ScriptErrors())
	}
	engine.Render(1024, 768)
	var target *widget
	for index := 1; index < 20; index++ {
		button := engine.Rt.widgets[fmt.Sprintf("VideoOptionsFrameCategoryFrameButton%d", index)]
		if button != nil && button.shown && button.kind == kindButton {
			target = button
			if index > 1 {
				break
			}
		}
	}
	if target == nil {
		t.Fatal("video options category buttons missing")
	}
	rect := target.renderRect
	x := (rect.X0 + rect.X1) * engine.uiScale / 2
	y := float64(768) - (rect.Y0+rect.Y1)*engine.uiScale/2
	if !engine.HandleMouse(x, y, window.MouseButtonLeft, true) || !engine.HandleMouse(x, y, window.MouseButtonLeft, false) {
		t.Fatalf("category button click was not handled at %g,%g", x, y)
	}
	if !target.highlightLocked {
		t.Fatalf("category button %s did not lock its native selection highlight", target.name)
	}
}

func TestLiveOptionsSliderDragUpdatesValue(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if !engine.Rt.Execute("VideoOptionsFrame:Show()", "@options-slider-test.lua") {
		t.Fatal(engine.Rt.ScriptErrors())
	}
	engine.Render(1024, 768)
	slider := engine.Rt.widgets["VideoOptionsResolutionPanelGammaSlider"]
	if slider == nil || slider.kind != kindSlider {
		t.Fatal("gamma slider missing")
	}
	rect := slider.renderRect
	y := float64(768) - (rect.Y0+rect.Y1)*engine.uiScale/2
	startX := (rect.X0 + 1) * engine.uiScale
	endX := (rect.X1 - 1) * engine.uiScale
	if !engine.HandleMouse(startX, y, window.MouseButtonLeft, true) {
		t.Fatal("slider press was not handled")
	}
	if !engine.HandleCursor(endX, y) {
		t.Fatal("slider drag was not handled")
	}
	if !engine.HandleMouse(endX, y, window.MouseButtonLeft, false) {
		t.Fatal("slider release was not handled")
	}
	if slider.value < slider.maxValue-0.05 {
		t.Fatalf("slider value=%v max=%v", slider.value, slider.maxValue)
	}
}

func TestLiveGlueEnterWorldIsDisabledWhileLoading(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.SetGlueState(GlueState{Connected: true, Characters: []CharacterEntry{{Name: "Test", Race: "RACE_HUMAN", Class: "WARRIOR", Level: 1}}})
	engine.Render(960, 640)
	button := engine.Rt.widgets["CharSelectEnterWorldButton"]
	if button == nil || !button.enabled {
		t.Fatal("enter-world button missing or disabled before load")
	}
	engine.SetWorldLoading(true)
	if button.enabled || !engine.HandleKey(window.KeyEnter) {
		t.Fatal("enter-world input was not consumed while loading")
	}
	engine.SetWorldLoading(false)
	if !button.enabled {
		t.Fatal("enter-world button did not re-enable after load failure")
	}
}

func TestLiveCharacterSelectTextAlignmentMatchesXML(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	characters := make([]CharacterEntry, 4)
	for index := range characters {
		characters[index] = CharacterEntry{Name: "Character", Race: "RACE_HUMAN", Class: "WARRIOR", Level: index + 1, Zone: "Stormwind City"}
	}
	engine.SetGlueState(GlueState{Connected: true, ServerName: "Moreno Land", Characters: characters})
	engine.Render(960, 640)
	for index := 1; index <= len(characters); index++ {
		button := engine.Rt.widgets[fmt.Sprintf("CharSelectCharacterButton%d", index)]
		name := engine.Rt.widgets[fmt.Sprintf("CharSelectCharacterButton%dButtonTextName", index)]
		info := engine.Rt.widgets[fmt.Sprintf("CharSelectCharacterButton%dButtonTextInfo", index)]
		location := engine.Rt.widgets[fmt.Sprintf("CharSelectCharacterButton%dButtonTextLocation", index)]
		if button == nil || name == nil || info == nil || location == nil {
			t.Fatalf("character %d text widgets missing", index)
		}
		if math.Abs(name.renderRect.X0-button.renderRect.X0) > 0.01 || math.Abs(info.renderRect.X0-name.renderRect.X0) > 0.01 || math.Abs(location.renderRect.X0-name.renderRect.X0) > 0.01 {
			t.Fatalf("character %d horizontal alignment button=%v name=%v info=%v location=%v", index, button.renderRect, name.renderRect, info.renderRect, location.renderRect)
		}
		t.Logf("character %d button=%v name=%v info=%v location=%v", index, button.renderRect, name.renderRect, info.renderRect, location.renderRect)
	}
}

func TestLiveGlueRenderBreaksButtonTextureCycles(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.SetGlueState(GlueState{Connected: true, Characters: []CharacterEntry{{Name: "Character", Race: "RACE_HUMAN", Class: "WARRIOR", Level: 1}}})
	button := engine.Rt.widgets["CharSelectCharacterButton1"]
	if button == nil || !button.shown {
		t.Fatal("character select button is not shown")
	}
	button.normalTexture = button
	engine.Render(960, 640)
}

func TestLiveCharacterSelectDragChangesFacing(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.SetGlueState(GlueState{Connected: true, Characters: []CharacterEntry{{Name: "Character", Race: "RACE_NIGHTELF", Class: "PALADIN", Level: 1, Zone: "Stormwind City"}}})
	engine.Render(960, 640)
	if !engine.HandleMouse(480, 320, window.MouseButtonLeft, true) {
		t.Fatal("character preview did not capture mouse down")
	}
	engine.HandleCursor(520, 320)
	engine.Update(1.0 / 60)
	engine.HandleMouse(520, 320, window.MouseButtonLeft, false)
	if math.Abs(float64(engine.Rt.Glue.CharacterFacing)) < 0.1 {
		t.Fatal("character drag did not change facing")
	}
}

func TestLiveCharacterCreateStateAndTextGeometry(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.SetGlueState(GlueState{Connected: true, Characters: []CharacterEntry{{Name: "Character", Race: "RACE_NIGHTELF", Class: "PALADIN", Level: 1}}})
	if !engine.Rt.Execute("SetGlueScreen('charcreate')", "@create-test.lua") {
		t.Fatalf("SetGlueScreen failed: %v", engine.Rt.ScriptErrors())
	}
	engine.Rt.FireEvent("SET_GLUE_SCREEN", lua.LString("charcreate"))
	frame := engine.Render(960, 640)
	if output := os.Getenv("WOW_TEST_CREATE_RENDER_OUT"); output != "" {
		file, createErr := os.Create(output)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if encodeErr := png.Encode(file, frame); encodeErr != nil {
			file.Close()
			t.Fatal(encodeErr)
		}
		file.Close()
	}
	create := engine.Rt.widgets["CharacterCreate"]
	if create == nil || !create.shown {
		t.Fatal("character create screen is not shown")
	}
	background, _ := engine.Rt.GetCVar("charCustomizeBackground")
	if !strings.HasSuffix(strings.ToLower(background), `ui_human\ui_human.m2`) {
		t.Fatalf("create background=%q", background)
	}
	for _, name := range []string{"CharacterCreateRaceLabel", "CharacterCreateClassLabel", "CharacterCreateRaceAbilityText", "CharacterCreateRaceText", "CharacterCreateClassRolesText", "CharacterCreateClassText", "CharacterCreateRaceScrollChild", "CharacterCreateClassScrollChild"} {
		widget := engine.Rt.widgets[name]
		if widget == nil {
			t.Fatalf("missing create widget %s", name)
		}
		t.Logf("create %s text=%q rect=%v", name, widget.text, widget.renderRect)
	}
	for _, pair := range [][2]string{{"CharacterCreateRaceScrollFrame", "CharacterCreateRaceText"}, {"CharacterCreateClassScrollFrame", "CharacterCreateClassText"}} {
		scroll := engine.Rt.widgets[pair[0]]
		textWidget := engine.Rt.widgets[pair[1]]
		if scroll == nil || textWidget == nil {
			t.Fatalf("missing create scroll widgets %s/%s", pair[0], pair[1])
		}
		if textWidget.renderRect.X0 < scroll.renderRect.X0-0.01 || textWidget.renderRect.X1 > scroll.renderRect.X1+0.01 {
			t.Fatalf("%s horizontal clip rect=%v scroll=%v", pair[1], textWidget.renderRect, scroll.renderRect)
		}
		t.Logf("create scroll=%s rect=%v text=%s rect=%v", pair[0], scroll.renderRect, pair[1], textWidget.renderRect)
	}
	for _, name := range []string{"CharacterCreateRaceScrollFrameScrollBar", "CharacterCreateClassScrollFrameScrollBar"} {
		scrollbar := engine.Rt.widgets[name]
		if scrollbar == nil || !scrollbar.shown || scrollbar.thumbTexture == nil || scrollbar.maxValue <= 0 {
			t.Fatalf("create scrollbar=%s state=%v", name, scrollbar)
		}
	}
	scroll := engine.Rt.widgets["CharacterCreateClassScrollFrame"]
	textWidget := engine.Rt.widgets["CharacterCreateClassText"]
	if scroll == nil || textWidget == nil {
		t.Fatal("class description scroll widgets missing")
	}
	before := textWidget.renderRect
	x := (scroll.renderRect.X0 + scroll.renderRect.X1) * engine.uiScale / 2
	y := float64(engine.screenHeight) - (scroll.renderRect.Y0+scroll.renderRect.Y1)*engine.uiScale/2
	engine.HandleCursor(x, y)
	handled := engine.HandleScroll(-1)
	if !handled || scroll.verticalScroll <= 0 {
		t.Fatalf("class description did not scroll range=%v offset=%v", scroll.verticalRange, scroll.verticalScroll)
	}
	engine.Render(960, 640)
	if textWidget.renderRect.Y1 >= before.Y1 {
		t.Fatalf("class description did not move before=%v after=%v", before, textWidget.renderRect)
	}
}

func TestLiveCharacterCreateDragChangesFacing(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.SetGlueState(GlueState{Connected: true})
	if !engine.Rt.Execute("SetGlueScreen('charcreate')", "@create-drag-test.lua") {
		t.Fatalf("SetGlueScreen failed: %v", engine.Rt.ScriptErrors())
	}
	engine.Rt.FireEvent("SET_GLUE_SCREEN", lua.LString("charcreate"))
	engine.Render(960, 640)
	initial, ok := engine.CreatePreviewState()
	if !ok || initial.Facing != -15 {
		t.Fatalf("create facing=%v state=%v", initial.Facing, ok)
	}
	if !engine.HandleMouse(480, 320, window.MouseButtonLeft, true) {
		t.Fatal("character create preview did not capture mouse down")
	}
	engine.HandleCursor(520, 320)
	engine.Update(1.0 / 60)
	engine.HandleMouse(520, 320, window.MouseButtonLeft, false)
	updated, ok := engine.CreatePreviewState()
	if !ok || math.Abs(float64(updated.Facing-initial.Facing)) < 0.1 {
		t.Fatalf("create drag facing=%v initial=%v state=%v", updated.Facing, initial.Facing, ok)
	}
}

func TestLiveCharacterCreateScrollbarDoesNotRotateModel(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if !engine.Rt.Execute("SetGlueScreen('charcreate')", "@create-scrollbar-input-test.lua") {
		t.Fatal(engine.Rt.ScriptErrors())
	}
	engine.Rt.FireEvent("SET_GLUE_SCREEN", lua.LString("charcreate"))
	engine.Render(960, 640)
	scroll := engine.Rt.widgets["CharacterCreateClassScrollFrame"]
	scrollbar := engine.Rt.widgets["CharacterCreateClassScrollFrameScrollBar"]
	if scroll == nil || scrollbar == nil || scrollbar.kind != kindSlider || scrollbar.maxValue <= 0 {
		t.Fatal("character create scrollbar missing range")
	}
	initialFacing := engine.Rt.createFacing
	rect := scrollbar.renderRect
	x := (rect.X0 + rect.X1) * engine.uiScale / 2
	y := float64(640) - (rect.Y0+rect.Y1)*engine.uiScale/2
	if !engine.HandleMouse(x, y, window.MouseButtonLeft, true) {
		t.Fatal("scrollbar press was not handled")
	}
	dragY := float64(640) - rect.Y1*engine.uiScale
	if !engine.HandleCursor(x, dragY) {
		t.Fatal("scrollbar drag was not handled")
	}
	if !engine.HandleMouse(x, dragY, window.MouseButtonLeft, false) {
		t.Fatal("scrollbar release was not handled")
	}
	if scrollbar.value <= 0 || scroll.verticalScroll <= 0 {
		t.Fatalf("scrollbar value=%v scroll=%v range=%v", scrollbar.value, scroll.verticalScroll, scrollbar.maxValue)
	}
	if engine.Rt.createFacing != initialFacing {
		t.Fatalf("scrollbar drag rotated model from %v to %v", initialFacing, engine.Rt.createFacing)
	}
}

func TestLiveCharacterCreateRaceContracts(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if !engine.Rt.Execute("race5Name, race5Faction = GetFactionForRace(5); race8Name, race8Faction = GetFactionForRace(8)", "@create-race-contract-test.lua") {
		t.Fatalf("race contract failed: %v", engine.Rt.ScriptErrors())
	}
	if engine.Rt.L.GetGlobal("race5Name") != lua.LString("Draenei") || engine.Rt.L.GetGlobal("race5Faction") != lua.LString("Alliance") || engine.Rt.L.GetGlobal("race8Name") != lua.LString("Tauren") || engine.Rt.L.GetGlobal("race8Faction") != lua.LString("Horde") {
		t.Fatalf("race order/factions draenei=%v/%v tauren=%v/%v", engine.Rt.L.GetGlobal("race5Name"), engine.Rt.L.GetGlobal("race5Faction"), engine.Rt.L.GetGlobal("race8Name"), engine.Rt.L.GetGlobal("race8Faction"))
	}
}

func TestLiveRealmRowDoubleClickRunsNativeOkayPath(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.SetGlueState(GlueState{Connected: true, Realms: []RealmInfo{{Name: "Other Realm", ID: 7, Characters: 2}}})
	if !engine.Rt.Execute("RequestRealmList()", "@realm-double-click-test.lua") {
		t.Fatalf("RequestRealmList failed: %v", engine.Rt.ScriptErrors())
	}
	engine.Render(960, 640)
	row := engine.Rt.widgets["RealmListRealmButton1"]
	list := engine.Rt.widgets["RealmList"]
	if row == nil || list == nil || !row.shown || !list.shown {
		t.Fatalf("realm row/list shown row=%v list=%v", row != nil && row.shown, list != nil && list.shown)
	}
	x := (row.renderRect.X0 + row.renderRect.X1) * engine.uiScale / 2
	y := float64(engine.screenHeight) - (row.renderRect.Y0+row.renderRect.Y1)*engine.uiScale/2
	engine.HandleMouse(x, y, window.MouseButtonLeft, true)
	engine.HandleMouse(x, y, window.MouseButtonLeft, false)
	engine.HandleMouse(x, y, window.MouseButtonLeft, true)
	engine.HandleMouse(x, y, window.MouseButtonLeft, false)
	if list.shown {
		t.Fatal("realm list stayed open after row double-click")
	}
	if engine.Rt.Glue.SelectedRealm != 7 {
		t.Fatalf("selected realm=%d", engine.Rt.Glue.SelectedRealm)
	}
}

func TestLiveGlueParentLetterboxesWideResize(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.SetInitialCredentials("", "", false)
	engine.Render(2560, 1392)
	root := engine.Rt.widgets["GlueParent"]
	if root == nil {
		t.Fatal("GlueParent missing")
	}
	wantWidth := 768.0 * 16.0 / 9.0
	if math.Abs(root.renderRect.W()-wantWidth) > 0.01 {
		t.Fatalf("wide GlueParent=%v want width=%v", root.renderRect, wantWidth)
	}
	if math.Abs(root.renderRect.X0-(engine.screen.W()-wantWidth)/2) > 0.01 {
		t.Fatalf("wide GlueParent margin=%v screen=%v", root.renderRect, engine.screen)
	}
}
