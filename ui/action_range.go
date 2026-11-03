package ui

import lua "github.com/yuin/gopher-lua"

func registerActionRangeAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("IsActionInRange", L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.Push(lua.LNil)
			return 1
		}
		action := rt.action(L.CheckInt(1))
		unit := L.OptString(2, "target")
		spell, known := rt.gameplay.Catalog[action.ID]
		info := rt.unitInfo(unit)
		if action.ID == 0 || action.Type != 0 || !known || spell.MaxRange <= 0 || info == nil || !info.Exists || rt.unitDistance == nil {
			L.Push(lua.LNil)
			return 1
		}
		distance, ok := rt.unitDistance(info.GUID)
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		if distance <= spell.MaxRange && distance >= spell.MinRange {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	}))
}
