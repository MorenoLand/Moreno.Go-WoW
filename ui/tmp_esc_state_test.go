package ui

import (
	"image/png"
	"os"
	"testing"
)

func TestTmpESCState(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	for _, action := range []struct{ button, panel string }{
		{"GameMenuButtonOptions", "VideoOptionsFrame"},
		{"GameMenuButtonSoundOptions", "AudioOptionsFrame"},
		{"GameMenuButtonUIOptions", "InterfaceOptionsFrame"},
		{"GameMenuButtonKeybindings", "KeyBindingFrame"},
		{"GameMenuButtonMacros", "MacroFrame"},
	} {
		func() {
			engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()
			engine.Rt.Host = &logoutProbeHost{hostScreen: hostScreen{w: 960, h: 640}}
			if err := engine.LoadWorldUI(); err != nil {
				t.Fatal(err)
			}
			if !engine.ToggleGameMenu() || !engine.GameMenuShown() {
				t.Fatal("menu did not open")
			}
			before := len(engine.Rt.ScriptErrors())
			if !engine.Rt.Execute(action.button+":Click()", "@tmp-esc-action.lua") {
				t.Fatal(engine.Rt.ScriptErrors()[before:])
			}
			frame := engine.Rt.widgets[action.panel]
			if action.panel == "InterfaceOptionsFrame" {
				image := engine.RenderWorld(960, 640)
				file, fileErr := os.Create("../bin/tmp-interface-options.png")
				if fileErr != nil {
					t.Fatal(fileErr)
				}
				if encodeErr := png.Encode(file, image); encodeErr != nil {
					t.Fatal(encodeErr)
				}
				_ = file.Close()
			}
			t.Logf("action=%s menu=%t panel=%t panelParent=%s errors=%v", action.button, engine.Rt.widgets["GameMenuFrame"].shown, frame != nil && frame.shown, tmpESCParent(frame), engine.Rt.ScriptErrors()[before:])
		}()
	}
}

func tmpESCParent(w *widget) string {
	if w == nil || w.parent == nil {
		return ""
	}
	return w.parent.name
}
