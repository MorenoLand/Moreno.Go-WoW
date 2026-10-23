package ui

import lua "github.com/yuin/gopher-lua"

// visibleThroughParents reports whether every ancestor of w is shown, so that
// showing or hiding w itself changes whether it is drawn.
func (w *widget) visibleThroughParents() bool {
	for parent := w.parent; parent != nil; parent = parent.parent {
		if !parent.shown {
			return false
		}
	}
	return true
}

// isScriptableFrame reports widgets that carry OnShow/OnHide scripts; regions
// (textures and font strings) only follow their parent.
func (w *widget) isScriptableFrame() bool {
	return w.kind != kindTexture && w.kind != kindFontString
}

// fireShowCascade runs OnShow for a widget that has just become visible and
// then for every frame below it that is itself shown: showing a frame shows
// the whole visible subtree, and each frame is told so.
func (rt *Runtime) fireShowCascade(w *widget) {
	rt.fireHandler(w, "OnShow")
	for _, child := range append([]*widget(nil), w.children...) {
		if child.shown && child.isScriptableFrame() {
			rt.fireShowCascade(child)
		}
	}
}

// fireHideCascade is the OnHide counterpart of fireShowCascade.
func (rt *Runtime) fireHideCascade(w *widget) {
	rt.fireHandler(w, "OnHide")
	for _, child := range append([]*widget(nil), w.children...) {
		if child.shown && child.isScriptableFrame() {
			rt.fireHideCascade(child)
		}
	}
}

// installWidgetSetmetatable lets scripts use a frame as the metatable of a
// table (setmetatable(cache, frame)): in the original client a frame is a Lua
// table, here it is userdata, so such a metatable becomes a proxy whose
// __index reads through to the frame.
func installWidgetSetmetatable(L *lua.LState) {
	base := L.GetGlobal("setmetatable")
	L.SetGlobal("setmetatable", L.NewFunction(func(L *lua.LState) int {
		if ud, ok := L.Get(2).(*lua.LUserData); ok {
			if w, ok := ud.Value.(*widget); ok {
				if w.metaProxy == nil {
					proxy := L.NewTable()
					frame := ud
					proxy.RawSetString("__index", L.NewFunction(func(L *lua.LState) int {
						L.Push(L.GetTable(frame, L.Get(2)))
						return 1
					}))
					w.metaProxy = proxy
				}
				L.Push(base)
				L.Push(L.Get(1))
				L.Push(w.metaProxy)
				L.Call(2, 1)
				return 1
			}
		}
		count := L.GetTop()
		L.Push(base)
		for i := 1; i <= count; i++ {
			L.Push(L.Get(i))
		}
		L.Call(count, 1)
		return 1
	}))
}

// shiftChildLevels moves the frame level of every frame below w by delta, so a
// frame whose level changes carries its children with it.
func (w *widget) shiftChildLevels(delta int) {
	if delta == 0 {
		return
	}
	for _, child := range w.children {
		if !child.isScriptableFrame() {
			continue
		}
		child.frameLevel += delta
		child.shiftChildLevels(delta)
	}
}

// setFrameLevel sets a frame's level; the frames below it keep their levels
// relative to it.
func (w *widget) setFrameLevel(level int) {
	delta := level - w.frameLevel
	w.frameLevel = level
	w.shiftChildLevels(delta)
}

// unanchoredTopLevelFrame reports a frame directly under a root frame that has
// no anchor points: the original client gives such a frame no position, so it
// is never drawn (for example the dropdown template frames chat creates only
// to anchor menus to).
func (w *widget) unanchoredTopLevelFrame() bool {
	if w == nil || w.parent == nil || len(w.points) != 0 || !w.isScriptableFrame() {
		return false
	}
	switch w.parent.name {
	case "UIParent", "GlueParent", "WorldFrame":
		return w.parent.parent == nil || w.parent.parent.name == ""
	}
	return false
}
