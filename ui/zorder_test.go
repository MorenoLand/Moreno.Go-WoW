package ui

import "testing"

func TestOrderedChildrenUsesFrameStrataAndLevels(t *testing.T) {
	low := newWidget(kindFrame, "low")
	low.frameStrata = frameStrataOrder("DIALOG")
	low.frameLevel = 1
	high := newWidget(kindFrame, "high")
	high.frameStrata = frameStrataOrder("TOOLTIP")
	ordered := orderedChildren([]*widget{high, low})
	if ordered[0] != low || ordered[1] != high {
		t.Fatalf("z order=%s,%s", ordered[0].name, ordered[1].name)
	}
}

func TestCharacterSelectionHighlightRendersBehindSiblingText(t *testing.T) {
	previous := newWidget(kindButton, "CharSelectCharacterButton1")
	selected := newWidget(kindButton, "CharSelectCharacterButton2")
	selected.highlighted = true
	ordered := orderedChildren([]*widget{previous, selected})
	if ordered[0] != selected || ordered[1] != previous {
		t.Fatalf("character selection z order=%s,%s", ordered[0].name, ordered[1].name)
	}
}

func TestLockHighlightPreservesSelectionState(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	button := newWidget(kindButton, "Button")
	rt.register(button)
	if !rt.Execute("Button:LockHighlight()", "@lock-highlight-test.lua") {
		t.Fatalf("LockHighlight failed: %v", rt.ScriptErrors())
	}
	if !button.highlightLocked || !isHighlighted(button) {
		t.Fatalf("locked highlight state=%t/%t", button.highlightLocked, button.highlighted)
	}
	if !rt.Execute("Button:UnlockHighlight()", "@lock-highlight-test.lua") {
		t.Fatalf("UnlockHighlight failed: %v", rt.ScriptErrors())
	}
	if button.highlightLocked || isHighlighted(button) {
		t.Fatalf("unlocked highlight state=%t/%t", button.highlightLocked, button.highlighted)
	}
}

func TestAddWidgetChildDeduplicates(t *testing.T) {
	parent := newWidget(kindFrame, "parent")
	child := newWidget(kindFrame, "child")
	addWidgetChild(parent, child)
	addWidgetChild(parent, child)
	if len(parent.children) != 1 {
		t.Fatalf("child count=%d", len(parent.children))
	}
}
