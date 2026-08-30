package ui

import (
	"fmt"

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

type color struct{ r, g, b, a float64 }

func (c color) isZero() bool { return c.r == 0 && c.g == 0 && c.b == 0 && c.a == 0 }

type anchorPoint struct {
	point, relativeTo, relativePoint string
	x, y                             float64
}

type backdrop struct {
	bgFile, edgeFile               string
	tile                           bool
	tileSize, edgeSize             float64
	insetL, insetR, insetT, insetB float64
	bgColor, edgeColor             color
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
	width           float64
	height          float64
	scale           float64
	alpha           float64
	points          []anchorPoint
	scripts         map[string]*lua.LFunction
	events          map[string]bool
	backdrop        *backdrop

	// Button and CheckButton state.
	buttonState      string
	desaturated      bool
	highlighted      bool
	enabled          bool
	normalTexture    *widget
	pushedTexture    *widget
	highlightTexture *widget
	normalFont       string
	highlightFont    string
	disabledFont     string
	checked          bool

	// EditBox state.
	text         string
	password     bool
	autoFocus    bool
	maxLetters   int
	maxBytes     int
	historyLines int

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
	subtitles bool

	// Texture state.
	textureFile                                string
	texCoordL, texCoordR, texCoordT, texCoordB float64
	vertexColor                                color

	// FontString state.
	fontObject string
	justifyH   string
	justifyV   string
	textColor  color
	textWidth  float64

	// Tooltip/HTML text lines.
	lines []string

	luaObj *lua.LUserData
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
		orientation: "VERTICAL",
		scripts:     make(map[string]*lua.LFunction),
		events:      make(map[string]bool),
		texCoordL:   0, texCoordR: 1, texCoordT: 0, texCoordB: 1,
	}
}

func (w *widget) objectType() string { return w.kind.objectType() }

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
			L.Push(lua.LBool(w.objectTypeMatches(L.CheckString(1))))
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
			ud := L.CheckUserData(1)
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
			w.id = L.CheckInt(1)
			return 0
		},
		"Show": func(L *lua.LState, w *widget) int {
			w.shown = true
			rt.fireHandler(w, "OnShow")
			return 0
		},
		"Hide": func(L *lua.LState, w *widget) int {
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
			w.width = float64(L.CheckNumber(1))
			return 0
		},
		"SetHeight": func(L *lua.LState, w *widget) int {
			w.height = float64(L.CheckNumber(1))
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
		"GetAlpha": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.alpha))
			return 1
		},
		"SetAlpha": func(L *lua.LState, w *widget) int {
			w.alpha = float64(L.CheckNumber(1))
			return 0
		},
		"SetScale": func(L *lua.LState, w *widget) int {
			w.scale = float64(L.CheckNumber(1))
			return 0
		},
		"SetFrameLevel": func(L *lua.LState, w *widget) int {
			w.frameLevel = L.CheckInt(1)
			return 0
		},
		"GetFrameLevel": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.frameLevel))
			return 1
		},
		"Raise": func(L *lua.LState, w *widget) int { return 0 },
		"EnableKeyboard": func(L *lua.LState, w *widget) int {
			w.enableKeyboard = L.CheckBool(1)
			return 0
		},
		"EnableMouse": func(L *lua.LState, w *widget) int {
			w.enableMouse = L.CheckBool(1)
			return 0
		},
		"RegisterForClicks": func(L *lua.LState, w *widget) int { return 0 },
		"SetScript": func(L *lua.LState, w *widget) int {
			handler := L.CheckString(1)
			fn := L.CheckFunction(2)
			w.scripts[handler] = fn
			return 0
		},
		"GetScript": func(L *lua.LState, w *widget) int {
			if fn, ok := w.scripts[L.CheckString(1)]; ok {
				L.Push(fn)
				return 1
			}
			L.Push(lua.LNil)
			return 1
		},
		"HookScript": func(L *lua.LState, w *widget) int {
			handler := L.CheckString(1)
			hook := L.CheckFunction(2)
			orig := w.scripts[handler]
			L.Push(hook)
			L.Push(w.luaValue(L))
			if orig != nil {
				L.Push(orig)
				L.Push(w.luaValue(L))
				if err := L.PCall(2, 0, nil); err != nil {
					rt.recordScriptError("HookScript", err.Error())
				}
			} else {
				w.scripts[handler] = hook
			}
			return 0
		},
		"RegisterEvent": func(L *lua.LState, w *widget) int {
			rt.registerEventWidget(L.CheckString(1), w)
			return 0
		},
		"UnregisterEvent": func(L *lua.LState, w *widget) int {
			rt.unregisterEventWidget(L.CheckString(1), w)
			return 0
		},
		"ClearAllPoints": func(L *lua.LState, w *widget) int {
			w.points = nil
			return 0
		},
		"SetPoint": func(L *lua.LState, w *widget) int {
			n := L.GetTop()
			p := anchorPoint{point: L.CheckString(1)}
			switch {
			case n >= 4:
				// point, relativeTo, relativePoint, x, y
				if ud, ok := L.Get(2).(*lua.LUserData); ok {
					if rel, ok := ud.Value.(*widget); ok {
						p.relativeTo = rel.name
					}
				}
				p.relativePoint = L.CheckString(3)
				p.x = float64(L.CheckNumber(4))
				if n >= 5 {
					p.y = float64(L.CheckNumber(5))
				}
			case n == 3:
				p.x = float64(L.CheckNumber(2))
				p.y = float64(L.CheckNumber(3))
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
				idx = L.CheckInt(1)
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
			w.buttonState = L.CheckString(1)
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
			w.checked = L.CheckBool(1)
			return 0
		},
		"GetChecked": func(L *lua.LState, w *widget) int {
			L.Push(lua.LBool(w.checked))
			return 1
		},
		"SetText": func(L *lua.LState, w *widget) int {
			w.text = L.CheckString(1)
			return 0
		},
		"SetFormattedText": func(L *lua.LState, w *widget) int {
			format := L.CheckString(1)
			args := make([]interface{}, 0, L.GetTop()-1)
			for i := 2; i <= L.GetTop(); i++ {
				args = append(args, L.Get(i).String())
			}
			w.text = sprintf(format, args)
			return 0
		},
		"GetText": func(L *lua.LState, w *widget) int {
			L.Push(lua.LString(w.text))
			return 1
		},
		"SetTextColor": func(L *lua.LState, w *widget) int {
			w.textColor.r = float64(L.CheckNumber(1))
			w.textColor.g = float64(L.CheckNumber(2))
			w.textColor.b = float64(L.CheckNumber(3))
			return 0
		},
		"SetVertexColor": func(L *lua.LState, w *widget) int {
			w.vertexColor.r = float64(L.CheckNumber(1))
			w.vertexColor.g = float64(L.CheckNumber(2))
			w.vertexColor.b = float64(L.CheckNumber(3))
			return 0
		},
		"SetAlphaAttr":  func(L *lua.LState, w *widget) int { return 0 },
		"SetTextInsets": func(L *lua.LState, w *widget) int { return 0 },
		"HighlightText": func(L *lua.LState, w *widget) int { return 0 },
		"SetFocus":      func(L *lua.LState, w *widget) int { return 0 },
		"ClearFocus":    func(L *lua.LState, w *widget) int { return 0 },
		"SetAutoFocus": func(L *lua.LState, w *widget) int {
			w.autoFocus = L.CheckBool(1)
			return 0
		},
		"SetMaxLetters": func(L *lua.LState, w *widget) int {
			w.maxLetters = L.CheckInt(1)
			return 0
		},
		"SetMaxBytes": func(L *lua.LState, w *widget) int {
			w.maxBytes = L.CheckInt(1)
			return 0
		},
		"SetMultiLine": func(L *lua.LState, w *widget) int { return 0 },
		"SetNumeric":   func(L *lua.LState, w *widget) int { return 0 },
		"SetMinMaxValues": func(L *lua.LState, w *widget) int {
			w.minValue = float64(L.CheckNumber(1))
			w.maxValue = float64(L.CheckNumber(2))
			return 0
		},
		"GetMinMaxValues": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.minValue))
			L.Push(lua.LNumber(w.maxValue))
			return 2
		},
		"SetValue": func(L *lua.LState, w *widget) int {
			w.value = float64(L.CheckNumber(1))
			rt.fire(w, "OnValueChanged", []lua.LValue{w.luaValue(L), lua.LNumber(w.value)})
			return 0
		},
		"GetValue": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.value))
			return 1
		},
		"SetValueStep": func(L *lua.LState, w *widget) int {
			w.valueStep = float64(L.CheckNumber(1))
			return 0
		},
		"SetOrientation": func(L *lua.LState, w *widget) int {
			w.orientation = L.CheckString(1)
			return 0
		},
		"SetThumbTexture": func(L *lua.LState, w *widget) int { return 0 },
		"SetVerticalScroll": func(L *lua.LState, w *widget) int {
			w.verticalScroll = float64(L.CheckNumber(1))
			return 0
		},
		"GetVerticalScroll": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.verticalScroll))
			return 1
		},
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
			w.normalFont = L.CheckString(1)
			return 0
		},
		"SetHighlightFontObject": func(L *lua.LState, w *widget) int {
			w.highlightFont = L.CheckString(1)
			return 0
		},
		"SetDisabledFontObject": func(L *lua.LState, w *widget) int {
			w.disabledFont = L.CheckString(1)
			return 0
		},
		"SetDisabledTextColor": func(L *lua.LState, w *widget) int { return 0 },
		"SetDesaturated": func(L *lua.LState, w *widget) int {
			w.desaturated = L.CheckBool(1)
			return 0
		},
		"SetTexture": func(L *lua.LState, w *widget) int {
			if L.Get(1).Type() == lua.LTString {
				w.textureFile = L.CheckString(1)
			}
			return 0
		},
		"SetTexCoord": func(L *lua.LState, w *widget) int {
			if L.GetTop() >= 8 {
				w.texCoordL = float64(L.CheckNumber(1))
				w.texCoordR = float64(L.CheckNumber(2))
				w.texCoordT = float64(L.CheckNumber(3))
				w.texCoordB = float64(L.CheckNumber(4))
			}
			return 0
		},
		"SetFontObject": func(L *lua.LState, w *widget) int {
			w.fontObject = L.CheckString(1)
			return 0
		},
		"SetFont": func(L *lua.LState, w *widget) int { return 0 },
		"SetJustifyH": func(L *lua.LState, w *widget) int {
			w.justifyH = L.CheckString(1)
			return 0
		},
		"SetJustifyV": func(L *lua.LState, w *widget) int {
			w.justifyV = L.CheckString(1)
			return 0
		},
		"SetShadowColor":  func(L *lua.LState, w *widget) int { return 0 },
		"SetShadowOffset": func(L *lua.LState, w *widget) int { return 0 },
		"SetSpacing":      func(L *lua.LState, w *widget) int { return 0 },
		"SetWordWrap":     func(L *lua.LState, w *widget) int { return 0 },
		"GetStringWidth": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.textWidth))
			return 1
		},
		"GetTextWidth": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.textWidth))
			return 1
		},
		"SetBackdrop": func(L *lua.LState, w *widget) int {
			t := L.CheckTable(1)
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
			w.backdrop.bgColor = color{float64(L.CheckNumber(1)), float64(L.CheckNumber(2)), float64(L.CheckNumber(3)), 1}
			return 0
		},
		"SetBackdropBorderColor": func(L *lua.LState, w *widget) int {
			if w.backdrop == nil {
				w.backdrop = &backdrop{}
			}
			w.backdrop.edgeColor = color{float64(L.CheckNumber(1)), float64(L.CheckNumber(2)), float64(L.CheckNumber(3)), 1}
			return 0
		},
		"SetSequence": func(L *lua.LState, w *widget) int {
			w.sequence = L.CheckInt(1)
			return 0
		},
		"SetSequenceTime": func(L *lua.LState, w *widget) int {
			w.sequence = L.CheckInt(1)
			if L.GetTop() >= 2 {
				w.sequenceTime = L.CheckInt(2)
			}
			return 0
		},
		"SetCamera": func(L *lua.LState, w *widget) int {
			w.camera = L.CheckInt(1)
			return 0
		},
		"SetModel": func(L *lua.LState, w *widget) int {
			w.modelFile = L.CheckString(1)
			return 0
		},
		"SetModelScale": func(L *lua.LState, w *widget) int {
			w.modelScale = float64(L.CheckNumber(1))
			return 0
		},
		"SetFogNear": func(L *lua.LState, w *widget) int {
			w.fogNear = float64(L.CheckNumber(1))
			w.hasFog = true
			return 0
		},
		"SetFogFar": func(L *lua.LState, w *widget) int {
			w.fogFar = float64(L.CheckNumber(1))
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
		"SetLight":    func(L *lua.LState, w *widget) int { return 0 },
		"SetPosition": func(L *lua.LState, w *widget) int { return 0 },
		"AdvanceTime": func(L *lua.LState, w *widget) int { return 0 },
		"StartMovie":  func(L *lua.LState, w *widget) int { return 0 },
		"StopMovie":   func(L *lua.LState, w *widget) int { return 0 },
		"EnableSubtitles": func(L *lua.LState, w *widget) int {
			w.subtitles = L.CheckBool(1)
			return 0
		},
		"SetOwner": func(L *lua.LState, w *widget) int {
			if ud, ok := L.Get(1).(*lua.LUserData); ok {
				if owner, ok := ud.Value.(*widget); ok {
					w.parent = owner
				}
			}
			return 0
		},
		"AddLine": func(L *lua.LState, w *widget) int {
			w.lines = append(w.lines, L.CheckString(1))
			return 0
		},
		"ClearLines": func(L *lua.LState, w *widget) int {
			w.lines = nil
			return 0
		},
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
		if fn, ok := methods[name]; ok {
			L.Push(L.NewFunction(func(L *lua.LState) int {
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
