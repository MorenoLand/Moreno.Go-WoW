package ui

import (
	"math"
	"sort"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// registerAddonSupportAPI defines what user add-ons expect of the client beyond
// what the default interface uses: function hooks, frame enumeration and the
// per-type method tables of widgets.
func registerAddonSupportAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }

	// hooksecurefunc([table,] name, hook) runs hook after the function of that
	// name, with the same arguments, and keeps that function's results.
	reg("hooksecurefunc", func(L *lua.LState) int {
		var holder *lua.LTable
		var object *lua.LUserData
		var name lua.LValue
		var hook lua.LValue
		if table, ok := L.Get(1).(*lua.LTable); ok && L.GetTop() >= 3 {
			holder, name, hook = table, L.Get(2), L.Get(3)
		} else if widget, ok := L.Get(1).(*lua.LUserData); ok && L.GetTop() >= 3 {
			object, name, hook = widget, L.Get(2), L.Get(3)
		} else {
			name, hook = L.Get(1), L.Get(2)
		}
		original := func() lua.LValue {
			switch {
			case holder != nil:
				return holder.RawGet(name)
			case object != nil:
				return L.GetField(object, name.String())
			}
			return L.GetGlobal(name.String())
		}()
		if original.Type() != lua.LTFunction || hook.Type() != lua.LTFunction {
			L.ArgError(1, "hooksecurefunc: the function to hook does not exist")
			return 0
		}
		wrapper := L.NewFunction(func(L *lua.LState) int {
			count := L.GetTop()
			args := make([]lua.LValue, count)
			for index := range args {
				args[index] = L.Get(index + 1)
			}
			base := L.GetTop()
			L.Push(original)
			for _, arg := range args {
				L.Push(arg)
			}
			L.Call(count, lua.MultRet)
			results := L.GetTop() - base
			saved := make([]lua.LValue, results)
			for index := range saved {
				saved[index] = L.Get(base + 1 + index)
			}
			L.Push(hook)
			for _, arg := range args {
				L.Push(arg)
			}
			L.Call(count, 0)
			L.SetTop(base)
			for _, result := range saved {
				L.Push(result)
			}
			return results
		})
		switch {
		case holder != nil:
			holder.RawSet(name, wrapper)
		case object != nil:
			L.SetField(object, name.String(), wrapper)
		default:
			L.SetGlobal(name.String(), wrapper)
		}
		return 0
	})

	// State drivers decide a frame's state from macro conditions the secure
	// environment evaluates; nothing here runs them.
	registerStateDriverAPI(rt)

	// GetPlayerFacing is the way the player faces, in radians.
	reg("GetPlayerFacing", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.playerFacing))
		return 1
	})

	// GetAddOnMetadata reads a field of an add-on's TOC.
	reg("GetAddOnMetadata", func(L *lua.LState) int {
		toc := rt.addonTOCs[strings.ToLower(L.CheckString(1))]
		if toc == nil {
			L.Push(lua.LNil)
			return 1
		}
		if value := addonTOCField(toc, L.CheckString(2)); value != "" {
			L.Push(lua.LString(value))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})

	// The vehicle and battlefield calls the add-on makes have nothing to report.
	reg("UnitHasVehiclePlayerFrameUI", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("GetAmountBattlefieldBonus", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("RequestBattlefieldScoreData", func(L *lua.LState) int { return 0 })
	reg("DevTools_Dump", func(L *lua.LState) int { return 0 })

	// IsLoggedIn is true once the player has entered the world.
	reg("IsLoggedIn", func(L *lua.LState) int {
		L.Push(lua.LBool(rt.loggedIn))
		return 1
	})

	// EnumerateFrames walks the frames in a fixed order: with no argument it
	// returns the first, with a frame the one after it.
	reg("EnumerateFrames", func(L *lua.LState) int {
		names := make([]string, 0, len(rt.widgets))
		for name, w := range rt.widgets {
			if w.kind != kindTexture && w.kind != kindFontString {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		start := 0
		if previous, ok := L.Get(1).(*lua.LUserData); ok {
			if w, ok := previous.Value.(*widget); ok {
				start = sort.SearchStrings(names, w.name)
				if start < len(names) && names[start] == w.name {
					start++
				}
			}
		}
		if start >= len(names) {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(rt.widgets[names[start]].luaValue(L))
		return 1
	})

	// GetAuctionItemClasses names the item classes the auction house lists, in
	// the order it lists them.
	reg("GetAuctionItemClasses", func(L *lua.LState) int {
		order := []uint32{2, 4, 1, 0, 16, 7, 6, 11, 9, 3, 15, 12}
		for _, class := range order {
			name := rt.inventory.classes.Class[class]
			L.Push(lua.LString(name))
		}
		return len(order)
	})
}

// addonMethodTable is the table add-ons write methods into to give every widget
// of a type those methods (getmetatable(widget).__index).
func (rt *Runtime) addonMethodTable(objectType string) *lua.LTable {
	if rt.addonMethods == nil {
		rt.addonMethods = make(map[string]*lua.LTable)
	}
	if table := rt.addonMethods[objectType]; table != nil {
		return table
	}
	// Each type starts as a copy of the built-in methods, so a script can read
	// them, hook them (hooksecurefunc) or add its own.
	table := rt.L.NewTable()
	if rt.widgetMethodIndex != nil {
		rt.widgetMethodIndex.ForEach(func(key, value lua.LValue) { table.RawSet(key, value) })
	}
	rt.addonMethods[objectType] = table
	return table
}

// anchorOffset is an anchor offset as GetPoint reports it. Wow.exe keeps an
// offset as a float scaled to the screen's aspect (SetPoint divides by the
// aspect's scale, GetPoint undoes it), so a value that went in as 18 comes out a
// few millionths off; scripts that compare it with the number they set (the
// Dragonflight action bar module does) depend on that.
func (rt *Runtime) anchorOffset(value float64) float64 {
	if rt.Host == nil {
		return value
	}
	width, height := rt.Host.ScreenSize()
	if width <= 0 || height <= 0 || value == 0 {
		return value
	}
	aspect := width / height
	sine := float32(1 / math.Sqrt(aspect*aspect+1))
	factor := float32(float64(sine) * aspect)
	unit := float64(float32(0.75*aspect)) * 1024
	stored := float32(float64(factor) * float64(float32(value/unit)))
	return float64(stored) / float64(factor) * unit
}

// stateDriver keeps a frame's state in step with a macro-conditional string
// (RegisterStateDriver): a "visibility" driver shows or hides the frame, any
// other state is written to the frame's "state-<name>" attribute.
type stateDriver struct {
	frame      *lua.LUserData
	widget     *widget
	state      string
	conditions string
	last       string
	applied    bool
}

func (d *stateDriver) apply(rt *Runtime) {
	result := rt.evaluateMacroConditions(d.conditions)
	if d.applied && result == d.last {
		return
	}
	d.last, d.applied = result, true
	L := rt.L
	if strings.EqualFold(d.state, "visibility") {
		method := "Hide"
		switch strings.ToLower(strings.TrimSpace(result)) {
		case "show":
			method = "Show"
		case "hide":
		default:
			return
		}
		L.Push(L.GetField(d.frame, method))
		L.Push(d.frame)
		_ = L.PCall(1, 0, nil)
		return
	}
	// The state is stored where GetAttribute reads it; the frame's own
	// OnAttributeChanged belongs to the secure environment and is not run.
	value := lua.LValue(lua.LString(strings.TrimSpace(result)))
	if number, err := strconv.ParseFloat(strings.TrimSpace(result), 64); err == nil {
		value = lua.LNumber(number)
	}
	d.widget.ensureFields(L).RawSetString("state-"+d.state, value)
	rt.runSecureSnippet(d.widget, "", d.state, strings.TrimSpace(result))
}

// updateStateDrivers re-evaluates the drivers a few times a second.
func (rt *Runtime) updateStateDrivers(elapsed float64) {
	rt.stateDriverClock += elapsed
	if rt.stateDriverClock < 0.2 || len(rt.stateDrivers) == 0 {
		return
	}
	rt.stateDriverClock = 0
	for _, driver := range append([]*stateDriver(nil), rt.stateDrivers...) {
		driver.apply(rt)
	}
}

// evaluateMacroConditions picks the clause of a macro-conditional string whose
// conditions hold: "[mounted] fly; [combat,target=focus,exists] fight; walk".
func (rt *Runtime) evaluateMacroConditions(spec string) string {
	for _, clause := range strings.Split(spec, ";") {
		clause = strings.TrimSpace(clause)
		matched := false
		rest := clause
		hadBrackets := false
		for strings.HasPrefix(rest, "[") {
			end := strings.Index(rest, "]")
			if end < 0 {
				break
			}
			hadBrackets = true
			if rt.macroConditionsHold(rest[1:end]) {
				matched = true
			}
			rest = strings.TrimSpace(rest[end+1:])
		}
		if !hadBrackets || matched {
			return rest
		}
	}
	return ""
}

// macroConditionsHold reports whether every comma separated condition holds.
func (rt *Runtime) macroConditionsHold(conditions string) bool {
	unit := "target"
	for _, condition := range strings.Split(conditions, ",") {
		condition = strings.ToLower(strings.TrimSpace(condition))
		switch {
		case condition == "":
			continue
		case strings.HasPrefix(condition, "@"):
			unit = condition[1:]
			continue
		case strings.HasPrefix(condition, "target="):
			unit = strings.TrimPrefix(condition, "target=")
			continue
		}
		if !rt.macroCondition(condition, unit) {
			return false
		}
	}
	return true
}

func (rt *Runtime) macroCondition(condition, unit string) bool {
	negate := false
	if strings.HasPrefix(condition, "no") && condition != "nomod" {
		negate, condition = true, strings.TrimPrefix(condition, "no")
	}
	name, argument := condition, ""
	if colon := strings.IndexByte(condition, ':'); colon >= 0 {
		name, argument = condition[:colon], condition[colon+1:]
	}
	player := rt.unitInfo("player")
	target := rt.unitInfo(unit)
	known := target != nil && target.Exists
	var holds bool
	switch name {
	case "exists":
		holds = known
	case "dead":
		holds = known && target.Dead
	case "help":
		holds = known && target.Friendly
	case "harm":
		holds = known && target.Hostile
	case "combat":
		holds = player != nil && player.InCombat
	case "mounted":
		holds = player != nil && player.Mounted
	case "bar", "actionbar":
		page, _ := strconv.Atoi(argument)
		holds = page == rt.gameplay.Page
	default:
		// vehicleui, bonusbar, stance, form, stealth, flying, swimming, mod and
		// the rest: none holds here.
	}
	return holds != negate
}

// runSecureSnippet runs a secure handler snippet against a frame: body is its
// text, or, when empty, the frame's "_onstate-<state>" attribute with the new
// state. The snippet sees the frame as self and control; there is no restricted
// environment here, so it runs with the ordinary globals.
func (rt *Runtime) runSecureSnippet(w *widget, body, state, newState string) {
	L := rt.L
	if body == "" {
		if w.fields == nil {
			return
		}
		attribute, ok := w.fields.RawGetString("_onstate-" + state).(lua.LString)
		if !ok {
			return
		}
		body = string(attribute)
	}
	fn, err := L.Load(strings.NewReader("local self, stateid, newstate = ...\nlocal control = self\n"+body), "=secure snippet")
	if err != nil {
		rt.recordScriptError("secure snippet", err.Error())
		return
	}
	// A handler's snippets share one environment, so what one defines the next
	// sees; it falls back on the ordinary globals.
	if w.secureEnv == nil {
		w.secureEnv = L.NewTable()
		meta := L.NewTable()
		meta.RawSetString("__index", L.Get(lua.GlobalsIndex))
		L.SetMetatable(w.secureEnv, meta)
		w.secureEnv.RawSetString("newtable", L.NewFunction(func(L *lua.LState) int {
			table := L.NewTable()
			for index := 1; index <= L.GetTop(); index++ {
				table.Append(L.Get(index))
			}
			L.Push(table)
			return 1
		}))
	}
	fn.Env = w.secureEnv
	top := L.GetTop()
	defer L.SetTop(top)
	L.Push(fn)
	L.Push(w.luaValue(L))
	L.Push(lua.LString(state))
	L.Push(lua.LString(newState))
	if err := L.PCall(3, 0, nil); err != nil {
		rt.recordScriptError("secure snippet", err.Error())
	}
}

// registerStateDriverAPI defines the state driver functions. FrameXML defines
// its own when it loads, which need the secure environment; world UI loading
// calls this again afterwards so the ones here are the ones in use.
func registerStateDriverAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	// FrameXML copies these onto secure handler frames as their Execute and
	// SetFrameRef methods.
	reg("SecureHandlerExecute", func(L *lua.LState) int {
		if frame, ok := L.Get(1).(*lua.LUserData); ok {
			if w, ok := frame.Value.(*widget); ok {
				rt.runSecureSnippet(w, L.CheckString(2), "", "")
			}
		}
		return 0
	})
	reg("SecureHandlerSetFrameRef", func(L *lua.LState) int {
		frame, _ := L.Get(1).(*lua.LUserData)
		target, _ := L.Get(3).(*lua.LUserData)
		if frame == nil || target == nil {
			return 0
		}
		w, _ := frame.Value.(*widget)
		ref, _ := target.Value.(*widget)
		if w != nil && ref != nil {
			if w.frameRefs == nil {
				w.frameRefs = make(map[string]*widget)
			}
			w.frameRefs[L.CheckString(2)] = ref
		}
		return 0
	})
	registerState := func(L *lua.LState) int {
		frame, ok := L.Get(1).(*lua.LUserData)
		if !ok {
			return 0
		}
		w, ok := frame.Value.(*widget)
		if !ok {
			return 0
		}
		rt.stateDrivers = append(rt.stateDrivers, &stateDriver{frame: frame, widget: w, state: L.CheckString(2), conditions: L.CheckString(3)})
		// Evaluated on the next update, after the addon has finished setting
		// the handler up.
		rt.stateDriverClock = 1
		return 0
	}
	reg("RegisterStateDriver", registerState)
	reg("RegisterAttributeDriver", registerState)
	for _, name := range []string{"UnregisterStateDriver", "UnregisterAttributeDriver"} {
		reg(name, func(L *lua.LState) int {
			if frame, ok := L.Get(1).(*lua.LUserData); ok {
				kept := rt.stateDrivers[:0]
				for _, driver := range rt.stateDrivers {
					if driver.frame != frame {
						kept = append(kept, driver)
					}
				}
				rt.stateDrivers = kept
			}
			return 0
		})
	}

}
