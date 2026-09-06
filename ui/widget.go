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
	kindStatusBar
	kindScrollFrame
	kindScrollingMessageFrame
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
	case kindStatusBar:
		return "StatusBar"
	case kindScrollFrame:
		return "ScrollFrame"
	case kindScrollingMessageFrame:
		return "ScrollingMessageFrame"
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

type messageLine struct {
	text      string
	color     rgba
	typeID    int
	backFill  int
	accessID  int
	extraData lua.LValue
	age       float64
}

// widget holds the state of one frame or region created from XML or the API.
// The original client models these as C++ frame classes; this struct keeps
// the fields the glue scripts observe through their methods.
type widget struct {
	kind            widgetKind
	name            string
	id              int
	parent          *widget
	owner           *widget
	children        []*widget
	shown           bool
	topLevel        bool
	movable         bool
	enableMouse     bool
	enableKeyboard  bool
	clampedToScreen bool
	frameStrata     int
	frameLevel      int
	layerLevel      int
	hitInsetL       float64
	hitInsetR       float64
	hitInsetT       float64
	hitInsetB       float64
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
	highlightLocked        bool
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
	thumbTexture       *widget
	verticalRange      float64
	horizontalRange    float64

	// StatusBar state.
	statusBarTexture *widget
	statusBarColor   rgba

	// ScrollFrame state.
	verticalScroll   float64
	horizontalScroll float64
	scrollChild      *widget

	// ScrollingMessageFrame state.
	messages        []messageLine
	messageMaxLines int
	messageDuration float64
	messageFading   bool
	messageOffset   int
	messageFontSize float64
	messageIndented bool
	messageSpacing  float64
	messageInsert   string

	// Model state.
	modelFile       string
	sequence        int
	sequenceTime    int
	camera          int
	modelScale      float64
	modelPosition   [3]float64
	modelFacing     float64
	fogNear, fogFar float64
	hasFog          bool

	// MovieFrame state.
	subtitles   bool
	movieFile   string
	movieActive bool
	movieVolume int

	// Texture state.
	textureFile                                string
	portraitUnit                               string
	texCoordL, texCoordR, texCoordT, texCoordB float64
	vertexColor                                rgba
	alphaMode                                  string
	blendMode                                  string
	horizTile, vertTile                        bool

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
	textInsetsSet   bool
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
	w := &widget{
		kind:        kind,
		name:        name,
		shown:       true,
		scale:       1,
		alpha:       1,
		buttonState: "NORMAL",
		enabled:     true,
		frameStrata: 2,
		layerLevel:  layerArtwork,
		orientation: "VERTICAL",
		scripts:     make(map[string]*lua.LFunction),
		events:      make(map[string]bool),
		texCoordL:   0, texCoordR: 1, texCoordT: 0, texCoordB: 1,
	}
	// Buttons, sliders, and edit boxes receive mouse by default in the client.
	switch kind {
	case kindButton, kindCheckButton, kindEditBox, kindSlider:
		w.enableMouse = true
	}
	return w
}

func addWidgetChild(parent, child *widget) {
	if parent == nil || child == nil {
		return
	}
	for _, existing := range parent.children {
		if existing == child {
			return
		}
	}
	parent.children = append(parent.children, child)
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
		"CreateTexture": func(L *lua.LState, w *widget) int {
			name := ""
			if L.GetTop() >= 2 && L.Get(2).Type() == lua.LTString {
				name = resolveParentName(L.CheckString(2), w.name)
			}
			texture := newWidget(kindTexture, name)
			texture.parent = w
			if L.GetTop() >= 3 && L.Get(3).Type() == lua.LTString {
				texture.layerLevel = layerOrder(L.CheckString(3))
			}
			if L.GetTop() >= 4 && L.Get(4).Type() == lua.LTString && rt.instantiateTemplate != nil {
				rt.instantiateTemplate(texture, L.CheckString(4))
			}
			addWidgetChild(w, texture)
			rt.register(texture)
			L.Push(texture.luaValue(L))
			return 1
		},
		"CreateFontString": func(L *lua.LState, w *widget) int {
			name := ""
			if L.GetTop() >= 2 && L.Get(2).Type() == lua.LTString {
				name = resolveParentName(L.CheckString(2), w.name)
			}
			fontString := newWidget(kindFontString, name)
			fontString.parent = w
			if L.GetTop() >= 3 && L.Get(3).Type() == lua.LTString {
				fontString.layerLevel = layerOrder(L.CheckString(3))
			}
			if L.GetTop() >= 4 && L.Get(4).Type() == lua.LTString && rt.instantiateTemplate != nil {
				rt.instantiateTemplate(fontString, L.CheckString(4))
			}
			addWidgetChild(w, fontString)
			rt.register(fontString)
			L.Push(fontString.luaValue(L))
			return 1
		},
		"SetParent": func(L *lua.LState, w *widget) int {
			var p *widget
			switch value := L.Get(2).(type) {
			case *lua.LUserData:
				p, _ = value.Value.(*widget)
			case lua.LString:
				p = rt.widgets[value.String()]
			}
			if p == nil {
				L.ArgError(2, "known frame userdata or name")
				return 0
			}
			if w.parent != nil && w.parent != p {
				children := w.parent.children[:0]
				for _, child := range w.parent.children {
					if child != w {
						children = append(children, child)
					}
				}
				w.parent.children = children
			}
			w.parent = p
			addWidgetChild(p, w)
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
			width := float64(L.CheckNumber(2))
			w.width = width
			if w.kind == kindFontString {
				// FrameXML (PanelTemplates_TabResize) sets width 0 to mean
				// "use the natural string width" on the next GetWidth.
				if width <= 0 {
					w.width = 0
					w.explicitWidth = false
					w.autoTextWidth = true
					if rt.measureText != nil {
						rt.measureText(w)
					}
				} else {
					w.explicitWidth = true
					w.autoTextWidth = false
				}
			}
			return 0
		},
		"SetHeight": func(L *lua.LState, w *widget) int {
			w.height = float64(L.CheckNumber(2))
			if w.kind == kindFontString {
				w.explicitHeight = true
				w.autoTextHeight = false
			}
			return 0
		},
		"SetSize": func(L *lua.LState, w *widget) int {
			w.width = float64(L.CheckNumber(2))
			w.height = float64(L.CheckNumber(3))
			if w.kind == kindFontString {
				w.explicitWidth = true
				w.autoTextWidth = false
				w.explicitHeight = true
				w.autoTextHeight = false
			}
			return 0
		},
		"GetWidth": func(L *lua.LState, w *widget) int {
			if w.kind == kindFontString && w.autoTextWidth && rt.measureText != nil {
				rt.measureText(w)
			}
			L.Push(lua.LNumber(w.width))
			return 1
		},
		"GetHeight": func(L *lua.LState, w *widget) int {
			if w.kind == kindFontString && w.autoTextHeight && rt.measureText != nil {
				rt.measureText(w)
			}
			L.Push(lua.LNumber(w.height))
			return 1
		},
		"GetSize": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.width))
			L.Push(lua.LNumber(w.height))
			return 2
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
		"SetFrameStrata": func(L *lua.LState, w *widget) int {
			w.frameStrata = frameStrataOrder(L.CheckString(2))
			return 0
		},
		"GetFrameStrata": func(L *lua.LState, w *widget) int {
			L.Push(lua.LString(frameStrataName(w.frameStrata)))
			return 1
		},
		"GetFrameLevel": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.frameLevel))
			return 1
		},
		"Raise": func(L *lua.LState, w *widget) int {
			if w.parent != nil {
				level := w.frameLevel
				for _, sibling := range w.parent.children {
					if sibling != w && sibling.frameLevel >= level {
						level = sibling.frameLevel + 1
					}
				}
				w.frameLevel = level
			}
			return 0
		},
		"EnableKeyboard": func(L *lua.LState, w *widget) int {
			w.enableKeyboard = L.CheckBool(2)
			return 0
		},
		"EnableMouse": func(L *lua.LState, w *widget) int {
			w.enableMouse = L.CheckBool(2)
			return 0
		},
		"SetHitRectInsets": func(L *lua.LState, w *widget) int {
			w.hitInsetL = float64(L.CheckNumber(2))
			w.hitInsetR = float64(L.CheckNumber(3))
			w.hitInsetT = float64(L.CheckNumber(4))
			w.hitInsetB = float64(L.CheckNumber(5))
			return 0
		},
		"SetMovable":        func(L *lua.LState, w *widget) int { w.movable = L.CheckBool(2); return 0 },
		"RegisterForClicks": func(L *lua.LState, w *widget) int { return 0 },
		"RegisterForDrag":   func(L *lua.LState, w *widget) int { return 0 },
		"SetScript": func(L *lua.LState, w *widget) int {
			handler := L.CheckString(2)
			if L.Get(3) == lua.LNil {
				delete(w.scripts, handler)
				return 0
			}
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
			// SetAllPoints([relativeTo]) fills relativeTo (default: parent).
			// FCFDock_AddChatFrame uses SetAllPoints(dock.primary); ignoring the
			// argument filled UIParent and painted a second fullscreen chat layer.
			relName := ""
			if L.GetTop() >= 2 && L.Get(2) != lua.LNil {
				switch v := L.Get(2).(type) {
				case *lua.LUserData:
					if rel, ok := v.Value.(*widget); ok {
						relName = rel.name
					}
				case lua.LString:
					relName = resolveParentName(v.String(), w.parentName())
				}
			}
			w.points = []anchorPoint{
				{point: "TOPLEFT", relativeTo: relName, relativePoint: "TOPLEFT"},
				{point: "BOTTOMRIGHT", relativeTo: relName, relativePoint: "BOTTOMRIGHT"},
			}
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
		"GetNumPoints": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(len(w.points)))
			return 1
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
			w.highlightLocked = true
			w.highlighted = true
			return 0
		},
		"UnlockHighlight": func(L *lua.LState, w *widget) int {
			w.highlightLocked = false
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
			text := ""
			if L.Get(2).Type() != lua.LTNil {
				text = L.Get(2).String()
			}
			rt.setText(w, text)
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
		"GetFontString": func(L *lua.LState, w *widget) int {
			if w.buttonLabel == nil {
				L.Push(lua.LNil)
			} else {
				L.Push(w.buttonLabel.luaValue(L))
			}
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
			w.vertexColor.a = 1
			return 0
		},
		"SetAlphaAttr": func(L *lua.LState, w *widget) int { return 0 },
		"SetTextInsets": func(L *lua.LState, w *widget) int {
			w.textInsetL = float64(L.CheckNumber(2))
			w.textInsetR = float64(L.CheckNumber(3))
			w.textInsetT = float64(L.CheckNumber(4))
			w.textInsetB = float64(L.CheckNumber(5))
			w.textInsetsSet = true
			return 0
		},
		"GetTextInsets": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.textInsetL))
			L.Push(lua.LNumber(w.textInsetR))
			L.Push(lua.LNumber(w.textInsetT))
			L.Push(lua.LNumber(w.textInsetB))
			return 4
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
			value := float64(L.CheckNumber(2))
			if value < w.minValue {
				value = w.minValue
			}
			if value > w.maxValue {
				value = w.maxValue
			}
			if value == w.value {
				return 0
			}
			w.value = value
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
		"SetStatusBarColor": func(L *lua.LState, w *widget) int {
			w.statusBarColor = rgba{float64(L.CheckNumber(2)), float64(L.CheckNumber(3)), float64(L.CheckNumber(4)), 1}
			if L.GetTop() >= 5 {
				w.statusBarColor.a = float64(L.CheckNumber(5))
			}
			return 0
		},
		"GetStatusBarColor": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.statusBarColor.r))
			L.Push(lua.LNumber(w.statusBarColor.g))
			L.Push(lua.LNumber(w.statusBarColor.b))
			L.Push(lua.LNumber(w.statusBarColor.a))
			return 4
		},
		"SetStatusBarTexture": func(L *lua.LState, w *widget) int {
			w.statusBarTexture = rt.buttonTextureArg(w, w.statusBarTexture, L, 2)
			return 0
		},
		"GetStatusBarTexture": func(L *lua.LState, w *widget) int {
			if w.statusBarTexture != nil {
				L.Push(w.statusBarTexture.luaValue(L))
			} else {
				L.Push(lua.LNil)
			}
			return 1
		},
		"SetDrawLayer": func(L *lua.LState, w *widget) int {
			w.layerLevel = layerOrder(L.CheckString(2))
			return 0
		},
		"SetOrientation": func(L *lua.LState, w *widget) int {
			w.orientation = L.CheckString(2)
			return 0
		},
		"SetThumbTexture": func(L *lua.LState, w *widget) int {
			w.thumbTexture = rt.buttonTextureArg(w, w.thumbTexture, L, 2)
			return 0
		},
		"SetVerticalScroll": func(L *lua.LState, w *widget) int {
			value := float64(L.CheckNumber(2))
			if value < 0 {
				value = 0
			}
			if value > w.verticalRange {
				value = w.verticalRange
			}
			if value == w.verticalScroll {
				return 0
			}
			w.verticalScroll = value
			rt.fire(w, "OnVerticalScroll", []lua.LValue{w.luaValue(L), lua.LNumber(value)})
			return 0
		},
		"GetVerticalScroll": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.verticalScroll))
			return 1
		},
		"SetHorizontalScroll": func(L *lua.LState, w *widget) int {
			w.horizontalScroll = float64(L.CheckNumber(2))
			return 0
		},
		"GetHorizontalScroll": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.horizontalScroll))
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
			L.Push(lua.LNumber(w.verticalRange))
			return 1
		},
		"SetScrollChild": func(L *lua.LState, w *widget) int {
			if ud, ok := L.Get(2).(*lua.LUserData); ok {
				if child, ok := ud.Value.(*widget); ok {
					w.scrollChild = child
				}
			}
			return 0
		},
		"GetScrollChild": func(L *lua.LState, w *widget) int {
			if w.scrollChild == nil {
				L.Push(lua.LNil)
			} else {
				L.Push(w.scrollChild.luaValue(L))
			}
			return 1
		},
		"AddMessage": func(L *lua.LState, w *widget) int {
			if w.kind != kindScrollingMessageFrame && w.kind != kindSimpleHTML && w.kind != kindFrame {
				return 0
			}
			line := messageLine{text: L.CheckString(2), color: rgba{1, 1, 1, 1}, extraData: lua.LNil}
			arg := 3
			// FrameXML uses AddMessage(text, r, g, b[, id...]) with r/g/b in 0..1.
			// Require GetTop()>=5 (self+text+r+g+b) and treat channels >1 as the
			// non-color metadata form used by tests (AddMessage(text, typeID, ...)).
			if L.GetTop() >= 5 && L.Get(3).Type() == lua.LTNumber && L.Get(4).Type() == lua.LTNumber && L.Get(5).Type() == lua.LTNumber {
				r, g, b := float64(L.CheckNumber(3)), float64(L.CheckNumber(4)), float64(L.CheckNumber(5))
				if r <= 1 && g <= 1 && b <= 1 {
					line.color = rgba{r, g, b, 1}
					arg = 6
				}
			}
			if L.GetTop() >= arg && L.Get(arg).Type() == lua.LTNumber {
				line.typeID = L.CheckInt(arg)
			}
			if L.GetTop() >= arg+1 && L.Get(arg+1).Type() == lua.LTNumber {
				line.backFill = L.CheckInt(arg + 1)
			}
			if L.GetTop() >= arg+2 && L.Get(arg+2).Type() == lua.LTNumber {
				line.accessID = L.CheckInt(arg + 2)
			}
			if L.GetTop() >= arg+3 {
				line.extraData = L.Get(arg + 3)
			}
			w.messages = append(w.messages, line)
			maxLines := w.messageMaxLines
			if maxLines <= 0 {
				maxLines = 128
			}
			if len(w.messages) > maxLines {
				w.messages = w.messages[len(w.messages)-maxLines:]
			}
			if w.messageOffset > len(w.messages)-1 {
				w.messageOffset = len(w.messages) - 1
			}
			if w.messageOffset < 0 {
				w.messageOffset = 0
			}
			return 0
		},
		"Clear": func(L *lua.LState, w *widget) int { w.messages = nil; w.messageOffset = 0; return 0 },
		"SetMaxLines": func(L *lua.LState, w *widget) int {
			w.messageMaxLines = L.CheckInt(2)
			if w.messageMaxLines > 0 && len(w.messages) > w.messageMaxLines {
				w.messages = w.messages[len(w.messages)-w.messageMaxLines:]
			}
			if w.messageOffset > len(w.messages)-1 {
				w.messageOffset = len(w.messages) - 1
			}
			if w.messageOffset < 0 {
				w.messageOffset = 0
			}
			return 0
		},
		"GetMaxLines":    func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.messageMaxLines)); return 1 },
		"SetTimeVisible": func(L *lua.LState, w *widget) int { w.messageDuration = float64(L.CheckNumber(2)); return 0 },
		"GetTimeVisible": func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.messageDuration)); return 1 },
		"SetFading":      func(L *lua.LState, w *widget) int { w.messageFading = L.CheckBool(2); return 0 },
		"IsFading":       func(L *lua.LState, w *widget) int { L.Push(lua.LBool(w.messageFading)); return 1 },
		"AtBottom": func(L *lua.LState, w *widget) int {
			L.Push(lua.LBool(w.messageOffset == 0))
			return 1
		},
		"ScrollDown": func(L *lua.LState, w *widget) int {
			if w.messageOffset > 0 {
				w.messageOffset--
			}
			return 0
		},
		"ScrollUp": func(L *lua.LState, w *widget) int {
			if w.messageOffset < len(w.messages)-1 {
				w.messageOffset++
			}
			return 0
		},
		"ScrollToBottom": func(L *lua.LState, w *widget) int { w.messageOffset = 0; return 0 },
		"ScrollToTop": func(L *lua.LState, w *widget) int {
			if len(w.messages) > 0 {
				w.messageOffset = len(w.messages) - 1
			}
			return 0
		},
		"SetScrollOffset": func(L *lua.LState, w *widget) int {
			w.messageOffset = L.CheckInt(2)
			if w.messageOffset < 0 {
				w.messageOffset = 0
			}
			if w.messageOffset > len(w.messages)-1 {
				w.messageOffset = len(w.messages) - 1
			}
			if w.messageOffset < 0 {
				w.messageOffset = 0
			}
			return 0
		},
		"GetCurrentScroll": func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.messageOffset)); return 1 },
		"GetNumMessages": func(L *lua.LState, w *widget) int {
			if L.GetTop() < 2 || L.Get(2).Type() != lua.LTNumber {
				L.Push(lua.LNumber(len(w.messages)))
				return 1
			}
			accessID := L.CheckInt(2)
			count := 0
			for _, line := range w.messages {
				if line.accessID == accessID {
					count++
				}
			}
			L.Push(lua.LNumber(count))
			return 1
		},
		"GetMessageInfo": func(L *lua.LState, w *widget) int {
			index := L.CheckInt(2)
			if index < 1 || index > len(w.messages) {
				L.Push(lua.LNil)
				return 1
			}
			line := w.messages[index-1]
			L.Push(lua.LString(line.text))
			L.Push(lua.LNumber(line.accessID))
			L.Push(lua.LNumber(line.typeID))
			if line.extraData == nil {
				L.Push(lua.LNil)
			} else {
				L.Push(line.extraData)
			}
			return 4
		},
		"RemoveMessagesByAccessID": func(L *lua.LState, w *widget) int {
			accessID := 0
			if L.GetTop() >= 2 && L.Get(2).Type() == lua.LTNumber {
				accessID = L.CheckInt(2)
			}
			kept := w.messages[:0]
			for _, line := range w.messages {
				if line.accessID != accessID {
					kept = append(kept, line)
				}
			}
			w.messages = kept
			if w.messageOffset > len(w.messages)-1 {
				w.messageOffset = len(w.messages) - 1
			}
			if w.messageOffset < 0 {
				w.messageOffset = 0
			}
			return 0
		},
		"SetIndented":          func(L *lua.LState, w *widget) int { w.messageIndented = L.CheckBool(2); return 0 },
		"SetInsertMode":        func(L *lua.LState, w *widget) int { w.messageInsert = L.CheckString(2); return 0 },
		"SetNonSpaceWrap":      func(L *lua.LState, w *widget) int { return 0 },
		"SetHyperlinksEnabled": func(L *lua.LState, w *widget) int { return 0 },
		"GetFont": func(L *lua.LState, w *widget) int {
			L.Push(lua.LString(`Fonts\FRIZQT__.TTF`))
			fontSize := w.messageFontSize
			if fontSize == 0 {
				fontSize = 14
			}
			L.Push(lua.LNumber(fontSize))
			L.Push(lua.LString(""))
			return 3
		},
		"SetClampRectInsets": func(L *lua.LState, w *widget) int { return 0 },
		"SetMinResize":       func(L *lua.LState, w *widget) int { return 0 },
		"SetResizable":       func(L *lua.LState, w *widget) int { return 0 },
		"StartMoving":        func(L *lua.LState, w *widget) int { return 0 },
		"StopMovingOrSizing": func(L *lua.LState, w *widget) int { return 0 },
		"SetUserPlaced":      func(L *lua.LState, w *widget) int { return 0 },
		"IsUserPlaced":       func(L *lua.LState, w *widget) int { L.Push(lua.LFalse); return 1 },
		"SetAttribute": func(L *lua.LState, w *widget) int {
			attribute := L.CheckString(2)
			value := L.Get(3)
			w.ensureFields(L).RawSetString(attribute, value)
			rt.fire(w, "OnAttributeChanged", []lua.LValue{w.luaValue(L), lua.LString(attribute), value})
			return 0
		},
		"GetAttribute": func(L *lua.LState, w *widget) int {
			if w.fields == nil {
				L.Push(lua.LNil)
			} else {
				L.Push(w.fields.RawGet(L.Get(2)))
			}
			return 1
		},
		"SetNormalTexture": func(L *lua.LState, w *widget) int {
			w.normalTexture = rt.buttonTextureArg(w, w.normalTexture, L, 2)
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
			w.pushedTexture = rt.buttonTextureArg(w, w.pushedTexture, L, 2)
			return 0
		},
		"SetDisabledTexture": func(L *lua.LState, w *widget) int {
			w.disabledTexture = rt.buttonTextureArg(w, w.disabledTexture, L, 2)
			return 0
		},
		"GetDisabledTexture": func(L *lua.LState, w *widget) int {
			if w.disabledTexture != nil {
				L.Push(w.disabledTexture.luaValue(L))
			} else {
				L.Push(lua.LNil)
			}
			return 1
		},
		"SetHighlightTexture": func(L *lua.LState, w *widget) int {
			w.highlightTexture = rt.buttonTextureArg(w, w.highlightTexture, L, 2)
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
			} else if L.Get(2).Type() == lua.LTNumber && L.GetTop() >= 5 {
				w.textureFile = ""
				w.vertexColor = rgba{float64(L.CheckNumber(2)), float64(L.CheckNumber(3)), float64(L.CheckNumber(4)), float64(L.CheckNumber(5))}
			}
			return 0
		},
		"SetBlendMode": func(L *lua.LState, w *widget) int {
			w.blendMode = L.CheckString(2)
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
		"SetFont": func(L *lua.LState, w *widget) int {
			if L.GetTop() >= 3 && L.Get(3).Type() == lua.LTNumber {
				w.messageFontSize = float64(L.CheckNumber(3))
			}
			return 0
		},
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
		"SetSpacing":  func(L *lua.LState, w *widget) int { w.messageSpacing = float64(L.CheckNumber(2)); return 0 },
		"SetWordWrap": func(L *lua.LState, w *widget) int { return 0 },
		"GetStringWidth": func(L *lua.LState, w *widget) int {
			if w.kind == kindFontString && rt.measureText != nil {
				rt.measureText(w)
			}
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
		"SetFacing": func(L *lua.LState, w *widget) int {
			w.modelFacing = float64(L.CheckNumber(2))
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
		"SetPosition": func(L *lua.LState, w *widget) int {
			w.modelPosition[0] = float64(L.CheckNumber(2))
			w.modelPosition[1] = float64(L.CheckNumber(3))
			w.modelPosition[2] = float64(L.CheckNumber(4))
			return 0
		},
		"AdvanceTime": func(L *lua.LState, w *widget) int { return 0 },
		"StartMovie": func(L *lua.LState, w *widget) int {
			// Native FUN_0095eb30 returns 0 unless AVI load + DivxDecoder init succeed.
			file := L.CheckString(2)
			volume := 255
			if L.GetTop() >= 3 {
				volume = L.CheckInt(3)
			}
			w.movieFile = file
			w.movieVolume = volume
			w.movieActive = true
			ok := true
			if rt.validateMovie != nil {
				ok = rt.validateMovie(file, float64(volume)/255)
			}
			if !ok {
				w.movieActive = false
				L.Push(lua.LFalse)
				return 1
			}
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
					w.owner = owner
					w.parent = owner
				}
			}
			return 0
		},
		"GetOwner": func(L *lua.LState, w *widget) int {
			if w.owner != nil {
				L.Push(w.owner.luaValue(L))
			} else {
				L.Push(lua.LNil)
			}
			return 1
		},
		"IsOwned": func(L *lua.LState, w *widget) int {
			owner, ok := L.Get(2).(*lua.LUserData)
			if !ok {
				L.Push(lua.LFalse)
				return 1
			}
			ownerWidget, ok := owner.Value.(*widget)
			L.Push(lua.LBool(ok && w.parent == ownerWidget))
			return 1
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
// texture widget reference. Path strings create an unparented texture with no
// layout; prefer buttonTextureArg for Button/Slider Set*Texture APIs.
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

// buttonTextureArg applies SetNormalTexture-style arguments the way the
// original client does: a path updates the existing region (preserving
// TexCoords/alphaMode/fill anchors) or creates a setAllPoints child texture.
func (rt *Runtime) buttonTextureArg(button, existing *widget, L *lua.LState, idx int) *widget {
	switch v := L.Get(idx).(type) {
	case lua.LString:
		path := v.String()
		if existing != nil {
			existing.textureFile = path
			existing.shown = true
			if existing.parent == nil {
				existing.parent = button
			}
			if len(existing.points) == 0 && existing.width == 0 && existing.height == 0 {
				existing.points = []anchorPoint{
					{point: "TOPLEFT", relativePoint: "TOPLEFT"},
					{point: "BOTTOMRIGHT", relativePoint: "BOTTOMRIGHT"},
				}
			}
			return existing
		}
		tex := newWidget(kindTexture, "")
		tex.parent = button
		tex.textureFile = path
		tex.points = []anchorPoint{
			{point: "TOPLEFT", relativePoint: "TOPLEFT"},
			{point: "BOTTOMRIGHT", relativePoint: "BOTTOMRIGHT"},
		}
		return tex
	case *lua.LUserData:
		if w, ok := v.Value.(*widget); ok {
			return w
		}
	}
	return existing
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
