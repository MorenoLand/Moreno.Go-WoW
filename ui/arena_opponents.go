package ui

import lua "github.com/yuin/gopher-lua"

// SetArenaOpponents is how many opposing players the arena has shown so far.
func (rt *Runtime) SetArenaOpponents(count int) { rt.arenaOpponents = count }

func registerArenaOpponentsAPI(rt *Runtime) {
	rt.L.SetGlobal("GetNumArenaOpponents", rt.L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.arenaOpponents))
		return 1
	}))
}
