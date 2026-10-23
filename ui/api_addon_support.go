package ui

import (
	"math"
	"sort"

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
	for _, name := range []string{"RegisterStateDriver", "UnregisterStateDriver", "RegisterAttributeDriver", "UnregisterAttributeDriver"} {
		reg(name, func(L *lua.LState) int { return 0 })
	}

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
