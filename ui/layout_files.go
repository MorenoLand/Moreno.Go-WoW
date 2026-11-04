package ui

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// layoutPoints are the anchor points of a frame in the order the client numbers
// them.
var layoutPoints = [9]string{"TOPLEFT", "TOP", "TOPRIGHT", "LEFT", "CENTER", "RIGHT", "BOTTOMLEFT", "BOTTOM", "BOTTOMRIGHT"}

func layoutPointIndex(name string) int {
	for index, point := range layoutPoints {
		if strings.EqualFold(point, name) {
			return index
		}
	}
	return -1
}

// rectPoints are the nine anchor positions of a rectangle, numbered as above.
func rectPoints(rect Rect) [9][2]float64 {
	midX, midY := (rect.X0+rect.X1)/2, (rect.Y0+rect.Y1)/2
	return [9][2]float64{
		{rect.X0, rect.Y1}, {midX, rect.Y1}, {rect.X1, rect.Y1},
		{rect.X0, midY}, {midX, midY}, {rect.X1, midY},
		{rect.X0, rect.Y0}, {midX, rect.Y0}, {rect.X1, rect.Y0},
	}
}

// nearestAnchor picks the anchor point of the frame that lies nearest the same
// point of the screen, and how far the frame's point is from it.
func nearestAnchor(frame, screen Rect) (index int, dx, dy float64) {
	own, base := rectPoints(frame), rectPoints(screen)
	best := math.MaxFloat32
	for point := range own {
		x, y := own[point][0]-base[point][0], own[point][1]-base[point][1]
		if distance := float64(float32(y*y + x*x)); distance < best {
			index, dx, dy, best = point, x, y, distance
		}
	}
	return index, dx, dy
}

// screenRect is the area the frames are placed against.
func (rt *Runtime) screenRect() (Rect, bool) {
	root := rt.widgets["UIParent"]
	if root == nil || rt.layoutOf == nil {
		return Rect{}, false
	}
	return rt.layoutOf(root)
}

func frameLabel(w *widget) string {
	if w.name == "" {
		return "<unnamed>"
	}
	return w.name
}

// frameDrag is the frame being moved or resized by the mouse.
type frameDrag struct {
	frame        *widget
	mode         int
	lastX, lastY float64
}

// anchorToScreen holds the frame by its top left corner at where it is now,
// shifted by the given amounts and with the given size.
func (rt *Runtime) anchorToScreen(w *widget, shiftX, shiftY, width, height float64) bool {
	if rt.layoutOf == nil {
		return false
	}
	frame, ok := rt.layoutOf(w)
	screen, okScreen := rt.screenRect()
	if !ok || !okScreen {
		return false
	}
	scale := effectiveScaleOf(w)
	if scale <= 0 {
		scale = 1
	}
	if width == 0 {
		width = frame.W() / scale
	}
	if height == 0 {
		height = frame.H() / scale
	}
	w.points = []anchorPoint{{
		point: "TOPLEFT", relativeTo: "UIParent", relativePoint: "TOPLEFT",
		x: (frame.X0 + shiftX - screen.X0) / scale,
		y: (frame.Y1 + shiftY - screen.Y1) / scale,
	}}
	w.unanchored = false
	w.width, w.height = width, height
	return true
}

// settleFrame puts the frame back on the anchor point nearest its screen
// counterpart, as it is when the mouse lets it go.
func (rt *Runtime) settleFrame(w *widget) {
	frame, ok := rt.layoutOf(w)
	screen, okScreen := rt.screenRect()
	if !ok || !okScreen {
		return
	}
	scale := effectiveScaleOf(w)
	if scale <= 0 {
		scale = 1
	}
	index, dx, dy := nearestAnchor(frame, screen)
	w.points = []anchorPoint{{
		point: layoutPoints[index], relativeTo: "UIParent", relativePoint: layoutPoints[index],
		x: dx / scale, y: dy / scale,
	}}
	w.unanchored = false
}

// startDrag begins moving (mode 4) or resizing the frame by one of its points.
func (rt *Runtime) startDrag(w *widget, mode int) {
	if rt.frameDrag.frame != nil || rt.cursorUnits == nil {
		return
	}
	if !rt.anchorToScreen(w, 0, 0, 0, 0) {
		return
	}
	w.userPlaced = true
	rt.frameDrag = frameDrag{frame: w, mode: mode}
	rt.frameDrag.lastX, rt.frameDrag.lastY = rt.cursorUnits()
}

// stopDrag ends the move or resize of a frame.
func (rt *Runtime) stopDrag(w *widget) {
	if rt.frameDrag.frame != w {
		return
	}
	rt.frameDrag = frameDrag{}
	rt.settleFrame(w)
}

// dragCursor carries the frame along with the cursor.
func (rt *Runtime) dragCursor() {
	frame := rt.frameDrag.frame
	if frame == nil || rt.cursorUnits == nil {
		return
	}
	x, y := rt.cursorUnits()
	dx, dy := x-rt.frameDrag.lastX, y-rt.frameDrag.lastY
	if dx == 0 && dy == 0 {
		return
	}
	rt.dragFrame(frame, rt.frameDrag.mode, dx, dy)
	rt.frameDrag.lastX += dx
	rt.frameDrag.lastY += dy
}

// dragFrame applies a cursor movement to the frame the way the client does for
// each point being dragged.
func (rt *Runtime) dragFrame(w *widget, mode int, dx, dy float64) {
	scale := effectiveScaleOf(w)
	if scale <= 0 {
		scale = 1
	}
	if mode == 4 {
		if len(w.points) > 0 {
			w.points[0].x += dx / scale
			w.points[0].y += dy / scale
		}
		return
	}
	width, height := w.width, w.height
	if frame, ok := rt.layoutOf(w); ok {
		width, height = frame.W()/scale, frame.H()/scale
	}
	minW, maxW, minH, maxH := w.minResizeW, w.maxResizeW, w.minResizeH, w.maxResizeH
	shiftX, shiftY := 0.0, 0.0
	newW, newH := width, height
	leftEdge := mode == 0 || mode == 3 || mode == 6
	rightEdge := mode == 2 || mode == 5 || mode == 8
	topEdge := mode == 0 || mode == 1 || mode == 2
	bottomEdge := mode == 6 || mode == 7 || mode == 8
	if leftEdge {
		shiftX = dx
		newW = width - dx/scale
		if minW != 0 && newW < minW {
			shiftX -= (minW - newW) * scale
			newW = minW
		}
		if maxW != 0 && maxW < newW {
			shiftX += (newW - maxW) * scale
			newW = maxW
		}
	}
	if rightEdge {
		newW = width + dx/scale
		if minW != 0 && newW < minW {
			newW = minW
		}
		if maxW != 0 && maxW < newW {
			newW = maxW
		}
	}
	if topEdge {
		shiftY = dy
		newH = height + dy/scale
		if minH != 0 && newH < minH {
			shiftY += (minH - newH) * scale
			newH = minH
		}
		if maxH != 0 && maxH < newH {
			shiftY -= (newH - maxH) * scale
			newH = maxH
		}
	}
	if bottomEdge {
		newH = height - dy/scale
		if minH != 0 && newH < minH {
			newH = minH
		}
		if maxH != 0 && maxH < newH {
			newH = maxH
		}
	}
	if !leftEdge && !rightEdge && !topEdge && !bottomEdge {
		return
	}
	rt.anchorToScreen(w, shiftX, shiftY, newW, newH)
}

// layoutFile is the file the places of the frames the player moved are kept in.
func (rt *Runtime) layoutFile() string {
	folder := rt.characterFolder()
	if folder == "" {
		return ""
	}
	return filepath.Join(folder, "layout-local.txt")
}

// userPlacedFrames are the named frames the player moved or resized, in the
// order they were made.
func (rt *Runtime) userPlacedFrames() []*widget {
	var frames []*widget
	for _, w := range rt.widgets {
		if w.name != "" && w.userPlaced && !w.dontSavePosition && (w.movable || w.resizable) {
			frames = append(frames, w)
		}
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i].seq < frames[j].seq })
	return frames
}

// saveLayoutFile keeps the places of the moved frames, and the file it replaces
// as a backup.
func (rt *Runtime) saveLayoutFile() {
	path := rt.layoutFile()
	if path == "" || rt.layoutOf == nil {
		return
	}
	screen, ok := rt.screenRect()
	if !ok {
		return
	}
	var out strings.Builder
	out.WriteString("Version: 1\n")
	for _, w := range rt.userPlacedFrames() {
		frame, ok := rt.layoutOf(w)
		if !ok {
			continue
		}
		scale := effectiveScaleOf(w)
		if scale <= 0 {
			scale = 1
		}
		index, dx, dy := nearestAnchor(frame, screen)
		fmt.Fprintf(&out, "Frame: %s\n", w.name)
		fmt.Fprintf(&out, "FrameLevel: %d\n", w.frameLevel)
		fmt.Fprintf(&out, "Anchor: %s\n", layoutPoints[index])
		fmt.Fprintf(&out, "X: %d\n", int(math.RoundToEven(dx/scale)))
		fmt.Fprintf(&out, "Y: %d\n", int(math.RoundToEven(dy/scale)))
		fmt.Fprintf(&out, "W: %d\n", int(math.RoundToEven(frame.W()/scale)))
		fmt.Fprintf(&out, "H: %d\n", int(math.RoundToEven(frame.H()/scale)))
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if old, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(strings.TrimSuffix(path, ".txt")+".old", old, 0o644)
	}
	_ = os.WriteFile(path, []byte(out.String()), 0o644)
}

// loadLayoutFile puts the frames back where the file says they were.
func (rt *Runtime) loadLayoutFile() {
	path := rt.layoutFile()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var (
		frame               *widget
		level               = -1
		anchor              int
		x, y, width, height int
	)
	apply := func() {
		if frame == nil {
			return
		}
		if level >= 0 {
			frame.frameLevel = level
		}
		if frame.movable {
			frame.points = []anchorPoint{{
				point: layoutPoints[anchor], relativeTo: "UIParent", relativePoint: layoutPoints[anchor],
				x: float64(x), y: float64(y),
			}}
			frame.unanchored = false
			frame.userPlaced = true
		}
		if frame.resizable {
			frame.width, frame.height = float64(width), float64(height)
			frame.userPlaced = true
		}
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "Version: "):
		case strings.HasPrefix(line, "Frame: "):
			apply()
			frame, level, anchor, x, y, width, height = rt.widgets[line[len("Frame: "):]], -1, 0, 0, 0, 0, 0
		case strings.HasPrefix(line, "FrameLevel: "):
			level, _ = strconv.Atoi(line[len("FrameLevel: "):])
		case strings.HasPrefix(line, "Anchor: "):
			if index := layoutPointIndex(line[len("Anchor: "):]); index >= 0 {
				anchor = index
			}
		case strings.HasPrefix(line, "X: "):
			x, _ = strconv.Atoi(line[len("X: "):])
		case strings.HasPrefix(line, "Y: "):
			y, _ = strconv.Atoi(line[len("Y: "):])
		case strings.HasPrefix(line, "W: "):
			width, _ = strconv.Atoi(line[len("W: "):])
		case strings.HasPrefix(line, "H: "):
			height, _ = strconv.Atoi(line[len("H: "):])
		}
	}
	apply()
}

// layoutMethods are the calls that move and size frames and say whether the
// player placed them.
func (rt *Runtime) layoutMethods() map[string]func(L *lua.LState, w *widget) int {
	methods := map[string]func(L *lua.LState, w *widget) int{}
	raise := func(L *lua.LState, w *widget, format string) {
		L.RaiseError(format, frameLabel(w))
	}
	method := func(name string, fn func(L *lua.LState, w *widget) int) {
		methods[name] = fn
	}
	method("StartMoving", func(L *lua.LState, w *widget) int {
		if !w.movable {
			raise(L, w, "Frame %s is not movable")
			return 0
		}
		rt.startDrag(w, 4)
		return 0
	})
	method("StartSizing", func(L *lua.LState, w *widget) int {
		if !w.resizable {
			raise(L, w, "Frame %s is not resizable")
			return 0
		}
		mode := 8
		if L.GetTop() >= 2 && L.Get(2) != lua.LNil {
			if index := layoutPointIndex(L.CheckString(2)); index >= 0 {
				mode = index
			}
		}
		rt.startDrag(w, mode)
		return 0
	})
	method("StopMovingOrSizing", func(L *lua.LState, w *widget) int {
		rt.stopDrag(w)
		return 0
	})
	method("SetUserPlaced", func(L *lua.LState, w *widget) int {
		if !w.movable && !w.resizable {
			raise(L, w, "Frame %s is not movable or resizable")
			return 0
		}
		w.userPlaced = luaTruthyOpt(L, 2, true)
		return 0
	})
	method("IsUserPlaced", func(L *lua.LState, w *widget) int {
		if w.userPlaced {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	method("SetDontSavePosition", func(L *lua.LState, w *widget) int {
		w.dontSavePosition = luaTruthyOpt(L, 2, true)
		return 0
	})
	return methods
}
