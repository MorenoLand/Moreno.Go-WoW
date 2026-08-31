package ui

import (
	"testing"

	"github.com/g3n/engine/window"
)

func TestSoundHotkeysShowStatusText(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	eng := &UIEngine{Rt: rt}
	if !eng.HandleKeyWithMods(window.KeyM, window.ModControl) || eng.statusText != "Music Disabled" {
		t.Fatalf("music status=%q", eng.statusText)
	}
	if !eng.HandleKeyWithMods(window.KeyS, window.ModControl) || eng.statusText != "Sound Effects Disabled" {
		t.Fatalf("sound status=%q", eng.statusText)
	}
	eng.Update(3)
	if eng.statusText != "" {
		t.Fatalf("status did not expire: %q", eng.statusText)
	}
}
