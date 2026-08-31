package ui

import (
	"testing"

	"github.com/g3n/engine/window"
	"golang.org/x/image/font/basicfont"
)

func TestEditBoxSelectionReplacement(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	edit := newWidget(kindEditBox, "Edit")
	rt.setText(edit, "denveous")
	rt.setFocus(edit)
	edit.selectionStart = 0
	edit.selectionEnd = len([]rune(edit.text))
	edit.selectionAnchor = 0
	edit.cursor = edit.selectionEnd
	eng := &UIEngine{Rt: rt}
	if !eng.HandleChar('x') || edit.text != "x" || edit.cursor != 1 {
		t.Fatalf("replacement text=%q cursor=%d", edit.text, edit.cursor)
	}
}

func TestEditBoxCursorUsesPhysicalTextPosition(t *testing.T) {
	if position := editCursorIndex(basicfont.Face7x13, []rune("abcd"), 14); position != 2 {
		t.Fatalf("cursor index=%d", position)
	}
}

func TestEditBoxControlAAndDeleteSelection(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	edit := newWidget(kindEditBox, "Edit")
	rt.setText(edit, "abc")
	rt.setFocus(edit)
	eng := &UIEngine{Rt: rt}
	if !eng.HandleKeyWithMods(window.KeyA, window.ModControl) {
		t.Fatal("control-a was not handled")
	}
	if !eng.HandleKey(window.KeyBackspace) || edit.text != "" || edit.cursor != 0 {
		t.Fatalf("selection deletion text=%q cursor=%d", edit.text, edit.cursor)
	}
}
