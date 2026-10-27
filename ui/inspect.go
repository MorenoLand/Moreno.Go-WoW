package ui

import (
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// InspectHost is what looking at another player asks of the connection.
type InspectHost interface {
	// Inspect asks for the player's talents.
	Inspect(guid uint64) error
	// UnitVisibleItem is the entry of the item another unit wears in a
	// 1-based inventory slot, as the world shows it; zero for none.
	UnitVisibleItem(guid uint64, slot int) uint32
}

func (rt *Runtime) inspectHost() InspectHost {
	host, _ := rt.Host.(InspectHost)
	return host
}

// otherUnitItem is what another unit wears in a slot.
func (rt *Runtime) otherUnitItem(unit string, slot int) *ItemInstance {
	host := rt.inspectHost()
	guid := rt.UnitGUID(unit)
	if host == nil || guid == 0 || slot < 1 {
		return nil
	}
	entry := host.UnitVisibleItem(guid, slot)
	if entry == 0 {
		return nil
	}
	return &ItemInstance{GUID: guid<<8 | uint64(slot), Entry: entry, Count: 1}
}

// SetInspectedTalents installs the talents of the player looked at.
func (rt *Runtime) SetInspectedTalents(guid uint64, talents world.Talents) {
	specs := make([]TalentSpecState, 0, len(talents.Specs))
	for _, spec := range talents.Specs {
		specs = append(specs, TalentSpecState{Ranks: spec.Ranks})
	}
	rt.talents.inspected, rt.talents.inspectedActive = specs, talents.Active
	rt.talents.inspectedUnspent, rt.talents.inspectedGUID = int(talents.Unspent), guid
	rt.FireEvent("INSPECT_TALENT_READY")
}

// registerInspectAPI defines the calls the inspect window makes.
func registerInspectAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("CanInspect", func(L *lua.LState) int {
		info := rt.unitInfo(L.CheckString(1))
		L.Push(lua.LBool(info != nil && info.Exists && info.Player && !strings.EqualFold(L.CheckString(1), "player")))
		return 1
	})
	reg("NotifyInspect", func(L *lua.LState) int {
		guid := rt.UnitGUID(L.CheckString(1))
		if host := rt.inspectHost(); host != nil && guid != 0 {
			rt.talents.inspected, rt.talents.inspectedGUID = nil, guid
			_ = host.Inspect(guid)
		}
		return 0
	})
	reg("ClearInspectPlayer", func(L *lua.LState) int {
		rt.talents.inspected, rt.talents.inspectedGUID = nil, 0
		return 0
	})
	// Honor and arena figures of the inspected player are not sent to a client
	// that did not ask in the old way: the window shows none.
	reg("HasInspectHonorData", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("RequestInspectHonorData", func(L *lua.LState) int { return 0 })
	reg("GetInspectHonorData", func(L *lua.LState) int {
		for index := 0; index < 8; index++ {
			L.Push(lua.LNumber(0))
		}
		return 8
	})
	reg("GetInspectArenaTeamData", func(L *lua.LState) int { return 0 })
}

// SetUnitDistance gives the interface the distance in yards between the
// player and a unit.
func (rt *Runtime) SetUnitDistance(source func(guid uint64) (float64, bool)) {
	rt.unitDistance = source
}

// interactDistances are how near a unit must be for each kind of interaction
// (1 inspect, 2 trade, 3 duel, 4 follow), in yards.
var interactDistances = map[int]float64{1: 28, 2: 11.11, 3: 9.9, 4: 28}

func registerInteractDistanceAPI(rt *Runtime) {
	rt.L.SetGlobal("CheckInteractDistance", rt.L.NewFunction(func(L *lua.LState) int {
		limit, known := interactDistances[L.CheckInt(2)]
		guid := rt.UnitGUID(L.CheckString(1))
		if !known || guid == 0 || rt.unitDistance == nil {
			L.Push(lua.LFalse)
			return 1
		}
		distance, ok := rt.unitDistance(guid)
		L.Push(lua.LBool(ok && distance <= limit))
		return 1
	}))
}

// InteractionGUIDs are the objects and creatures whose windows are open: the
// mailbox, the guild bank, the auctioneer.
func (rt *Runtime) InteractionGUIDs() (mailbox, bank, auctioneer uint64) {
	if rt.mail.open {
		mailbox = rt.mail.mailbox
	}
	if rt.guildBank.open {
		bank = rt.guildBank.bank
	}
	if rt.auction.open {
		auctioneer = rt.auction.auctioneer
	}
	return
}
