package ui

import (
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// extraWidgetMethods are the frame, region, button and model methods of the
// widget API that the glue scripts never call but FrameXML does. They are added
// to the method table only where registerWidgetMethods has no implementation.
func (rt *Runtime) extraWidgetMethods() map[string]func(L *lua.LState, w *widget) int {
	pushBool := func(L *lua.LState, value bool) int { L.Push(lua.LBool(value)); return 1 }
	pushWidget := func(L *lua.LState, w *widget) int {
		if w == nil {
			L.Push(lua.LNil)
		} else {
			L.Push(w.luaValue(L))
		}
		return 1
	}
	noop := func(L *lua.LState, w *widget) int { return 0 }
	textureGetter := func(pick func(w *widget) *widget) func(L *lua.LState, w *widget) int {
		return func(L *lua.LState, w *widget) int { return pushWidget(L, pick(w)) }
	}
	return map[string]func(L *lua.LState, w *widget) int{
		"GetPushedTexture":          textureGetter(func(w *widget) *widget { return w.pushedTexture }),
		"GetCheckedTexture":         textureGetter(func(w *widget) *widget { return w.checkedTexture }),
		"GetDisabledCheckedTexture": textureGetter(func(w *widget) *widget { return w.disabledCheckedTexture }),
		"SetCheckedTexture": func(L *lua.LState, w *widget) int {
			w.checkedTexture = rt.buttonTextureArg(w, w.checkedTexture, L, 2)
			return 0
		},
		"SetDisabledCheckedTexture": func(L *lua.LState, w *widget) int {
			w.disabledCheckedTexture = rt.buttonTextureArg(w, w.disabledCheckedTexture, L, 2)
			return 0
		},
		"GetNormalFontObject":    func(L *lua.LState, w *widget) int { return rt.pushFontObject(L, w.normalFont) },
		"GetHighlightFontObject": func(L *lua.LState, w *widget) int { return rt.pushFontObject(L, w.highlightFont) },
		"GetDisabledFontObject":  func(L *lua.LState, w *widget) int { return rt.pushFontObject(L, w.disabledFont) },
		"GetPushedTextOffset": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.pushedTextOffsetX))
			L.Push(lua.LNumber(w.pushedTextOffsetY))
			return 2
		},
		"SetPushedTextOffset": func(L *lua.LState, w *widget) int {
			w.pushedTextOffsetX, w.pushedTextOffsetY = float64(L.CheckNumber(2)), float64(L.CheckNumber(3))
			return 0
		},
		"GetTextHeight": func(L *lua.LState, w *widget) int {
			label := w
			if w.buttonLabel != nil {
				label = w.buttonLabel
			}
			if rt.measureText != nil && label.kind == kindFontString {
				rt.measureText(label)
			}
			L.Push(lua.LNumber(label.height))
			return 1
		},
		"GetStringHeight": func(L *lua.LState, w *widget) int {
			if rt.measureText != nil && w.kind == kindFontString {
				rt.measureText(w)
			}
			L.Push(lua.LNumber(w.height))
			return 1
		},
		"SetFontString":     noop,
		"IsEventRegistered": func(L *lua.LState, w *widget) int { return pushBool(L, w.events[L.CheckString(2)]) },
		"UnregisterAllEvents": func(L *lua.LState, w *widget) int {
			for event := range w.events {
				delete(w.events, event)
				kept := rt.events[event][:0]
				for _, registered := range rt.events[event] {
					if registered != w {
						kept = append(kept, registered)
					}
				}
				rt.events[event] = kept
			}
			return 0
		},
		"RegisterAllEvents":   noop,
		"EnableMouseWheel":    func(L *lua.LState, w *widget) int { w.enableMouseWheel = luaTruthyOpt(L, 2, true); return 0 },
		"IsMouseWheelEnabled": func(L *lua.LState, w *widget) int { return pushBool(L, w.enableMouseWheel) },
		"IsMouseEnabled":      func(L *lua.LState, w *widget) int { return pushBool(L, w.enableMouse) },
		"IsKeyboardEnabled":   func(L *lua.LState, w *widget) int { return pushBool(L, w.enableKeyboard) },
		"IsMovable":           func(L *lua.LState, w *widget) int { return pushBool(L, w.movable) },
		"IsResizable":         func(L *lua.LState, w *widget) int { return pushBool(L, w.resizable) },
		"SetResizable":        func(L *lua.LState, w *widget) int { w.resizable = luaTruthyOpt(L, 2, true); return 0 },
		"SetToplevel":         func(L *lua.LState, w *widget) int { w.topLevel = luaTruthyOpt(L, 2, true); return 0 },
		"IsToplevel":          func(L *lua.LState, w *widget) int { return pushBool(L, w.topLevel) },
		"SetClampedToScreen":  func(L *lua.LState, w *widget) int { w.clampedToScreen = luaTruthyOpt(L, 2, true); return 0 },
		"IsClampedToScreen":   func(L *lua.LState, w *widget) int { return pushBool(L, w.clampedToScreen) },
		"GetScale":            func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.scale)); return 1 },
		"GetEffectiveScale":   func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(rt.effectiveScale(w))); return 1 },
		"GetEffectiveAlpha": func(L *lua.LState, w *widget) int {
			alpha := 1.0
			for current := w; current != nil; current = current.parent {
				alpha *= current.alpha
			}
			L.Push(lua.LNumber(alpha))
			return 1
		},
		"GetNumChildren": func(L *lua.LState, w *widget) int {
			count := 0
			for _, child := range w.children {
				if child.isScriptableFrame() {
					count++
				}
			}
			L.Push(lua.LNumber(count))
			return 1
		},
		"GetChildren": func(L *lua.LState, w *widget) int {
			count := 0
			for _, child := range w.children {
				if child.isScriptableFrame() {
					L.Push(child.luaValue(L))
					count++
				}
			}
			return count
		},
		"GetNumRegions": func(L *lua.LState, w *widget) int {
			count := 0
			for _, child := range w.children {
				if !child.isScriptableFrame() {
					count++
				}
			}
			L.Push(lua.LNumber(count))
			return 1
		},
		"GetRegions": func(L *lua.LState, w *widget) int {
			count := 0
			for _, child := range w.children {
				if !child.isScriptableFrame() {
					L.Push(child.luaValue(L))
					count++
				}
			}
			return count
		},
		"GetHitRectInsets": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.hitInsetL))
			L.Push(lua.LNumber(w.hitInsetR))
			L.Push(lua.LNumber(w.hitInsetT))
			L.Push(lua.LNumber(w.hitInsetB))
			return 4
		},
		"GetMinResize": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.minResizeW))
			L.Push(lua.LNumber(w.minResizeH))
			return 2
		},
		"SetMinResize": func(L *lua.LState, w *widget) int {
			w.minResizeW, w.minResizeH = float64(L.CheckNumber(2)), float64(L.CheckNumber(3))
			return 0
		},
		"GetMaxResize": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.maxResizeW))
			L.Push(lua.LNumber(w.maxResizeH))
			return 2
		},
		"SetMaxResize": func(L *lua.LState, w *widget) int {
			w.maxResizeW, w.maxResizeH = float64(L.CheckNumber(2)), float64(L.CheckNumber(3))
			return 0
		},
		"GetClampRectInsets": func(L *lua.LState, w *widget) int {
			for range 4 {
				L.Push(lua.LNumber(0))
			}
			return 4
		},
		"Lower": func(L *lua.LState, w *widget) int {
			if w.parent != nil {
				level := w.frameLevel
				for _, sibling := range w.parent.children {
					if sibling != w && sibling.frameLevel <= level {
						level = sibling.frameLevel - 1
					}
				}
				if level < 0 {
					level = 0
				}
				w.setFrameLevel(level)
			}
			return 0
		},
		"IgnoreDepth":       func(L *lua.LState, w *widget) int { w.ignoreDepth = luaTruthyOpt(L, 2, true); return 0 },
		"IsIgnoringDepth":   func(L *lua.LState, w *widget) int { return pushBool(L, w.ignoreDepth) },
		"SetDepth":          func(L *lua.LState, w *widget) int { w.depth = float64(L.CheckNumber(2)); return 0 },
		"GetDepth":          func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.depth)); return 1 },
		"GetEffectiveDepth": func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.depth)); return 1 },
		"IsFrameType":       func(L *lua.LState, w *widget) int { return pushBool(L, w.objectTypeMatches(L.CheckString(2))) },
		"IsProtected": func(L *lua.LState, w *widget) int {
			L.Push(lua.LFalse)
			L.Push(lua.LFalse)
			return 2
		},
		"CanChangeProtectedState": func(L *lua.LState, w *widget) int { return pushBool(L, true) },
		"IsDragging":              func(L *lua.LState, w *widget) int { return pushBool(L, false) },
		"IsMouseOver":             func(L *lua.LState, w *widget) int { return pushBool(L, false) },
		"HasScript": func(L *lua.LState, w *widget) int {
			_, ok := w.scripts[L.CheckString(2)]
			return pushBool(L, ok)
		},
		"StartSizing":       noop,
		"EnableDrawLayer":   noop,
		"DisableDrawLayer":  noop,
		"CreateTitleRegion": func(L *lua.LState, w *widget) int { return pushWidget(L, newWidget(kindFrame, "")) },
		"GetTitleRegion":    func(L *lua.LState, w *widget) int { return pushWidget(L, nil) },
		"EnableJoystick":    noop,
		"GetBackdropColor": func(L *lua.LState, w *widget) int {
			color := rgba{r: 1, g: 1, b: 1, a: 1}
			if w.backdrop != nil {
				color = w.backdrop.bgColor
			}
			L.Push(lua.LNumber(color.r))
			L.Push(lua.LNumber(color.g))
			L.Push(lua.LNumber(color.b))
			L.Push(lua.LNumber(color.a))
			return 4
		},
		"GetBackdropBorderColor": func(L *lua.LState, w *widget) int {
			color := rgba{r: 1, g: 1, b: 1, a: 1}
			if w.backdrop != nil {
				color = w.backdrop.edgeColor
			}
			L.Push(lua.LNumber(color.r))
			L.Push(lua.LNumber(color.g))
			L.Push(lua.LNumber(color.b))
			L.Push(lua.LNumber(color.a))
			return 4
		},
		"GetBackdrop": func(L *lua.LState, w *widget) int {
			if w.backdrop == nil {
				L.Push(lua.LNil)
				return 1
			}
			table := L.NewTable()
			table.RawSetString("bgFile", lua.LString(w.backdrop.bgFile))
			table.RawSetString("edgeFile", lua.LString(w.backdrop.edgeFile))
			table.RawSetString("tile", lua.LBool(w.backdrop.tile))
			table.RawSetString("tileSize", lua.LNumber(w.backdrop.tileSize))
			table.RawSetString("edgeSize", lua.LNumber(w.backdrop.edgeSize))
			insets := L.NewTable()
			insets.RawSetString("left", lua.LNumber(w.backdrop.insetL))
			insets.RawSetString("right", lua.LNumber(w.backdrop.insetR))
			insets.RawSetString("top", lua.LNumber(w.backdrop.insetT))
			insets.RawSetString("bottom", lua.LNumber(w.backdrop.insetB))
			table.RawSetString("insets", insets)
			L.Push(table)
			return 1
		},
		"GetJustifyH": func(L *lua.LState, w *widget) int { L.Push(lua.LString(orDefault(w.justifyH, "CENTER"))); return 1 },
		"GetJustifyV": func(L *lua.LState, w *widget) int { L.Push(lua.LString(orDefault(w.justifyV, "MIDDLE"))); return 1 },
		"GetSpacing":  func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.messageSpacing)); return 1 },
		"GetShadowOffset": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.shadowOffsetX))
			L.Push(lua.LNumber(w.shadowOffsetY))
			return 2
		},
		"GetShadowColor": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.shadowColor.r))
			L.Push(lua.LNumber(w.shadowColor.g))
			L.Push(lua.LNumber(w.shadowColor.b))
			L.Push(lua.LNumber(w.shadowColor.a))
			return 4
		},
		"GetVertexColor": func(L *lua.LState, w *widget) int {
			color := w.vertexColor
			if color.isZero() {
				color = rgba{r: 1, g: 1, b: 1, a: 1}
			}
			L.Push(lua.LNumber(color.r))
			L.Push(lua.LNumber(color.g))
			L.Push(lua.LNumber(color.b))
			L.Push(lua.LNumber(color.a))
			return 4
		},
		"GetBlendMode": func(L *lua.LState, w *widget) int {
			L.Push(lua.LString(orDefault(w.blendMode, orDefault(w.alphaMode, "BLEND"))))
			return 1
		},
		"GetDrawLayer":     func(L *lua.LState, w *widget) int { L.Push(lua.LString(drawLayerName(w.layerLevel))); return 1 },
		"IsDesaturated":    func(L *lua.LState, w *widget) int { return pushBool(L, w.desaturated) },
		"SetGradient":      noop,
		"SetGradientAlpha": noop,
		"SetRotation":      func(L *lua.LState, w *widget) int { w.rotation = float64(L.CheckNumber(2)); return 0 },
		"GetRotation":      func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.rotation)); return 1 },
		"SetAlphaGradient": noop,
		"SetTextHeight":    noop,
		"GetNumber":        func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(parseLuaNumber(w.text))); return 1 },
		"SetNumber": func(L *lua.LState, w *widget) int {
			rt.setText(w, lua.LNumber(L.CheckNumber(2)).String())
			return 0
		},
		"HasFocus":          func(L *lua.LState, w *widget) int { return pushBool(L, rt.focused == w) },
		"GetNumLetters":     func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(len([]rune(w.text)))); return 1 },
		"GetCursorPosition": func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.cursor)); return 1 },
		"SetCursorPosition": func(L *lua.LState, w *widget) int {
			w.cursor = clampInt(L.CheckInt(2), 0, len([]rune(w.text)))
			w.selectionStart, w.selectionEnd, w.selectionAnchor = w.cursor, w.cursor, w.cursor
			return 0
		},
		"IsMultiLine":        func(L *lua.LState, w *widget) int { return pushBool(L, w.multiLine) },
		"IsNumeric":          func(L *lua.LState, w *widget) int { return pushBool(L, w.numeric) },
		"IsPassword":         func(L *lua.LState, w *widget) int { return pushBool(L, w.password) },
		"SetPassword":        func(L *lua.LState, w *widget) int { w.password = luaTruthyOpt(L, 2, true); return 0 },
		"IsAutoFocus":        func(L *lua.LState, w *widget) int { return pushBool(L, w.autoFocus) },
		"GetMaxLetters":      func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.maxLetters)); return 1 },
		"GetMaxBytes":        func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.maxBytes)); return 1 },
		"GetHistoryLines":    func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.historyLines)); return 1 },
		"SetHistoryLines":    func(L *lua.LState, w *widget) int { w.historyLines = L.CheckInt(2); return 0 },
		"SetAltArrowKeyMode": noop,
		"GetAltArrowKeyMode": func(L *lua.LState, w *widget) int { return pushBool(L, false) },
		"SetBlinkSpeed": func(L *lua.LState, w *widget) int {
			w.blinkSpeed, w.blinkSet = float64(L.CheckNumber(2)), true
			return 0
		},
		"GetBlinkSpeed": func(L *lua.LState, w *widget) int {
			if w.blinkSet {
				L.Push(lua.LNumber(w.blinkSpeed))
			} else {
				L.Push(lua.LNumber(0.5))
			}
			return 1
		},
		"GetInputLanguage":    func(L *lua.LState, w *widget) int { L.Push(lua.LString("ROMAN")); return 1 },
		"ToggleInputLanguage": noop,
		"Insert": func(L *lua.LState, w *widget) int {
			runes := []rune(w.text)
			insert := []rune(L.CheckString(2))
			position := clampInt(w.cursor, 0, len(runes))
			updated := append(append(append([]rune{}, runes[:position]...), insert...), runes[position:]...)
			rt.setText(w, string(updated))
			w.cursor = position + len(insert)
			w.selectionStart, w.selectionEnd, w.selectionAnchor = w.cursor, w.cursor, w.cursor
			return 0
		},
		"GetOrientation":           func(L *lua.LState, w *widget) int { L.Push(lua.LString(w.orientation)); return 1 },
		"GetValueStep":             func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.valueStep)); return 1 },
		"GetThumbTexture":          textureGetter(func(w *widget) *widget { return w.thumbTexture }),
		"GetHorizontalScrollRange": func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.horizontalRange)); return 1 },
		"AtTop":                    func(L *lua.LState, w *widget) int { return pushBool(L, w.messageOffset >= len(w.messages)-1) },
		"PageUp": func(L *lua.LState, w *widget) int {
			w.messageOffset = clampInt(w.messageOffset+5, 0, maxInt(len(w.messages)-1, 0))
			return 0
		},
		"PageDown": func(L *lua.LState, w *widget) int {
			w.messageOffset = clampInt(w.messageOffset-5, 0, maxInt(len(w.messages)-1, 0))
			return 0
		},
		"GetFading":       func(L *lua.LState, w *widget) int { return pushBool(L, w.messageFading) },
		"GetFadeDuration": func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(3)); return 1 },
		"SetFadeDuration": noop,
		"GetInsertMode": func(L *lua.LState, w *widget) int {
			L.Push(lua.LString(orDefault(w.messageInsert, "BOTTOM")))
			return 1
		},
		"GetNumLinesDisplayed": func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(len(w.messages))); return 1 },
		"UpdateColorByID":      noop,
		"SetCooldown":          noop,
		"SetDrawEdge":          noop,
		"SetReverse":           noop,
		"GetCooldownDuration":  func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(0)); return 1 },
		"GetColorRGB": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(1))
			L.Push(lua.LNumber(1))
			L.Push(lua.LNumber(1))
			return 3
		},
		"SetColorRGB": noop,
		"GetColorHSV": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(1))
			return 3
		},
		"SetColorHSV":          noop,
		"SetColorValueTexture": noop, "SetColorValueThumbTexture": noop, "SetColorWheelTexture": noop, "SetColorWheelThumbTexture": noop,
		"SetUnit":            noop,
		"SetCreature":        noop,
		"RefreshUnit":        noop,
		"Undress":            noop,
		"TryOn":              noop,
		"SetCustomRace":      noop,
		"RefreshCamera":      noop,
		"ReplaceIconTexture": noop,
		"TransformCameraSpaceToModelSpace": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(0))
			return 3
		},
		"SetFillTexture":    noop,
		"SetFillAlpha":      noop,
		"SetBorderAlpha":    noop,
		"EnableSmoothing":   noop,
		"SetBorderTexture":  noop,
		"SetBorderScalar":   noop,
		"DrawBlob":          noop,
		"DrawNone":          noop,
		"EnableMerging":     noop,
		"SetMergeThreshold": noop,
		"SetZoom": func(L *lua.LState, w *widget) int {
			// Wow.exe keeps the level in 0..5; anything above 4 is 5.
			level := L.CheckInt(2)
			if level < 0 {
				level = 0
			}
			if level > 4 {
				level = 5
			}
			w.zoom, w.zoomSet = level, true
			if rt.onMinimapZoom != nil {
				rt.onMinimapZoom()
			}
			return 0
		},
		"GetZoom":             func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.zoomLevel())); return 1 },
		"SetHorizTile":        func(L *lua.LState, w *widget) int { w.horizTile = luaTruthy(L, 2); return 0 },
		"SetVertTile":         func(L *lua.LState, w *widget) int { w.vertTile = luaTruthy(L, 2); return 0 },
		"SetFrameRef":         noop,
		"Execute":             noop,
		"WrapScript":          noop,
		"UnwrapScript":        noop,
		"GetZoomLevels":       func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(minimapZoomLevels)); return 1 },
		"GetPingPosition":     noop,
		"PingLocation":        noop,
		"SetMaskTexture":      noop,
		"SetBlipTexture":      noop,
		"SetIconTexture":      noop,
		"SetPlayerModel":      noop,
		"SetArrowModel":       noop,
		"SetClassBlipTexture": noop, "SetCorpsePOIArrowTexture": noop, "SetPOIArrowTexture": noop, "SetStaticPOIArrowTexture": noop,
		"SetPlayerTexture": noop,
		"CreateAnimationGroup": func(L *lua.LState, w *widget) int {
			return pushWidget(L, rt.newAnimationGroup(w, L.OptString(2, ""), L.OptString(3, "")))
		},
		"GetAnimationGroups": noop,
	}
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func drawLayerName(level int) string {
	switch level {
	case layerBackground:
		return "BACKGROUND"
	case layerBorder:
		return "BORDER"
	case layerOverlay:
		return "OVERLAY"
	case layerHighlight:
		return "HIGHLIGHT"
	}
	return "ARTWORK"
}

func parseLuaNumber(text string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return 0
	}
	return value
}

// pushFontObject pushes the global font object named name, or nil.
func (rt *Runtime) pushFontObject(L *lua.LState, name string) int {
	if name == "" {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(L.GetGlobal(name))
	return 1
}

// effectiveScale is the widget's scale multiplied through its ancestors and the
// UI scale: the factor between UI units and screen pixels.
func (rt *Runtime) effectiveScale(w *widget) float64 {
	scale := effectiveScaleOf(w)
	if rt.Host != nil {
		if _, height := rt.Host.ScreenSize(); height > 0 {
			scale *= height / 768
		}
	}
	return scale
}

// ownRect is a widget's rectangle in its own units: the on-demand layout in
// root units divided by the widget's effective scale. ok is false when the
// widget has no position yet.
func (rt *Runtime) ownRect(w *widget) (Rect, bool) {
	if rt.layoutOf == nil {
		if w.hasRenderRect {
			return w.renderRect, true
		}
		return Rect{}, false
	}
	rect, ok := rt.layoutOf(w)
	if !ok {
		return Rect{}, false
	}
	scale := effectiveScaleOf(w)
	if scale <= 0 {
		scale = 1
	}
	return Rect{X0: rect.X0 / scale, Y0: rect.Y0 / scale, X1: rect.X1 / scale, Y1: rect.Y1 / scale}, true
}

// coordinateMethods are GetLeft, GetCenter and friends: the widget's
// position in its own units, laid out on demand.
func (rt *Runtime) coordinateMethods() map[string]func(L *lua.LState, w *widget) int {
	edge := func(pick func(Rect) float64) func(L *lua.LState, w *widget) int {
		return func(L *lua.LState, w *widget) int {
			rect, ok := rt.ownRect(w)
			if !ok {
				L.Push(lua.LNil)
				return 1
			}
			L.Push(lua.LNumber(pick(rect)))
			return 1
		}
	}
	return map[string]func(L *lua.LState, w *widget) int{
		"GetLeft":   edge(func(r Rect) float64 { return r.X0 }),
		"GetRight":  edge(func(r Rect) float64 { return r.X1 }),
		"GetTop":    edge(func(r Rect) float64 { return r.Y1 }),
		"GetBottom": edge(func(r Rect) float64 { return r.Y0 }),
		"GetCenter": func(L *lua.LState, w *widget) int {
			rect, ok := rt.ownRect(w)
			if !ok {
				L.Push(lua.LNil)
				return 1
			}
			L.Push(lua.LNumber((rect.X0 + rect.X1) / 2))
			L.Push(lua.LNumber((rect.Y0 + rect.Y1) / 2))
			return 2
		},
		"GetRect": func(L *lua.LState, w *widget) int {
			rect, ok := rt.ownRect(w)
			if !ok {
				return 0
			}
			L.Push(lua.LNumber(rect.X0))
			L.Push(lua.LNumber(rect.Y0))
			L.Push(lua.LNumber(rect.W()))
			L.Push(lua.LNumber(rect.H()))
			return 4
		},
		"GetBoundsRect": func(L *lua.LState, w *widget) int {
			rect, ok := rt.ownRect(w)
			if !ok {
				return 0
			}
			L.Push(lua.LNumber(rect.X0))
			L.Push(lua.LNumber(rect.Y0))
			L.Push(lua.LNumber(rect.W()))
			L.Push(lua.LNumber(rect.H()))
			return 4
		},
	}
}
