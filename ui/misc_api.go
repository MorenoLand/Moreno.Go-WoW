package ui

import (
	"encoding/binary"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Packets and fields behind the small unit and world queries.
const (
	packetBindPoint          uint16 = 0x0155
	packetInstanceDifficulty uint16 = 0x033B
	dynamicFlagFeignDeath           = 0x20
	pvpFlagSanctuary                = 0x08
	glyphSocketCount                = 6
)

// worldQueryState is what the small world queries read.
type worldQueryState struct {
	bindArea   uint32
	difficulty int
}

// trivialLevelGap is how many levels under a character a creature has to be to
// give no experience, by tens of the character's level.
var trivialLevelGap = [...]int{4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8}

// handleWorldQueryPacket takes the packets that set those values.
func (rt *Runtime) handleWorldQueryPacket(packet world.Packet) bool {
	reader := world.NewReader(packet.Body, "world query")
	switch packet.Opcode {
	case packetBindPoint:
		if err := reader.Skip(12); err != nil {
			return true
		}
		if _, err := reader.U32(); err != nil {
			return true
		}
		if area, err := reader.U32(); err == nil {
			rt.worldQuery.bindArea = area
		}
	case packetInstanceDifficulty:
		if difficulty, err := reader.U32(); err == nil {
			rt.worldQuery.difficulty = int(difficulty)
		}
	default:
		return false
	}
	return true
}

// unitIsTrivial reports whether a creature is too low for the character to gain
// from.
func (rt *Runtime) unitIsTrivial(info *UnitInfo) bool {
	player := rt.unitInfo("player")
	if player == nil || info == nil || info.Player {
		return false
	}
	gap := player.Level - info.Level
	if gap <= 0 {
		return false
	}
	if tens := player.Level / 5; tens < len(trivialLevelGap) {
		return trivialLevelGap[tens] < gap
	}
	return gap > 8
}

// registerWorldQueryAPI defines the small queries of units and the world.
func registerWorldQueryAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	flag := func(name string, test func(*UnitInfo) bool) {
		reg(name, func(L *lua.LState) int {
			info := rt.unitInfo(L.CheckString(1))
			if info != nil && info.Exists && test(info) {
				L.Push(lua.LNumber(1))
			} else {
				L.Push(lua.LNil)
			}
			return 1
		})
	}
	flag("UnitIsFeignDeath", func(info *UnitInfo) bool { return info.DynamicFlags&dynamicFlagFeignDeath != 0 && info.Health > 0 })
	flag("UnitIsPVPSanctuary", func(info *UnitInfo) bool { return info.PVPFlags&pvpFlagSanctuary != 0 })
	flag("UnitIsTrivial", rt.unitIsTrivial)
	reg("GetBindLocation", func(L *lua.LState) int {
		if name := rt.zoneName(rt.worldQuery.bindArea); name != "" {
			L.Push(lua.LString(name))
		} else {
			L.Push(lua.LString(rt.globalString("HOME_INN", "")))
		}
		return 1
	})
	reg("GetInstanceDifficulty", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.worldQuery.difficulty + 1))
		return 1
	})
	reg("GetNumGlyphSockets", func(L *lua.LState) int {
		L.Push(lua.LNumber(glyphSocketCount))
		return 1
	})
	reg("PetStopAttack", func(L *lua.LState) int {
		if pet := rt.unitInfo("pet"); pet != nil && pet.Exists {
			rt.send(world.ClientPetStopAttack, binary.LittleEndian.AppendUint64(nil, pet.GUID))
		}
		return 0
	})
}
