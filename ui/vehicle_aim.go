package ui

import (
	"math"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// The aim of a vehicle weapon is the pitch of the vehicle, between limits the
// vehicle names or straight up and down.
const (
	vehicleAimLimit   = math.Pi / 2
	vehicleAimStep    = 0.1
	vehicleFlagPitch  = 0x40
	vehicleAimRate    = 1.0
	vehicleAimNoLimit = 0
)

type vehicleAimHooks struct {
	get func() float64
	set func(pitch float64)
	// moving is the direction a key holds the aim in: 1 up, -1 down.
	moving int
	last   time.Time
}

// SetVehicleAim gives the interface the pitch of the character and a way to
// change it.
func (rt *Runtime) SetVehicleAim(get func() float64, set func(float64)) {
	rt.vehicleAim.get, rt.vehicleAim.set = get, set
}

// vehicleAimLimits are the lowest and the highest pitch the vehicle allows.
func (rt *Runtime) vehicleAimLimits() (float64, float64) {
	row := rt.vehicleRowOf(rt.playerVehicle())
	if row != nil && row.Flags&vehicleFlagPitch != 0 {
		return float64(row.PitchMin), float64(row.PitchMax)
	}
	return -vehicleAimLimit, vehicleAimLimit
}

// vehicleAimTo sets the pitch, held between the limits.
func (rt *Runtime) vehicleAimTo(pitch float64) {
	if rt.vehicleAim.set == nil || rt.playerVehicle() == 0 {
		return
	}
	low, high := rt.vehicleAimLimits()
	rt.vehicleAim.set(math.Min(math.Max(pitch, low), high))
}

// VehicleAimTick moves the aim while a key holds it, by the rate of the
// camera's pitch turn.
func (rt *Runtime) VehicleAimTick() {
	now := time.Now()
	seconds := now.Sub(rt.vehicleAim.last).Seconds()
	rt.vehicleAim.last = now
	if seconds > 0.25 {
		seconds = 0.25
	}
	if rt.vehicleAim.moving != 0 && rt.vehicleAim.get != nil {
		rt.vehicleAimTo(rt.vehicleAim.get() + float64(rt.vehicleAim.moving)*vehicleAimRate*seconds)
	}
}

func registerVehicleAimAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	change := func(sign float64) lua.LGFunction {
		return func(L *lua.LState) int {
			if rt.vehicleAim.get == nil {
				return 0
			}
			step := vehicleAimStep
			if L.Get(1).Type() == lua.LTNumber {
				step = float64(L.CheckNumber(1))
			}
			rt.vehicleAimTo(rt.vehicleAim.get() + sign*step)
			return 0
		}
	}
	reg("VehicleAimIncrement", change(1))
	reg("VehicleAimDecrement", change(-1))
	hold := func(direction int, start bool) lua.LGFunction {
		return func(L *lua.LState) int {
			if rt.vehicleAim.get == nil || rt.playerVehicle() == 0 {
				return 0
			}
			if start {
				rt.vehicleAim.moving = direction
				rt.vehicleAimTo(rt.vehicleAim.get() + float64(direction)*vehicleAimStep)
			} else if rt.vehicleAim.moving == direction {
				rt.vehicleAim.moving = 0
			}
			return 0
		}
	}
	reg("VehicleAimUpStart", hold(1, true))
	reg("VehicleAimUpStop", hold(1, false))
	reg("VehicleAimDownStart", hold(-1, true))
	reg("VehicleAimDownStop", hold(-1, false))
	reg("VehicleAimRequestNormAngle", func(L *lua.LState) int {
		if rt.playerVehicle() == 0 || L.Get(1).Type() != lua.LTNumber {
			return 0
		}
		low, high := rt.vehicleAimLimits()
		rt.vehicleAimTo(low + (high-low)*float64(L.CheckNumber(1)))
		return 0
	})
}
