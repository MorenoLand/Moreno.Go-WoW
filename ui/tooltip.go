package ui

import (
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// tooltipLine is one row of a GameTooltip: the text in its TextLeftN and
// TextRightN font strings and the colours they were added with.
type tooltipLine struct {
	left, right string
	leftColor   rgba
	rightColor  rgba
	hasLeft     bool
	hasRight    bool
	wrap        bool
}

// tooltipState is the content and placement of one GameTooltip frame.
type tooltipState struct {
	owner       *widget
	anchor      string
	anchorX     float64
	anchorY     float64
	lines       []tooltipLine
	minWidth    float64
	padding     float64
	unit        string
	itemLink    string
	spellID     uint32
	spellKind   string
	hasContents bool
}

// Layout constants from the GameTooltipTemplate XML: the first line is
// anchored 10 units inside the frame, lines are two units apart and the right
// column starts 40 units after the left text.
const (
	tooltipEdge       = 10
	tooltipLineGap    = 2
	tooltipColumnGap  = 40
	tooltipWrapWidth  = 275
	tooltipFirstLines = 8
)

func (w *widget) tooltip() *tooltipState {
	if w.tip == nil {
		w.tip = &tooltipState{anchor: "ANCHOR_NONE"}
	}
	return w.tip
}

// tooltipFontString returns the TextLeftN / TextRightN font string, creating
// it the way the client does once the lines declared in XML are used up: the
// new left line hangs under the previous one and the right text sits in the
// right column of the same row.
func (rt *Runtime) tooltipFontString(tooltip *widget, side string, index int) *widget {
	name := fmt.Sprintf("%sText%s%d", tooltip.name, side, index)
	if existing := rt.widgets[name]; existing != nil {
		return existing
	}
	fs := newWidget(kindFontString, name)
	fs.parent = tooltip
	fs.frameStrata = tooltip.frameStrata
	fs.layerLevel = layerArtwork
	fs.fontObject = "GameTooltipText"
	fs.shown = false
	fs.autoTextWidth = true
	fs.autoTextHeight = true
	left := fmt.Sprintf("%sTextLeft%d", tooltip.name, index)
	if side == "Left" {
		fs.points = []anchorPoint{{point: "TOPLEFT", relativeTo: fmt.Sprintf("%sTextLeft%d", tooltip.name, index-1), relativePoint: "BOTTOMLEFT", x: 0, y: -tooltipLineGap}}
		fs.justifyH = "LEFT"
	} else {
		fs.points = []anchorPoint{{point: "RIGHT", relativeTo: left, relativePoint: "LEFT", x: tooltipColumnGap}}
		fs.justifyH = "RIGHT"
	}
	addWidgetChild(tooltip, fs)
	rt.register(fs)
	return fs
}

func (rt *Runtime) measureTooltipText(fs *widget) (width, height float64) {
	if rt.measureText != nil {
		rt.measureText(fs)
	}
	return fs.textWidth, fs.height
}

// layoutTooltip puts the lines into the tooltip's font strings and sizes the
// frame around them.
func (rt *Runtime) layoutTooltip(w *widget) {
	state := w.tooltip()
	maxWidth := 0.0
	wrapNatural := 0.0
	for index, line := range state.lines {
		left := rt.tooltipFontString(w, "Left", index+1)
		right := rt.tooltipFontString(w, "Right", index+1)
		left.text = line.left
		left.shown = true
		left.autoTextWidth = true
		left.explicitWidth = false
		left.width = 0
		if index == 0 && left.fontObject == "GameTooltipText" {
			left.fontObject = "GameTooltipHeaderText"
			right.fontObject = "GameTooltipHeaderText"
		}
		if line.hasLeft {
			left.textColor = line.leftColor
		}
		right.text = line.right
		right.shown = line.right != ""
		if line.hasRight {
			right.textColor = line.rightColor
		}
		leftWidth, _ := rt.measureTooltipText(left)
		rightWidth := 0.0
		if line.right != "" {
			rightWidth, _ = rt.measureTooltipText(right)
		}
		width := leftWidth
		if line.right != "" {
			width += tooltipColumnGap + rightWidth
		}
		if !line.wrap && width > maxWidth {
			maxWidth = width
		}
		if line.wrap && width > wrapNatural {
			wrapNatural = width
		}
	}
	textWidth := maxWidth
	// Wrapped text widens the tooltip only as far as it needs, up to the wrap
	// width.
	if wrapNatural > tooltipWrapWidth {
		wrapNatural = tooltipWrapWidth
	}
	if wrapNatural > textWidth {
		textWidth = wrapNatural
	}
	for index, line := range state.lines {
		if !line.wrap {
			continue
		}
		left := rt.tooltipFontString(w, "Left", index+1)
		left.autoTextWidth = false
		left.explicitWidth = true
		left.width = textWidth
		rt.measureTooltipText(left)
	}
	height := 0.0
	for index := range state.lines {
		left := rt.tooltipFontString(w, "Left", index+1)
		if index > 0 {
			height += tooltipLineGap
		}
		_, lineHeight := rt.measureTooltipText(left)
		height += lineHeight
	}
	// Lines that are no longer in use stay in the frame but hidden.
	for index := len(state.lines); ; index++ {
		left := rt.widgets[fmt.Sprintf("%sTextLeft%d", w.name, index+1)]
		if left == nil {
			break
		}
		left.shown = false
		left.text = ""
		if right := rt.widgets[fmt.Sprintf("%sTextRight%d", w.name, index+1)]; right != nil {
			right.shown = false
			right.text = ""
		}
	}
	padding := state.padding
	width := textWidth + 2*tooltipEdge + padding
	if width < state.minWidth {
		width = state.minWidth
	}
	w.width = width
	w.height = height + 2*tooltipEdge
	// The right text of each row sits in the right column of the frame, level
	// with its left text.
	rowTop := float64(tooltipEdge)
	for index := range state.lines {
		left := rt.tooltipFontString(w, "Left", index+1)
		right := rt.tooltipFontString(w, "Right", index+1)
		_, rowHeight := rt.measureTooltipText(left)
		center := rowTop + rowHeight/2
		right.points = []anchorPoint{{point: "RIGHT", relativeTo: w.name, relativePoint: "RIGHT", x: -tooltipEdge, y: -(center - w.height/2)}}
		rowTop += rowHeight + tooltipLineGap
	}
	for index, line := range state.lines {
		if !line.wrap {
			left := rt.tooltipFontString(w, "Left", index+1)
			left.width = 0
		}
	}
}

// placeTooltip anchors the tooltip next to its owner according to the anchor
// type SetOwner was given.
func (rt *Runtime) placeTooltip(w *widget) {
	state := w.tooltip()
	if state.owner == nil {
		return
	}
	var point, relative string
	switch state.anchor {
	case "ANCHOR_RIGHT":
		point, relative = "BOTTOMLEFT", "TOPRIGHT"
	case "ANCHOR_LEFT":
		point, relative = "BOTTOMRIGHT", "TOPLEFT"
	case "ANCHOR_TOP":
		point, relative = "BOTTOM", "TOP"
	case "ANCHOR_BOTTOM":
		point, relative = "TOP", "BOTTOM"
	case "ANCHOR_TOPRIGHT":
		point, relative = "BOTTOMRIGHT", "TOPRIGHT"
	case "ANCHOR_TOPLEFT":
		point, relative = "BOTTOMLEFT", "TOPLEFT"
	case "ANCHOR_BOTTOMRIGHT":
		point, relative = "TOPRIGHT", "BOTTOMRIGHT"
	case "ANCHOR_BOTTOMLEFT":
		point, relative = "TOPLEFT", "BOTTOMLEFT"
	case "ANCHOR_CURSOR":
		point, relative = "BOTTOMLEFT", "BOTTOMLEFT"
		w.points = []anchorPoint{{point: point, relativeTo: "UIParent", relativePoint: relative, x: state.anchorX, y: state.anchorY}}
		return
	default:
		return
	}
	w.points = []anchorPoint{{point: point, relativeTo: state.owner.name, relativePoint: relative, x: state.anchorX, y: state.anchorY}}
}

func (rt *Runtime) clearTooltip(w *widget) {
	state := w.tooltip()
	state.lines = nil
	state.unit, state.itemLink, state.spellID, state.spellKind = "", "", 0, ""
	state.hasContents = false
	rt.layoutTooltip(w)
	rt.fire(w, "OnTooltipCleared", []lua.LValue{w.luaValue(rt.L)})
}

func (rt *Runtime) tooltipAddLine(w *widget, line tooltipLine) {
	state := w.tooltip()
	state.lines = append(state.lines, line)
	state.hasContents = true
}

// showTooltip lays the tooltip out and shows it if it has anything to say.
func (rt *Runtime) showTooltip(w *widget) {
	state := w.tooltip()
	if len(state.lines) == 0 {
		return
	}
	rt.layoutTooltip(w)
	rt.placeTooltip(w)
	if !w.shown {
		w.shown = true
		if w.visibleThroughParents() {
			rt.fireShowCascade(w)
		}
	}
}

func luaColorArgs(L *lua.LState, index int) (rgba, bool) {
	if L.GetTop() < index+2 || L.Get(index).Type() != lua.LTNumber {
		return rgba{}, false
	}
	return rgba{r: float64(L.CheckNumber(index)), g: float64(L.CheckNumber(index + 1)), b: float64(L.CheckNumber(index + 2)), a: 1}, true
}

func luaOptionalBool(L *lua.LState, index int) bool {
	value := L.Get(index)
	return value != lua.LNil && value != lua.LFalse
}

// tooltipMethods are the GameTooltip frame methods.
func (rt *Runtime) tooltipMethods() map[string]func(L *lua.LState, w *widget) int {
	methods := map[string]func(L *lua.LState, w *widget) int{
		"SetOwner": func(L *lua.LState, w *widget) int {
			owner := rt.widgetArg(L, 2)
			if owner == nil {
				L.ArgError(2, "frame expected")
				return 0
			}
			if owner == w {
				L.RaiseError("%s:SetOwner(): Can't set owner to self", w.name)
				return 0
			}
			state := w.tooltip()
			rt.clearTooltip(w)
			state.owner = owner
			state.anchor = L.OptString(3, "ANCHOR_RIGHT")
			state.anchorX = float64(L.OptNumber(4, 0))
			state.anchorY = float64(L.OptNumber(5, 0))
			return 0
		},
		"GetOwner": func(L *lua.LState, w *widget) int {
			if owner := w.tooltip().owner; owner != nil {
				L.Push(owner.luaValue(L))
			} else {
				L.Push(lua.LNil)
			}
			return 1
		},
		"IsOwned": func(L *lua.LState, w *widget) int {
			owner := rt.widgetArg(L, 2)
			L.Push(lua.LBool(owner != nil && w.tooltip().owner == owner))
			return 1
		},
		"SetAnchorType": func(L *lua.LState, w *widget) int {
			state := w.tooltip()
			state.anchor = L.CheckString(2)
			state.anchorX = float64(L.OptNumber(3, 0))
			state.anchorY = float64(L.OptNumber(4, 0))
			rt.placeTooltip(w)
			return 0
		},
		"GetAnchorType": func(L *lua.LState, w *widget) int {
			state := w.tooltip()
			L.Push(lua.LString(state.anchor))
			L.Push(lua.LNumber(state.anchorX))
			L.Push(lua.LNumber(state.anchorY))
			return 3
		},
		"ClearLines": func(L *lua.LState, w *widget) int {
			rt.clearTooltip(w)
			return 0
		},
		"NumLines": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(len(w.tooltip().lines)))
			return 1
		},
		"SetText": func(L *lua.LState, w *widget) int {
			rt.clearTooltip(w)
			line := tooltipLine{left: L.OptString(2, ""), wrap: luaOptionalBool(L, 7)}
			if color, ok := luaColorArgs(L, 3); ok {
				line.leftColor, line.hasLeft = color, true
			}
			line.wrap = luaOptionalBool(L, 6)
			rt.tooltipAddLine(w, line)
			rt.showTooltip(w)
			return 0
		},
		"AddLine": func(L *lua.LState, w *widget) int {
			line := tooltipLine{left: L.OptString(2, ""), wrap: luaOptionalBool(L, 6)}
			if color, ok := luaColorArgs(L, 3); ok {
				line.leftColor, line.hasLeft = color, true
			}
			rt.tooltipAddLine(w, line)
			return 0
		},
		"AddDoubleLine": func(L *lua.LState, w *widget) int {
			line := tooltipLine{left: L.OptString(2, ""), right: L.OptString(3, "")}
			if color, ok := luaColorArgs(L, 4); ok {
				line.leftColor, line.hasLeft = color, true
			}
			if color, ok := luaColorArgs(L, 7); ok {
				line.rightColor, line.hasRight = color, true
			}
			rt.tooltipAddLine(w, line)
			return 0
		},
		"AppendText": func(L *lua.LState, w *widget) int {
			state := w.tooltip()
			if len(state.lines) > 0 {
				state.lines[len(state.lines)-1].left += L.OptString(2, "")
			}
			return 0
		},
		"SetMinimumWidth": func(L *lua.LState, w *widget) int {
			w.tooltip().minWidth = float64(L.OptNumber(2, 0))
			return 0
		},
		"SetPadding": func(L *lua.LState, w *widget) int {
			w.tooltip().padding = float64(L.OptNumber(2, 0))
			return 0
		},
		"FadeOut": func(L *lua.LState, w *widget) int {
			rt.hideWidget(w)
			return 0
		},
		"AddFontStrings": func(L *lua.LState, w *widget) int { return 0 },
		"AddTexture":     func(L *lua.LState, w *widget) int { return 0 },
		"GetUnit": func(L *lua.LState, w *widget) int {
			state := w.tooltip()
			if state.unit == "" {
				L.Push(lua.LNil)
				return 1
			}
			info := rt.unitInfo(state.unit)
			if info == nil {
				L.Push(lua.LNil)
				return 1
			}
			L.Push(lua.LString(info.Name))
			L.Push(lua.LString(state.unit))
			return 2
		},
		"IsUnit": func(L *lua.LState, w *widget) int {
			state := w.tooltip()
			token := L.CheckString(2)
			L.Push(lua.LBool(state.unit != "" && (state.unit == token || rt.unitsAreSame(state.unit, token))))
			return 1
		},
		"GetItem": func(L *lua.LState, w *widget) int {
			state := w.tooltip()
			if state.itemLink == "" {
				L.Push(lua.LNil)
				return 1
			}
			name, link := rt.itemNameAndLink(state.itemLink)
			L.Push(lua.LString(name))
			L.Push(lua.LString(link))
			return 2
		},
		"GetSpell": func(L *lua.LState, w *widget) int {
			state := w.tooltip()
			if state.spellID == 0 {
				L.Push(lua.LNil)
				return 1
			}
			info := rt.gameplay.Catalog[state.spellID]
			L.Push(lua.LString(info.Name))
			L.Push(lua.LString(info.Rank))
			L.Push(lua.LNumber(state.spellID))
			return 3
		},
		"IsEquippedItem": func(L *lua.LState, w *widget) int {
			L.Push(lua.LBool(w.tooltip().spellKind == "equipped"))
			return 1
		},
	}
	for name, fn := range rt.tooltipContentMethods() {
		methods[name] = fn
	}
	return methods
}

// widgetArg returns the widget passed as Lua argument index, or nil.
func (rt *Runtime) widgetArg(L *lua.LState, index int) *widget {
	if ud, ok := L.Get(index).(*lua.LUserData); ok {
		if w, ok := ud.Value.(*widget); ok {
			return w
		}
	}
	return nil
}

func (rt *Runtime) hideWidget(w *widget) {
	if !w.shown {
		return
	}
	visible := w.visibleThroughParents()
	w.shown = false
	if visible {
		rt.fireHideCascade(w)
	}
}

func (rt *Runtime) unitsAreSame(a, b string) bool {
	left, right := rt.unitInfo(a), rt.unitInfo(b)
	return left != nil && right != nil && left.Exists && right.Exists && left.GUID != 0 && left.GUID == right.GUID
}

func (rt *Runtime) itemNameAndLink(link string) (string, string) {
	if start := strings.Index(link, "["); start >= 0 {
		if end := strings.Index(link[start:], "]"); end > 0 {
			return link[start+1 : start+end], link
		}
	}
	return link, link
}
