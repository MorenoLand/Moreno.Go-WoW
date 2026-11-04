package ui

import (
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// UnitRelations answers how units stand to each other, from the objects the
// world holds.
type UnitRelations interface {
	// Reaction is how unit a stands to unit b, 0 hated to 7 exalted.
	Reaction(a, b uint64) int
	CanAttack(a, b uint64) bool
	CanAssist(a, b uint64) bool
	// Friendly is whether a counts b a friend: a good reaction, or the flag of
	// a creature that treats everyone so.
	Friendly(a, b uint64) bool
	CanCooperate(a, b uint64) bool
}

// SetUnitRelations installs the object based answers.
func (rt *Runtime) SetUnitRelations(relations UnitRelations) { rt.relations = relations }

const packetSetForcedReactions = uint16(0x02A5)

// handleReactionPacket takes the standings the server forces for factions.
func (rt *Runtime) handleReactionPacket(packet world.Packet) bool {
	if packet.Opcode != packetSetForcedReactions {
		return false
	}
	reader := world.NewReader(packet.Body, "SMSG_SET_FORCED_REACTIONS")
	count, err := reader.U32()
	if err != nil {
		return true
	}
	rt.forcedReactions = make(map[uint32]int, count)
	for index := uint32(0); index < count; index++ {
		faction, err := reader.U32()
		if err != nil {
			return true
		}
		rank, err := reader.U32()
		if err != nil {
			return true
		}
		rt.forcedReactions[faction] = int(rank)
	}
	return true
}

// ForcedReaction is the standing the server forces with a faction.
func (rt *Runtime) ForcedReaction(faction uint32) (int, bool) {
	rank, ok := rt.forcedReactions[faction]
	return rank, ok
}

// FactionStanding is the character's standing with a faction, whether it is at
// war with it, and whether the faction is in the reputation list.
func (rt *Runtime) FactionStanding(id uint32) (standing int32, atWar, listed bool) {
	for index := range rt.factions.entries {
		if entry := &rt.factions.entries[index]; entry.ID == id {
			return entry.Total(), entry.Flags&factionFlagAtWar != 0, true
		}
	}
	return 0, false, false
}

// DuelOpponent is the player the character has been challenged by or has
// challenged to a duel.
func (rt *Runtime) DuelOpponent() uint64 { return rt.misc.duelOpponent }

// MemberInGroup is whether a player is in the character's party or raid.
func (rt *Runtime) MemberInGroup(guid uint64) bool {
	if guid == 0 {
		return false
	}
	if guid == rt.playerGUID() {
		return true
	}
	return rt.memberByGUID(guid) != nil
}

// VehicleHasFlag is whether the vehicle record of a unit carries a flag.
func (rt *Runtime) VehicleHasFlag(guid uint64, flag uint32) bool {
	row := rt.vehicleRowOf(guid)
	return row != nil && row.Flags&flag != 0
}

// unitPair finds the two units a call names.
func (rt *Runtime) unitPair(L *lua.LState) (a, b uint64, ok bool) {
	if L.Get(1).Type() != lua.LTString || L.Get(2).Type() != lua.LTString {
		return 0, 0, false
	}
	a, b = rt.UnitGUID(L.CheckString(1)), rt.UnitGUID(L.CheckString(2))
	return a, b, a != 0 && b != 0
}

func registerRelationsAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	yes := func(L *lua.LState, held bool) int {
		if held {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	}
	// both runs a check on two units that exist; the calls take two unit names.
	both := func(name string, check func(a, b uint64) bool) {
		reg(name, func(L *lua.LState) int {
			if L.Get(1).Type() != lua.LTString || L.Get(2).Type() != lua.LTString {
				L.RaiseError("Usage: %s(\"unit\", \"otherUnit\")", name)
				return 0
			}
			a, b, ok := rt.unitPair(L)
			return yes(L, ok && rt.relations != nil && check(a, b))
		})
	}
	reg("UnitReaction", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString || L.Get(2).Type() != lua.LTString {
			L.RaiseError("Usage: UnitReaction(\"unit\", \"otherUnit\")")
			return 0
		}
		a, b, ok := rt.unitPair(L)
		if !ok || rt.relations == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LNumber(rt.relations.Reaction(a, b) + 1))
		return 1
	})
	both("UnitCanAttack", func(a, b uint64) bool { return rt.relations.CanAttack(a, b) })
	both("UnitIsEnemy", func(a, b uint64) bool { return rt.relations.Reaction(a, b) < 2 })
	// UnitCanAssist asks about the second unit, as the original does.
	reg("UnitCanAssist", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString || L.Get(2).Type() != lua.LTString {
			L.RaiseError("Usage: UnitCanAssist(\"unit\", \"otherUnit\")")
			return 0
		}
		a, b, ok := rt.unitPair(L)
		return yes(L, ok && rt.relations != nil && rt.relations.CanAssist(a, b))
	})
	// The friend calls also count the members of the character's group.
	grouped := func(name string, check func(a, b uint64) bool) {
		reg(name, func(L *lua.LState) int {
			if L.Get(1).Type() != lua.LTString || L.Get(2).Type() != lua.LTString {
				L.RaiseError("Usage: %s(\"unit\", \"otherUnit\")", name)
				return 0
			}
			first, second := L.CheckString(1), L.CheckString(2)
			a, b, ok := rt.unitPair(L)
			if ok && rt.relations != nil && check(a, b) {
				return yes(L, true)
			}
			if strings.EqualFold(first, "player") && rt.MemberInGroup(b) || strings.EqualFold(second, "player") && rt.MemberInGroup(a) {
				return yes(L, true)
			}
			return yes(L, false)
		})
	}
	grouped("UnitIsFriend", func(a, b uint64) bool { return rt.relations.Friendly(a, b) })
	grouped("UnitCanCooperate", func(a, b uint64) bool { return rt.relations.CanCooperate(a, b) })
}

// The map types of battlegrounds and arenas.
const (
	mapTypeBattleground = 3
	mapTypeArena        = 4
)

// inPVPInstance is whether the character is on a battleground or arena map.
func (rt *Runtime) inPVPInstance() bool {
	catalog := rt.worldMap.catalog
	if catalog == nil || !rt.worldMap.hasPlayer {
		return false
	}
	kind := catalog.MapTypes[rt.worldMap.playerMap]
	return kind == mapTypeBattleground || kind == mapTypeArena
}

func registerBattlegroundUnitAPI(rt *Runtime) {
	rt.L.SetGlobal("UnitInBattleground", rt.L.NewFunction(func(L *lua.LState) int {
		guid := rt.UnitGUID(L.OptString(1, ""))
		if guid != 0 && rt.inPVPInstance() {
			// The place of the unit in the roster: the character, then the rest.
			if guid == rt.playerGUID() {
				L.Push(lua.LNumber(0))
				return 1
			}
			for index := range rt.group.members {
				if rt.group.members[index].GUID == guid {
					L.Push(lua.LNumber(index + 1))
					return 1
				}
			}
		}
		L.Push(lua.LNil)
		return 1
	}))
}
