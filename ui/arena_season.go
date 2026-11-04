package ui

import lua "github.com/yuin/gopher-lua"

// The arena season is two world states: the season now and the one before.
const (
	worldStateArenaSeason         = 3191
	worldStateArenaPreviousSeason = 3901
)

func registerArenaSeasonAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("GetSuggestedGroupNum", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.quests.dialog.suggested))
		return 1
	}))
	L.SetGlobal("GetCurrentArenaSeason", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.worldStateValue(worldStateArenaSeason)))
		return 1
	}))
	L.SetGlobal("GetPreviousArenaSeason", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.worldStateValue(worldStateArenaPreviousSeason)))
		return 1
	}))
}
