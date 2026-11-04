package ui

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// actionTypeEquipmentSet is the type of an action button that holds an
// equipment set; the id is the index of the set, which may be 0.
const actionTypeEquipmentSet = 32

// actionFilled reports whether an action button holds something.
func actionFilled(action world.ActionButton) bool {
	return action.ID != 0 || action.Type == actionTypeEquipmentSet
}

func registerEquipmentCursorAPI(rt *Runtime) {
	L := rt.L
	pick := func(set *equipmentSet) {
		rt.gameplay.Cursor = world.ActionButton{ID: set.index, Type: actionTypeEquipmentSet}
		rt.FireEvent("CURSOR_UPDATE")
	}
	L.SetGlobal("PickupEquipmentSet", L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: PickupEquipmentSet(setIndex)")
			return 0
		}
		if index := L.CheckInt(1) - 1; index >= 0 && index < len(rt.equipment.sets) {
			pick(rt.equipment.sets[index])
		}
		return 0
	}))
	L.SetGlobal("PickupEquipmentSetByName", L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError("Usage: PickupEquipmentSetByName(\"setName\")")
			return 0
		}
		if !rt.equipmentManagerOn() {
			return 0
		}
		if set := rt.equipmentSetByName(L.CheckString(1)); set != nil {
			pick(set)
		}
		return 0
	}))
}
