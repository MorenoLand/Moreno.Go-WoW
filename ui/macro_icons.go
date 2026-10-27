package ui

import (
	lua "github.com/yuin/gopher-lua"
)

// SetMacroIcons installs the list of icon textures a macro or a guild bank tab
// can be given, as full texture paths.
func (rt *Runtime) SetMacroIcons(paths []string) { rt.macroIcons = paths }

// macroIconPath is the texture of a 1-based icon of the list.
func (rt *Runtime) macroIconPath(index int) string {
	if index < 1 || index > len(rt.macroIcons) {
		return ""
	}
	return rt.macroIcons[index-1]
}

// registerMacroIconAPI defines the calls that list the icons to choose from.
func registerMacroIconAPI(rt *Runtime) {
	L := rt.L
	count := func(L *lua.LState) int { L.Push(lua.LNumber(len(rt.macroIcons))); return 1 }
	info := func(L *lua.LState) int {
		if path := rt.macroIconPath(L.CheckInt(1)); path != "" {
			L.Push(lua.LString(path))
			return 1
		}
		return 0
	}
	for _, name := range []string{"GetNumMacroIcons", "GetNumMacroItemIcons"} {
		L.SetGlobal(name, L.NewFunction(count))
	}
	for _, name := range []string{"GetMacroIconInfo", "GetMacroItemIconInfo"} {
		L.SetGlobal(name, L.NewFunction(info))
	}
}
