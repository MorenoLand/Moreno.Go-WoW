package ui

import (
	"math"
	"os"
	"strings"
	"testing"
)

func TestLiveOptionsSliderAndHardwareDropDownGeometry(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	var baselineThumbH, baselineLeftH, baselineLeftYOff float64
	for _, size := range [][2]int{{1024, 768}, {2560, 1440}} {
		if !engine.Rt.Execute(`
			VideoOptionsFrame:Show()
			AudioOptionsFrame:Show()
			if AudioOptionsSoundPanel then AudioOptionsSoundPanel:Show() end
		`, "@options-geom-diag.lua") {
			t.Fatal(engine.Rt.ScriptErrors())
		}
		engine.Render(size[0], size[1])
		wantScale := float64(size[1]) / 768.0
		if math.Abs(engine.uiScale-wantScale) > 0.001 {
			t.Fatalf("uiScale=%v want %v at %dx%d", engine.uiScale, wantScale, size[0], size[1])
		}

		slider := engine.Rt.widgets["VideoOptionsResolutionPanelGammaSlider"]
		if slider == nil || slider.kind != kindSlider {
			t.Fatal("gamma slider missing")
		}
		thumb := slider.thumbTexture
		if thumb == nil {
			t.Fatal("slider thumb missing")
		}
		t.Logf("%dx%d uiScale=%v slider size=%gx%g rect=%v value=%v", size[0], size[1], engine.uiScale, slider.width, slider.height, slider.renderRect, slider.value)
		t.Logf("  thumb size=%gx%g rect=%v", thumb.width, thumb.height, thumb.renderRect)

		if slider.height != 17 {
			t.Fatalf("slider height want 17 got %g", slider.height)
		}
		if thumb.width != 32 || thumb.height != 32 {
			t.Fatalf("thumb authored size want 32x32 got %gx%g", thumb.width, thumb.height)
		}
		trackCY := (slider.renderRect.Y0 + slider.renderRect.Y1) / 2
		thumbCY := (thumb.renderRect.Y0 + thumb.renderRect.Y1) / 2
		if math.Abs(trackCY-thumbCY) > 0.01 {
			t.Fatalf("thumb vertical center=%v track center=%v at %dx%d", thumbCY, trackCY, size[0], size[1])
		}
		if math.Abs(thumb.renderRect.H()-32) > 0.01 {
			t.Fatalf("thumb render height=%v want 32 at %dx%d", thumb.renderRect.H(), size[0], size[1])
		}
		fraction := 0.0
		if slider.maxValue > slider.minValue {
			fraction = (slider.value - slider.minValue) / (slider.maxValue - slider.minValue)
		}
		trackStart := slider.renderRect.X0 + thumb.width/2
		trackEnd := slider.renderRect.X1 - thumb.width/2
		wantCX := trackStart + (trackEnd-trackStart)*fraction
		gotCX := (thumb.renderRect.X0 + thumb.renderRect.X1) / 2
		if math.Abs(gotCX-wantCX) > 0.5 {
			t.Fatalf("thumb centerX=%v want %v fraction=%v at %dx%d", gotCX, wantCX, fraction, size[0], size[1])
		}
		if math.Abs(trackCY*engine.uiScale-thumbCY*engine.uiScale) > 0.01 {
			t.Fatalf("screen thumb/track center diverge at scale=%v", engine.uiScale)
		}
		if baselineThumbH == 0 {
			baselineThumbH = thumb.renderRect.H()
		} else if math.Abs(thumb.renderRect.H()-baselineThumbH) > 0.01 {
			t.Fatalf("thumb logical height changed with resolution: %v vs %v", thumb.renderRect.H(), baselineThumbH)
		}

		dd := engine.Rt.widgets["AudioOptionsSoundPanelHardwareDropDown"]
		left := engine.Rt.widgets["AudioOptionsSoundPanelHardwareDropDownLeft"]
		mid := engine.Rt.widgets["AudioOptionsSoundPanelHardwareDropDownMiddle"]
		right := engine.Rt.widgets["AudioOptionsSoundPanelHardwareDropDownRight"]
		if dd == nil || left == nil || mid == nil || right == nil {
			for name, w := range engine.Rt.widgets {
				if strings.Contains(strings.ToLower(name), "hardwaredropdown") ||
					(strings.Contains(name, "SoundPanel") && strings.Contains(name, "DropDown")) {
					t.Logf("widget %s kind=%v size=%gx%g shown=%v", name, w.kind, w.width, w.height, w.shown)
				}
			}
			t.Fatalf("HardwareDropDown widgets missing dd=%v left=%v mid=%v right=%v", dd != nil, left != nil, mid != nil, right != nil)
		}
		t.Logf("  dropdown frame size=%gx%g rect=%v", dd.width, dd.height, dd.renderRect)
		t.Logf("  Left size=%gx%g rect=%v", left.width, left.height, left.renderRect)
		t.Logf("  Middle size=%gx%g rect=%v", mid.width, mid.height, mid.renderRect)
		t.Logf("  Right size=%gx%g rect=%v", right.width, right.height, right.renderRect)

		if dd.height != 32 {
			t.Fatalf("dropdown height want 32 got %g", dd.height)
		}
		if left.width != 25 || right.width != 25 {
			t.Fatalf("label ends want width 25 got left=%g right=%g", left.width, right.width)
		}
		for _, tex := range []*widget{left, mid, right} {
			if tex.height != 64 {
				t.Fatalf("%s height want 64 got %g", tex.name, tex.height)
			}
			if math.Abs(tex.renderRect.H()-64) > 0.01 {
				t.Fatalf("%s render height=%v want 64 at %dx%d", tex.name, tex.renderRect.H(), size[0], size[1])
			}
		}
		if math.Abs(dd.width-(mid.width+50)) > 0.01 {
			t.Fatalf("dropdown width=%g want middle+50=%g", dd.width, mid.width+50)
		}
		if math.Abs(left.renderRect.X1-mid.renderRect.X0) > 0.01 || math.Abs(mid.renderRect.X1-right.renderRect.X0) > 0.01 {
			t.Fatalf("label segments not abutted left=%v mid=%v right=%v", left.renderRect, mid.renderRect, right.renderRect)
		}
		wantLeftY1 := dd.renderRect.Y1 + 17
		if math.Abs(left.renderRect.Y1-wantLeftY1) > 0.01 {
			t.Fatalf("Left Y1=%v want frame.Y1+17=%v (frame=%v)", left.renderRect.Y1, wantLeftY1, dd.renderRect)
		}
		if baselineLeftH == 0 {
			baselineLeftH = left.renderRect.H()
			baselineLeftYOff = left.renderRect.Y1 - dd.renderRect.Y1
		} else {
			if math.Abs(left.renderRect.H()-baselineLeftH) > 0.01 {
				t.Fatalf("Left logical height changed with resolution")
			}
			if math.Abs((left.renderRect.Y1-dd.renderRect.Y1)-baselineLeftYOff) > 0.01 {
				t.Fatalf("Left Y offset changed with resolution")
			}
		}
	}
}

func TestHorizontalSliderThumbTravelMatchesHitGeometry(t *testing.T) {
	thumbW := 32.0
	rect := Rect{X0: 100, Y0: 200, X1: 254, Y1: 217}
	for _, fraction := range []float64{0, 0.25, 0.5, 1} {
		trackStart := rect.X0 + thumbW/2
		trackEnd := rect.X1 - thumbW/2
		travel := math.Max(0, trackEnd-trackStart)
		centerX := trackStart + travel*fraction
		centerY := (rect.Y0 + rect.Y1) / 2
		thumb := Rect{X0: centerX - thumbW/2, Y0: centerY - 16, X1: centerX + thumbW/2, Y1: centerY + 16}
		if math.Abs(thumb.H()-32) > 0.01 {
			t.Fatalf("thumb H=%v", thumb.H())
		}
		if math.Abs((thumb.Y0+thumb.Y1)/2-centerY) > 0.01 {
			t.Fatal("thumb not vertically centered on track")
		}
		gotFrac := (centerX - trackStart) / (trackEnd - trackStart)
		if math.Abs(gotFrac-fraction) > 1e-9 {
			t.Fatalf("hit fraction=%v want %v", gotFrac, fraction)
		}
	}
}

func TestGlueDropDownLabelFrameAnchorSizing(t *testing.T) {
	left := newWidget(kindTexture, "DDLeft")
	left.width, left.height = 25, 64
	left.points = []anchorPoint{{point: "TOPLEFT", y: 17}}
	mid := newWidget(kindTexture, "DDMiddle")
	mid.width, mid.height = 136, 64
	mid.points = []anchorPoint{{point: "LEFT", relativeTo: "DDLeft", relativePoint: "RIGHT"}}
	right := newWidget(kindTexture, "DDRight")
	right.width, right.height = 25, 64
	right.points = []anchorPoint{{point: "LEFT", relativeTo: "DDMiddle", relativePoint: "RIGHT"}}

	parent := Rect{X0: 0, Y0: 0, X1: 186, Y1: 32}
	lookup := map[string]Rect{}
	leftRect := ResolveRect(left, parent)
	lookup["DDLeft"] = leftRect
	midRect := resolveRect(mid, parent, func(name string) (Rect, bool) {
		r, ok := lookup[name]
		return r, ok
	})
	lookup["DDMiddle"] = midRect
	rightRect := resolveRect(right, parent, func(name string) (Rect, bool) {
		r, ok := lookup[name]
		return r, ok
	})

	if leftRect.H() != 64 || midRect.H() != 64 || rightRect.H() != 64 {
		t.Fatalf("label heights left=%v mid=%v right=%v want 64", leftRect.H(), midRect.H(), rightRect.H())
	}
	if math.Abs(leftRect.Y1-49) > 0.01 {
		t.Fatalf("Left Y1=%v want 49", leftRect.Y1)
	}
	if math.Abs(leftRect.Y0-(-15)) > 0.01 {
		t.Fatalf("Left Y0=%v want -15", leftRect.Y0)
	}
	if math.Abs(midRect.X0-leftRect.X1) > 0.01 || math.Abs(rightRect.X0-midRect.X1) > 0.01 {
		t.Fatalf("segments not abutted left=%v mid=%v right=%v", leftRect, midRect, rightRect)
	}
	if math.Abs(rightRect.X1-186) > 0.01 {
		t.Fatalf("Right X1=%v want 186", rightRect.X1)
	}
}
