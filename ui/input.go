package ui

import (
	"math"
	"strings"
	"time"

	"github.com/g3n/engine/window"
	lua "github.com/yuin/gopher-lua"
)

// Mouse buttons a widget can be registered for (RegisterForClicks and
// RegisterForDrag), as bits.
const (
	clickLeftUp uint8 = 1 << iota
	clickLeftDown
	clickRightUp
	clickRightDown
	clickMiddleUp
	clickMiddleDown
)

const (
	dragLeft uint8 = 1 << iota
	dragRight
	dragMiddle
)

// dragThreshold is how far (in screen pixels) a pressed button moves before a
// registered widget starts a drag.
const dragThreshold = 5

type mousePress struct {
	widget  *widget
	x, y    float64
	dragged bool
}

type dragState struct {
	widget *widget
	button string
}

func mouseButtonName(button window.MouseButton) string {
	switch button {
	case window.MouseButtonLeft:
		return "LeftButton"
	case window.MouseButtonRight:
		return "RightButton"
	case window.MouseButtonMiddle:
		return "MiddleButton"
	}
	return ""
}

// clickBits parses RegisterForClicks arguments.
func clickBits(names []string) uint8 {
	var bits uint8
	for _, name := range names {
		switch strings.ToLower(name) {
		case "anyup":
			bits |= clickLeftUp | clickRightUp | clickMiddleUp
		case "anydown":
			bits |= clickLeftDown | clickRightDown | clickMiddleDown
		case "leftbuttonup":
			bits |= clickLeftUp
		case "leftbuttondown":
			bits |= clickLeftDown
		case "rightbuttonup":
			bits |= clickRightUp
		case "rightbuttondown":
			bits |= clickRightDown
		case "middlebuttonup":
			bits |= clickMiddleUp
		case "middlebuttondown":
			bits |= clickMiddleDown
		}
	}
	return bits
}

func dragBits(names []string) uint8 {
	var bits uint8
	for _, name := range names {
		switch strings.ToLower(name) {
		case "leftbutton":
			bits |= dragLeft
		case "rightbutton":
			bits |= dragRight
		case "middlebutton":
			bits |= dragMiddle
		}
	}
	return bits
}

func (w *widget) clicksOn(button string, down bool) bool {
	mask := w.clickMask
	if mask == 0 {
		mask = clickLeftUp
	}
	var bit uint8
	switch button {
	case "LeftButton":
		bit = clickLeftUp
		if down {
			bit = clickLeftDown
		}
	case "RightButton":
		bit = clickRightUp
		if down {
			bit = clickRightDown
		}
	case "MiddleButton":
		bit = clickMiddleUp
		if down {
			bit = clickMiddleDown
		}
	}
	return mask&bit != 0
}

func (w *widget) dragsWith(button string) bool {
	switch button {
	case "LeftButton":
		return w.dragMask&dragLeft != 0
	case "RightButton":
		return w.dragMask&dragRight != 0
	case "MiddleButton":
		return w.dragMask&dragMiddle != 0
	}
	return false
}

func isClickable(w *widget) bool {
	return w != nil && (w.kind == kindButton || w.kind == kindCheckButton)
}

// PointOverUI reports whether a window position is over a mouse-enabled
// interface frame (anything but the world itself).
func (eng *UIEngine) PointOverUI(x, y float64) bool {
	x, y = eng.worldInputPoint(x, y)
	target := eng.hitTest(x, y)
	return target != nil && target.name != "WorldFrame" && target.name != "UIParent"
}

// fireHoverChange runs OnLeave for the frame the cursor left and OnEnter for
// the one it entered.
func (eng *UIEngine) fireHoverChange(old, target *widget) {
	if eng.Rt == nil {
		return
	}
	if old != nil {
		eng.Rt.fire(old, "OnLeave", []lua.LValue{old.luaValue(eng.Rt.L), lua.LBool(true)})
	}
	if target != nil {
		eng.Rt.fire(target, "OnEnter", []lua.LValue{target.luaValue(eng.Rt.L), lua.LBool(true)})
	}
}

// startDragsIfNeeded begins a drag for a pressed, drag-registered widget once
// the cursor has moved far enough.
func (eng *UIEngine) startDragsIfNeeded(x, y float64) {
	if eng.Rt == nil || eng.dragging != nil {
		return
	}
	for button, press := range eng.presses {
		if press == nil || press.widget == nil || press.dragged || !press.widget.dragsWith(button) {
			continue
		}
		if math.Hypot(x-press.x, y-press.y) < dragThreshold {
			continue
		}
		press.dragged = true
		eng.dragging = &dragState{widget: press.widget, button: button}
		if isClickable(press.widget) {
			press.widget.buttonState = "NORMAL"
		}
		eng.Rt.fire(press.widget, "OnDragStart", []lua.LValue{press.widget.luaValue(eng.Rt.L), lua.LString(button)})
		return
	}
}

func (eng *UIEngine) fireClick(w *widget, button string, down bool) {
	if eng.Rt == nil {
		return
	}
	args := func() []lua.LValue {
		return []lua.LValue{w.luaValue(eng.Rt.L), lua.LString(button), lua.LBool(down)}
	}
	eng.Rt.fire(w, "PreClick", args())
	if w.kind == kindCheckButton {
		w.checked = !w.checked
	}
	eng.Rt.fire(w, "OnClick", args())
	eng.Rt.fire(w, "PostClick", args())
}

// HandleMouse routes a mouse button press or release to the interface and
// reports whether the interface used it.
func (eng *UIEngine) HandleMouse(x, y float64, button window.MouseButton, down bool) bool {
	name := mouseButtonName(button)
	if name == "" {
		return false
	}
	left := name == "LeftButton"
	x, y = eng.worldInputPoint(x, y)
	if eng.Rt != nil {
		eng.Rt.cursorX = x
		eng.Rt.cursorY = y
	}
	if left && eng.debugPanel.handleMouse(x, y, down, eng) {
		return true
	}
	if eng.presses == nil {
		eng.presses = make(map[string]*mousePress)
	}
	target := eng.hitTest(x, y)
	if down {
		if target == nil {
			eng.presses[name] = &mousePress{x: x, y: y}
			if left && eng.Rt != nil {
				eng.Rt.setFocus(nil)
			}
			return false
		}
		eng.presses[name] = &mousePress{widget: target, x: x, y: y}
		if left {
			eng.pressed = target
			switch {
			case target.kind == kindEditBox:
				eng.Rt.setFocus(target)
				eng.setEditCursor(target, x)
				eng.selecting = target
			case target.kind == kindSlider:
				eng.sliderDragging = target
				eng.setSliderValueAt(target, x, y)
			case isClickable(target):
				target.buttonState = "PUSHED"
			}
		} else if isClickable(target) && target.clicksOn(name, false) {
			target.buttonState = "PUSHED"
		}
		if eng.Rt != nil {
			eng.Rt.fire(target, "OnMouseDown", []lua.LValue{target.luaValue(eng.Rt.L), lua.LString(name)})
		}
		if isClickable(target) && target.clicksOn(name, true) {
			eng.fireClick(target, name, true)
		}
		return true
	}
	press := eng.presses[name]
	delete(eng.presses, name)
	if left {
		eng.pressed = nil
		eng.selecting = nil
		if slider := eng.sliderDragging; slider != nil {
			eng.setSliderValueAt(slider, x, y)
			eng.sliderDragging = nil
			if eng.Rt != nil {
				eng.Rt.fire(slider, "OnMouseUp", []lua.LValue{slider.luaValue(eng.Rt.L), lua.LString(name)})
			}
			return true
		}
	}
	if eng.dragging != nil && eng.dragging.button == name {
		dragged := eng.dragging.widget
		eng.dragging = nil
		if eng.Rt != nil {
			eng.Rt.fire(dragged, "OnDragStop", []lua.LValue{dragged.luaValue(eng.Rt.L)})
			if target != nil {
				eng.Rt.fire(target, "OnReceiveDrag", []lua.LValue{target.luaValue(eng.Rt.L)})
			}
		}
		if isClickable(dragged) {
			dragged.buttonState = "NORMAL"
		}
		return true
	}
	if press == nil || press.widget == nil {
		return target != nil
	}
	pressed := press.widget
	if isClickable(pressed) {
		pressed.buttonState = "NORMAL"
	}
	if eng.Rt != nil {
		eng.Rt.fire(pressed, "OnMouseUp", []lua.LValue{pressed.luaValue(eng.Rt.L), lua.LString(name)})
	}
	if pressed == target && isClickable(pressed) && pressed.clicksOn(name, false) {
		eng.fireClick(pressed, name, false)
		if left {
			if eng.lastClick == pressed && time.Since(eng.lastClickAt) <= 500*time.Millisecond {
				if eng.Rt != nil {
					eng.Rt.fire(pressed, "OnDoubleClick", []lua.LValue{pressed.luaValue(eng.Rt.L), lua.LString(name)})
				}
				eng.lastClick = nil
			} else {
				eng.lastClick = pressed
				eng.lastClickAt = time.Now()
			}
		}
		return true
	}
	return pressed == target
}

// fireMouseWheel runs OnMouseWheel on the topmost wheel-enabled frame under
// the cursor and reports whether one handled it.
func (eng *UIEngine) fireMouseWheel(delta float64) bool {
	if eng == nil || eng.Rt == nil || eng.uiScale <= 0 {
		return false
	}
	point := struct{ x, y float64 }{eng.Rt.cursorX / eng.uiScale, (float64(eng.screenHeight) - eng.Rt.cursorY) / eng.uiScale}
	var visit func(w *widget, parent Rect) *widget
	visit = func(w *widget, parent Rect) *widget {
		if !w.shown {
			return nil
		}
		rect := eng.layoutRect(w, parent)
		children := orderedChildren(w.children)
		for i := len(children) - 1; i >= 0; i-- {
			if target := visit(children[i], rect); target != nil {
				return target
			}
		}
		if w.enableMouseWheel && w.scripts["OnMouseWheel"] != nil &&
			point.x >= rect.X0 && point.x <= rect.X1 && point.y >= rect.Y0 && point.y <= rect.Y1 {
			return w
		}
		return nil
	}
	root := eng.worldRoot
	if root == nil {
		return false
	}
	target := visit(root, eng.screen)
	if target == nil {
		return false
	}
	eng.Rt.fire(target, "OnMouseWheel", []lua.LValue{target.luaValue(eng.Rt.L), lua.LNumber(delta)})
	return true
}

// luaStringArgs collects the string arguments from index first on.
func luaStringArgs(L *lua.LState, first int) []string {
	var out []string
	for i := first; i <= L.GetTop(); i++ {
		if s, ok := L.Get(i).(lua.LString); ok {
			out = append(out, string(s))
		}
	}
	return out
}
