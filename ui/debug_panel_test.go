package ui

import (
	"testing"

	"github.com/g3n/engine/window"
)

func TestDebugPanelF2AndTitleCollapse(t *testing.T) {
	engine := &UIEngine{screen: Rect{X1: 1152, Y1: 768}, uiScale: 5.0 / 6.0, screenHeight: 640}
	engine.SetDebugPanelLines([]string{"renderer", "frame"})
	if engine.DebugPanelVisible() {
		t.Fatal("debug panel should start hidden")
	}
	if !engine.HandleKeyWithMods(window.KeyF2, 0) || !engine.DebugPanelVisible() {
		t.Fatal("F2 did not show the debug panel")
	}
	if !engine.HandleMouse(30, 20, window.MouseButtonLeft, true) || !engine.debugPanel.collapsed {
		t.Fatal("title click did not collapse the debug panel")
	}
	engine.HandleMouse(30, 20, window.MouseButtonLeft, true)
	old := engine.debugPanelRect()
	if !engine.HandleMouse(100, 20, window.MouseButtonLeft, true) {
		t.Fatal("title click did not start dragging the debug panel")
	}
	engine.HandleCursor(220, 100)
	engine.HandleMouse(220, 100, window.MouseButtonLeft, false)
	if engine.debugPanelRect().X0 <= old.X0 {
		t.Fatal("drag did not move the debug panel")
	}
	if !engine.HandleKeyWithMods(window.KeyF2, 0) || engine.DebugPanelVisible() {
		t.Fatal("F2 did not hide the debug panel")
	}
}
