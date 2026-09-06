package ui

import (
	"image"
	"image/color"
	"os"
	"testing"
)

func TestChatTabSelectedMiddleMatchesBGMiddle(t *testing.T) {
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
	engine.RenderWorld(960, 640)

	middle := engine.Rt.widgets["ChatFrame1TabMiddle"]
	selected := engine.Rt.widgets["ChatFrame1TabSelectedMiddle"]
	highlight := engine.Rt.widgets["ChatFrame1TabHighlightMiddle"]
	if middle == nil || selected == nil || highlight == nil {
		t.Fatal("chat tab mid textures missing")
	}
	if selected.renderRect != middle.renderRect {
		t.Fatalf("SelectedMiddle renderRect=%v want Middle=%v (relativeTo fill must not use parent setAllPoints)", selected.renderRect, middle.renderRect)
	}

	tab := engine.Rt.widgets["ChatFrame1Tab"]
	tab.highlighted = true
	engine.RenderWorld(960, 640)
	if highlight.renderRect != middle.renderRect {
		t.Fatalf("HighlightMiddle renderRect=%v want Middle=%v", highlight.renderRect, middle.renderRect)
	}
}

func TestChatMessagesDrawOnceAtUIScaleWithoutFontStringDouble(t *testing.T) {
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
	chat := engine.Rt.widgets["ChatFrame1"]
	if chat == nil {
		t.Fatal("ChatFrame1 missing")
	}

	var fontAttr *widget
	for _, child := range chat.children {
		if child.kind == kindFontString {
			fontAttr = child
			break
		}
	}
	if fontAttr == nil {
		t.Fatal("ChatFrame1 font attribute FontString missing")
	}
	fontAttr.text = "This server is running the AutoBalance module."
	fontAttr.width = chat.width
	fontAttr.height = chat.height
	fontAttr.autoTextWidth = false
	fontAttr.autoTextHeight = false
	fontAttr.explicitWidth = true
	fontAttr.points = nil

	if !engine.Rt.Execute(`
ChatFrame1:AddMessage("This server is running the AutoBalance module.", 1.0, 1.0, 0.0, 1)
ChatFrame1:AddMessage("This server is running Solo Dungeon Finder module.", 1.0, 1.0, 0.0, 1)
`, "@chat-msg-color.lua") {
		t.Fatalf("AddMessage failed: %v", engine.Rt.ScriptErrors())
	}
	if len(chat.messages) != 2 {
		t.Fatalf("messages=%d", len(chat.messages))
	}
	if chat.messages[0].color.r != 1 || chat.messages[0].color.g != 1 || chat.messages[0].color.b != 0 {
		t.Fatalf("AddMessage color=%v want yellow", chat.messages[0].color)
	}

	const width, height = 960, 640
	img := engine.RenderWorld(width, height)
	scale := engine.uiScale
	r := chat.renderRect
	x0 := int(r.X0 * scale)
	x1 := int(r.X1 * scale)
	yTop := height - int(r.Y1*scale)
	yBot := height - int(r.Y0*scale)

	below := countBrightChatGlyphs(img, x0, x1, yBot+1, height)
	if below > 0 {
		t.Fatalf("chat glyphs spilled below frame: %d pixels (y>%d)", below, yBot)
	}

	bands := countBrightChatBands(img, x0, x1, yTop, yBot)
	if bands < 2 || bands > 3 {
		t.Fatalf("message bands=%d want 2 (or 3 with shadow bleed)", bands)
	}

	face := engine.faceFor(fontAttr, nil, nil)
	if face == nil {
		t.Fatal("message face nil")
	}
	wantH := face.Metrics().Height.Ceil()
	gotH := brightBandHeight(img, x0, x1, yTop, yBot)
	if gotH > wantH+3 {
		t.Fatalf("message glyph band height=%d want <=%d (uiScale=%g)", gotH, wantH+3, scale)
	}
}

func countBrightChatGlyphs(img *image.RGBA, x0, x1, y0, y1 int) int {
	n := 0
	b := img.Bounds()
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if isBrightChatGlyph(img.RGBAAt(x, y)) {
				n++
			}
		}
	}
	return n
}

func countBrightChatBands(img *image.RGBA, x0, x1, y0, y1 int) int {
	bands := 0
	in := false
	b := img.Bounds()
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	for y := y0; y < y1; y++ {
		row := false
		for x := x0; x < x1; x++ {
			if isBrightChatGlyph(img.RGBAAt(x, y)) {
				row = true
				break
			}
		}
		if row && !in {
			bands++
			in = true
		}
		if !row {
			in = false
		}
	}
	return bands
}

func brightBandHeight(img *image.RGBA, x0, x1, y0, y1 int) int {
	first, last := -1, -1
	b := img.Bounds()
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	for y := y0; y < y1; y++ {
		hit := false
		for x := x0; x < x1; x++ {
			if isBrightChatGlyph(img.RGBAAt(x, y)) {
				hit = true
				break
			}
		}
		if hit {
			if first < 0 {
				first = y
			}
			last = y
			continue
		}
		if first >= 0 {
			return last - first + 1
		}
	}
	if first < 0 {
		return 0
	}
	return last - first + 1
}

func isBrightChatGlyph(c color.RGBA) bool {
	if c.A < 80 {
		return false
	}
	if c.R > 180 && c.G > 180 && c.B < 100 {
		return true
	}
	return c.R > 200 && c.G > 200 && c.B > 200
}
