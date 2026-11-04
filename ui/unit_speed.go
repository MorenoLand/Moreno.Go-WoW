package ui

import lua "github.com/yuin/gopher-lua"

// SetPlayerSpeed gives the interface the speed the character moves at now.
func (rt *Runtime) SetPlayerSpeed(speed func() float64) { rt.playerSpeed = speed }

func registerUnitSpeedAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("GetUnitSpeed", L.NewFunction(func(L *lua.LState) int {
		token, ok := L.Get(1).(lua.LString)
		if !ok {
			L.RaiseError("Usage: GetUnitSpeed(\"unit\")")
			return 0
		}
		speed := 0.0
		if info := rt.unitByNameOrToken(string(token)); info != nil && info.GUID == rt.playerGUID() && rt.playerSpeed != nil {
			speed = rt.playerSpeed()
		}
		L.Push(lua.LNumber(speed))
		return 1
	}))
}
