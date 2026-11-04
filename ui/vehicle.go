package ui

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Flags of a vehicle seat (VehicleSeat.dbc) and of a vehicle (Vehicle.dbc) the
// interface reads.
const (
	seatFlagControl  = 0x800
	seatFlagUI       = 0x20000000
	seatFlagExit     = 0x2000000
	seatFlagSwitch   = 0x4000000
	seatFlagControls = 0x800000
	seatFlagsBEject  = 0x20
	seatFlagsBRaidUI = 0x8
	vehicleFlagAngle = 0x400
	vehicleFlagPower = 0x800
	vehicleSeats     = 8
)

// VehicleRow is a row of Vehicle.dbc.
type VehicleRow struct {
	ID    uint32
	Flags uint32
	// PitchMin and PitchMax limit the aim angle when the vehicle sets its own.
	PitchMin, PitchMax float32
	Seats [vehicleSeats]uint32
}

// VehicleSeatRow is a row of VehicleSeat.dbc.
type VehicleSeatRow struct {
	ID     uint32
	Flags  uint32
	UISkin uint32
	FlagsB uint32
}

// VehicleIndicatorSeat is a row of VehicleUIIndicSeat.dbc.
type VehicleIndicatorSeat struct {
	Indicator, Virtual uint32
	X, Y               float32
}

// VehicleTables are the tables the vehicle interface reads.
type VehicleTables struct {
	Vehicles       []VehicleRow
	Seats          []VehicleSeatRow
	Indicators     map[uint32]string
	IndicatorSeats []VehicleIndicatorSeat
}

// VehicleEntity is what the world knows of one unit for the vehicle interface.
type VehicleEntity struct {
	Exists bool
	Player bool
	// Transport is the vehicle the unit rides and Seat the seat it has there.
	Transport uint64
	Seat      int8
	// VehicleID is set for a unit that is a vehicle.
	VehicleID uint32
	CharmedBy uint64
}

type vehicleState struct {
	tables     VehicleTables
	vehicles   map[uint32]*VehicleRow
	seats      map[uint32]*VehicleSeatRow
	entity     func(guid uint64) VehicleEntity
	passengers func(vehicle uint64) []uint64
	power      float64
}

// SetVehicleTables gives the interface the tables it reads.
func (rt *Runtime) SetVehicleTables(tables VehicleTables) {
	s := &rt.veh
	s.tables = tables
	s.vehicles = make(map[uint32]*VehicleRow, len(tables.Vehicles))
	for index := range s.tables.Vehicles {
		row := &s.tables.Vehicles[index]
		s.vehicles[row.ID] = row
	}
	s.seats = make(map[uint32]*VehicleSeatRow, len(tables.Seats))
	for index := range s.tables.Seats {
		row := &s.tables.Seats[index]
		s.seats[row.ID] = row
	}
}

// SetVehicleSource gives the interface a way to learn what units ride on
// what.
func (rt *Runtime) SetVehicleSource(entity func(uint64) VehicleEntity, passengers func(uint64) []uint64) {
	rt.veh.entity, rt.veh.passengers = entity, passengers
}

func (rt *Runtime) vehicleEntity(guid uint64) VehicleEntity {
	if rt.veh.entity == nil || guid == 0 {
		return VehicleEntity{}
	}
	return rt.veh.entity(guid)
}

func (rt *Runtime) vehicleRowOf(guid uint64) *VehicleRow {
	e := rt.vehicleEntity(guid)
	if !e.Exists || e.VehicleID == 0 {
		return nil
	}
	return rt.veh.vehicles[e.VehicleID]
}

// seatOf is the seat a unit sits in and the vehicle that has it.
func (rt *Runtime) seatOf(guid uint64) (*VehicleSeatRow, uint64) {
	e := rt.vehicleEntity(guid)
	if !e.Exists || e.Transport == 0 || e.Seat < 0 || e.Seat >= vehicleSeats {
		return nil, 0
	}
	row := rt.vehicleRowOf(e.Transport)
	if row == nil {
		return nil, 0
	}
	return rt.veh.seats[row.Seats[e.Seat]], e.Transport
}

func (rt *Runtime) unitGUID(unit string) uint64 {
	if info := rt.unitInfo(unit); info != nil && info.Exists {
		return info.GUID
	}
	return 0
}

// vehicleSeatCount counts the seats a vehicle has, those of the vehicles that
// ride on it taking the place of the seats they sit in.
func (rt *Runtime) vehicleSeatCount(guid uint64, depth int) int {
	row := rt.vehicleRowOf(guid)
	if row == nil || depth > 4 {
		return 0
	}
	count := 0
	for _, seat := range row.Seats {
		if seat > 0 {
			count++
		}
	}
	if rt.veh.passengers != nil {
		for _, passenger := range rt.veh.passengers(guid) {
			e := rt.vehicleEntity(passenger)
			if e.Exists && !e.Player && rt.veh.vehicles[e.VehicleID] != nil {
				count = count - 1 + rt.vehicleSeatCount(passenger, depth+1)
			}
		}
	}
	return count
}

// resolveSeat finds the vehicle and the seat the n-th seat of a vehicle's
// interface list stands for.
func (rt *Runtime) resolveSeat(guid uint64, index *int, depth int) (uint64, int, bool) {
	row := rt.vehicleRowOf(guid)
	if row == nil || depth > 4 {
		return 0, 0, false
	}
	bySeat := map[int]uint64{}
	if rt.veh.passengers != nil {
		for _, passenger := range rt.veh.passengers(guid) {
			e := rt.vehicleEntity(passenger)
			if e.Exists && !e.Player && rt.veh.vehicles[e.VehicleID] != nil && e.Seat >= 0 && e.Seat < vehicleSeats {
				bySeat[int(e.Seat)] = passenger
			}
		}
	}
	for seat := 0; seat < vehicleSeats; seat++ {
		if child, found := bySeat[seat]; found {
			if v, s, ok := rt.resolveSeat(child, index, depth+1); ok {
				return v, s, true
			}
			continue
		}
		if row.Seats[seat] > 0 {
			if *index == 0 {
				return guid, seat, true
			}
			*index--
		}
	}
	return 0, 0, false
}

func (rt *Runtime) passengerAt(vehicle uint64, seat int) uint64 {
	if rt.veh.passengers == nil {
		return 0
	}
	for _, passenger := range rt.veh.passengers(vehicle) {
		if e := rt.vehicleEntity(passenger); e.Exists && int(e.Seat) == seat {
			return passenger
		}
	}
	return 0
}

// playerSeatAllows reports whether the character sits in a seat that has the
// flag.
func (rt *Runtime) playerSeatAllows(mask uint32) bool {
	seat, _ := rt.seatOf(rt.playerGUID())
	return seat != nil && seat.Flags&mask != 0
}

func registerVehicleAPI(rt *Runtime) {
	L := rt.L
	s := &rt.veh
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	unitArg := func(L *lua.LState, usage string) uint64 {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError("%s", usage)
		}
		return rt.unitGUID(L.ToString(1))
	}
	flagOrNil := func(L *lua.LState, value bool) int {
		if value {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	}
	reg("UnitInVehicle", func(L *lua.LState) int {
		seat, _ := rt.seatOf(unitArg(L, `Usage: UnitInVehicle("unit")`))
		return flagOrNil(L, seat != nil)
	})
	reg("UnitUsingVehicle", func(L *lua.LState) int {
		seat, _ := rt.seatOf(unitArg(L, `Usage: UnitUsingVehicle("unit")`))
		return flagOrNil(L, seat != nil)
	})
	reg("UnitControllingVehicle", func(L *lua.LState) int {
		guid := rt.unitGUID(L.OptString(1, ""))
		seat, vehicle := rt.seatOf(guid)
		L.Push(lua.LBool(seat != nil && guid != 0 && rt.vehicleEntity(vehicle).CharmedBy == guid))
		return 1
	})
	seatFlags := func(name string, mask uint32, fromB bool) {
		reg(name, func(L *lua.LState) int {
			seat, _ := rt.seatOf(rt.unitGUID(L.OptString(1, "")))
			if seat == nil {
				L.Push(lua.LFalse)
				return 1
			}
			flags := seat.Flags
			if fromB {
				flags = seat.FlagsB
			}
			L.Push(lua.LBool(flags&mask != 0))
			return 1
		})
	}
	seatFlags("UnitInVehicleControlSeat", seatFlagControl, false)
	seatFlags("UnitHasVehicleUI", seatFlagUI, false)
	seatFlags("UnitTargetsVehicleInRaidUI", seatFlagsBRaidUI, true)
	reg("UnitVehicleSkin", func(L *lua.LState) int {
		seat, _ := rt.seatOf(rt.unitGUID(L.OptString(1, "")))
		if seat == nil {
			return 0
		}
		if seat.UISkin < 2 {
			L.Push(lua.LString([]string{"Natural", "Mechanical"}[seat.UISkin]))
		} else {
			L.Push(lua.LString(""))
		}
		return 1
	})
	reg("UnitVehicleSeatCount", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.vehicleSeatCount(rt.unitGUID(L.OptString(1, "")), 0)))
		return 1
	})
	reg("UnitVehicleSeatInfo", func(L *lua.LState) int {
		guid := rt.unitGUID(L.OptString(1, ""))
		index := L.CheckInt(2) - 1
		if guid == 0 || rt.vehicleRowOf(guid) == nil {
			return 0
		}
		vehicle, seatIndex, ok := rt.resolveSeat(guid, &index, 0)
		if !ok {
			return 0
		}
		row := rt.vehicleRowOf(vehicle)
		seat := s.seats[row.Seats[seatIndex]]
		if seat == nil {
			return 0
		}
		control := "None"
		if seat.Flags&seatFlagControl != 0 {
			control = "Child"
			if vehicle == guid {
				control = "Root"
			}
		}
		L.Push(lua.LString(control))
		if occupant := rt.passengerAt(vehicle, seatIndex); occupant != 0 {
			L.Push(lua.LString(rt.nameOf(occupant)))
		} else {
			L.Push(lua.LNil)
		}
		L.Push(lua.LNil)
		L.Push(lua.LBool(seat.FlagsB&seatFlagsBEject != 0))
		L.Push(lua.LBool(seat.Flags>>26&1 != 0))
		return 5
	})
	reg("VehicleExit", func(L *lua.LState) int {
		if rt.vehicleEntity(rt.playerGUID()).Exists {
			if !rt.playerSeatAllows(seatFlagExit) {
				rt.ShowGameError(0x30, rt.globalString("SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW", ""))
				return 0
			}
			rt.send(world.ClientRequestVehicleExit, nil)
		}
		return 0
	})
	reg("VehicleNextSeat", func(L *lua.LState) int {
		if rt.playerSeatAllows(seatFlagSwitch) {
			rt.send(world.ClientRequestVehicleNextSeat, nil)
		}
		return 0
	})
	reg("VehiclePrevSeat", func(L *lua.LState) int {
		if rt.playerSeatAllows(seatFlagSwitch) {
			rt.send(world.ClientRequestVehiclePrevSeat, nil)
		}
		return 0
	})
	reg("CanExitVehicle", func(L *lua.LState) int { return flagOrNil(L, rt.playerSeatAllows(seatFlagExit)) })
	reg("IsUsingVehicleControls", func(L *lua.LState) int { return flagOrNil(L, rt.playerSeatAllows(seatFlagControls)) })
	reg("CanSwitchVehicleSeats", func(L *lua.LState) int { return flagOrNil(L, rt.playerSeatAllows(seatFlagSwitch)) })
	reg("CanSwitchVehicleSeat", func(L *lua.LState) int {
		L.Push(lua.LBool(rt.playerSeatAllows(seatFlagSwitch)))
		return 1
	})
	reg("UnitSwitchToVehicleSeat", func(L *lua.LState) int {
		guid := rt.unitGUID(L.OptString(1, ""))
		index := L.CheckInt(2) - 1
		if guid == 0 || guid != rt.playerGUID() {
			return 0
		}
		_, vehicle := rt.seatOf(guid)
		if vehicle == 0 {
			return 0
		}
		target, seatIndex, ok := rt.resolveSeat(vehicle, &index, 0)
		if !ok {
			return 0
		}
		row := rt.vehicleRowOf(target)
		seat := s.seats[row.Seats[seatIndex]]
		if seat == nil || seat.Flags&seatFlagSwitch == 0 || rt.passengerAt(target, seatIndex) != 0 || !rt.playerSeatAllows(seatFlagSwitch) {
			return 0
		}
		rt.send(world.ClientRequestVehicleSwitchSeat, append(world.AppendPackedGUID(nil, target), byte(seatIndex)))
		return 0
	})
	ejectable := func(index int) (uint64, bool) {
		vehicle := rt.playerVehicle()
		if vehicle == 0 {
			return 0, false
		}
		target, seatIndex, ok := rt.resolveSeat(vehicle, &index, 0)
		if !ok {
			return 0, false
		}
		passenger := rt.passengerAt(target, seatIndex)
		row := rt.vehicleRowOf(target)
		seat := s.seats[row.Seats[seatIndex]]
		if passenger == 0 || seat == nil || seat.FlagsB&seatFlagsBEject == 0 {
			return 0, false
		}
		return passenger, true
	}
	reg("CanEjectPassengerFromSeat", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: CanEjectPassengerFromSeat(seatIndex)")
		}
		_, ok := ejectable(L.CheckInt(1) - 1)
		L.Push(lua.LBool(ok))
		return 1
	})
	reg("EjectPassengerFromSeat", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: EjectPassengerFromSeat(seatIndex)")
		}
		if passenger, ok := ejectable(L.CheckInt(1) - 1); ok {
			rt.send(world.ClientControllerEjectPassenger, world.BuildGUIDBody(passenger))
		}
		return 0
	})
	reg("IsVehicleAimAngleAdjustable", func(L *lua.LState) int {
		row := rt.vehicleRowOf(rt.playerVehicle())
		return flagOrNil(L, row != nil && row.Flags&vehicleFlagAngle != 0)
	})
	reg("IsVehicleAimPowerAdjustable", func(L *lua.LState) int {
		row := rt.vehicleRowOf(rt.playerVehicle())
		return flagOrNil(L, row != nil && row.Flags&vehicleFlagPower != 0)
	})
	reg("VehicleAimGetNormPower", func(L *lua.LState) int {
		L.Push(lua.LNumber(s.power))
		return 1
	})
	reg("VehicleAimSetNormPower", func(L *lua.LState) int {
		if rt.playerVehicle() == 0 || L.Get(1).Type() != lua.LTNumber {
			return 0
		}
		power := float64(L.CheckNumber(1))
		if power < 0 || power >= 1 {
			if power < 0 {
				power = 0
			} else {
				power = 1
			}
		}
		s.power = power
		return 0
	})

	// The pictures of the seats of a vehicle.
	reg("GetVehicleUIIndicator", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetVehicleUIIndicator(indicatorID)")
		}
		id := uint32(L.CheckInt(1))
		texture, found := s.tables.Indicators[id]
		if !found || id == 0 {
			return 0
		}
		count := 0
		for _, seat := range s.tables.IndicatorSeats {
			if seat.Indicator == id {
				count++
			}
		}
		L.Push(lua.LString(texture))
		L.Push(lua.LNumber(count))
		return 2
	})
	reg("GetVehicleUIIndicatorSeat", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetVehicleUIIndicatorSeat(indicatorID, indicatorSeatIndex)")
		}
		id, index := uint32(L.CheckInt(1)), L.CheckInt(2)-1
		for _, seat := range s.tables.IndicatorSeats {
			if seat.Indicator != id {
				continue
			}
			if index == 0 {
				L.Push(lua.LNumber(seat.Virtual))
				L.Push(lua.LNumber(seat.X))
				L.Push(lua.LNumber(seat.Y))
				return 3
			}
			index--
		}
		return 0
	})
}

// playerVehicle is the vehicle the character rides.
func (rt *Runtime) playerVehicle() uint64 {
	_, vehicle := rt.seatOf(rt.playerGUID())
	return vehicle
}
