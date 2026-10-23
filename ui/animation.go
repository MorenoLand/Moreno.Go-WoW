package ui

import (
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// animationState is the playback state of an AnimationGroup widget or the
// parameters of one Animation inside it.
type animationState struct {
	group        *widget   // for an animation: its group
	animations   []*widget // for a group: its animations in creation order
	kind         string    // Alpha, Translation, Scale, Rotation, Path, Animation
	playing      bool
	paused       bool
	finished     bool
	looping      string // NONE, REPEAT, BOUNCE
	elapsed      float64
	duration     float64
	order        int
	startDelay   float64
	endDelay     float64
	smoothing    string
	change       float64 // Alpha: alpha delta over the animation
	baseAlpha    float64
	offsetX      float64
	offsetY      float64
	scaleX       float64
	scaleY       float64
	degrees      float64
	progress     float64
	hasBaseAlpha bool
}

var animationTypeNames = map[string]bool{
	"AnimationGroup": true, "Animation": true, "Alpha": true, "Translation": true,
	"Scale": true, "Rotation": true, "Path": true,
}

func (w *widget) isAnimation() bool { return animationTypeNames[w.typeName] }

func (w *widget) anim() *animationState {
	if w.animation == nil {
		w.animation = &animationState{looping: "NONE", smoothing: "NONE", scaleX: 1, scaleY: 1}
	}
	return w.animation
}

// newAnimationGroup creates an AnimationGroup owned by a frame or region.
func (rt *Runtime) newAnimationGroup(owner *widget, name, template string) *widget {
	group := newWidget(kindFrame, name)
	group.typeName = "AnimationGroup"
	group.parent = owner
	group.shown = true
	group.anim()
	if name != "" {
		rt.register(group)
	}
	owner.animationGroups = append(owner.animationGroups, group)
	return group
}

func (rt *Runtime) newAnimation(group *widget, kind, name string) *widget {
	animation := newWidget(kindFrame, name)
	animation.typeName = kind
	animation.parent = group
	state := animation.anim()
	state.group = group
	state.kind = kind
	state.duration = 0
	if name != "" {
		rt.register(animation)
	}
	group.anim().animations = append(group.anim().animations, animation)
	return animation
}

// applyAnimationAttrs reads the XML attributes of an animation element.
func (l *Loader) applyAnimationAttrs(w *widget, node *xmlNode) {
	state := w.anim()
	number := func(key string, target *float64) {
		if value, ok := node.attr(key); ok {
			if parsed, err := strconv.ParseFloat(value, 64); err == nil {
				*target = parsed
			}
		}
	}
	number("duration", &state.duration)
	number("startDelay", &state.startDelay)
	number("endDelay", &state.endDelay)
	number("change", &state.change)
	number("degrees", &state.degrees)
	number("scaleX", &state.scaleX)
	number("scaleY", &state.scaleY)
	number("offsetX", &state.offsetX)
	number("offsetY", &state.offsetY)
	if value, ok := node.attr("order"); ok {
		state.order, _ = strconv.Atoi(value)
	}
	if value, ok := node.attr("smoothing"); ok {
		state.smoothing = strings.ToUpper(value)
	}
	if value, ok := node.attr("looping"); ok {
		state.looping = strings.ToUpper(value)
	}
}

// buildAnimations creates the animation groups of an <Animations> element.
func (l *Loader) buildAnimations(element *xmlNode, owner *widget, interfacePath string) {
	for _, groupEl := range element.children {
		if !strings.EqualFold(groupEl.name, "AnimationGroup") {
			continue
		}
		name := resolveParentName(groupEl.attrDefault("name", ""), owner.nameContext())
		group := l.rt.newAnimationGroup(owner, name, "")
		l.applyAnimationAttrs(group, groupEl)
		if key, ok := groupEl.attr("parentKey"); ok && key != "" {
			owner.ensureFields(l.rt.L).RawSetString(key, group.luaValue(l.rt.L))
		}
		l.buildAnimationScripts(group, groupEl, interfacePath)
		for _, animEl := range groupEl.children {
			if strings.EqualFold(animEl.name, "Scripts") {
				continue
			}
			kind := animEl.name
			if !animationTypeNames[kind] {
				continue
			}
			animation := l.rt.newAnimation(group, kind, resolveParentName(animEl.attrDefault("name", ""), owner.nameContext()))
			l.applyAnimationAttrs(animation, animEl)
			if key, ok := animEl.attr("parentKey"); ok && key != "" {
				owner.ensureFields(l.rt.L).RawSetString(key, animation.luaValue(l.rt.L))
			}
			l.buildAnimationScripts(animation, animEl, interfacePath)
		}
	}
}

func (l *Loader) buildAnimationScripts(w *widget, element *xmlNode, interfacePath string) {
	for _, scriptsEl := range element.children {
		if !strings.EqualFold(scriptsEl.name, "Scripts") {
			continue
		}
		for _, scriptEl := range scriptsEl.children {
			handler := scriptEl.name
			if fnName, ok := scriptEl.attr("function"); ok {
				w.scripts[handler] = l.namedHandler(fnName)
				continue
			}
			body := strings.TrimSpace(scriptEl.text.String())
			if body == "" {
				continue
			}
			if fn := l.compileScript(body, handler, "@"+interfacePath+":"+handler); fn != nil {
				w.scripts[handler] = fn
			}
		}
	}
}

func (rt *Runtime) groupDuration(group *widget) float64 {
	total := 0.0
	byOrder := map[int]float64{}
	for _, animation := range group.anim().animations {
		state := animation.anim()
		span := state.startDelay + state.duration + state.endDelay
		if span > byOrder[state.order] {
			byOrder[state.order] = span
		}
	}
	for _, span := range byOrder {
		total += span
	}
	return total
}

func (rt *Runtime) playAnimationGroup(group *widget) {
	state := group.anim()
	if state.playing && !state.paused {
		return
	}
	if !state.paused {
		state.elapsed = 0
	}
	state.playing, state.paused, state.finished = true, false, false
	rt.animatingGroups[group] = true
	rt.fireHandler(group, "OnPlay")
}

func (rt *Runtime) stopAnimationGroup(group *widget, finished bool) {
	state := group.anim()
	wasPlaying := state.playing
	state.playing, state.paused = false, false
	state.finished = finished
	state.elapsed = 0
	delete(rt.animatingGroups, group)
	rt.applyAnimationProgress(group, 0)
	if wasPlaying {
		if finished {
			rt.fireHandler(group, "OnFinished")
		} else {
			rt.fireHandler(group, "OnStop")
		}
	}
}

// applyAnimationProgress writes the effect of the group's animations at the given
// elapsed time onto the region the group belongs to. Only alpha animations change
// what is drawn; the other kinds keep their timing and scripts.
func (rt *Runtime) applyAnimationProgress(group *widget, elapsed float64) {
	owner := group.parent
	if owner == nil {
		return
	}
	var order int
	var start float64
	orders := map[int]bool{}
	for _, animation := range group.anim().animations {
		orders[animation.anim().order] = true
	}
	var sequence []int
	for value := range orders {
		sequence = append(sequence, value)
	}
	sortInts(sequence)
	for _, order = range sequence {
		span := 0.0
		for _, animation := range group.anim().animations {
			state := animation.anim()
			if state.order != order {
				continue
			}
			if length := state.startDelay + state.duration + state.endDelay; length > span {
				span = length
			}
		}
		for _, animation := range group.anim().animations {
			state := animation.anim()
			if state.order != order {
				continue
			}
			local := elapsed - start - state.startDelay
			progress := 0.0
			switch {
			case state.duration <= 0, local >= state.duration:
				progress = 1
			case local > 0:
				progress = local / state.duration
			}
			if elapsed < start+state.startDelay {
				progress = 0
			}
			progress = smoothAnimation(progress, state.smoothing)
			state.progress = progress
			if state.kind == "Alpha" {
				if !state.hasBaseAlpha {
					state.baseAlpha, state.hasBaseAlpha = owner.alpha, true
				}
				value := state.baseAlpha + state.change*progress
				owner.alpha = clampFloat(value, 0, 1)
			}
		}
		start += span
	}
}

func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func smoothAnimation(progress float64, mode string) float64 {
	switch mode {
	case "IN":
		return progress * progress
	case "OUT":
		return 1 - (1-progress)*(1-progress)
	case "IN_OUT":
		if progress < 0.5 {
			return 2 * progress * progress
		}
		return 1 - 2*(1-progress)*(1-progress)
	}
	return progress
}

func sortInts(values []int) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// tickAnimations advances the playing groups; it reports whether anything that
// is drawn may have changed.
func (rt *Runtime) tickAnimations(elapsed float64) bool {
	if len(rt.animatingGroups) == 0 {
		return false
	}
	changed := false
	for group := range rt.animatingGroups {
		state := group.anim()
		if !state.playing || state.paused {
			continue
		}
		state.elapsed += elapsed
		total := rt.groupDuration(group)
		if total > 0 && state.elapsed >= total {
			switch state.looping {
			case "REPEAT":
				state.elapsed -= total
				rt.fireHandler(group, "OnLoop")
			case "BOUNCE":
				state.elapsed -= total
				rt.fireHandler(group, "OnLoop")
			default:
				rt.applyAnimationProgress(group, total)
				rt.stopAnimationGroup(group, true)
				changed = true
				continue
			}
		}
		if _, ok := group.scripts["OnUpdate"]; ok {
			rt.fire(group, "OnUpdate", []lua.LValue{group.luaValue(rt.L), lua.LNumber(elapsed)})
		}
		rt.applyAnimationProgress(group, state.elapsed)
		changed = true
	}
	return changed
}

// animationMethods are the AnimationGroup and Animation methods.
func (rt *Runtime) animationMethods() map[string]func(L *lua.LState, w *widget) int {
	number := func(get func(*animationState) *float64) func(L *lua.LState, w *widget) int {
		return func(L *lua.LState, w *widget) int {
			*get(w.anim()) = float64(L.CheckNumber(2))
			return 0
		}
	}
	getNumber := func(get func(*animationState) *float64) func(L *lua.LState, w *widget) int {
		return func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(*get(w.anim())))
			return 1
		}
	}
	return map[string]func(L *lua.LState, w *widget) int{
		"Play": func(L *lua.LState, w *widget) int { rt.playAnimationGroup(w); return 0 },
		"Pause": func(L *lua.LState, w *widget) int {
			if w.anim().playing {
				w.anim().paused = true
				rt.fireHandler(w, "OnPause")
			}
			return 0
		},
		"Stop":   func(L *lua.LState, w *widget) int { rt.stopAnimationGroup(w, false); return 0 },
		"Finish": func(L *lua.LState, w *widget) int { rt.stopAnimationGroup(w, true); return 0 },
		"IsPlaying": func(L *lua.LState, w *widget) int {
			L.Push(lua.LBool(w.anim().playing && !w.anim().paused))
			return 1
		},
		"IsPaused": func(L *lua.LState, w *widget) int { L.Push(lua.LBool(w.anim().paused)); return 1 },
		"IsDone":   func(L *lua.LState, w *widget) int { L.Push(lua.LBool(w.anim().finished)); return 1 },
		"SetLooping": func(L *lua.LState, w *widget) int { w.anim().looping = strings.ToUpper(L.CheckString(2)); return 0 },
		"GetLooping": func(L *lua.LState, w *widget) int { L.Push(lua.LString(w.anim().looping)); return 1 },
		"CreateAnimation": func(L *lua.LState, w *widget) int {
			kind := L.OptString(2, "Animation")
			for name := range animationTypeNames {
				if strings.EqualFold(name, kind) {
					kind = name
				}
			}
			animation := rt.newAnimation(w, kind, L.OptString(3, ""))
			L.Push(animation.luaValue(L))
			return 1
		},
		"GetAnimations": func(L *lua.LState, w *widget) int {
			for _, animation := range w.anim().animations {
				L.Push(animation.luaValue(L))
			}
			return len(w.anim().animations)
		},
		"GetParent":       func(L *lua.LState, w *widget) int { return pushWidgetOrNil(L, w.parent) },
		"GetRegionParent": func(L *lua.LState, w *widget) int {
			if w.anim().group != nil {
				return pushWidgetOrNil(L, w.anim().group.parent)
			}
			return pushWidgetOrNil(L, w.parent)
		},
		"GetTarget": func(L *lua.LState, w *widget) int { return 0 },
		"SetDuration":    number(func(s *animationState) *float64 { return &s.duration }),
		"GetDuration":    getNumber(func(s *animationState) *float64 { return &s.duration }),
		"SetStartDelay":  number(func(s *animationState) *float64 { return &s.startDelay }),
		"GetStartDelay":  getNumber(func(s *animationState) *float64 { return &s.startDelay }),
		"SetEndDelay":    number(func(s *animationState) *float64 { return &s.endDelay }),
		"GetEndDelay":    getNumber(func(s *animationState) *float64 { return &s.endDelay }),
		"SetChange":      number(func(s *animationState) *float64 { return &s.change }),
		"GetChange":      getNumber(func(s *animationState) *float64 { return &s.change }),
		"SetOrder":       func(L *lua.LState, w *widget) int { w.anim().order = L.CheckInt(2); return 0 },
		"GetOrder":       func(L *lua.LState, w *widget) int { L.Push(lua.LNumber(w.anim().order)); return 1 },
		"SetSmoothing":   func(L *lua.LState, w *widget) int { w.anim().smoothing = strings.ToUpper(L.CheckString(2)); return 0 },
		"GetSmoothing":   func(L *lua.LState, w *widget) int { L.Push(lua.LString(w.anim().smoothing)); return 1 },
		"GetProgress":    getNumber(func(s *animationState) *float64 { return &s.progress }),
		"GetElapsed":     getNumber(func(s *animationState) *float64 { return &s.elapsed }),
		"SetDegrees":     number(func(s *animationState) *float64 { return &s.degrees }),
		"GetDegrees":     getNumber(func(s *animationState) *float64 { return &s.degrees }),
		"SetOffset": func(L *lua.LState, w *widget) int {
			w.anim().offsetX, w.anim().offsetY = float64(L.CheckNumber(2)), float64(L.CheckNumber(3))
			return 0
		},
		"GetOffset": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.anim().offsetX))
			L.Push(lua.LNumber(w.anim().offsetY))
			return 2
		},
		"SetScale": func(L *lua.LState, w *widget) int {
			w.anim().scaleX, w.anim().scaleY = float64(L.CheckNumber(2)), float64(L.CheckNumber(3))
			return 0
		},
		"GetScale": func(L *lua.LState, w *widget) int {
			L.Push(lua.LNumber(w.anim().scaleX))
			L.Push(lua.LNumber(w.anim().scaleY))
			return 2
		},
		"SetOrigin": func(L *lua.LState, w *widget) int { return 0 },
		"SetTarget": func(L *lua.LState, w *widget) int { return 0 },
	}
}

func pushWidgetOrNil(L *lua.LState, w *widget) int {
	if w == nil {
		L.Push(lua.LNil)
	} else {
		L.Push(w.luaValue(L))
	}
	return 1
}
