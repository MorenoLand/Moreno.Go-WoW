package ui

import "image"

// Layout resolves widget geometry using the anchor rules of the original
// interface system: each anchor attaches a named point of the widget to a
// named point of another widget (default: the parent), with offsets.
// Widgets with a size but no anchors center on their parent; widgets with
// setAllPoints (TOPLEFT+BOTTOMRIGHT with empty relativeTo) fill their parent;
// relativeTo fill anchors resolve against the named region instead.

// Rect is a resolved screen-space rectangle. Y grows downward to match the
// interface coordinate system (BOTTOMLEFT is the origin of the client's UI
// space; callers convert when drawing).
type Rect struct {
	X0, Y0, X1, Y1 float64 // top-left, bottom-right in client UI space (Y up)
}

func (r Rect) W() float64 { return r.X1 - r.X0 }
func (r Rect) H() float64 { return r.Y1 - r.Y0 }

// pointFactor returns the unit-space position of a named anchor point
// within a rect: (x, y) in [0,1], with y=0 at the bottom.
func pointFactor(point string) (float64, float64) {
	switch point {
	case "TOPLEFT":
		return 0, 1
	case "TOP":
		return 0.5, 1
	case "TOPRIGHT":
		return 1, 1
	case "LEFT":
		return 0, 0.5
	case "CENTER":
		return 0.5, 0.5
	case "RIGHT":
		return 1, 0.5
	case "BOTTOMLEFT":
		return 0, 0
	case "BOTTOM":
		return 0.5, 0
	case "BOTTOMRIGHT":
		return 1, 0
	}
	return 0.5, 0.5
}

// ResolveRect computes the layout rect of a widget against its parent rect.
func ResolveRect(w *widget, parent Rect) Rect {
	return resolveRect(w, parent, nil)
}

func resolveRect(w *widget, parent Rect, relative func(string) (Rect, bool)) Rect {
	// setAllPoints or explicit fill anchors against the parent only.
	// TOPLEFT+BOTTOMRIGHT with a relativeTo (e.g. ChatFrameTab SelectedMid
	// stretching to TabMiddle) must resolve through the normal anchor path.
	e := effectiveScaleOf(w)
	width, height := w.width*e, w.height*e
	if len(w.points) == 0 {
		cx := (parent.X0 + parent.X1) / 2
		cy := (parent.Y0 + parent.Y1) / 2
		return Rect{cx - width/2, cy - height/2, cx + width/2, cy + height/2}
	}
	if len(w.points) == 2 && w.points[0].point == "TOPLEFT" && w.points[1].point == "BOTTOMRIGHT" && w.points[0].relativePoint == "TOPLEFT" && w.points[1].relativePoint == "BOTTOMRIGHT" && w.points[0].x == 0 && w.points[0].y == 0 && w.points[1].x == 0 && w.points[1].y == 0 && w.points[0].relativeTo == "" && w.points[1].relativeTo == "" {
		return parent
	}

	// Compute each anchor's absolute position, then derive the rect from
	// the anchors present plus the widget size.
	anchorPos := make(map[string][2]float64)
	for _, p := range w.points {
		base := parent
		if p.relativeTo != "" && relative != nil {
			if target, ok := relative(p.relativeTo); ok {
				base = target
			}
		}
		fx, fy := pointFactor(p.point)
		gx, gy := pointFactor(p.relativePoint)
		if p.relativePoint == "" {
			gx, gy = fx, fy
		}
		ax := base.X0 + gx*base.W() + p.x*e
		ay := base.Y0 + gy*base.H() + p.y*e
		anchorPos[p.point] = [2]float64{ax, ay}
	}

	rect := Rect{}
	x0Set, x1Set, y0Set, y1Set := false, false, false, false
	// Resolve corner anchors so an offset on one side is not overwritten by the
	// opposite corner (OptionsFrameListTemplate spacer uses TOPLEFT y=7 /
	// BOTTOMLEFT y=-2 while the other corner stays at 0).
	if v, ok := anchorPos["TOPLEFT"]; ok {
		rect.X0, rect.Y1 = v[0], v[1]
		x0Set, y1Set = true, true
	}
	if v, ok := anchorPos["BOTTOMLEFT"]; ok {
		if !x0Set {
			rect.X0 = v[0]
			x0Set = true
		}
		rect.Y0 = v[1]
		y0Set = true
	}
	if v, ok := anchorPos["TOPRIGHT"]; ok {
		rect.X1 = v[0]
		x1Set = true
		if !y1Set {
			rect.Y1 = v[1]
			y1Set = true
		}
	}
	if v, ok := anchorPos["BOTTOMRIGHT"]; ok {
		if !x1Set {
			rect.X1 = v[0]
			x1Set = true
		}
		if !y0Set {
			rect.Y0 = v[1]
			y0Set = true
		}
	}
	if v, ok := anchorPos["LEFT"]; ok {
		rect.X0 = v[0]
		x0Set = true
	}
	if v, ok := anchorPos["RIGHT"]; ok {
		rect.X1 = v[0]
		x1Set = true
	}
	if v, ok := anchorPos["TOP"]; ok {
		rect.Y1 = v[1]
		y1Set = true
	}
	if v, ok := anchorPos["BOTTOM"]; ok {
		rect.Y0 = v[1]
		y0Set = true
	}
	if v, ok := anchorPos["CENTER"]; ok {
		if !x0Set && !x1Set {
			rect.X0, rect.X1 = v[0]-width/2, v[0]+width/2
			x0Set, x1Set = true, true
		}
		if !y0Set && !y1Set {
			rect.Y0, rect.Y1 = v[1]-height/2, v[1]+height/2
			y0Set, y1Set = true, true
		}
	}

	if !x0Set && !x1Set {
		aligned := false
		for _, point := range []string{"TOP", "BOTTOM", "CENTER"} {
			if v, ok := anchorPos[point]; ok {
				rect.X0, rect.X1 = v[0]-width/2, v[0]+width/2
				aligned = true
				break
			}
		}
		if !aligned {
			cx := (parent.X0 + parent.X1) / 2
			rect.X0, rect.X1 = cx-width/2, cx+width/2
		}
	} else if x0Set && !x1Set {
		rect.X1 = rect.X0 + width
	} else if !x0Set && x1Set {
		rect.X0 = rect.X1 - width
	}

	if !y0Set && !y1Set {
		aligned := false
		for _, point := range []string{"LEFT", "RIGHT", "CENTER"} {
			if v, ok := anchorPos[point]; ok {
				rect.Y0, rect.Y1 = v[1]-height/2, v[1]+height/2
				aligned = true
				break
			}
		}
		if !aligned {
			cy := (parent.Y0 + parent.Y1) / 2
			rect.Y0, rect.Y1 = cy-height/2, cy+height/2
		}
	} else if y0Set && !y1Set {
		rect.Y1 = rect.Y0 + height
	} else if !y0Set && y1Set {
		rect.Y0 = rect.Y1 - height
	}
	if rect.X1 < rect.X0 {
		rect.X0, rect.X1 = rect.X1, rect.X0
	}
	if rect.Y1 < rect.Y0 {
		rect.Y0, rect.Y1 = rect.Y1, rect.Y0
	}
	return rect
}

func scaleRect(rect Rect, scale float64) Rect {
	if scale <= 0 || scale == 1 {
		return rect
	}
	cx := (rect.X0 + rect.X1) / 2
	cy := (rect.Y0 + rect.Y1) / 2
	halfWidth := (rect.X1 - rect.X0) * scale / 2
	halfHeight := (rect.Y1 - rect.Y0) * scale / 2
	return Rect{X0: cx - halfWidth, Y0: cy - halfHeight, X1: cx + halfWidth, Y1: cy + halfHeight}
}

// ScreenRect converts a client-space rect (Y up, BOTTOMLEFT origin) to an
// image rect (Y down, TOPLEFT origin) for the given screen height.
func ScreenRect(r Rect, screenHeight float64) image.Rectangle {
	return image.Rect(
		int(r.X0), int(screenHeight-r.Y1),
		int(r.X1), int(screenHeight-r.Y0),
	)
}

// effectiveScaleOf is the widget's scale multiplied through its ancestors,
// leaving out the roots whose scale is the screen's (the engine's UI scale
// already maps UI units to pixels). Sizes and offsets of a widget are in its
// own units, which are this many root units each.
func effectiveScaleOf(w *widget) float64 {
	scale := 1.0
	for current := w; current != nil; current = current.parent {
		switch current.name {
		case "UIParent", "GlueParent", "WorldFrame":
			return scale
		}
		if current.scale > 0 {
			scale *= current.scale
		}
	}
	return scale
}

// pxUnit is how many pixels one unit of the widget being painted is wide:
// the UI scale times the widget's effective scale.
func (eng *UIEngine) pxUnit() float64 {
	if eng.curScale <= 0 {
		return eng.uiScale
	}
	return eng.uiScale * eng.curScale
}

// onDemandRect lays a widget out from the screen down, without disturbing the
// layout cache of the frame being painted, so scripts can ask for GetLeft and
// friends at any time.
func (eng *UIEngine) onDemandRect(w *widget) (Rect, bool) {
	if eng == nil || w == nil {
		return Rect{}, false
	}
	screen := eng.screen
	if screen.W() <= 0 {
		// Nothing has been painted yet: use the host's screen, or the 4:3 default.
		screen = Rect{X1: 1024, Y1: 768}
		if eng.Rt != nil && eng.Rt.Host != nil {
			if width, height := eng.Rt.Host.ScreenSize(); width > 0 && height > 0 {
				screen.X1 = 768 * width / height
			}
		}
	}
	saved, savedActive := eng.rects, eng.layoutActive
	eng.rects, eng.layoutActive = make(map[*widget]Rect), make(map[*widget]bool)
	defer func() { eng.rects, eng.layoutActive = saved, savedActive }()
	var root *widget
	for current := w; current != nil; current = current.parent {
		root = current
	}
	if root == nil || (root.name != "UIParent" && root.name != "GlueParent" && root != w) {
		return Rect{}, false
	}
	return eng.layoutRect(w, screen), true
}
