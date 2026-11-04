package ui

import lua "github.com/yuin/gopher-lua"

// creatureForceGossip is the creature type flag that makes a creature open its
// menu even when it has a single thing to offer.
const creatureForceGossip = 0x08000000

// SetCreatureTypeFlags records the type flags the server gave for a creature.
func (rt *Runtime) SetCreatureTypeFlags(entry, flags uint32) {
	if rt.creatureFlags == nil {
		rt.creatureFlags = make(map[uint32]uint32)
	}
	rt.creatureFlags[entry] = flags
}

func registerForceGossipAPI(rt *Runtime) {
	rt.L.SetGlobal("ForceGossip", rt.L.NewFunction(func(L *lua.LState) int {
		forced := false
		if rt.gossip.open && rt.gossip.menu.GUID>>48&0xFFF == 0xF13 {
			forced = rt.creatureFlags[uint32(rt.gossip.menu.GUID>>24&0xFFFFFF)]&creatureForceGossip != 0
		}
		L.Push(lua.LBool(forced))
		return 1
	}))
}
