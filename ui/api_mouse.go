package ui

import (
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// registerMouseAPI defines the calls that ask what the mouse is doing.
func registerMouseAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("IsMouseButtonDown", L.NewFunction(func(L *lua.LState) int {
		down := false
		if rt.engine != nil {
			var name string
			switch value := L.Get(1).(type) {
			case lua.LString:
				name = string(value)
			case lua.LNumber:
				name = map[lua.LNumber]string{1: "LeftButton", 2: "RightButton", 3: "MiddleButton"}[value]
			}
			if name != "" && !strings.HasSuffix(name, "Button") {
				name += "Button"
			}
			_, down = rt.engine.presses[name]
		}
		L.Push(lua.LBool(down))
		return 1
	}))
	L.SetGlobal("GetMouseFocus", L.NewFunction(func(L *lua.LState) int {
		if rt.engine != nil && rt.engine.hovered != nil {
			L.Push(rt.engine.hovered.luaValue(L))
			return 1
		}
		if world := rt.widgets["WorldFrame"]; world != nil {
			L.Push(world.luaValue(L))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}))
	L.SetGlobal("GetSpellDescription", L.NewFunction(func(L *lua.LState) int {
		text := ""
		if info, ok := rt.gameplay.Catalog[rt.spellForCall(L)]; ok {
			text = rt.spellTextContext().expand(info.Description, info)
		}
		L.Push(lua.LString(text))
		return 1
	}))
	// GetText gives a global string, in the feminine form (token_FEMALE) when
	// the gender is female (3) and the string has one.
	L.SetGlobal("GetText", L.NewFunction(func(L *lua.LState) int {
		token := L.CheckString(1)
		if gender, ok := L.Get(2).(lua.LNumber); ok && gender == 3 {
			if value, ok := L.GetGlobal(token + "_FEMALE").(lua.LString); ok {
				L.Push(value)
				return 1
			}
		}
		L.Push(L.GetGlobal(token))
		return 1
	}))
}

// SelectionColor is the colour UnitSelectionColor gives a unit: how the
// selection ring under a target is tinted.
func (rt *Runtime) SelectionColor(unit string) [3]float32 {
	result := [3]float32{1, 1, 0}
	fn, ok := rt.L.GetGlobal("UnitSelectionColor").(*lua.LFunction)
	if !ok {
		return result
	}
	if err := rt.L.CallByParam(lua.P{Fn: fn, NRet: 3, Protect: true}, lua.LString(unit)); err != nil {
		return result
	}
	for index := 0; index < 3; index++ {
		if value, ok := rt.L.Get(-3 + index).(lua.LNumber); ok {
			result[index] = float32(value)
		}
	}
	rt.L.Pop(3)
	return result
}
