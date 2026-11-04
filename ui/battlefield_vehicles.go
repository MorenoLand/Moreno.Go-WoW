package ui

import lua "github.com/yuin/gopher-lua"

// BattlefieldVehicle is a vehicle the map of a battlefield shows.
type BattlefieldVehicle struct {
	MapID      uint32
	X, Y       float32
	Name       string
	Occupied   bool
	Facing     float32
	Controlled bool
	Alive      bool
}

// SetBattlefieldVehicles gives the interface the vehicles the world has loaded.
func (rt *Runtime) SetBattlefieldVehicles(list func() []BattlefieldVehicle) {
	rt.bfVehicles = list
}

// battlefieldVehicles are the vehicles on the map page shown.
func (rt *Runtime) battlefieldVehicles() []BattlefieldVehicle {
	if rt.bfVehicles == nil {
		return nil
	}
	page := rt.worldMap.currentPage()
	var list []BattlefieldVehicle
	for _, vehicle := range rt.bfVehicles() {
		if page != nil && page.hasBounds() && page.MapID == vehicle.MapID && page.contains(vehicle.X, vehicle.Y) {
			list = append(list, vehicle)
		}
	}
	return list
}

func registerBattlefieldVehicleAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("GetNumBattlefieldVehicles", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.battlefieldVehicles())))
		return 1
	}))
	L.SetGlobal("GetBattlefieldVehicleInfo", L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetBattlefieldVehicleInfo(index)")
			return 0
		}
		list := rt.battlefieldVehicles()
		index := L.CheckInt(1) - 1
		page := rt.worldMap.currentPage()
		if index < 0 || index >= len(list) || page == nil {
			return 0
		}
		vehicle := list[index]
		u, v := page.project(vehicle.X, vehicle.Y)
		return pushAll(L, lua.LNumber(u), lua.LNumber(v), lua.LString(vehicle.Name), lua.LBool(vehicle.Occupied), lua.LNil,
			lua.LNumber(vehicle.Facing), lua.LBool(vehicle.Controlled), lua.LBool(vehicle.Alive))
	}))
}
