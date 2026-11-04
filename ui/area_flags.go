package ui

import lua "github.com/yuin/gopher-lua"

// AreaTable flags the interface asks about.
const (
	areaFlagPVPPOI          = 0x8000
	areaFlagHearthResurrect = 0x8000000
)

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
