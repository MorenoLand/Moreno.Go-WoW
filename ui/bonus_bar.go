package ui

import lua "github.com/yuin/gopher-lua"

// ShapeshiftBar is the action bar a form brings and whether it comes with
// spells of its own.
type ShapeshiftBar struct {
	Bar        uint32
	HasPresets bool
}

// SetShapeshiftBars gives the bars of ShapeshiftForm.dbc.
func (rt *Runtime) SetShapeshiftBars(bars map[uint32]ShapeshiftBar) { rt.shapeshiftBars = bars }

// bonusBarOffsetVehicle is the bar a vehicle or a form with spells of its own
// puts up.
const bonusBarOffsetVehicle = 5

// bonusBarOffset is the bonus bar the character's form or vehicle puts up, 0
// for none.
func (rt *Runtime) bonusBarOffset() int {
	if rt.playerVehicle() != 0 && rt.vehicleUIShown(rt.unitInfo("player")) {
		return bonusBarOffsetVehicle
	}
	form := rt.field(fieldUnitBytes2) >> 24
	bar, ok := rt.shapeshiftBars[form]
	switch {
	case form == 0 || !ok:
		return 0
	case bar.HasPresets:
		return bonusBarOffsetVehicle
	}
	return int(bar.Bar)
}

func registerBonusBarAPI(rt *Runtime) {
	rt.L.SetGlobal("GetBonusBarOffset", rt.L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.bonusBarOffset()))
		return 1
	}))
}
