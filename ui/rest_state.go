package ui

import (
	lua "github.com/yuin/gopher-lua"
)

// RestState is a row of Exhaustion.dbc: how rested the player is and the
// factor experience is multiplied by.
type RestState struct {
	ID     uint32
	Name   string
	Factor float32
}

// SetRestStates installs the rest states of the client's tables.
func (rt *Runtime) SetRestStates(states []RestState) { rt.restStates = states }

func registerRestStateAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("GetRestState", L.NewFunction(func(L *lua.LState) int {
		if info := rt.unitInfo("player"); info != nil {
			for _, state := range rt.restStates {
				if state.ID == uint32(info.RestState) {
					L.Push(lua.LNumber(state.ID))
					L.Push(lua.LString(state.Name))
					L.Push(lua.LNumber(state.Factor))
					return 3
				}
			}
		}
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		return 3
	}))
}
