package ui

import (
	"image"
	"os"
	"testing"
)

func TestLoadingBarFillFitsAuthoredBorder(t *testing.T) {
	bar := loadingBarRect(960, 640)
	if bar != image.Rect(192, 576, 768, 608) {
		t.Fatalf("bar=%v", bar)
	}
	for _, progress := range []float64{0, 0.5, 1, 2, -1} {
		fill := loadingBarFillRect(bar, progress)
		if progress <= 0 {
			if !fill.Empty() {
				t.Fatalf("progress %.2f fill should be empty: %v", progress, fill)
			}
			continue
		}
		if fill.Min.X < bar.Min.X || fill.Min.Y < bar.Min.Y || fill.Max.X > bar.Max.X || fill.Max.Y > bar.Max.Y {
			t.Fatalf("progress %.2f fill=%v bar=%v", progress, fill, bar)
		}
	}
	half := loadingBarFillRect(bar, 0.5)
	if half != image.Rect(228, 584, 480, 600) {
		t.Fatalf("half progress fill=%v bar=%v", half, bar)
	}
}

func TestLiveWorldChatGeometryMatchesFrameXML(t *testing.T) {
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
	chat := engine.Rt.widgets["ChatFrame1"]
	if chat == nil {
		t.Fatal("ChatFrame1 missing")
	}
	if chat.width != 430 || chat.height != 120 {
		t.Fatalf("ChatFrame1 size=%gx%g", chat.width, chat.height)
	}
	if chat.renderRect.X0 < 31.9 || chat.renderRect.X0 > 32.1 || chat.renderRect.Y0 < 95 || chat.renderRect.Y0 > 96 {
		t.Fatalf("ChatFrame1 origin=%v", chat.renderRect)
	}
	background := engine.Rt.widgets["ChatFrame1Background"]
	if background == nil || background.textureFile != `Interface\ChatFrame\ChatFrameBackground` {
		t.Fatalf("ChatFrame1 background=%v", background)
	}
	if background.renderRect != (Rect{30, 89, 464, 218}) {
		t.Fatalf("ChatFrame1 background rect=%v", background.renderRect)
	}
	var chatFont *widget
	for _, child := range chat.children {
		if child.kind == kindFontString {
			chatFont = child
			break
		}
	}
	if chatFont == nil || chatFont.fontObject != "ChatFontNormal" || !chatFont.nonSpaceWrap {
		t.Fatalf("ChatFrame1 font region=%#v", chatFont)
	}
	for _, path := range []string{`Interface\ChatFrame\ChatFrameBackground`, `Interface\ChatFrame\UI-ChatFrame-BorderCorner`, `Interface\ChatFrame\UI-ChatFrame-BorderLeft`, `Interface\ChatFrame\UI-ChatFrame-BorderTop`} {
		if engine.loadBLP(path) == nil {
			t.Fatalf("missing chat asset %s", path)
		}
	}
	buttonFrame := engine.Rt.widgets["ChatFrame1ButtonFrame"]
	if buttonFrame == nil {
		t.Fatal("ChatFrame1 button frame missing")
	}
	if buttonFrame.renderRect != (Rect{-1, 95, 28, 215}) {
		t.Fatalf("ChatFrame1 button frame rect=%v", buttonFrame.renderRect)
	}
	if len(buttonFrame.points) != 2 || buttonFrame.points[0].point != "TOPRIGHT" || buttonFrame.points[0].relativeTo != "ChatFrame1" || buttonFrame.points[0].relativePoint != "TOPLEFT" || buttonFrame.points[1].point != "BOTTOMRIGHT" || buttonFrame.points[1].relativeTo != "ChatFrame1" || buttonFrame.points[1].relativePoint != "BOTTOMLEFT" {
		t.Fatalf("ChatFrame1 button frame points=%v", buttonFrame.points)
	}
	if side := chat.fields.RawGetString("buttonSide").String(); side != "left" {
		t.Fatalf("ChatFrame1 button side=%q", side)
	}
	if errors := engine.Rt.ScriptErrors(); len(errors) != 0 {
		t.Fatalf("chat UI script errors=%v", errors)
	}
}
