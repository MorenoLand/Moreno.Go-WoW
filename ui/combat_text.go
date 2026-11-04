package ui

import lua "github.com/yuin/gopher-lua"

// CombatTextUnit is the unit the floating combat text follows, 0 for the
// character.
func (rt *Runtime) CombatTextUnit() uint64 { return rt.combatTextGUID }

func registerCombatTextAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("CombatTextSetActiveUnit", L.NewFunction(func(L *lua.LState) int {
		rt.combatTextGUID = 0
		if info := rt.unitByNameOrToken(L.OptString(1, "player")); info != nil {
			rt.combatTextGUID = info.GUID
		}
		return 0
	}))
}
