package ui

import (
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// An instance the player is saved to warns of it when entering: how long the
// choice stays open, which bosses are dead and whether the lock is extended.
const instanceLockWarningOpcode = 0x0147

// Encounter is a row of DungeonEncounter.dbc.
type Encounter struct {
	Map, Difficulty, Bit uint32
	Name, Icon           string
}

type instanceLock struct {
	deadline time.Time
	mask     uint32
	extended bool
}

// SetEncounters installs the boss list of the client's tables.
func (rt *Runtime) SetEncounters(list []Encounter) { rt.encounters = list }

func (rt *Runtime) handleInstanceLockPacket(packet world.Packet) bool {
	if packet.Opcode != instanceLockWarningOpcode {
		return false
	}
	reader := world.NewReader(packet.Body, "SMSG_INSTANCE_LOCK_WARNING_QUERY")
	left, err := reader.U32()
	if err != nil {
		return true
	}
	mask, err := reader.U32()
	if err != nil {
		return true
	}
	extended, _ := reader.U8()
	if int32(left) > 0 {
		rt.instanceLock = instanceLock{deadline: time.Now().Add(time.Duration(left) * time.Millisecond), mask: mask, extended: extended != 0}
		rt.FireEvent("INSTANCE_LOCK_START")
		return true
	}
	rt.instanceLock = instanceLock{}
	rt.FireEvent("INSTANCE_LOCK_STOP")
	return true
}

// lockEncounters lists the bosses of the instance the player is in.
func (rt *Runtime) lockEncounters() []Encounter {
	var list []Encounter
	for _, encounter := range rt.encounters {
		if encounter.Map == rt.worldMap.playerMap && encounter.Difficulty == rt.group.dungeon {
			list = append(list, encounter)
		}
	}
	return list
}

func registerInstanceLockAPI(rt *Runtime) {
	L := rt.L
	remaining := func() (int, uint32, bool) {
		if rt.instanceLock.deadline.IsZero() {
			return 0, 0, false
		}
		left := time.Until(rt.instanceLock.deadline)
		if left <= 0 {
			return 0, 0, false
		}
		return int(left / time.Millisecond), rt.instanceLock.mask, rt.instanceLock.extended
	}
	L.SetGlobal("GetInstanceLockTimeRemaining", L.NewFunction(func(L *lua.LState) int {
		left, mask, extended := remaining()
		list := rt.lockEncounters()
		killed := 0
		for _, encounter := range list {
			if mask&(1<<(encounter.Bit&31)) != 0 {
				killed++
			}
		}
		L.Push(lua.LNumber(left / 1000))
		L.Push(lua.LBool(extended))
		L.Push(lua.LNumber(len(list)))
		L.Push(lua.LNumber(killed))
		return 4
	}))
	L.SetGlobal("GetInstanceLockTimeRemainingEncounter", L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetInstanceLockTimeRemainingEncounter(encounterIndex)")
		}
		index := L.CheckInt(1) - 1
		left, mask, _ := remaining()
		var list []Encounter
		if left > 0 {
			list = rt.lockEncounters()
		}
		if index >= 0 && index < len(list) {
			encounter := list[index]
			L.Push(lua.LString(encounter.Name))
			if encounter.Icon == "" {
				L.Push(lua.LNil)
			} else {
				L.Push(lua.LString(encounter.Icon))
			}
			L.Push(lua.LBool(mask&(1<<(encounter.Bit&31)) != 0))
			return 3
		}
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		return 3
	}))
}
