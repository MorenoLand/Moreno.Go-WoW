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
	t.Logf("ChatFrame1 rect=%v", chat.renderRect)
}
