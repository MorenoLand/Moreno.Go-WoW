package ui

import lua "github.com/yuin/gopher-lua"

// npcReachSlack is how far past its own reach a creature can be talked to.
const npcReachSlack = 4.0

// nearNPC reports whether the character is close enough to the creature it is
// talking to.
func (rt *Runtime) nearNPC(guid uint64) bool {
	if guid == 0 || rt.unitDistance == nil {
		return false
	}
	distance, ok := rt.unitDistance(guid)
	if !ok {
		return false
	}
	reach := 0.0
	if rt.threat.reach != nil {
		reach, _ = rt.threat.reach(guid)
	}
	return distance < reach+npcReachSlack
}

func registerNPCDistanceAPI(rt *Runtime) {
	L := rt.L
	check := func(name string, guid func() uint64) {
		L.SetGlobal(name, L.NewFunction(func(L *lua.LState) int {
			if rt.nearNPC(guid()) {
				L.Push(lua.LNumber(1))
			} else {
				L.Push(lua.LNil)
			}
			return 1
		}))
	}
	talking := func() uint64 {
		if rt.gossip.open {
			return rt.gossip.menu.GUID
		}
		return 0
	}
	check("CheckSpiritHealerDist", func() uint64 {
		if guid := talking(); guid != 0 {
			return guid
		}
		return rt.spiritHealerGUID
	})
	check("CheckBinderDist", talking)
}
