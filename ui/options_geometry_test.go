package ui

import (
	"os"
	"testing"
)

func TestOptionsListSpacerKeepsTopOffset(t *testing.T) {
	parent := newWidget(kindFrame, "Category")
	parent.width, parent.height = 175, 449
	topLeft := newWidget(kindTexture, "CategoryTopLeft")
	topLeft.width, topLeft.height = 16, 16
	topLeft.points = []anchorPoint{{point: "TOPLEFT"}}
	topRight := newWidget(kindTexture, "CategoryTopRight")
	topRight.width, topRight.height = 16, 16
	topRight.points = []anchorPoint{{point: "TOPRIGHT"}}
	top := newWidget(kindTexture, "CategoryTop")
	top.width, top.height = 0, 16
	top.points = []anchorPoint{
		{point: "TOPLEFT", relativeTo: "CategoryTopLeft", relativePoint: "TOPRIGHT", y: 7},
		{point: "TOPRIGHT", relativeTo: "CategoryTopRight", relativePoint: "TOPLEFT"},
	}
	parentRect := Rect{X0: 0, Y0: 0, X1: 175, Y1: 449}
	lookup := map[string]Rect{
		"CategoryTopLeft":  ResolveRect(topLeft, parentRect),
		"CategoryTopRight": ResolveRect(topRight, parentRect),
	}
	got := resolveRect(top, parentRect, func(name string) (Rect, bool) {
		r, ok := lookup[name]
		return r, ok
	})
	wantTop := lookup["CategoryTopLeft"].Y1 + 7
	if got.Y1 != wantTop {
		t.Fatalf("top spacer Y1=%v want %v (rect=%v topLeft=%v)", got.Y1, wantTop, got, lookup["CategoryTopLeft"])
	}
}

func TestChildDrawsBehindParentByFrameLevel(t *testing.T) {
	parent := newWidget(kindFrame, "AudioOptionsFrame")
	parent.frameStrata = frameStrataOrder("HIGH")
	parent.frameLevel = 0
	backdrop := newWidget(kindFrame, "AudioOptionsFrameBackdrop")
	backdrop.frameStrata = parent.frameStrata
	backdrop.frameLevel = -1
	category := newWidget(kindFrame, "AudioOptionsFrameCategoryFrame")
	category.frameStrata = parent.frameStrata
	category.frameLevel = 0
	texture := newWidget(kindTexture, "AudioOptionsFrameHeader")
	texture.frameStrata = parent.frameStrata
	if !childDrawsBehindParent(backdrop, parent) {
		t.Fatal("backdrop should draw behind options frame")
	}
	if childDrawsBehindParent(category, parent) {
		t.Fatal("category should not draw behind options frame")
	}
	if childDrawsBehindParent(texture, parent) {
		t.Fatal("layer textures stay with the parent")
	}
}

func TestInteractiveWidgetsEnableMouseByDefault(t *testing.T) {
	if !newWidget(kindButton, "B").enableMouse || !newWidget(kindSlider, "S").enableMouse {
		t.Fatal("interactive widgets should enable mouse by default")
	}
	if newWidget(kindFrame, "F").enableMouse {
		t.Fatal("frames should not enable mouse by default")
	}
}

func TestLiveOptionsFrameBackdropStaysBehindDialog(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if !engine.Rt.Execute("AudioOptionsFrame:Show()", "@options-backdrop-test.lua") {
		t.Fatal(engine.Rt.ScriptErrors())
	}
	img := engine.Render(2560, 1440)
	frame := engine.Rt.widgets["AudioOptionsFrame"]
	backdrop := engine.Rt.widgets["AudioOptionsFrameBackdrop"]
	category := engine.Rt.widgets["AudioOptionsFrameCategoryFrame"]
	button := engine.Rt.widgets["AudioOptionsFrameCategoryFrameButton1"]
	top := engine.Rt.widgets["AudioOptionsFrameCategoryFrameTop"]
	topLeft := engine.Rt.widgets["AudioOptionsFrameCategoryFrameTopLeft"]
	if frame == nil || backdrop == nil || category == nil || button == nil || top == nil || topLeft == nil {
		t.Fatal("options frame widgets missing")
	}
	if backdrop.frameLevel >= frame.frameLevel {
		t.Fatalf("backdrop level=%d frame level=%d", backdrop.frameLevel, frame.frameLevel)
	}
	if category.frameStrata != frame.frameStrata {
		t.Fatalf("category strata=%d frame strata=%d", category.frameStrata, frame.frameStrata)
	}
	if !button.enableMouse {
		t.Fatal("category button should enable mouse by default")
	}
	if top.renderRect.Y1 != topLeft.renderRect.Y1+7 {
		t.Fatalf("top spacer Y1=%v topLeft Y1=%v", top.renderRect.Y1, topLeft.renderRect.Y1)
	}
	pc := engine.Rt.widgets["AudioOptionsFramePanelContainer"]
	x := int((pc.renderRect.X0 + pc.renderRect.X1) / 2 * engine.uiScale)
	y := engine.screenHeight - int((pc.renderRect.Y0+pc.renderRect.Y1)/2*engine.uiScale)
	pixel := img.RGBAAt(x, y)
	if pixel.R <= 2 && pixel.G <= 2 && pixel.B <= 2 {
		t.Fatalf("panel interior still crushed to black at %d,%d: %v", x, y, pixel)
	}
}
