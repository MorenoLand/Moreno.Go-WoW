package ui

import (
	"fmt"
	"math"

	lua "github.com/yuin/gopher-lua"
)

// widgetKind enumerates the region and frame types the glue XML defines.
type widgetKind uint8

const (
	kindFrame widgetKind = iota
	kindButton
	kindCheckButton
	kindEditBox
	kindSlider
	kindScrollFrame
	kindSimpleHTML
	kindModel
	kindModelFFX
	kindMovieFrame
	kindTexture
	kindFontString
)

const (
	layerBackground = iota
	layerBorder
	layerArtwork
	layerOverlay
	layerHighlight
)

func (k widgetKind) objectType() string {
	switch k {
	case kindFrame:
		return "Frame"
	case kindButton:
		return "Button"
	case kindCheckButton:
		return "CheckButton"
	case kindEditBox:
		return "EditBox"
	case kindSlider:
		return "Slider"
	case kindScrollFrame:
		return "ScrollFrame"
	case kindSimpleHTML:
		return "SimpleHTML"
	case kindModel:
		return "Model"
	case kindModelFFX:
		return "ModelFFX"
	case kindMovieFrame:
		return "MovieFrame"
	case kindTexture:
		return "Texture"
	case kindFontString:
		return "FontString"
	}
	return "Region"
}

type rgba struct{ r, g, b, a float64 }

func (c rgba) isZero() bool { return c.r == 0 && c.g == 0 && c.b == 0 && c.a == 0 }

type anchorPoint struct {
	point, relativeTo, relativePoint string
	x, y                             float64
}

type backdrop struct {
	bgFile, edgeFile               string
	tile                           bool
	tileSize, edgeSize             float64
	insetL, insetR, insetT, insetB float64
	bgColor, edgeColor             rgba
}

// widget holds the state of one frame or region created from XML or the API.
// The original client models these as C++ frame classes; this struct keeps
// the fields the glue scripts observe through their methods.
type widget struct {
	kind            widgetKind
	name            string
	id              int
	parent          *widget
	children        []*widget
	shown           bool
	topLevel        bool
	movable         bool
	enableMouse     bool
	enableKeyboard  bool
	clampedToScreen bool
	frameLevel      int
	layerLevel      int
	renderRect      Rect
	hasRenderRect   bool
	width           float64
	height          float64
	explicitWidth   bool
	explicitHeight  bool
	scale           float64
	alpha           float64
	points          []anchorPoint
	scripts         map[string]*lua.LFunction
	events          map[string]bool
	backdrop        *backdrop

	// Button and CheckButton state.
	buttonState            string
	desaturated            bool
	highlighted            bool
	enabled                bool
	normalTexture          *widget
	pushedTexture          *widget
	highlightTexture       *widget
	disabledTexture        *widget
	checkedTexture         *widget
	disabledCheckedTexture *widget
	buttonLabel            *widget
	normalFont             string
	highlightFont          string
	disabledFont           string
	checked                bool

	// EditBox state.
	text            string
	cursor          int
	selectionStart  int
	selectionEnd    int
	selectionAnchor int
	password        bool
	autoFocus       bool
	maxLetters      int
	maxBytes        int
	historyLines    int

	// Slider state.
	minValue, maxValue float64
	value              float64
	valueStep          float64
	orientation        string

	// ScrollFrame state.
	verticalScroll float64

	// Model state.
	modelFile       string
	sequence        int
	sequenceTime    int
	camera          int
	modelScale      float64
	fogNear, fogFar float64
	hasFog          bool

	// MovieFrame state.
	subtitles   bool
	movieFile   string
	movieActive bool
	movieVolume int

	// Texture state.
	textureFile                                string
	texCoordL, texCoordR, texCoordT, texCoordB float64
	vertexColor                                rgba
	alphaMode                                  string

	// FontString state.
	fontObject      string
	justifyH        string
	justifyV        string
	textColor       rgba
	textWidth       float64
	autoTextWidth   bool
	autoTextHeight  bool
	nonSpaceWrap    bool
	maxLines        int
	textInsetL      float64
	textInsetR      float64
	textInsetT      float64
	textInsetB      float64
	shadowColor     rgba
	shadowColorSet  bool
	shadowOffsetX   float64
	shadowOffsetY   float64
	shadowOffsetSet bool

	// Tooltip/HTML text lines.
	lines []string

	// fields holds script-assigned member values; frames in the original
	// client accept arbitrary key assignment.
	fields *lua.LTable

	luaObj *lua.LUserData
}

func (w *widget) ensureFields(L *lua.LState) *lua.LTable {
	if w.fields == nil {
		w.fields = L.NewTable()
	}
	return w.fields
}

func newWidget(kind widgetKind, name string) *widget {
	return &widget{
		kind:        kind,
		name:        name,
		shown:       true,
		scale:       1,
		alpha:       1,
		buttonState: "NORMAL",
		enabled:     true,
		layerLevel:  layerArtwork,
		orientation: "VERTICAL",
		scripts:     make(map[string]*lua.LFunction),
		events:      make(map[string]bool),
		texCoordL:   0, texCoordR: 1, texCoordT: 0, texCoordB: 1,
	}
}

func (w *widget) objectType() string { return w.kind.objectType() }

func (w *widget) parentName() string {
	if w == nil || w.parent == nil {
		return ""
	}
	return w.parent.name
}

func (w *widget) objectTypeMatches(name string) bool {
	if name == w.objectType() {
		return true
	}
	// Buttons count as frames for inheritance checks, models likewise.
	switch name {
	case "Frame":
		return w.kind >= kindFrame
	case "Button":
		return w.kind == kindButton || w.kind == kindCheckButton
	case "Model":
		return w.kind == kindModel || w.kind == kindModelFFX
	}
	return name == w.objectType()
}

// findDescendant locates a named widget anywhere below w, depth first.
func (w *widget) findDescendant(name string) *widget {
	for _, c := range w.children {
		if c.name == name {
			return c
		}
		if found := c.findDescendant(name); found != nil {
			return found
		}
	}
	return nil
}

// registerWidgetMethods installs the widget method table used by all frame
// and region userdata objects. Method coverage follows the calls the glue
// scripts make; unimplemented interactions are absent rather than guessed.
func registerWidgetMethods(L *lua.LState, rt *Runtime) {
	methods := map[string]func(L *lua.LState, w *widget) int{
		"GetName": func(L *lua.LState, w *widget) int {
			L.Push(lua.LString(w.name))
			return 1
		},
		"GetObjectType": func(L *lua.LState, w *widget) int {
			L.Push(lua.LString(w.objectType()))
			return 1
		},
		"IsObjectType": func(L *lua.LState, w *widget) int {
			L.Push(lua.LBool(w.objectTypeMatches(L.CheckString(2))))
			return 1
		},
		"GetParent": func(L *lua.LState, w *widget) int {
			if w.parent == nil {
				L.Push(lua.LNil)
				return 1
			}
			L.Push(w.parent.luaValue(L))
			return 1
		},
		"SetParent": func(L *lua.LState, w *widget) int {
			ud := L.CheckUserData(2)
			if p, ok := ud.Value.(*widget); ok {
				w.parent = p
			}
			return 0
		},
		"GetID": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.id))
			return 1
		},
		"SetID": func(L *lua.LState, w *widget) int {
			w.id = L.CheckInt(2)
			return 0
		},
		"Show": func(L *lua.LState, w *widget) int {
			if w.shown {
				return 0
			}
			w.shown = true
			rt.fireHandler(w, "OnShow")
			return 0
		},
		"Hide": func(L *lua.LState, w *widget) int {
			if !w.shown {
				return 0
			}
			w.shown = false
			rt.fireHandler(w, "OnHide")
			return 0
		},
		"IsShown": func(L *lua.LState, w *widget) int {
			L.Push(lua.LBool(w.shown))
			return 1
		},
		"IsVisible": func(L *lua.LState, w *widget) int {
			visible := w.shown
			for p := w.parent; p != nil && visible; p = p.parent {
				visible = p.shown
			}
			L.Push(lua.LBool(visible))
			return 1
		},
		"SetWidth": func(L *lua.LState, w *widget) int {
			w.width = float64(L.CheckNumber(2))
			return 0
		},
		"SetHeight": func(L *lua.LState, w *widget) int {
			w.height = float64(L.CheckNumber(2))
			return 0
		},
		"GetWidth": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.width))
			return 1
		},
		"GetHeight": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.height))
			return 1
		},
		"GetTop": func(L *lua.LState, w *widget) int {
			if w.hasRenderRect {
				L.Push(lua.LNumber(w.renderRect.Y1))
			} else {
				L.Push(lua.LNumber(w.height))
			}
			return 1
		},
		"GetBottom": func(L *lua.LState, w *widget) int {
			if w.hasRenderRect {
				L.Push(lua.LNumber(w.renderRect.Y0))
			} else {
				L.Push(lua.LNumber(0))
			}
			return 1
		},
		"GetLeft": func(L *lua.LState, w *widget) int {
			if w.hasRenderRect {
				L.Push(lua.LNumber(w.renderRect.X0))
			} else {
				L.Push(lua.LNumber(0))
			}
			return 1
		},
		"GetRight": func(L *lua.LState, w *widget) int {
			if w.hasRenderRect {
				L.Push(lua.LNumber(w.renderRect.X1))
			} else {
				L.Push(lua.LNumber(w.width))
			}
			return 1
		},
		"GetAlpha": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.alpha))
			return 1
		},
		"SetAlpha": func(L *lua.LState, w *widget) int {
			w.alpha = float64(L.CheckNumber(2))
			return 0
		},
		"SetScale": func(L *lua.LState, w *widget) int {
			w.scale = float64(L.CheckNumber(2))
			return 0
		},
		"SetFrameLevel": func(L *lua.LState, w *widget) int {
			w.frameLevel = L.CheckInt(2)
			return 0
		},
		"GetFrameLevel": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.frameLevel))
			return 1
		},
		"Raise": func(L *lua.LState, w *widget) int { return 0 },
		"EnableKeyboard": func(L *lua.LState, w *widget) int {
			w.enableKeyboard = L.CheckBool(2)
			return 0
		},
		"EnableMouse": func(L *lua.LState, w *widget) int {
			w.enableMouse = L.CheckBool(2)
			return 0
		},
		"RegisterForClicks": func(L *lua.LState, w *widget) int { return 0 },
		"SetScript": func(L *lua.LState, w *widget) int {
			handler := L.CheckString(2)
			fn := L.CheckFunction(3)
			w.scripts[handler] = fn
			return 0
		},
		"GetScript": func(L *lua.LState, w *widget) int {
			if fn, ok := w.scripts[L.CheckString(2)]; ok {
				L.Push(fn)
				return 1
			}
			L.Push(lua.LNil)
			return 1
		},
		"HookScript": func(L *lua.LState, w *widget) int {
			handler := L.CheckString(2)
			hook := L.CheckFunction(3)
			orig := w.scripts[handler]
			// The original handler runs first, then the hook, with the same
			// arguments, matching script hooking in the original client.
			composite := L.NewFunction(func(L *lua.LState) int {
				nargs := L.GetTop()
				if orig != nil {
					L.Push(orig)
					for i := 1; i <= nargs; i++ {
						L.Push(L.Get(i))
					}
					if err := L.PCall(nargs, 0, nil); err != nil {
						rt.recordScriptError(w.name+"/"+handler, err.Error())
					}
				}
				L.Push(hook)
				for i := 1; i <= nargs; i++ {
					L.Push(L.Get(i))
				}
				if err := L.PCall(nargs, 0, nil); err != nil {
					rt.recordScriptError(w.name+"/"+handler+"/hook", err.Error())
				}
				return 0
			})
			w.scripts[handler] = composite
			return 0
		},
		"RegisterEvent": func(L *lua.LState, w *widget) int {
			rt.registerEventWidget(L.CheckString(2), w)
			return 0
		},
		"UnregisterEvent": func(L *lua.LState, w *widget) int {
			rt.unregisterEventWidget(L.CheckString(2), w)
			return 0
		},
		"ClearAllPoints": func(L *lua.LState, w *widget) int {
			w.points = nil
			return 0
		},
		"SetPoint": func(L *lua.LState, w *widget) int {
			n := L.GetTop()
			p := anchorPoint{point: L.CheckString(2)}
			switch {
			case n >= 6 && L.Get(6).Type() == lua.LTNumber:
				// point, relativeTo, relativePoint, x, y
				switch v := L.Get(3).(type) {
				case *lua.LUserData:
					if rel, ok := v.Value.(*widget); ok {
						p.relativeTo = rel.name
					}
				case lua.LString:
					p.relativeTo = resolveParentName(v.String(), w.parentName())
				}
				p.relativePoint = L.CheckString(4)
				p.x = float64(L.CheckNumber(5))
				p.y = float64(L.CheckNumber(6))
			case n >= 5 && L.Get(5).Type() == lua.LTNumber:
				// point, relativeTo, x, y (relativePoint defaults to point)
				switch v := L.Get(3).(type) {
				case *lua.LUserData:
					if rel, ok := v.Value.(*widget); ok {
						p.relativeTo = rel.name
					}
				case lua.LString:
					p.relativeTo = resolveParentName(v.String(), w.parentName())
				}
				p.relativePoint = p.point
				p.x = float64(L.CheckNumber(4))
				p.y = float64(L.CheckNumber(5))
			case n == 4 && (L.Get(3).Type() == lua.LTUserData || L.Get(3).Type() == lua.LTString):
				switch v := L.Get(3).(type) {
				case *lua.LUserData:
					if rel, ok := v.Value.(*widget); ok {
						p.relativeTo = rel.name
					}
				case lua.LString:
					p.relativeTo = resolveParentName(v.String(), w.parentName())
				}
				p.relativePoint = L.CheckString(4)
			case n == 4 && L.Get(3).Type() == lua.LTNumber:
				p.x = float64(L.CheckNumber(3))
				p.y = float64(L.CheckNumber(4))
			}
			for index, existing := range w.points {
				if existing.point == p.point {
					w.points[index] = p
					return 0
				}
			}
			w.points = append(w.points, p)
			return 0
		},
		"SetAllPoints": func(L *lua.LState, w *widget) int {
			w.points = []anchorPoint{{point: "TOPLEFT", relativePoint: "TOPLEFT"}, {point: "BOTTOMRIGHT", relativePoint: "BOTTOMRIGHT"}}
			return 0
		},
		"GetPoint": func(L *lua.LState, w *widget) int {
			idx := 1
			if L.GetTop() >= 1 {
				idx = L.CheckInt(2)
			}
			if idx < 1 || idx > len(w.points) {
				L.Push(lua.LNil)
				return 1
			}
			p := w.points[idx-1]
			L.Push(lua.LString(p.point))
			if p.relativeTo != "" {
				if rel := rt.lookup(p.relativeTo); rel != nil {
					L.Push(rel.luaValue(L))
				} else {
					L.Push(lua.LNil)
				}
			} else if w.parent != nil {
				L.Push(w.parent.luaValue(L))
			} else {
				L.Push(lua.LNil)
			}
			L.Push(lua.LString(p.relativePoint))
			L.Push(lua.LNumber(p.x))
			L.Push(lua.LNumber(p.y))
			return 5
		},
		"GetCenter": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.width / 2))
			L.Push(lua.LNumber(w.height / 2))
			return 2
		},
		"GetBoundsRect": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(w.width))
			L.Push(lua.LNumber(w.height))
			return 4
		},
		"Enable": func(L *lua.LState, w *widget) int {
			w.enabled = true
			w.buttonState = "NORMAL"
			return 0
		},
		"Disable": func(L *lua.LState, w *widget) int {
			w.enabled = false
			w.buttonState = "DISABLED"
			return 0
		},
		"IsEnabled": func(L *lua.LState, w *widget) int {
			L.Push(lua.LBool(w.enabled))
			return 1
		},
		"GetButtonState": func(L *lua.LState, w *widget) int {
			L.Push(lua.LString(w.buttonState))
			return 1
		},
		"SetButtonState": func(L *lua.LState, w *widget) int {
			w.buttonState = L.CheckString(2)
			return 0
		},
		"LockHighlight": func(L *lua.LState, w *widget) int {
			w.highlighted = true
			return 0
		},
		"UnlockHighlight": func(L *lua.LState, w *widget) int {
			w.highlighted = false
			return 0
		},
		"Click": func(L *lua.LState, w *widget) int {
			rt.fire(w, "OnClick", []lua.LValue{w.luaValue(L), lua.LString("LeftButton"), lua.LBool(false)})
			return 0
		},
		"SetChecked": func(L *lua.LState, w *widget) int {
			value := L.Get(2)
			switch value.Type() {
			case lua.LTBool:
				w.checked = L.CheckBool(2)
			case lua.LTNumber:
				w.checked = L.CheckNumber(2) != 0
			case lua.LTString:
				text := L.CheckString(2)
				w.checked = text != "" && text != "0"
			default:
				w.checked = false
			}
			return 0
		},
		"GetChecked": func(L *lua.LState, w *widget) int {
			L.Push(lua.LBool(w.checked))
			return 1
		},
		"SetText": func(L *lua.LState, w *widget) int {
			rt.setText(w, L.CheckString(2))
			return 0
		},
		"SetFormattedText": func(L *lua.LState, w *widget) int {
			format := L.CheckString(2)
			args := make([]interface{}, 0, L.GetTop()-1)
			for i := 3; i <= L.GetTop(); i++ {
				value := L.Get(i)
				if number, ok := value.(lua.LNumber); ok {
					if math.Trunc(float64(number)) == float64(number) {
						args = append(args, int64(number))
					} else {
						args = append(args, float64(number))
					}
				} else {
					args = append(args, value.String())
				}
			}
			rt.setText(w, sprintf(format, args))
			return 0
		},
		"GetText": func(L *lua.LState, w *widget) int {
			L.Push(lua.LString(w.text))
			return 1
		},
		"SetTextColor": func(L *lua.LState, w *widget) int {
			w.textColor.r = float64(L.CheckNumber(2))
			w.textColor.g = float64(L.CheckNumber(3))
			w.textColor.b = float64(L.CheckNumber(4))
			return 0
		},
		"SetVertexColor": func(L *lua.LState, w *widget) int {
			w.vertexColor.r = float64(L.CheckNumber(2))
			w.vertexColor.g = float64(L.CheckNumber(3))
			w.vertexColor.b = float64(L.CheckNumber(4))
			return 0
		},
		"SetAlphaAttr": func(L *lua.LState, w *widget) int { return 0 },
		"SetTextInsets": func(L *lua.LState, w *widget) int {
			w.textInsetL = float64(L.CheckNumber(2))
			w.textInsetR = float64(L.CheckNumber(3))
			w.textInsetT = float64(L.CheckNumber(4))
			w.textInsetB = float64(L.CheckNumber(5))
			return 0
		},
		"HighlightText": func(L *lua.LState, w *widget) int {
			length := len([]rune(w.text))
			start, end := 0, length
			if L.GetTop() >= 2 && L.Get(2).Type() != lua.LTNil {
				start = L.CheckInt(2)
			}
			if L.GetTop() >= 3 && L.Get(3).Type() != lua.LTNil {
				end = L.CheckInt(3)
			}
			w.selectionStart = clampCursor(start, length)
			w.selectionEnd = clampCursor(end, length)
			w.selectionAnchor = w.selectionStart
			w.cursor = w.selectionEnd
			return 0
		},
		"SetFocus": func(L *lua.LState, w *widget) int { rt.setFocus(w); return 0 },
		"ClearFocus": func(L *lua.LState, w *widget) int {
			if rt.focused == w {
				rt.setFocus(nil)
			}
			return 0
		},
		"SetAutoFocus": func(L *lua.LState, w *widget) int {
			w.autoFocus = L.CheckBool(2)
			return 0
		},
		"SetMaxLetters": func(L *lua.LState, w *widget) int {
			w.maxLetters = L.CheckInt(2)
			return 0
		},
		"SetMaxBytes": func(L *lua.LState, w *widget) int {
			w.maxBytes = L.CheckInt(2)
			return 0
		},
		"SetMultiLine": func(L *lua.LState, w *widget) int { return 0 },
		"SetNumeric":   func(L *lua.LState, w *widget) int { return 0 },
		"SetMinMaxValues": func(L *lua.LState, w *widget) int {
			w.minValue = float64(L.CheckNumber(2))
			w.maxValue = float64(L.CheckNumber(3))
			return 0
		},
		"GetMinMaxValues": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.minValue))
			L.Push(lua.LNumber(w.maxValue))
			return 2
		},
		"SetValue": func(L *lua.LState, w *widget) int {
			w.value = float64(L.CheckNumber(2))
			rt.fire(w, "OnValueChanged", []lua.LValue{w.luaValue(L), lua.LNumber(w.value)})
			return 0
		},
		"GetValue": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.value))
			return 1
		},
		"SetValueStep": func(L *lua.LState, w *widget) int {
			w.valueStep = float64(L.CheckNumber(2))
			return 0
		},
		"SetOrientation": func(L *lua.LState, w *widget) int {
			w.orientation = L.CheckString(2)
			return 0
		},
		"SetThumbTexture": func(L *lua.LState, w *widget) int { return 0 },
		"SetVerticalScroll": func(L *lua.LState, w *widget) int {
			w.verticalScroll = float64(L.CheckNumber(2))
			return 0
		},
		"GetVerticalScroll": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.verticalScroll))
			return 1
		},
		"GetTextColor": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.textColor.r))
			L.Push(lua.LNumber(w.textColor.g))
			L.Push(lua.LNumber(w.textColor.b))
			return 3
		},
		"GetFontObject": func(L *lua.LState, w *widget) int {
			if w.fontObject == "" {
				L.Push(lua.LNil)
				return 1
			}
			L.Push(L.GetGlobal(w.fontObject))
			return 1
		},
		"GetCurrentValue": func(L *lua.LState, w *widget) int {
			switch w.kind {
			case kindCheckButton:
				L.Push(lua.LBool(w.checked))
			default:
				L.Push(lua.LNumber(w.value))
			}
			return 1
		},
		"SetDisplayValue": func(L *lua.LState, w *widget) int {
			if L.Get(2).Type() == lua.LTBool {
				w.checked = L.CheckBool(2)
			} else {
				w.value = float64(L.CheckNumber(2))
			}
			return 0
		},
		"GetDisplayValue": func(L *lua.LState, w *widget) int {
			if w.kind == kindCheckButton {
				L.Push(lua.LBool(w.checked))
			} else {
				L.Push(lua.LNumber(w.value))
			}
			return 1
		},
		"SetGlow":            func(L *lua.LState, w *widget) int { return 0 },
		"SetLogo":            func(L *lua.LState, w *widget) int { return 0 },
		"SetClearConfigData": func(L *lua.LState, w *widget) int { return 0 },
		"GetVerticalScrollRange": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(0))
			return 1
		},
		"SetScrollChild": func(L *lua.LState, w *widget) int { return 0 },
		"SetNormalTexture": func(L *lua.LState, w *widget) int {
			w.normalTexture = rt.textureArg(L, 1)
			return 0
		},
		"GetNormalTexture": func(L *lua.LState, w *widget) int {
			if w.normalTexture != nil {
				L.Push(w.normalTexture.luaValue(L))
			} else {
				L.Push(lua.LNil)
			}
			return 1
		},
		"SetPushedTexture": func(L *lua.LState, w *widget) int {
			w.pushedTexture = rt.textureArg(L, 1)
			return 0
		},
		"SetHighlightTexture": func(L *lua.LState, w *widget) int {
			w.highlightTexture = rt.textureArg(L, 1)
			return 0
		},
		"GetHighlightTexture": func(L *lua.LState, w *widget) int {
			if w.highlightTexture != nil {
				L.Push(w.highlightTexture.luaValue(L))
			} else {
				L.Push(lua.LNil)
			}
			return 1
		},
		"SetNormalFontObject": func(L *lua.LState, w *widget) int {
			w.normalFont = fontObjectName(L, 2)
			return 0
		},
		"SetHighlightFontObject": func(L *lua.LState, w *widget) int {
			w.highlightFont = fontObjectName(L, 2)
			return 0
		},
		"SetDisabledFontObject": func(L *lua.LState, w *widget) int {
			w.disabledFont = fontObjectName(L, 2)
			return 0
		},
		"SetDisabledTextColor": func(L *lua.LState, w *widget) int { return 0 },
		"SetDesaturated": func(L *lua.LState, w *widget) int {
			w.desaturated = false
			switch value := L.Get(2); value.Type() {
			case lua.LTBool:
				w.desaturated = L.CheckBool(2)
			case lua.LTNumber:
				w.desaturated = L.CheckNumber(2) != 0
			}
			return 0
		},
		"SetTexture": func(L *lua.LState, w *widget) int {
			if L.Get(2).Type() == lua.LTString {
				w.textureFile = L.CheckString(2)
			}
			return 0
		},
		"SetTexCoord": func(L *lua.LState, w *widget) int {
			if L.GetTop() >= 5 {
				w.texCoordL = float64(L.CheckNumber(2))
				w.texCoordR = float64(L.CheckNumber(3))
				w.texCoordT = float64(L.CheckNumber(4))
				w.texCoordB = float64(L.CheckNumber(5))
			}
			return 0
		},
		"GetTexCoord": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.texCoordL))
			L.Push(lua.LNumber(w.texCoordR))
			L.Push(lua.LNumber(w.texCoordT))
			L.Push(lua.LNumber(w.texCoordB))
			return 4
		},
		"SetFontObject": func(L *lua.LState, w *widget) int {
			w.fontObject = fontObjectName(L, 2)
			return 0
		},
		"SetFont": func(L *lua.LState, w *widget) int { return 0 },
		"SetJustifyH": func(L *lua.LState, w *widget) int {
			w.justifyH = L.CheckString(2)
			return 0
		},
		"SetJustifyV": func(L *lua.LState, w *widget) int {
			w.justifyV = L.CheckString(2)
			return 0
		},
		"SetShadowColor": func(L *lua.LState, w *widget) int {
			w.shadowColor = rgba{float64(L.CheckNumber(2)), float64(L.CheckNumber(3)), float64(L.CheckNumber(4)), 1}
			if L.GetTop() >= 5 {
				w.shadowColor.a = float64(L.CheckNumber(5))
			}
			w.shadowColorSet = true
			return 0
		},
		"SetShadowOffset": func(L *lua.LState, w *widget) int {
			w.shadowOffsetX = float64(L.CheckNumber(2))
			w.shadowOffsetY = float64(L.CheckNumber(3))
			w.shadowOffsetSet = true
			return 0
		},
		"SetSpacing":  func(L *lua.LState, w *widget) int { return 0 },
		"SetWordWrap": func(L *lua.LState, w *widget) int { return 0 },
		"GetStringWidth": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.textWidth))
			return 1
		},
		"GetTextWidth": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.textWidth))
			return 1
		},
		"SetBackdrop": func(L *lua.LState, w *widget) int {
			t := L.CheckTable(2)
			bd := &backdrop{}
			bd.bgFile = tableString(t, "bgFile")
			bd.edgeFile = tableString(t, "edgeFile")
			bd.tile = tableBool(t, "tile", false)
			bd.tileSize = float64(tableNumber(t, "tileSize"))
			bd.edgeSize = float64(tableNumber(t, "edgeSize"))
			if insets := t.RawGetString("insets"); insets.Type() == lua.LTTable {
				it := insets.(*lua.LTable)
				bd.insetL = float64(tableNumber(it, "left"))
				bd.insetR = float64(tableNumber(it, "right"))
				bd.insetT = float64(tableNumber(it, "top"))
				bd.insetB = float64(tableNumber(it, "bottom"))
			}
			w.backdrop = bd
			return 0
		},
		"SetBackdropColor": func(L *lua.LState, w *widget) int {
			if w.backdrop == nil {
				w.backdrop = &backdrop{}
			}
			w.backdrop.bgColor = rgba{float64(L.CheckNumber(2)), float64(L.CheckNumber(3)), float64(L.CheckNumber(4)), 1}
			if L.GetTop() >= 5 {
				w.backdrop.bgColor.a = float64(L.CheckNumber(5))
			}
			return 0
		},
		"SetBackdropBorderColor": func(L *lua.LState, w *widget) int {
			if w.backdrop == nil {
				w.backdrop = &backdrop{}
			}
			w.backdrop.edgeColor = rgba{float64(L.CheckNumber(2)), float64(L.CheckNumber(3)), float64(L.CheckNumber(4)), 1}
			if L.GetTop() >= 5 {
				w.backdrop.edgeColor.a = float64(L.CheckNumber(5))
			}
			return 0
		},
		"SetSequence": func(L *lua.LState, w *widget) int {
			w.sequence = L.CheckInt(2)
			return 0
		},
		"SetSequenceTime": func(L *lua.LState, w *widget) int {
			w.sequence = L.CheckInt(2)
			if L.GetTop() >= 2 {
				w.sequenceTime = L.CheckInt(3)
			}
			return 0
		},
		"SetCamera": func(L *lua.LState, w *widget) int {
			w.camera = L.CheckInt(2)
			return 0
		},
		"SetModel": func(L *lua.LState, w *widget) int {
			w.modelFile = L.CheckString(2)
			return 0
		},
		"SetModelScale": func(L *lua.LState, w *widget) int {
			w.modelScale = float64(L.CheckNumber(2))
			return 0
		},
		"SetFogNear": func(L *lua.LState, w *widget) int {
			w.fogNear = float64(L.CheckNumber(2))
			w.hasFog = true
			return 0
		},
		"SetFogFar": func(L *lua.LState, w *widget) int {
			w.fogFar = float64(L.CheckNumber(2))
			w.hasFog = true
			return 0
		},
		"SetFogColor": func(L *lua.LState, w *widget) int {
			w.hasFog = true
			return 0
		},
		"ClearFog": func(L *lua.LState, w *widget) int {
			w.hasFog = false
			return 0
		},
		"SetLight":          func(L *lua.LState, w *widget) int { return 0 },
		"ResetLights":       func(L *lua.LState, w *widget) int { return 0 },
		"AddCharacterLight": func(L *lua.LState, w *widget) int { return 0 },
		"AddLight":          func(L *lua.LState, w *widget) int { return 0 },
		"AddPetLight":       func(L *lua.LState, w *widget) int { return 0 },
		"SetPosition":       func(L *lua.LState, w *widget) int { return 0 },
		"AdvanceTime":       func(L *lua.LState, w *widget) int { return 0 },
		"StartMovie": func(L *lua.LState, w *widget) int {
			w.movieFile = L.CheckString(2)
			w.movieVolume = 255
			if L.GetTop() >= 3 {
				w.movieVolume = L.CheckInt(3)
			}
			w.movieActive = true
			L.Push(lua.LTrue)
			return 1
		},
		"StopMovie": func(L *lua.LState, w *widget) int {
			w.movieActive = false
			return 0
		},
		"EnableSubtitles": func(L *lua.LState, w *widget) int {
			w.subtitles = L.CheckBool(2)
			return 0
		},
		"SetOwner": func(L *lua.LState, w *widget) int {
			if ud, ok := L.Get(2).(*lua.LUserData); ok {
				if owner, ok := ud.Value.(*widget); ok {
					w.parent = owner
				}
			}
			return 0
		},
		"AddLine": func(L *lua.LState, w *widget) int {
			w.lines = append(w.lines, L.CheckString(2))
			return 0
		},
		"ClearLines": func(L *lua.LState, w *widget) int {
			w.lines = nil
			return 0
		},
	}

	methodIndex := L.NewTable()
	for methodName, method := range methods {
		method := method
		methodIndex.RawSetString(methodName, L.NewFunction(func(L *lua.LState) int {
			self := L.CheckUserData(1)
			w, ok := self.Value.(*widget)
			if !ok {
				L.ArgError(1, "widget expected")
				return 0
			}
			return method(L, w)
		}))
	}
	mt := L.NewTypeMetatable("wowWidget")
	L.SetGlobal("__wowWidgetMT", mt) // keep reference alive
	mt.RawSetString("__index", L.NewFunction(func(L *lua.LState) int {
		self := L.CheckUserData(1)
		w, ok := self.Value.(*widget)
		if !ok {
			L.ArgError(1, "widget expected")
			return 0
		}
		name := L.CheckString(2)
		// Script-assigned members take precedence over built-in methods.
		if w.fields != nil {
			if v := w.fields.RawGetString(name); v != lua.LNil {
				L.Push(v)
				return 1
			}
		}
		if fn, ok := methods[name]; ok {
			L.Push(L.NewFunction(func(L *lua.LState) int {
				defer func() {
					if r := recover(); r != nil {
						L.RaiseError("go panic in method %s on %s(%q): %v", name, w.objectType(), w.name, r)
					}
				}()
				return fn(L, w)
			}))
			return 1
		}
		if w.kind == kindSimpleHTML || w.kind == kindFrame || w.kind == kindMovieFrame {
			// Container helpers the glue scripts use on html/frames.
			switch name {
			case "GetFontString":
				L.Push(lua.LNil)
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	}))
	mt.RawSetString("__newindex", L.NewFunction(func(L *lua.LState) int {
		self := L.CheckUserData(1)
		w, ok := self.Value.(*widget)
		if !ok {
			L.ArgError(1, "widget expected")
			return 0
		}
		w.ensureFields(L).RawSetString(L.CheckString(2), L.Get(3))
		return 0
	}))
	baseGetMetatable := L.GetGlobal("getmetatable")
	L.SetGlobal("getmetatable", L.NewFunction(func(L *lua.LState) int {
		if ud, ok := L.Get(1).(*lua.LUserData); ok {
			if _, ok := ud.Value.(*widget); ok {
				proxy := L.NewTable()
				proxy.RawSetString("__index", methodIndex)
				L.Push(proxy)
				return 1
			}
		}
		L.Push(baseGetMetatable)
		L.Push(L.Get(1))
		if err := L.PCall(1, 1, nil); err != nil {
			L.Push(lua.LNil)
			return 1
		}
		return 1
	}))
}

// luaValue returns the Lua userdata handle for this widget, creating it once.
func (w *widget) luaValue(L *lua.LState) *lua.LUserData {
	if w.luaObj == nil {
		ud := L.NewUserData()
		ud.Value = w
		if mt := L.GetGlobal("__wowWidgetMT"); mt != nil && mt.Type() == lua.LTTable {
			ud.Metatable = mt.(*lua.LTable)
		}
		w.luaObj = ud
	}
	return w.luaObj
}

func tableString(t *lua.LTable, key string) string {
	if v := t.RawGetString(key); v.Type() == lua.LTString {
		return v.String()
	}
	return ""
}

func tableBool(t *lua.LTable, key string, def bool) bool {
	if v := t.RawGetString(key); v != lua.LNil {
		return v == lua.LTrue
	}
	return def
}

func tableNumber(t *lua.LTable, key string) lua.LNumber {
	if v := t.RawGetString(key); v.Type() == lua.LTNumber {
		return v.(lua.LNumber)
	}
	return 0
}

// textureArg resolves a texture argument that may be a path string or a
// texture widget reference.
func (rt *Runtime) textureArg(L *lua.LState, idx int) *widget {
	switch v := L.Get(idx).(type) {
	case lua.LString:
		tex := newWidget(kindTexture, "")
		tex.textureFile = v.String()
		return tex
	case *lua.LUserData:
		if w, ok := v.Value.(*widget); ok {
			return w
		}
	}
	return nil
}

var _ = fmt.Sprintf

// fontObjectName resolves a font argument that may be a name string, a font
// object table with a name field, or nil.
func fontObjectName(L *lua.LState, idx int) string {
	switch v := L.Get(idx).(type) {
	case lua.LString:
		return v.String()
	case *lua.LTable:
		if n := v.RawGetString("name"); n.Type() == lua.LTString {
			return n.String()
		}
	}
	return ""
}
