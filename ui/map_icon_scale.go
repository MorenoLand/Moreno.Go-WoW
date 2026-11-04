package ui

import lua "github.com/yuin/gopher-lua"

// SetMapIconScales gives the size each map asks its battlefield icons to be.
func (rt *Runtime) SetMapIconScales(scales map[uint32]float32) { rt.mapIconScales = scales }

func registerMapIconScaleAPI(rt *Runtime) {
	rt.L.SetGlobal("GetBattlefieldMapIconScale", rt.L.NewFunction(func(L *lua.LState) int {
		scale := float32(1)
		if value, ok := rt.mapIconScales[rt.worldMap.playerMap]; ok && rt.worldMap.hasPlayer {
			scale = value
		}
		L.Push(lua.LNumber(scale))
		return 1
	}))
}
