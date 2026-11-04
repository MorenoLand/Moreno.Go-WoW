package ui

import lua "github.com/yuin/gopher-lua"

// AreaTable flags the interface asks about.
const (
	areaFlagPVPPOI          = 0x8000
	areaFlagHearthResurrect = 0x8000000
)

// areaFlagFlyable and areaFlagNoFlying are the area flags that decide whether
// the character may fly where they stand.
const (
	areaFlagFlyable  = 0x400
	areaFlagNoFlying = 0x20000000
)

// flyableArea is whether flying is allowed in the zone, or the sub zone when the
// zone says nothing.
func (rt *Runtime) flyableArea() bool {
	flags := rt.area.zoneFlags
	if flags == 0 {
		flags = rt.area.subZoneFlags
	}
	return flags&areaFlagFlyable != 0 && flags&areaFlagNoFlying == 0
}

// SetAreaFlags records the AreaTable flags of the zone and the sub zone the
// player is in.
func (rt *Runtime) SetAreaFlags(zone, subZone uint32) {
	rt.area.zoneFlags, rt.area.subZoneFlags = zone, subZone
}

func registerAreaFlagsAPI(rt *Runtime) {
	L := rt.L
	flag := func(name string, set func() bool) {
		L.SetGlobal(name, L.NewFunction(func(L *lua.LState) int {
			if set() {
				L.Push(lua.LNumber(1))
			} else {
				L.Push(lua.LNil)
			}
			return 1
		}))
	}
	flag("IsSubZonePVPPOI", func() bool { return rt.area.subZoneFlags&areaFlagPVPPOI != 0 })
	flag("CanHearthAndResurrectFromArea", func() bool { return rt.area.zoneFlags&areaFlagHearthResurrect != 0 })
}
